import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { Bank, CaseRow, Project, StaffUser } from "../api/types";
import { btn, card, muted, page } from "../components/ui";
import { projectTypeLabel } from "../api/projectTypes";
import { CaseTable } from "./home/CaseTable";

const chipSelect = "rounded-full border border-slate-200 bg-white px-3 py-1 text-sm text-slate-700 hover:border-slate-400";

/**
 * One project: its type, address and builder contacts (spec 2026-11-24 §4.4),
 * then all its cases for every role (no "My cases" default) and a CSV
 * download with the same filters (spec 2026-11-21 §4.5).
 */
export function ProjectDetail() {
  const { id } = useParams();
  const [project, setProject] = useState<Project | null>(null);
  const [rows, setRows] = useState<CaseRow[] | null>(null);
  const [banks, setBanks] = useState<Bank[]>([]);
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [bankId, setBankId] = useState("");
  const [assigneeId, setAssigneeId] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<Project>("GET", `/api/config/projects/${id}`)
      .then(setProject)
      .catch(() => setProject(null));
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
    api.request<StaffUser[]>("GET", "/api/users").then(setUsers).catch(() => setUsers([]));
  }, [id]);

  const qs = useMemo(() => {
    const p = new URLSearchParams();
    p.set("project_id", id ?? "");
    if (bankId) p.set("bank_id", bankId);
    if (assigneeId) p.set("assignee_id", assigneeId);
    return p.toString();
  }, [id, bankId, assigneeId]);

  useEffect(() => {
    api.request<CaseRow[]>("GET", `/api/cases?${qs}`)
      .then((r) => { setRows(r); setError(null); })
      .catch(() => { setRows([]); setError("Could not load cases."); });
  }, [qs]);

  return (
    <main className={page}>
      <Link to="/projects" className={muted}>← All projects</Link>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="min-w-0 break-words text-xl font-semibold">{project?.name ?? "Project"}</h1>
        <a className={btn} href={`/api/reports/export.csv?${qs}`}>⬇ Download CSV</a>
      </div>
      {project && !project.is_system && (project.type || project.address || (project.contacts?.length ?? 0) > 0) && (
        <section aria-label="About this project" className={card}>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-base">
            {project.type && <><dt className="text-slate-500">Type</dt><dd className="font-semibold">{projectTypeLabel(project.type)}</dd></>}
            {project.address && <><dt className="text-slate-500">Address</dt><dd className="min-w-0 break-words font-semibold">{project.address}</dd></>}
            {(project.contacts ?? []).length > 0 && (
              <>
                <dt className="text-slate-500">Contacts</dt>
                <dd className="min-w-0">
                  <ul className="space-y-1">
                    {(project.contacts ?? []).map((c) => (
                      <li key={c.id} className="min-w-0 [overflow-wrap:anywhere]">
                        <span className="font-semibold">{[c.label, c.name].filter(Boolean).join(" · ")}</span>{" "}
                        <a href={`tel:+91${c.phone}`} className="text-blue-700">{c.phone}</a>
                        {c.email && <> · <a href={`mailto:${c.email}`} className="text-blue-700">{c.email}</a></>}
                      </li>
                    ))}
                  </ul>
                </dd>
              </>
            )}
          </dl>
        </section>
      )}
      <div className="flex flex-wrap gap-1.5">
        <select aria-label="Bank" className={chipSelect} value={bankId} onChange={(e) => setBankId(e.target.value)}>
          <option value="">Bank: all</option>
          {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
        </select>
        <select aria-label="Assigned to" className={chipSelect} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)}>
          <option value="">Assigned: anyone</option>
          {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
        </select>
      </div>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      {rows === null ? <p className={muted}>Loading…</p> : rows.length === 0 ? (
        <p className={muted}>No cases in this project yet.</p>
      ) : (
        <CaseTable rows={rows} />
      )}
    </main>
  );
}
