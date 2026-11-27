import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import type { StaffUser } from "../api/types";
import { Dialog } from "./Dialog";
import { alertBox, btn, btnPrimary, input, label } from "./ui";

type Props = { open: boolean; title: string; path: string; current: number; onClose: () => void; onSaved: () => void };

/** Reassigns a case or a lead: posts { assignee_id } to path. */
export function AssignDialog({ open, title, path, current, onClose, onSaved }: Props) {
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [assigneeId, setAssigneeId] = useState(String(current));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setAssigneeId(String(current));
    api.request<StaffUser[]>("GET", "/api/users").then(setUsers).catch(() => setUsers([]));
  }, [open, current]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      await api.request("POST", path, { assignee_id: Number(assigneeId) });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reassign.");
    }
  }

  return (
    <Dialog open={open} title={title} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Assigned to
          <select className={input} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)}>
            {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
          </select>
        </label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
