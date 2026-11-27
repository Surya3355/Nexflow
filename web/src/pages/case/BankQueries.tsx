import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import type { BankQuery, CaseQueries, StaffUser } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { alertBox, btn, btnPrimary, btnSmall, input, label } from "../../components/ui";

type Props = {
  caseId: number;
  data: CaseQueries;
  readOnly: boolean;
  onChanged: () => void;
  onOpenDocument: (documentId: number) => void;
};

/** Today in India as YYYY-MM-DD. */
export function todayIST(): string {
  return new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Kolkata" });
}

/** "2026-11-24" → "24 Nov". */
function shortDate(d: string): string {
  return new Date(`${d}T00:00:00Z`).toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "UTC" });
}

const chip: Record<BankQuery["status"], { text: string; cls: string }> = {
  open: { text: "Open", cls: "bg-amber-50 text-amber-800" },
  sent: { text: "Sent to bank", cls: "bg-blue-50 text-blue-700" },
  resolved: { text: "Resolved", cls: "bg-green-50 text-green-800" },
};

/** The bank's queries on a case; each is resolved only with proof the bank accepted (spec 2026-11-26 bank queries §5.5). */
export function BankQueries({ caseId, data, readOnly, onChanged, onOpenDocument }: Props) {
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<BankQuery | null>(null);
  const [sending, setSending] = useState<BankQuery | null>(null);
  const [rejecting, setRejecting] = useState<BankQuery | null>(null);
  const [error, setError] = useState<string | null>(null);
  const unresolved = data.queries.filter((q) => q.status !== "resolved");
  const resolved = data.queries.filter((q) => q.status === "resolved");
  const today = todayIST();

  async function act(question: string, method: "POST" | "DELETE", path: string) {
    if (!window.confirm(question)) return;
    setError(null);
    try {
      await api.request(method, path);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save. Try again.");
    }
  }

  const done = () => { setAdding(false); setEditing(null); setSending(null); setRejecting(null); onChanged(); };

  return (
    <section aria-label="Bank queries"
      className={`rounded-xl bg-white p-3.5 ${data.open_count > 0 ? "border-2 border-red-300" : "border border-slate-200"}`}>
      <div className="flex items-center justify-between gap-2">
        <h2 className="font-semibold">
          Bank queries
          {data.open_count > 0 && <span className="ml-1.5 rounded-full bg-red-700 px-2 py-0.5 text-xs font-semibold text-white">{data.open_count} open</span>}
        </h2>
        {!readOnly && <button type="button" className={btnSmall} onClick={() => setAdding(true)}>+ Add query</button>}
      </div>
      {error && <p role="alert" className={`${alertBox} mt-2`}>{error}</p>}
      {unresolved.length === 0 && <p className="mt-2 text-sm text-slate-500">No open queries.</p>}

      {unresolved.map((q) => {
        const overdue = q.status === "open" && q.due_on !== null && q.due_on < today;
        return (
          <div key={q.id} className="mt-2.5 rounded-lg border border-slate-200 p-3">
            <div className="flex items-start justify-between gap-2">
              <strong className="break-words">{q.body}</strong>
              <span className={`flex-none rounded-full px-2 py-0.5 text-xs font-semibold ${chip[q.status].cls}`}>{chip[q.status].text}</span>
            </div>
            <p className="mt-1 text-sm text-slate-500">
              Raised {shortDate(q.raised_on)} · {q.assignee.name}
              {q.due_on && (overdue
                ? <> · <span className="font-semibold text-red-700">Due {shortDate(q.due_on)}, overdue</span></>
                : <> · Due {shortDate(q.due_on)}</>)}
            </p>
            {!readOnly && (
              <div className="mt-2.5 flex flex-wrap gap-2">
                {q.status === "open" && <button type="button" className={btnPrimary} onClick={() => setSending(q)}>Send proof</button>}
                {q.status === "sent" && <>
                  <button type="button" className="rounded-lg bg-green-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-green-800"
                    onClick={() => act("Did the bank accept the proof? The query will be marked resolved.", "POST", `/api/queries/${q.id}/accept`)}>Bank accepted</button>
                  <button type="button" className={btnSmall} onClick={() => setRejecting(q)}>Bank rejected</button>
                </>}
                <button type="button" className={btnSmall} onClick={() => setEditing(q)}>Edit</button>
                <button type="button" aria-label={`Remove ${q.body}`} className="rounded-lg border border-red-200 bg-white px-3 py-1 text-sm text-red-700"
                  onClick={() => act("Remove this query? It stays in the case history.", "DELETE", `/api/queries/${q.id}`)}>Remove</button>
              </div>
            )}
            {q.status === "sent" && q.proof && (
              <details className="mt-2">
                <summary className="cursor-pointer text-sm text-blue-700">View proof</summary>
                <div className="mt-2 space-y-1.5 rounded-lg bg-slate-50 px-3 py-2 text-sm">
                  {q.proof.note && <p className="text-slate-700">“{q.proof.note}”</p>}
                  {q.proof.document
                    ? <p><button type="button" className="text-blue-700" onClick={() => onOpenDocument(q.proof!.document!.id)}>{q.proof.document.file_name}</button>
                        <span className="text-slate-500"> · {q.proof.document.label}</span></p>
                    : q.proof.file_deleted && <p className="text-slate-500">{q.proof.file_name} · File deleted</p>}
                  <p className="text-slate-500">Sent by {q.proof.by}</p>
                  {!readOnly && (
                    <button type="button" className="rounded-lg border border-red-200 bg-white px-2.5 py-1 text-sm text-red-700"
                      onClick={() => act("Remove this proof? The query goes back to Open.", "POST", `/api/queries/${q.id}/remove-proof`)}>Remove proof</button>
                  )}
                </div>
              </details>
            )}
          </div>
        );
      })}

      {resolved.length > 0 && (
        <details className="mt-2.5">
          <summary className="cursor-pointer text-sm text-slate-500">Resolved ({resolved.length})</summary>
          <ul className="mt-1.5 space-y-1 text-sm">
            {resolved.map((q) => <li key={q.id}>{q.body} · <span className="text-green-800">Accepted</span></li>)}
          </ul>
        </details>
      )}

      {(adding || editing) && (
        <QueryDialog caseId={caseId} query={editing} onClose={() => { setAdding(false); setEditing(null); }} onSaved={done} />
      )}
      {sending && <SendProofDialog query={sending} onClose={() => setSending(null)} onSaved={done} />}
      {rejecting && <RejectDialog query={rejecting} onClose={() => setRejecting(null)} onSaved={done} />}
    </section>
  );
}

