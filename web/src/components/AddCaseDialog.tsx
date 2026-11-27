import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import type { Bank, BuilderContact, Case, Project, ProjectType, StaffUser, Stage } from "../api/types";
import { unitLabel, unitPlaceholder } from "../api/projectTypes";
import { Dialog } from "./Dialog";
import { OfficerSelect } from "./OfficerSelect";
import { ProjectPicker } from "./ProjectPicker";
import { alertBox, btn, btnPrimary, input, label, muted } from "./ui";

type Props = {
  open: boolean;
  onClose: () => void;
  onSaved: (c: Case) => void;
  customerName: string;
  /** Adding from a lead: the project is fixed and the lead becomes the case. */
  fromLead?: { id: number; projectId: number; projectName: string; assigneeId: number; builderContactId?: number | null };
  /** The customer, for a case added without a lead. */
  customerId?: number;
};

/**
 * One Add case form for a lead, the customer page and the case page
 * (spec 2026-11-23 §4.5, mockup round15/add-case.html). Starting at Logged in
 * needs the bank's tracking number. Save is disabled while it is sending.
 */
export function AddCaseDialog({ open, onClose, onSaved, customerName, fromLead, customerId }: Props) {
  const [banks, setBanks] = useState<Bank[]>([]);
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState<number | null>(null);
  const [unit, setUnit] = useState("");
  const [bankId, setBankId] = useState("");
  const [amount, setAmount] = useState("");
  const [assigneeId, setAssigneeId] = useState(fromLead ? String(fromLead.assigneeId) : "");
  const [startAt, setStartAt] = useState<"received" | "logged_in">("received");
  const [tracking, setTracking] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [contacts, setContacts] = useState<BuilderContact[]>([]);
  const [contactId, setContactId] = useState("");
  const [fetchedType, setFetchedType] = useState<ProjectType | null>(null);
  const [bankStages, setBankStages] = useState<Stage[]>([]);
  const [picked, setPicked] = useState<Record<string, number | null>>({});
  const sending = useRef(false);
  // Banks on their own stage list (HDFC) always start at their start stage (spec 2026-11-25 §5.5).
  const ownList = banks.find((b) => String(b.id) === bankId)?.stage_list === "hdfc";
  const start = ownList ? "" : startAt;

  // The bank's stages, for the officer teams the new case passes through (spec 2026-11-26 §5.3).
  useEffect(() => {
    setBankStages([]);
    setPicked({});
    if (!open || !bankId) return;
    let live = true;
    api.request<Stage[]>("GET", `/api/config/banks/${bankId}/stages`)
      .then((st) => live && setBankStages(st))
      .catch(() => live && setBankStages([]));
    return () => { live = false; };
  }, [open, bankId]);
  const landing = bankStages.find((s) => s.role === (start === "logged_in" ? "tracking" : "start"));
  const teams = landing ? bankStages.filter((s) => s.officer_team && s.sort_order <= landing.sort_order) : [];

  // The project's current contacts, prefilled with the lead's contact while it
  // is still on the list, else the first one (spec 2026-11-24 §4.3).
  const pid = fromLead ? fromLead.projectId : projectId;
  const isNA = !fromLead && projects.find((p) => p.id === projectId)?.is_system === true;
  const leadContact = fromLead?.builderContactId ?? null;
  useEffect(() => {
    setContacts([]);
    setContactId("");
    setFetchedType(null);
    if (!open || pid === null || isNA) return;
    let live = true;
    api.request<Project>("GET", `/api/config/projects/${pid}`)
      .then((p) => {
        if (!live) return;
        const list = p.contacts ?? [];
        setContacts(list);
        setFetchedType(p.type ?? null);
        const keep = list.find((c) => c.id === leadContact) ?? list[0];
        setContactId(keep ? String(keep.id) : "");
      })
      .catch(() => live && setContacts([]));
    return () => { live = false; };
  }, [open, pid, isNA, leadContact]);

  useEffect(() => {
    if (!open) return;
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
    api.request<StaffUser[]>("GET", "/api/users").then(setUsers).catch(() => setUsers([]));
    if (!fromLead) api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
  }, [open, fromLead]);

  // The unit no. is named by the project's type and required when it has one.
  const projectType = fromLead ? fetchedType : projects.find((p) => p.id === projectId)?.type ?? null;
  const unitName = unitLabel(projectType);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (sending.current) return;
    setError(null);
    if (!fromLead && projectId === null) {
      setError("Choose a project, or NA.");
      return;
    }
    if (!bankId) {
      setError("Choose a bank.");
      return;
    }
    if (!assigneeId) {
      setError("Choose who it is assigned to.");
      return;
    }
    if (projectType && !unit.trim()) {
      setError(`Enter the ${unitName.toLowerCase()}.`);
      return;
    }
    if (start === "logged_in" && !tracking.trim()) {
      setError("Enter the tracking number.");
      return;
    }
    if (contacts.length > 0 && !contactId) {
      setError("Choose a builder contact.");
      return;
    }
    const body = {
      bank_id: Number(bankId), loan_amount: amount.trim(), assignee_id: Number(assigneeId),
      unit_number: unit.trim(), start_at: start, tracking_number: start === "logged_in" ? tracking.trim() : "",
      ...(contacts.length > 0 && contactId ? { builder_contact_id: Number(contactId) } : {}),
    };
    const chosen = Object.fromEntries(teams.flatMap((s) => {
      const v = picked[s.officer_team ?? ""];
      return v ? [[s.officer_team, v]] : [];
    }));
    if (Object.keys(chosen).length > 0) Object.assign(body, { officers: chosen });
    sending.current = true;
    setBusy(true);
    try {
      const c = fromLead
        ? await api.request<Case>("POST", `/api/leads/${fromLead.id}/case`, body)
        : await api.request<Case>("POST", "/api/cases", { customer_id: customerId, project_id: projectId, ...body });
      onSaved(c);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not open the case.");
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} title={fromLead ? `Add case for ${customerName}` : "New case"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        {fromLead ? (
          <p className={muted}>Project: <b className="text-slate-900">{fromLead.projectName}</b></p>
        ) : (
          <div className={label}>Project
            <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} aria-label="Project" />
          </div>
        )}
        {contacts.length > 0 && (
          <label className={label}>Builder contact
            <select className={input} value={contactId} onChange={(e) => setContactId(e.target.value)}>
              {contacts.map((c) => <option key={c.id} value={c.id}>{[c.label, c.name, c.phone].filter(Boolean).join(" · ")}</option>)}
            </select>
          </label>
        )}
        <label className={label}>{unitName}<input className={input} placeholder={unitPlaceholder} value={unit} onChange={(e) => setUnit(e.target.value.toUpperCase())} /></label>
        <label className={label}>Bank
          <select aria-label="Bank for new case" className={input} value={bankId} onChange={(e) => setBankId(e.target.value)}>
            <option value="">Choose a bank</option>
            {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
          </select>
        </label>
        <label className={label}>Loan amount (₹)<input inputMode="decimal" className={input} value={amount} onChange={(e) => setAmount(e.target.value)} /></label>
        <label className={label}>Assigned to
          <select aria-label="Assigned to for new case" className={input} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)}>
            <option value="">Choose a person</option>
            {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
          </select>
        </label>
        {!ownList && (
          <label className={label}>Start at
            <select className={input} value={startAt} onChange={(e) => setStartAt(e.target.value as "received" | "logged_in")}>
              <option value="received">Received</option>
              <option value="logged_in">Logged in</option>
            </select>
          </label>
        )}
        {start === "logged_in" && (
          <label className={label}>Tracking number
            <input className={input} value={tracking} onChange={(e) => setTracking(e.target.value.toUpperCase())} placeholder="Bank's loan application number" />
          </label>
        )}
        {bankId && teams.map((s) => (
          <OfficerSelect key={s.officer_team} bankId={Number(bankId)} team={s.officer_team ?? ""} teamName={s.officer_team_name ?? ""}
            value={picked[s.officer_team ?? ""] ?? null} onChange={(v) => setPicked((p) => ({ ...p, [s.officer_team ?? ""]: v }))} />
        ))}
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={busy}>Open case</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
