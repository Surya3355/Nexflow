import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, ApiError } from "../api/client";
import type { Customer, LeadRow } from "../api/types";
import { Badge } from "../components/Badge";
import { AddCaseDialog } from "../components/AddCaseDialog";
import { NewLeadDialog } from "./lead/NewLeadDialog";
import { ResponseBadge, shortDay } from "./home/LeadList";
import { Dialog } from "../components/Dialog";
import { DropDialog } from "../components/DropDialog";
import { ProgressBar } from "../components/ProgressBar";
import { alertBox, btn, btnDanger, btnPrimary, btnSmall, card, input, label, muted, page, rupees } from "../components/ui";

/** One customer: contact details, their cases, and edit / new case / drop (spec §5.3). */
export function CustomerDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [opening, setOpening] = useState(false);
  const [addingLead, setAddingLead] = useState(false);
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
  const leads = customer.leads ?? [];

  return (
    <main className={`${page} max-w-[980px]`}>
      <Link to="/" className={muted}>← Back to home</Link>

      <section className={`${card} flex flex-wrap items-center justify-between gap-3`}>
        <div>
          <h1 className="text-2xl font-bold [overflow-wrap:anywhere]">{customer.name}</h1>
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

      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-semibold">Leads ({leads.length})</h2>
        {active && <button type="button" className={btnSmall} onClick={() => setAddingLead(true)}>+ New lead</button>}
      </div>
      {leads.length === 0 ? <p className={`${card} ${muted}`}>No leads.</p> : <CustomerLeads leads={leads} />}

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
      <NewLeadDialog open={addingLead} customerId={customer.id} customerName={customer.name} assigneeId={leads[0]?.assignee_id ?? cases[0]?.assignee_id ?? 0}
        onClose={() => setAddingLead(false)} onSaved={(l) => navigate(`/leads/${l.id}`)} />
      <AddCaseDialog open={opening} customerId={customer.id} customerName={customer.name} onClose={() => setOpening(false)} onSaved={(c) => navigate(`/cases/${c.id}`)} />
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

function CustomerLeads({ leads }: { leads: LeadRow[] }) {
  return (
    <ul className="divide-y divide-slate-200 overflow-hidden rounded-xl border border-slate-200 bg-white">
      {leads.map((l) => (
        <li key={l.id}>
          <Link to={`/leads/${l.id}`} className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1 px-4 py-3 text-sm text-slate-900 hover:bg-slate-50">
            <b>{l.project_name}</b>
            <span className="text-right">
              {l.status === "open" ? <ResponseBadge lead={l} /> : l.status === "converted" ? <Badge tone="ok">Case opened</Badge> : <Badge tone="bad">Dropped</Badge>}
            </span>
            <span className="text-slate-500">Assigned to {l.assignee_name}</span>
            <span className="text-right text-slate-500">{l.status === "open" && l.next_call_date ? `Next call ${shortDay(l.next_call_date)}` : ""}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