function FormButtons({ busy, label: text, onClose }: { busy: boolean; label: string; onClose: () => void }) {
  return (
    <div className="flex gap-2">
      <button type="submit" className={btnPrimary} disabled={busy}>{text}</button>
      <button type="button" className={btn} onClick={onClose}>Cancel</button>
    </div>
  );
}

/** Add a query, or edit one (what was asked, dates, who it is assigned to). */
function QueryDialog({ caseId, query, onClose, onSaved }: { caseId: number; query: BankQuery | null; onClose: () => void; onSaved: () => void }) {
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [body, setBody] = useState(query?.body ?? "");
  const [raisedOn, setRaisedOn] = useState(query?.raised_on ?? todayIST());
  const [assignee, setAssignee] = useState(query ? String(query.assignee.id) : "");
  const [dueOn, setDueOn] = useState(query?.due_on ?? "");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.request<StaffUser[]>("GET", "/api/users").then(setUsers).catch(() => setUsers([]));
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    if (!assignee) {
      setError("Choose who the query is assigned to.");
      return;
    }
    setBusy(true);
    setError(null);
    const payload = { body, raised_on: raisedOn, due_on: dueOn || null, assignee_id: Number(assignee) };
    try {
      if (query) await api.request("PUT", `/api/queries/${query.id}`, payload);
      else await api.request("POST", `/api/cases/${caseId}/queries`, payload);
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the query.");
    } finally {
      setBusy(false);
    }
  }

  const title = query ? "Edit bank query" : "Add bank query";
  return (
    <Dialog open title={title} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>What did the bank ask for?
          <textarea className={input} rows={3} maxLength={500} value={body} onChange={(e) => setBody(e.target.value)} />
        </label>
        <label className={label}>Raised on
          <input type="date" className={input} max={todayIST()} value={raisedOn} onChange={(e) => setRaisedOn(e.target.value)} />
        </label>
        <label className={label}>Assign to
          <select className={input} value={assignee} onChange={(e) => setAssignee(e.target.value)}>
            <option value="">Choose…</option>
            {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
          </select>
        </label>
        <label className={label}>Due date (optional)
          <input type="date" className={input} min={raisedOn} value={dueOn} onChange={(e) => setDueOn(e.target.value)} />
        </label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <FormButtons busy={busy} label={query ? "Save" : "Add query"} onClose={onClose} />
      </form>
    </Dialog>
  );
}

function SendProofDialog({ query, onClose, onSaved }: { query: BankQuery; onClose: () => void; onSaved: () => void }) {
  const [note, setNote] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    if (!note.trim() && !file) {
      setError("Add a note, a file, or both.");
      return;
    }
    setBusy(true);
    setError(null);
    const form = new FormData();
    form.append("note", note);
    if (file) form.append("file", file);
    try {
      await api.upload(`/api/queries/${query.id}/send`, form);
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not send the proof.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open title="Send proof to bank" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <p className="text-sm text-slate-500">{query.body}</p>
        <label className={label}>Note
          <textarea className={input} rows={3} maxLength={500} value={note} onChange={(e) => setNote(e.target.value)} />
        </label>
        <label className={label}>File or photo (PDF, JPG, PNG, up to 10 MB)
          <input type="file" accept="application/pdf,image/jpeg,image/png" className="mt-1 block w-full text-sm"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        </label>
        <p className="text-sm text-slate-500">Add a note, a file, or both. The query then shows as “Sent to bank”.</p>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <FormButtons busy={busy} label="Mark sent to bank" onClose={onClose} />
      </form>
    </Dialog>
  );
}

function RejectDialog({ query, onClose, onSaved }: { query: BankQuery; onClose: () => void; onSaved: () => void }) {
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await api.request("POST", `/api/queries/${query.id}/reject`, { reason });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open title="Bank rejected the proof" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <p className="text-sm text-slate-500">{query.body}</p>
        <label className={label}>What did the bank say?
          <textarea className={input} rows={3} maxLength={500} value={reason} onChange={(e) => setReason(e.target.value)} />
        </label>
        <p className="text-sm text-slate-500">The query goes back to “Open”. The old proof stays in its history; new proof is needed to send it again.</p>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <FormButtons busy={busy} label="Reopen query" onClose={onClose} />
      </form>
    </Dialog>
  );
}
