import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, ApiError } from "../../api/client";
import type { Customer, LeadRow, Project, StaffUser } from "../../api/types";
import { ProjectPicker } from "../../components/ProjectPicker";
import { SidePanel } from "../../components/SidePanel";
import { alertBox, btn, btnPrimary, btnSmall, input, label, warnBox } from "../../components/ui";

type Props = { open: boolean; onClose: () => void; projects: Project[]; users: StaffUser[]; meId: number };

const chooseProject = "Choose a project, or NA.";

/**
 * New customer: name, mobile, email, project and assignee. Saving creates the
 * customer and their first lead, then opens the lead (spec 2026-11-23 §4.2).
 * A known mobile creates nothing: the panel offers to add the lead to that
 * customer, bringing them back first if they were dropped. Nothing typed is
 * lost on an error.
 */
export function NewCustomerPanel({ open, onClose, projects, users, meId }: Props) {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [mobile, setMobile] = useState("");
  const [email, setEmail] = useState("");
  const [projectId, setProjectId] = useState<number | null>(null);
  const [assigneeId, setAssigneeId] = useState(String(meId));
  const [existing, setExisting] = useState<Customer | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setExisting(null);
    if (projectId === null) {
      setError(chooseProject);
      return;
    }
    setBusy(true);
    try {
      const res = await api.request<{ lead?: LeadRow; existing?: Customer }>("POST", "/api/leads", {
        name: name.trim(), mobile: mobile.trim(), email: email.trim(), project_id: projectId, assignee_id: Number(assigneeId),
      });
      if (res.existing) {
        setExisting(res.existing);
        return;
      }
      navigate(`/leads/${res.lead!.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the lead.");
    } finally {
      setBusy(false);
    }
  }

  /** Adds the lead to the existing customer, bringing them back if dropped. */
  async function addToExisting(customer: Customer) {
    setError(null);
    setBusy(true);
    try {
      const lead = await api.request<LeadRow>("POST", `/api/customers/${customer.id}/leads`, {
        project_id: projectId, assignee_id: Number(assigneeId), bring_back: customer.status === "dropped",
      });
      navigate(`/leads/${lead.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add the lead.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <SidePanel open={open} title="New customer" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Name<input className={input} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className={label}>Mobile number<input className={input} inputMode="tel" placeholder="10 digits, +91 allowed" value={mobile} onChange={(e) => setMobile(e.target.value)} /></label>
        {existing && (
          <div role="status" className={`${warnBox} space-y-2`}>
            <p>
              This mobile already belongs to <Link to={`/customers/${existing.id}`} className="underline">{existing.name}</Link>
              {existing.status === "dropped" ? ", who was dropped" : ""}. No new customer was created.
            </p>
            <button type="button" className={btnSmall} disabled={busy} onClick={() => addToExisting(existing)}>
              {existing.status === "dropped" ? `Bring back ${existing.name} and add lead` : `Add lead for ${existing.name}`}
            </button>
          </div>
        )}
        <label className={label}>Email (optional)<input type="email" className={input} value={email} onChange={(e) => setEmail(e.target.value)} /></label>
        <div className={label}>Project
          <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} aria-label="Project" />
        </div>
        <label className={label}>Assigned to
          <select className={input} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)}>
            {users.map((u) => <option key={u.id} value={u.id}>{u.id === meId ? `${u.full_name} (me)` : u.full_name}</option>)}
          </select>
        </label>

        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2 pt-2">
          <button type="submit" disabled={busy} className={btnPrimary}>Save lead</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </SidePanel>
  );
}
