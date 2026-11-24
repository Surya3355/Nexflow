import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import { PROJECT_TYPES } from "../../api/projectTypes";
import type { BuilderContact, Project, ProjectType } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { alertBox, btn, btnPrimary, btnSmall, input, label, muted } from "../../components/ui";

type Row = { key: number; id?: number; name: string; phone: string; email: string; label: string };
type Props = { project?: Project; onClose: () => void; onSaved: () => void };

const blank = { name: "", phone: "", email: "", label: "" };

/**
 * Add or edit a project: name, type, address and its own contacts (spec
 * 2026-11-24 §4.4). Nothing is sent until Save, which sends the whole form in
 * one request; Cancel discards every change.
 */
export function ProjectForm({ project, onClose, onSaved }: Props) {
  const [name, setName] = useState(project?.name ?? "");
  const [type, setType] = useState<ProjectType | "">(project?.type ?? "");
  const [address, setAddress] = useState(project?.address ?? "");
  const [rows, setRows] = useState<Row[]>([]);
  const [editing, setEditing] = useState<{ key: number | null; draft: typeof blank } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  // An edit starts from the saved contacts. Until they have loaded nothing can
  // change or be saved, because a save without them would remove them all.
  const [load, setLoad] = useState<"loading" | "ready" | "failed">(project ? "loading" : "ready");
  const nextKey = useRef(1);
  const ready = load === "ready";

  function fetchContacts() {
    if (!project) return () => {};
    let live = true;
    setLoad("loading");
    api.request<Project>("GET", `/api/config/projects/${project.id}`)
      .then((p) => { if (live) { setRows((p.contacts ?? []).map(toRow)); setLoad("ready"); } })
      .catch(() => live && setLoad("failed"));
    return () => { live = false; };
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(fetchContacts, [project?.id]);

  function toRow(c: BuilderContact): Row {
    return { key: nextKey.current++, id: c.id, name: c.name, phone: c.phone, email: c.email, label: c.label };
  }

  function saveDraft() {
    if (!editing) return;
    const d = { name: editing.draft.name.trim(), phone: editing.draft.phone.trim(), email: editing.draft.email.trim(), label: editing.draft.label.trim() };
    if (!d.name || !d.phone) {
      setError("Enter the contact's name and mobile.");
      return;
    }
    setError(null);
    if (editing.key === null) {
      setRows((r) => [...r, { key: nextKey.current++, ...d }]);
    } else {
      setRows((r) => r.map((x) => (x.key === editing.key ? { ...x, ...d } : x)));
    }
    setEditing(null);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy || !ready) return;
    if (editing && Object.values(editing.draft).some((v) => v.trim() !== "")) {
      setError("Add or discard the contact you are editing first.");
      return;
    }
    if (rows.length === 0) {
      setError("Add at least one contact.");
      return;
    }
    setError(null);
    setBusy(true);
    const body = {
      name: name.trim(), type, address: address.trim(),
      contacts: rows.map(({ id, name, phone, email, label }) => (id ? { id, name, phone, email, label } : { name, phone, email, label })),
    };
    try {
      if (project) await api.request("PUT", `/api/admin/projects/${project.id}`, body);
      else await api.request("POST", "/api/admin/projects", body);
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open title={project ? `Edit ${project.name}` : "Add a project"} onClose={onClose}>
      <form onSubmit={submit} className="max-h-[70vh] space-y-3 overflow-y-auto">
        <label className={label}>Project name<input className={input} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <fieldset role="radiogroup" aria-label="Type">
          <legend className={label}>Type</legend>
          <div className="mt-1 grid grid-cols-2 gap-1">
            {PROJECT_TYPES.map((t) => (
              <label key={t.value} className="flex items-center gap-2 text-sm text-slate-800">
                <input type="radio" name="project-type" value={t.value} checked={type === t.value} onChange={() => setType(t.value)} />
                {t.label}
              </label>
            ))}
          </div>
        </fieldset>
        <label className={label}>Address<input className={input} value={address} onChange={(e) => setAddress(e.target.value)} /></label>

        <div>
          <p className={label}>Contacts</p>
          {load === "loading" && <p className={muted}>Loading contacts…</p>}
          {load === "failed" && (
            <div role="alert" className={`${alertBox} flex items-center justify-between gap-2`}>
              Could not load the contacts.
              <button type="button" className="font-medium underline" onClick={fetchContacts}>Try again</button>
            </div>
          )}
          {ready && rows.length === 0 && <p className={muted}>No contacts yet. Add at least one.</p>}
          <ul className="mt-1 divide-y divide-slate-100">
            {rows.map((r) => (
              <li key={r.key} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
                <span className="min-w-0 break-words">{[r.label, r.name, r.phone].filter(Boolean).join(" · ")}</span>
                <span className="flex gap-3">
                  <button type="button" aria-label={`Edit ${r.name}`} className="text-blue-700" disabled={!ready}
                    onClick={() => setEditing({ key: r.key, draft: { name: r.name, phone: r.phone, email: r.email, label: r.label } })}>edit</button>
                  <button type="button" aria-label={`Remove ${r.name}`} className="text-red-700" disabled={!ready}
                    onClick={() => setRows((x) => x.filter((y) => y.key !== r.key))}>remove</button>
                </span>
              </li>
            ))}
          </ul>
          {editing ? (
            <div className="mt-2 space-y-2 rounded-lg border border-slate-200 p-3">
              <label className={label}>Contact name<input className={input} value={editing.draft.name}
                onChange={(e) => setEditing({ ...editing, draft: { ...editing.draft, name: e.target.value } })} /></label>
              <label className={label}>Mobile<input className={input} inputMode="tel" value={editing.draft.phone}
                onChange={(e) => setEditing({ ...editing, draft: { ...editing.draft, phone: e.target.value } })} /></label>
              <label className={label}>Email (optional)<input className={input} type="email" value={editing.draft.email}
                onChange={(e) => setEditing({ ...editing, draft: { ...editing.draft, email: e.target.value } })} /></label>
              <label className={label}>Label (optional)<input className={input} maxLength={40} placeholder="e.g. Sales office" value={editing.draft.label}
                onChange={(e) => setEditing({ ...editing, draft: { ...editing.draft, label: e.target.value } })} /></label>
              <div className="flex gap-2">
                <button type="button" className={btnSmall} onClick={saveDraft}>{editing.key === null ? "Add contact" : "Save contact"}</button>
                <button type="button" className={btnSmall} onClick={() => setEditing(null)}>Back</button>
              </div>
            </div>
          ) : (
            <button type="button" className={`${btnSmall} mt-2 disabled:opacity-50`} disabled={!ready} onClick={() => setEditing({ key: null, draft: { ...blank } })}>+ Add contact</button>
          )}
        </div>

        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={busy || !ready}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
