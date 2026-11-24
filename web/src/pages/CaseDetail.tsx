import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, ApiError, upload } from "../api/client";
import type { Case, CaseRow, Customer, DocumentInfo, LeadContact, Project, Stage, StageEvent } from "../api/types";
import { AddCaseDialog } from "../components/AddCaseDialog";
import { AssignDialog } from "../components/AssignDialog";
import { Badge } from "../components/Badge";
import { Dialog } from "../components/Dialog";
import { DropDialog } from "../components/DropDialog";
import { ProgressBar } from "../components/ProgressBar";
import { ProjectPicker } from "../components/ProjectPicker";
import { alertBox, btn, btnDanger, btnPrimary, btnSmall, card, input, istDate, label, muted, page, rupees, warnBox } from "../components/ui";
import { MoveStageDialog } from "./case/MoveStageDialog";
import { ContactLog } from "./lead/ContactLog";

type Moving = { target: Stage | null } | null;

/** One case: progress through the bank's stages, documents, assignee and drop (spec §5.4). */
export function CaseDetail() {
  const { id } = useParams();
  const caseId = Number(id);
  const navigate = useNavigate();
  const [c, setCase] = useState<Case | null>(null);
  const [row, setRow] = useState<CaseRow | null>(null);
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [history, setHistory] = useState<StageEvent[]>([]);
  const [stages, setStages] = useState<Stage[]>([]);
  const [docs, setDocs] = useState<DocumentInfo[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [moving, setMoving] = useState<Moving>(null);
  const [uploading, setUploading] = useState(false);
  const [assigning, setAssigning] = useState(false);
  const [changingProject, setChangingProject] = useState(false);
  const [dropping, setDropping] = useState(false);
  const [changingUnit, setChangingUnit] = useState(false);
  const [addingCase, setAddingCase] = useState(false);
  const [contacts, setContacts] = useState<LeadContact[]>([]);

  async function load() {
    const [caseData, hist, docList] = await Promise.all([
      api.request<Case>("GET", `/api/cases/${caseId}`),
      api.request<StageEvent[]>("GET", `/api/cases/${caseId}/history`),
      api.request<DocumentInfo[]>("GET", `/api/cases/${caseId}/documents`),
    ]);
    setCase(caseData);
    setHistory(hist);
    setDocs(docList);
    const [stageList, cust] = await Promise.all([
      api.request<Stage[]>("GET", `/api/config/banks/${caseData.bank_id}/stages`),
      api.request<Customer>("GET", `/api/customers/${caseData.customer_id}`).catch(() => null),
    ]);
    setStages([...stageList].sort((a, b) => a.sort_order - b.sort_order));
    setCustomer(cust);
    setRow(cust?.cases?.find((x) => x.id === caseData.id) ?? null);
    if (caseData.lead_id) {
      api.request<{ contacts: LeadContact[] }>("GET", `/api/leads/${caseData.lead_id}`)
        .then((r) => setContacts(r.contacts)).catch(() => setContacts([]));
    }
  }

  useEffect(() => {
    load().catch(() => setError("Could not load this case."));
    // load depends only on the case ID from the route.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [caseId]);

  if (error && !c) return <p role="alert" className="p-4 text-red-700">{error}</p>;
  if (!c) return <p className="p-4 text-slate-500">Loading…</p>;

  const active = c.status === "active";
  const currentIndex = stages.findIndex((s) => s.id === c.current_stage_id);
  const current = stages[currentIndex];
  const next = currentIndex >= 0 ? stages[currentIndex + 1] : undefined;
  const needsDocs = Boolean(current?.requires_document) && docs.length === 0;
  const lastEvent = (code: string) => [...history].reverse().find((e) => e.stage_code === code);

  async function moved(updated: Case & { warning?: string }) {
    setMoving(null);
    setNotice(updated.warning ?? null);
    await load();
  }

  async function openDocument(d: DocumentInfo) {
    setError(null);
    try {
      const link = await api.request<{ url: string }>("GET", `/api/cases/${caseId}/documents/${d.id}/link`);
      window.open(link.url, "_blank", "noopener");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not open the file.");
    }
  }

  const statusBadge = c.status === "dropped" ? <Badge tone="bad">Dropped</Badge>
    : c.status === "completed" ? <Badge tone="ok">Disbursed</Badge>
    : <Badge>{current?.name ?? "…"}</Badge>;

  return (
    <main className={`${page} max-w-[1100px]`}>
      <Link to="/" className={muted}>← Back to home</Link>

      <section className={card}>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="text-2xl font-bold">
              <Link to={`/customers/${c.customer_id}`} className="text-slate-900 hover:underline">{customer?.name ?? "Customer"}</Link>
            </h1>
            <p className="mt-1 text-base text-slate-500">
              {customer && <><a href={`tel:${customer.mobile}`} className="font-semibold text-blue-700">{customer.mobile}</a> · </>}
              {row ? `${row.bank_name} · ` : ""}Case #{c.id}
            </p>
          </div>
          <div className="text-right">
            <div className="text-xl font-bold">{rupees(c.loan_amount)}</div>
            {statusBadge}
          </div>
        </div>
        {stages.length > 0 && <div className="mt-4"><ProgressBar done={currentIndex + 1} total={stages.length} /></div>}
      </section>

      {error && <p role="alert" className={alertBox}>{error}</p>}
      {notice && <p role="status" className={warnBox}>{notice}</p>}

      <div className="grid gap-4 min-[900px]:grid-cols-[2fr_1fr]">
        <section className={card}>
          <h2 className="mb-2 font-semibold">Progress</h2>
          <ol aria-label="Progress" className="space-y-1">
            {stages.map((s, i) => {
              const state = i < currentIndex ? "done" : i === currentIndex ? "now" : "todo";
              const ev = state !== "todo" ? lastEvent(s.code) : undefined;
              return (
                <li key={s.id} aria-current={state === "now" ? "step" : undefined} className={`flex gap-3 py-1.5 ${state === "todo" ? "text-slate-400" : ""}`}>
                  <span className={`grid h-6 w-6 flex-none place-items-center rounded-full border-2 text-xs ${
                    state === "done" ? "border-blue-600 bg-blue-600 text-white" : state === "now" ? "border-blue-600 font-bold text-blue-700" : "border-slate-200"
                  }`}>{state === "done" ? "✓" : i + 1}</span>
                  <span>
                    {state === "now" ? <b>{s.name}</b> : s.name}
                    {ev && <span className="block text-sm text-slate-500">{istDate(ev.reached_at, true)} · {ev.changed_by}</span>}
                  </span>
                </li>
              );
            })}
          </ol>
          {active && (
            <div className="mt-3 flex flex-wrap gap-2">
              {next && <button type="button" className={btnPrimary} onClick={() => setMoving({ target: next })}>Move to {next.name} →</button>}
              <button type="button" className={btnSmall} onClick={() => setMoving({ target: null })}>Other stage…</button>
            </div>
          )}
          <details className="mt-4">
            <summary className="cursor-pointer text-sm text-slate-500">Full history ({history.length})</summary>
            <ol aria-label="Stage history" className="mt-2 space-y-1 text-sm">
              {history.map((e) => (
                <li key={e.id} className="rounded-lg bg-slate-50 px-3 py-1.5">
                  <span className="font-medium">{e.stage_name}</span> · {istDate(e.reached_at, true)} · {e.changed_by}
                </li>
              ))}
            </ol>
          </details>
        </section>

        <aside className="space-y-4">
          <section className={card}>
            <div className="flex items-center justify-between">
              <h2 className="font-semibold">Documents</h2>
              <button type="button" className={btnSmall} onClick={() => setUploading(true)}>+ Upload</button>
            </div>
            {needsDocs && !notice && <p className={`${warnBox} mt-3`}>This stage normally needs at least one document. None is uploaded yet.</p>}
            {docs.length === 0 ? <p className={`${muted} mt-3`}>No documents yet.</p> : (
              <ul className="mt-3 space-y-1 text-sm">
                {docs.map((d) => (
                  <li key={d.id}>
                    <button type="button" className="text-blue-700" onClick={() => openDocument(d)}>{d.label}</button>
                    <span className="text-slate-500"> · {d.file_name}</span>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <section aria-label="Details" className={card}>
            <h2 className="mb-2 font-semibold">Details</h2>
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-base">
              <dt className="text-slate-500">Project</dt>
              <dd className="font-semibold">
                {row?.project_name ?? "…"}
                {c.status !== "dropped" && <button type="button" aria-label="Change project" className="ml-2 text-sm font-normal text-blue-700" onClick={() => setChangingProject(true)}>change</button>}
              </dd>
              <dt className="text-slate-500">Unit no.</dt>
              <dd className="font-semibold">
                {c.unit_number || <span className="font-normal text-slate-500">Not set</span>}
                {c.status !== "dropped" && <button type="button" aria-label="Change unit no." className="ml-2 text-sm font-normal text-blue-700" onClick={() => setChangingUnit(true)}>change</button>}
              </dd>
              {c.tracking_number && <><dt className="text-slate-500">Tracking no.</dt><dd className="font-semibold">{c.tracking_number}</dd></>}
              <dt className="text-slate-500">Assigned to</dt>
              <dd className="font-semibold">
                {row?.assignee_name ?? "…"}
                <button type="button" aria-label="Change assignee" className="ml-2 text-sm font-normal text-blue-700" onClick={() => setAssigning(true)}>change</button>
              </dd>
              <dt className="text-slate-500">Opened</dt><dd className="font-semibold">{istDate(c.created_at)}</dd>
            </dl>
          </section>
          {c.lead_id && (
            <section className={card}>
              <h2 className="mb-1 font-semibold">Contact log (from lead)</h2>
              <ContactLog contacts={contacts} />
            </section>
          )}
          {customer && customer.status === "active" && (
            <button type="button" className={btn} onClick={() => setAddingCase(true)}>+ New case for {customer.name.trim().split(/\s+/)[0]}</button>
          )}
          {active && <button type="button" className={btnDanger} onClick={() => setDropping(true)}>Drop this case…</button>}
        </aside>
      </div>

      <MoveStageDialog open={moving !== null} caseData={c} stages={stages} target={moving?.target ?? null} onClose={() => setMoving(null)} onMoved={moved} />
      <UploadDialog open={uploading} caseId={caseId} onClose={() => setUploading(false)} onUploaded={async () => { setUploading(false); setDocs(await api.request<DocumentInfo[]>("GET", `/api/cases/${caseId}/documents`)); }} />
      <ProjectDialog open={changingProject} caseId={caseId} current={c.project_id} onClose={() => setChangingProject(false)} onSaved={async () => { setChangingProject(false); await load(); }} />
      <UnitDialog open={changingUnit} caseId={caseId} current={c.unit_number ?? ""} onClose={() => setChangingUnit(false)} onSaved={async () => { setChangingUnit(false); await load(); }} />
      {customer && (
        <AddCaseDialog open={addingCase} customerId={customer.id} customerName={customer.name} onClose={() => setAddingCase(false)}
          onSaved={(nc) => { setAddingCase(false); navigate(`/cases/${nc.id}`); }} />
      )}
      <AssignDialog open={assigning} title="Assign case" path={`/api/cases/${caseId}/assign`} current={c.assignee_id} onClose={() => setAssigning(false)} onSaved={async () => { setAssigning(false); await load(); }} />
      <DropDialog
        open={dropping}
        title={`Drop case #${c.id}?`}
        confirmLabel="Drop case"
        onClose={() => setDropping(false)}
        onConfirm={async (reasonId, remark) => {
          await api.request("POST", `/api/cases/${caseId}/drop`, { reason_id: reasonId, remark });
          setDropping(false);
          await load();
        }}
      />
    </main>
  );
}

function UploadDialog({ open, caseId, onClose, onUploaded }: { open: boolean; caseId: number; onClose: () => void; onUploaded: () => void }) {
  const [docLabel, setDocLabel] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!file || !docLabel.trim()) {
      setError("Enter a label and choose a file.");
      return;
    }
    setError(null);
    try {
      const form = new FormData();
      form.append("label", docLabel.trim());
      form.append("file", file);
      await upload(`/api/cases/${caseId}/documents`, form);
      setDocLabel("");
      setFile(null);
      onUploaded();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not upload the file.");
    }
  }

  return (
    <Dialog open={open} title="Upload a document" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>What is it?<input aria-label="Document label" className={input} placeholder="e.g. Sanction letter" value={docLabel} onChange={(e) => setDocLabel(e.target.value)} /></label>
        <label className={label}>File (PDF, JPG or PNG, up to 10 MB)
          <input aria-label="File" type="file" accept="application/pdf,image/jpeg,image/png" className="mt-1 block w-full text-sm" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        </label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Upload</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}

function ProjectDialog({ open, caseId, current, onClose, onSaved }: { open: boolean; caseId: number; current: number; onClose: () => void; onSaved: () => void }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState<number | null>(current);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setProjectId(current);
    setError(null);
    api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
  }, [open, current]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (projectId === null) {
      setError("Choose a project, or NA.");
      return;
    }
    try {
      await api.request("POST", `/api/cases/${caseId}/project`, { project_id: projectId });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not change the project.");
    }
  }

  return (
    <Dialog open={open} title="Change project" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <div className={label}>Project
          <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} aria-label="Project" />
        </div>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}

function UnitDialog({ open, caseId, current, onClose, onSaved }: { open: boolean; caseId: number; current: string; onClose: () => void; onSaved: () => void }) {
  const [unit, setUnit] = useState(current);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setUnit(current);
    setError(null);
  }, [open, current]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      await api.request("POST", `/api/cases/${caseId}/unit`, { unit_number: unit.trim() });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not change the unit no.");
    }
  }

  return (
    <Dialog open={open} title="Change unit no." onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Unit no.<input className={input} placeholder="Flat, villa or plot number" value={unit} onChange={(e) => setUnit(e.target.value.toUpperCase())} /></label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
