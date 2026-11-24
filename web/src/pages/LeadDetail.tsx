import { BuilderContactRow } from "../components/BuilderContactRow";
import { BuilderContactPicker } from "../components/BuilderContactPicker";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, ApiError } from "../api/client";
import type { LeadContact, LeadRow } from "../api/types";
import { AddCaseDialog } from "../components/AddCaseDialog";
import { AssignDialog } from "../components/AssignDialog";
import { Badge } from "../components/Badge";
import { DropDialog } from "../components/DropDialog";
import { alertBox, btn, btnDanger, btnPrimary, card, istDate, muted, page } from "../components/ui";
import { shortDay } from "./home/LeadList";
import { ContactLog } from "./lead/ContactLog";
import { LogContactDialog } from "./lead/LogContactDialog";
import { NewLeadDialog } from "./lead/NewLeadDialog";

/** One lead: the customer, the project, the contact log, and Add case (spec 2026-11-23 §4.4). */
export function LeadDetail() {
  const { id } = useParams();
  const leadId = Number(id);
  const navigate = useNavigate();
  const [lead, setLead] = useState<LeadRow | null>(null);
  const [contacts, setContacts] = useState<LeadContact[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [logging, setLogging] = useState(false);
  const [dropping, setDropping] = useState<"plain" | "not_interested" | null>(null);
  const [adding, setAdding] = useState(false);
  const [newProject, setNewProject] = useState(false);
  const [assigning, setAssigning] = useState(false);
  const [pickingContact, setPickingContact] = useState(false);

  async function load() {
    const res = await api.request<{ lead: LeadRow; contacts: LeadContact[] }>("GET", `/api/leads/${leadId}`);
    setLead(res.lead);
    setContacts(res.contacts);
  }

  useEffect(() => {
    load().catch(() => setError("Could not load this lead."));
    // load depends only on the lead ID from the route.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [leadId]);

  if (error && !lead) return <p role="alert" className="p-4 text-red-700">{error}</p>;
  if (!lead) return <p className="p-4 text-slate-500">Loading…</p>;

  const open = lead.status === "open";

  async function bringBack() {
    setError(null);
    try {
      await api.request("POST", `/api/leads/${leadId}/bring-back`);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not bring the lead back.");
    }
  }

  return (
    <main className={`${page} max-w-[900px]`}>
      <Link to="/" className={muted}>← Back to home</Link>

      <section className={card}>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="text-2xl font-bold">{lead.customer_name}</h1>
            <p className="mt-1 text-base text-slate-500">
              <a href={`tel:${lead.customer_mobile}`} className="font-semibold text-blue-700">{lead.customer_mobile}</a>
              {lead.customer_email ? ` · ${lead.customer_email}` : ""}
            </p>
          </div>
          {lead.status === "dropped" ? <Badge tone="bad">Dropped</Badge> : lead.status === "converted" ? <Badge tone="ok">Case opened</Badge> : <Badge>Lead</Badge>}
        </div>
      </section>

      {error && <p role="alert" className={alertBox}>{error}</p>}

      <section aria-label="Details" className={card}>
        <h2 className="mb-2 font-semibold">Details</h2>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-base">
          <dt className="text-slate-500">Project</dt><dd className="font-semibold">{lead.project_name}</dd>
          <BuilderContactRow contact={lead.builder_contact} projectId={lead.project_id} onChange={open ? () => setPickingContact(true) : undefined} />
          <dt className="text-slate-500">Assigned to</dt>
          <dd className="font-semibold">
            {lead.assignee_name}
            {open && <button type="button" aria-label="Change assignee" className="ml-2 text-sm font-normal text-blue-700" onClick={() => setAssigning(true)}>change</button>}
          </dd>
          {open && <><dt className="text-slate-500">Next call</dt><dd className="font-semibold">{lead.next_call_date ? shortDay(lead.next_call_date) : "Not set"}</dd></>}
          <dt className="text-slate-500">Added</dt><dd className="font-semibold">{istDate(lead.created_at)}</dd>
        </dl>
      </section>

      <div className="flex flex-wrap gap-2">
        {open && (
          <>
            <button type="button" className={btn} onClick={() => setLogging(true)}>Log contact</button>
            <button type="button" className={btnPrimary} onClick={() => setAdding(true)}>Customer is in: Add case</button>
            <button type="button" className={btn} onClick={() => setNewProject(true)}>+ New project</button>
            <button type="button" className={btnDanger} onClick={() => setDropping("plain")}>Drop…</button>
          </>
        )}
        {lead.status === "dropped" && <button type="button" className={btnPrimary} onClick={bringBack}>Bring back</button>}
        {lead.status === "converted" && (lead.case_id
          ? <Link to={`/cases/${lead.case_id}`} className={btnPrimary}>Moved to case #{lead.case_id}</Link>
          : <p className={muted}>Its case was deleted.</p>)}
      </div>

      <section className={card}>
        <h2 className="mb-1 font-semibold">Contact log</h2>
        <ContactLog contacts={contacts} />
      </section>

      <LogContactDialog
        open={logging}
        leadId={leadId}
        onClose={() => setLogging(false)}
        onSaved={async (response) => {
          setLogging(false);
          await load();
          if (response === "not_interested") setDropping("not_interested");
        }}
      />
      <DropDialog
        open={dropping !== null}
        title={dropping === "not_interested" ? "Drop this lead?" : `Drop the lead for ${lead.customer_name}?`}
        confirmLabel="Drop lead"
        onClose={() => setDropping(null)}
        onConfirm={async (reasonId, remark) => {
          await api.request("POST", `/api/leads/${leadId}/drop`, { reason_id: reasonId, remark });
          setDropping(null);
          await load();
        }}
      />
      <BuilderContactPicker
        open={pickingContact}
        projectId={lead.project_id}
        current={lead.builder_contact_id}
        onClose={() => setPickingContact(false)}
        onPick={async (contactId) => {
          await api.request("POST", `/api/leads/${leadId}/builder-contact`, { contact_id: contactId });
          setPickingContact(false);
          await load();
        }}
      />
      <AddCaseDialog
        open={adding}
        onClose={() => setAdding(false)}
        onSaved={(c) => navigate(`/cases/${c.id}`)}
        customerName={lead.customer_name}
        fromLead={{ id: lead.id, projectId: lead.project_id, projectName: lead.project_name, assigneeId: lead.assignee_id, builderContactId: lead.builder_contact_id }}
      />
      <NewLeadDialog
        open={newProject}
        customerId={lead.customer_id}
        customerName={lead.customer_name}
        assigneeId={lead.assignee_id}
        onClose={() => setNewProject(false)}
        onSaved={(l) => { setNewProject(false); navigate(`/leads/${l.id}`); }}
      />
      <AssignDialog
        open={assigning}
        title="Assign lead"
        path={`/api/leads/${leadId}/assign`}
        current={lead.assignee_id}
        onClose={() => setAssigning(false)}
        onSaved={async () => { setAssigning(false); await load(); }}
      />
    </main>
  );
}
