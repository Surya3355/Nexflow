import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import type { LeadRow, Project, StaffUser } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { ProjectPicker } from "../../components/ProjectPicker";
import { alertBox, btn, btnPrimary, input, label } from "../../components/ui";

type Props = {
  open: boolean; customerId: number; customerName: string; assigneeId: number;
  onClose: () => void; onSaved: (lead: LeadRow) => void;
};

/** Another lead (a new project) for a customer who already has one. */
export function NewLeadDialog({ open, customerId, customerName, assigneeId, onClose, onSaved }: Props) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [projectId, setProjectId] = useState<number | null>(null);
  const [assignee, setAssignee] = useState(String(assigneeId));
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setAssignee(String(assigneeId));
    api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
    api.request<StaffUser[]>("GET", "/api/users").then((list) => {
      setUsers(list);
      // With no known assignee, start on the first active staff member.
      if (!list.some((u) => u.id === assigneeId) && list.length > 0) setAssignee(String(list[0].id));
    }).catch(() => setUsers([]));
  }, [open, assigneeId]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (projectId === null) {
      setError("Choose a project, or NA.");
      return;
    }
    setError(null);
    setBusy(true);
    try {
      const lead = await api.request<LeadRow>("POST", `/api/customers/${customerId}/leads`, {
        project_id: projectId, assignee_id: Number(assignee), bring_back: false,
      });
      onSaved(lead);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add the lead.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} title={`New project for ${customerName}`} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <div className={label}>Project
          <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} aria-label="Project" />
        </div>
        <label className={label}>Assigned to
          <select className={input} value={assignee} onChange={(e) => setAssignee(e.target.value)}>
            {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
          </select>
        </label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={busy}>Save lead</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
