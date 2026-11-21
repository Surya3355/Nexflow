import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, ApiError } from "../api/client";
import type { Bank, Case, Customer, Project, StaffUser } from "../api/types";
import { Badge } from "../components/Badge";
import { Dialog } from "../components/Dialog";
import { DropDialog } from "../components/DropDialog";
import { ProgressBar } from "../components/ProgressBar";
import { ProjectPicker } from "../components/ProjectPicker";
import { alertBox, btn, btnDanger, btnPrimary, card, input, label, muted, page, rupees } from "../components/ui";

/** One customer: contact details, their cases, and edit / new case / drop (spec §5.3). */
export function CustomerDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [opening, setOpening] = useState(false);
  const [dropping, setDropping] = useState(false);
  const [bringError, setBringError] = useState<string | null>(null);
  const [bringing, setBringing] = useState(false);

  function load() {
    api.request<Customer>("GET", `/api/customers/${id}`)
      .then(setCustomer)
      .catch(() => setError("Could not load this customer."));
  }

  useEffect(() => {
    load();
    // load depends only on the customer ID from the route.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  if (error && !customer) return <p role="alert" className="p-4 text-red-700">{error}</p>;
  if (!customer) return <p className="p-4 text-slate-500">Loading…</p>;

  const active = customer.status === "active";

  async function bringBack() {
    if (bringing) return;
    setBringError(null);
    setBringing(true);
    try {
      await api.request<Customer>("POST", `/api/customers/${id}/reactivate`);
      load();
    } catch (err) {
      setBringError(err instanceof ApiError ? err.message : "Could not bring the customer back.");
    } finally {
      setBringing(false);
    }
  }
  const cases = customer.cases ?? [];

  return (
    <main className={`${page} max-w-[980px]`}>
      <Link to="/" className={muted}>← Back to home</Link>

      <section className={`${card} flex flex-wrap items-center justify-between gap-3`}>
        <div>
          <h1 className="text-xl font-semibold">{customer.name}</h1>
          <p className={`${muted} mt-1`}>
            {customer.mobile}{customer.email ? ` · ${customer.email}` : ""}
            {!active && <> · <Badge tone="bad">Dropped</Badge></>}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <a className={btn} href={`tel:${customer.mobile}`}>Call</a>
          <button type="button" className={btn} onClick={() => setEditing(true)}>Edit</button>
          {active ? (
            <button type="button" className={btnPrimary} onClick={() => setOpening(true)}>+ New case</button>
          ) : (
            <button type="button" className={btnPrimary} disabled={bringing} onClick={bringBack}>Bring back</button>
          )}
        </div>
        {bringError && <p role="alert" className={`${alertBox} w-full`}>{bringError}</p>}
      </section>

      <h2 className="font-semibold">Cases ({cases.length})</h2>
      {cases.length === 0 ? (
        <p className={`${card} ${muted}`}>No cases yet.</p>
      ) : (
        <ul className="divide-y divide-slate-200 overflow-hidden rounded-xl border border-slate-200 bg-white">
          {cases.map((c) => (
            <li key={c.id}>
              <Link to={`/cases/${c.id}`} className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1 px-4 py-3 text-sm text-slate-900 hover:bg-slate-50">
                <span>
                  <b>{c.bank_name} · {c.project_name} · Case #{c.id}</b>
                  <span className="my-1.5 block"><ProgressBar done={c.stage_order} total={c.stage_count} /></span>
                  <span className="text-slate-500">
                    {c.stage_name}{c.tracking_number ? ` · Tracking ${c.tracking_number}` : ""} · Assigned to {c.assignee_name}
                  </span>
                </span>
                <span className="text-right">
                  <b>{rupees(c.loan_amount)}</b>
                  <span className="mt-1 block">
                    {c.status === "dropped" ? <Badge tone="bad">Dropped</Badge>
                      : c.status === "completed" ? <Badge tone="ok">Disbursed</Badge>
                      : c.needs_document ? <Badge tone="warn">No documents</Badge> : null}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}

      {active && <button type="button" className={btnDanger} onClick={() => setDropping(true)}>Drop customer…</button>}

      <EditCustomerDialog open={editing} customer={customer} onClose={() => setEditing(false)} onSaved={(c) => { setCustomer({ ...customer, ...c }); setEditing(false); }} />
      <NewCaseDialog open={opening} customerId={customer.id} onClose={() => setOpening(false)} onOpened={(c) => navigate(`/cases/${c.id}`)} />
      <DropDialog
        open={dropping}
        title={`Drop ${customer.name}?`}
        confirmLabel="Drop customer"
        onClose={() => setDropping(false)}
        onConfirm={async (reasonId, remark) => {
          await api.request("POST", `/api/customers/${customer.id}/drop`, { reason_id: reasonId, remark });
          setDropping(false);
          load();
        }}
      />
    </main>
  );
}

function EditCustomerDialog({ open, customer, onClose, onSaved }: {
  open: boolean; customer: Customer; onClose: () => void; onSaved: (c: Customer) => void;
}) {
  const [name, setName] = useState(customer.name);
  const [mobile, setMobile] = useState(customer.mobile);
  const [email, setEmail] = useState(customer.email);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName(customer.name);
      setMobile(customer.mobile);
      setEmail(customer.email);
      setError(null);
    }
  }, [open, customer]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      const saved = await api.request<Customer>("PUT", `/api/customers/${customer.id}`, { name: name.trim(), mobile: mobile.trim(), email: email.trim() });
      onSaved(saved);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save.");
    }
  }

  return (
    <Dialog open={open} title="Edit customer" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Name<input className={input} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className={label}>Mobile<input className={input} inputMode="tel" value={mobile} onChange={(e) => setMobile(e.target.value)} /></label>
        <label className={label}>Email (optional)<input type="email" className={input} value={email} onChange={(e) => setEmail(e.target.value)} /></label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}

function NewCaseDialog({ open, customerId, onClose, onOpened }: {
  open: boolean; customerId: number; onClose: () => void; onOpened: (c: Case) => void;
}) {
  const [banks, setBanks] = useState<Bank[]>([]);
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [bankId, setBankId] = useState("");
  const [projectId, setProjectId] = useState<number | null>(null);
  const [amount, setAmount] = useState("");
  const [assigneeId, setAssigneeId] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
    api.request<StaffUser[]>("GET", "/api/users").then(setUsers).catch(() => setUsers([]));
    api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (projectId === null) {
      setError("Choose a project, or NA.");
      return;
    }
    try {
      const c = await api.request<Case>("POST", "/api/cases", {
        customer_id: customerId, bank_id: Number(bankId), project_id: projectId, loan_amount: amount.trim(), assignee_id: Number(assigneeId),
      });
      onOpened(c);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not open the case.");
    }
  }

  return (
    <Dialog open={open} title="New case" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Bank
          <select aria-label="Bank for new case" className={input} value={bankId} onChange={(e) => setBankId(e.target.value)}>
            <option value="">Choose a bank</option>
            {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
          </select>
        </label>
        <div className={label}>Project
          <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} aria-label="Project" />
        </div>
        <label className={label}>Loan amount (₹)<input inputMode="decimal" className={input} value={amount} onChange={(e) => setAmount(e.target.value)} /></label>
        <label className={label}>Assigned to
          <select aria-label="Assigned to for new case" className={input} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)}>
            <option value="">Choose a person</option>
            {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
          </select>
        </label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Open case</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
