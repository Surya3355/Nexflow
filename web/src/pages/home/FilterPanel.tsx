import { useEffect, useState } from "react";
import { api } from "../../api/client";
import type { Bank, Project, Stage, StaffUser } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { ProjectPicker } from "../../components/ProjectPicker";
import { btn, btnPrimary, input, label } from "../../components/ui";
import { noFilters, type Filters, type FilterScope } from "./leadFilter";

type Props = {
  open: boolean;
  scope: FilterScope;
  value: Filters;
  banks: Bank[];
  projects: Project[];
  users: StaffUser[];
  onClose: () => void;
  onApply: (f: Filters) => void;
};

/**
 * The Home filter panel (spec 2026-11-24 home filter). It edits a copy, so
 * nothing changes until Apply. Leads show only Project and Assigned to; My
 * cases hides Assigned to. Fields a tab hides keep their value for the others.
 */
export function FilterPanel({ open, scope, value, banks, projects, users, onClose, onApply }: Props) {
  const [draft, setDraft] = useState<Filters>(value);
  const [stages, setStages] = useState<Stage[]>([]);

  useEffect(() => {
    if (open) setDraft(value);
  }, [open, value]);

  useEffect(() => {
    if (!open || !draft.bankId) {
      setStages([]);
      return;
    }
    let live = true;
    api.request<Stage[]>("GET", `/api/config/banks/${draft.bankId}/stages`)
      .then((s) => live && setStages(s))
      .catch(() => live && setStages([]));
    return () => { live = false; };
  }, [open, draft.bankId]);

  const cases = scope !== "leads";
  const set = (patch: Partial<Filters>) => setDraft((d) => ({ ...d, ...patch }));

  return (
    <Dialog open={open} title="Filter" onClose={onClose}>
      <div className="max-h-[70vh] space-y-3 overflow-y-auto">
        {cases && (
          <label className={label}>Bank
            <select aria-label="Bank" className={input} value={draft.bankId} onChange={(e) => set({ bankId: e.target.value, stageId: "" })}>
              <option value="">All banks</option>
              {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
            </select>
          </label>
        )}
        <div className={label}>Project
          <ProjectPicker projects={projects} value={draft.projectId} onChange={(id) => set({ projectId: id })} allowAll aria-label="Project" placeholder="All projects" />
        </div>
        {cases && (
          <label className={label}>Stage
            <select aria-label="Stage" className={input} value={draft.stageId} onChange={(e) => set({ stageId: e.target.value })}
              disabled={!draft.bankId} title={draft.bankId ? undefined : "Choose a bank first"}>
              <option value="">{draft.bankId ? "All stages" : "Choose a bank first"}</option>
              {stages.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </label>
        )}
        {scope !== "mine" && (
          <label className={label}>Assigned to
            <select aria-label="Assigned to" className={input} value={draft.assigneeId} onChange={(e) => set({ assigneeId: e.target.value })}>
              <option value="">Anyone</option>
              {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
            </select>
          </label>
        )}
        {cases && (
          <div className="grid grid-cols-2 gap-2">
            <label className={label}>From<input type="date" aria-label="From" className={input} value={draft.from} onChange={(e) => set({ from: e.target.value })} /></label>
            <label className={label}>To<input type="date" aria-label="To" className={input} value={draft.to} onChange={(e) => set({ to: e.target.value })} /></label>
          </div>
        )}
        <div className="flex gap-2 pt-1">
          <button type="button" className={btnPrimary} onClick={() => onApply(draft)}>Apply</button>
          <button type="button" className={btn} onClick={() => onApply(noFilters)}>Clear all</button>
        </div>
      </div>
    </Dialog>
  );
}
