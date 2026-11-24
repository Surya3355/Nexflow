import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import type { Bank, DropReason, Project } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { alertBox, btn, btnPrimary, btnSmall, card, input, label, muted } from "../../components/ui";
import { projectTypeLabel } from "../../api/projectTypes";
import { ProjectForm } from "./ProjectForm";

type Editing =
  | { kind: "add-bank" } | { kind: "rename"; bank: Bank } | { kind: "add-reason" }
  | { kind: "add-project" } | { kind: "edit-project"; project: Project } | null;

/** Admin Banks & lists tab: banks (add, rename), projects (add, edit details and contacts) and drop reasons (add). */
export function BanksAndLists() {
  const [banks, setBanks] = useState<Bank[]>([]);
  const [reasons, setReasons] = useState<DropReason[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [editing, setEditing] = useState<Editing>(null);

  function load() {
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
    api.request<DropReason[]>("GET", "/api/config/dropdowns/drop_reason").then(setReasons).catch(() => setReasons([]));
    api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
  }
  useEffect(load, []);

  function done() {
    setEditing(null);
    load();
  }

  return (
    <div className="grid gap-4 min-[900px]:grid-cols-[2fr_1fr]">
      <section className={card}>
        <div className="flex items-center justify-between">
          <h2 className="font-semibold">Banks</h2>
          <button type="button" className={btnPrimary} onClick={() => setEditing({ kind: "add-bank" })}>+ Add bank</button>
        </div>
        <table className="mt-3 min-w-full text-sm">
          <tbody>
            {banks.map((b) => (
              <tr key={b.id} className="border-t border-slate-100">
                <td className="px-2 py-2">{b.name}</td>
                <td className={`px-2 py-2 ${muted}`}>{b.has_own_stages ? "Own stages" : "Standard stages"}</td>
                <td className="px-2 py-2 text-right">
                  <button type="button" aria-label={`Rename ${b.name}`} className="text-blue-700" onClick={() => setEditing({ kind: "rename", bank: b })}>Rename</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      <section aria-label="Projects" className={`${card} min-[900px]:col-start-1`}>
        <div className="flex items-center justify-between">
          <h2 className="font-semibold">Projects</h2>
          <button type="button" className={btnPrimary} onClick={() => setEditing({ kind: "add-project" })}>+ Add project</button>
        </div>
        <p className={`${muted} mt-1`}>NA is for loans without a builder project. It cannot be changed.</p>
        <table className="mt-3 min-w-full text-sm">
          <tbody>
            {projects.map((p) => (
              <tr key={p.id} className="border-t border-slate-100">
                <td className="px-2 py-2">{p.name}</td>
                <td className={`px-2 py-2 ${muted}`}>
                  {p.is_system ? null : p.type === null || p.active_contacts === 0 ? (
                    <span className="text-amber-700">Add type and contact</span>
                  ) : (
                    <>{projectTypeLabel(p.type)} · {p.active_contacts} {p.active_contacts === 1 ? "contact" : "contacts"}</>
                  )}
                </td>
                <td className={`px-2 py-2 ${muted}`}>{p.open_cases} open</td>
                <td className="px-2 py-2 text-right">
                  {!p.is_system && (
                    <button type="button" aria-label={`Edit ${p.name}`} className="text-blue-700" onClick={() => setEditing({ kind: "edit-project", project: p })}>Edit</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      <section className={`${card} min-[900px]:col-start-2 min-[900px]:row-start-1`}>
        <div className="flex items-center justify-between">
          <h2 className="font-semibold">Drop reasons</h2>
          <button type="button" className={btnSmall} onClick={() => setEditing({ kind: "add-reason" })}>+ Add reason</button>
        </div>
        <ul className="mt-3 list-disc space-y-1 pl-5 text-sm">
          {reasons.map((r) => <li key={r.id}>{r.value}</li>)}
        </ul>
      </section>

      {editing?.kind === "add-bank" && (
        <OneFieldDialog title="Add a bank" field="Bank name" initial="" submitLabel="Add" onClose={() => setEditing(null)}
          onSubmit={(name) => api.request("POST", "/api/admin/banks", { name }).then(done)} />
      )}
      {editing?.kind === "rename" && (
        <OneFieldDialog title={`Rename ${editing.bank.name}`} field="Bank name" initial={editing.bank.name} submitLabel="Save" onClose={() => setEditing(null)}
          onSubmit={(name) => api.request("PUT", `/api/admin/banks/${editing.bank.id}`, { name }).then(done)} />
      )}
      {editing?.kind === "add-project" && <ProjectForm onClose={() => setEditing(null)} onSaved={done} />}
      {editing?.kind === "edit-project" && <ProjectForm project={editing.project} onClose={() => setEditing(null)} onSaved={done} />}
      {editing?.kind === "add-reason" && (
        <OneFieldDialog title="Add a drop reason" field="Reason" initial="" submitLabel="Add" onClose={() => setEditing(null)}
          onSubmit={(value) => api.request("POST", "/api/admin/dropdowns", { category: "drop_reason", value, sort_order: 100 }).then(done)} />
      )}
    </div>
  );
}

function OneFieldDialog({ title, field, initial, submitLabel, onClose, onSubmit }: {
  title: string; field: string; initial: string; submitLabel: string; onClose: () => void; onSubmit: (value: string) => Promise<unknown>;
}) {
  const [value, setValue] = useState(initial);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await onSubmit(value.trim());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save.");
    }
  }

  return (
    <Dialog open title={title} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>{field}<input className={input} value={value} onChange={(e) => setValue(e.target.value)} /></label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>{submitLabel}</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
