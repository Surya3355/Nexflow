import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, ApiError } from "../../api/client";
import type { Bank, Case, Customer, Project, StaffUser } from "../../api/types";
import { ProjectPicker } from "../../components/ProjectPicker";
import { SidePanel } from "../../components/SidePanel";
import { alertBox, btn, btnPrimary, btnSmall, input, label, muted, warnBox } from "../../components/ui";

type Props = { open: boolean; onClose: () => void; banks: Bank[]; projects: Project[]; users: StaffUser[]; meId: number };

const chooseProject = "Choose a project, or NA.";

/**
 * Adds a customer and, if a bank is chosen, their first case in one step.
 * A known mobile creates no new customer: the panel offers to add the case to
 * the existing customer instead, bringing them back first if they were dropped
 * (spec 2026-11-21 §4.6). Nothing typed is lost on an error.
 */
export function NewCustomerPanel({ open, onClose, banks, projects, users, meId }: Props) {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [mobile, setMobile] = useState("");
  const [email, setEmail] = useState("");
  const [bankId, setBankId] = useState("");
  const [projectId, setProjectId] = useState<number | null>(null);
  const [amount, setAmount] = useState("");
  const [assigneeId, setAssigneeId] = useState(String(meId));
  const [existing, setExisting] = useState<Customer | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [savedButCaseFailed, setSavedButCaseFailed] = useState<{ customer: Customer; message: string } | null>(null);
  const [busy, setBusy] = useState(false);

  function openCase(customerId: number, bringBack: boolean) {
    return api.request<Case>("POST", "/api/cases", {
      customer_id: customerId, bank_id: Number(bankId), project_id: projectId, loan_amount: amount.trim(),
      assignee_id: Number(assigneeId), bring_back: bringBack,
    });
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setExisting(null);
    setSavedButCaseFailed(null);
    if (bankId && projectId === null) {
      setError(chooseProject);
      return;
    }
    setBusy(true);
    try {
      const res = await api.request<{ customer?: Customer; existing?: Customer }>("POST", "/api/customers", {
        name: name.trim(), mobile: mobile.trim(), email: email.trim(),
      });
      if (res.existing) {
        setExisting(res.existing);
        return;
      }
      const customer = res.customer!;
      if (!bankId) {
        navigate(`/customers/${customer.id}`);
        return;
      }
      try {
        const c = await openCase(customer.id, false);
        navigate(`/cases/${c.id}`);
      } catch (err) {
        setSavedButCaseFailed({ customer, message: err instanceof ApiError ? err.message : "Could not open the case." });
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the customer.");
    } finally {
      setBusy(false);
    }
  }

  /** Adds the typed case to the existing customer, bringing them back if dropped. */
  async function addToExisting(customer: Customer) {
    setError(null);
    setBusy(true);
    try {
      const c = await openCase(customer.id, customer.status === "dropped");
      navigate(`/cases/${c.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not open the case.");
    } finally {
      setBusy(false);
    }
  }

  async function bringBack(customer: Customer) {
    setError(null);
    setBusy(true);
    try {
      await api.request<Customer>("POST", `/api/customers/${customer.id}/reactivate`);
      navigate(`/customers/${customer.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not bring the customer back.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <SidePanel open={open} title="New customer" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Name<input className={input} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className={label}>Mobile<input className={input} inputMode="tel" placeholder="10 digits, +91 allowed" value={mobile} onChange={(e) => setMobile(e.target.value)} /></label>
        {existing && (
          <div role="status" className={`${warnBox} space-y-2`}>
            <p>
              This mobile already belongs to <Link to={`/customers/${existing.id}`} className="underline">{existing.name}</Link>
              {existing.status === "dropped" ? ", who was dropped" : ""}. No new customer was created.
            </p>
            {bankId ? (
              <button type="button" className={btnSmall} disabled={busy} onClick={() => addToExisting(existing)}>
                {existing.status === "dropped" ? `Bring back ${existing.name} and add this case` : `Add this case to ${existing.name}`}
              </button>
            ) : existing.status === "dropped" ? (
              <button type="button" className={btnSmall} disabled={busy} onClick={() => bringBack(existing)}>Bring back {existing.name}</button>
            ) : (
              <Link to={`/customers/${existing.id}`} className={btnSmall}>Open {existing.name}</Link>
            )}
          </div>
        )}
        <label className={label}>Email (optional)<input type="email" className={input} value={email} onChange={(e) => setEmail(e.target.value)} /></label>

        <h3 className="pt-3 font-semibold">First case</h3>
        <p className={muted}>Leave the bank empty to save just the customer.</p>
        <label className={label}>Bank
          <select className={input} value={bankId} onChange={(e) => setBankId(e.target.value)}>
            <option value="">No case yet</option>
            {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
          </select>
        </label>
        {bankId && (
          <div className={label}>Project
            <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} aria-label="Project" />
          </div>
        )}
        <label className={label}>Loan amount (₹)<input className={input} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} disabled={!bankId} /></label>
        <label className={label}>Assigned to
          <select className={input} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)} disabled={!bankId}>
            {users.map((u) => <option key={u.id} value={u.id}>{u.id === meId ? `${u.full_name} (me)` : u.full_name}</option>)}
          </select>
        </label>

        {error && <p role="alert" className={alertBox}>{error}</p>}
        {savedButCaseFailed && (
          <p role="alert" className={alertBox}>
            {savedButCaseFailed.customer.name} was saved, but the case was not opened: {savedButCaseFailed.message}{" "}
            <Link to={`/customers/${savedButCaseFailed.customer.id}`} className="underline">Open {savedButCaseFailed.customer.name}</Link>
          </p>
        )}
        <div className="flex gap-2 pt-2">
          <button type="submit" disabled={busy} className={btnPrimary}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </SidePanel>
  );
}
