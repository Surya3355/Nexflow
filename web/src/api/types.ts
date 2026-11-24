// Shapes returned by the Go API. Money is a decimal string, never a float.

export type Role = "admin" | "manager" | "employee";

export type Case = {
  id: number;
  customer_id: number;
  bank_id: number;
  project_id: number;
  loan_amount: string;
  tracking_number?: string;
  current_stage_id: number;
  assignee_id: number;
  status: "active" | "dropped" | "completed";
  follow_up_date?: string;
  created_at: string;
  warning?: string;
  /** The lead this case came from, if any. */
  lead_id?: number | null;
  /** Flat, villa or plot number. */
  unit_number?: string | null;
  /** The project contact staff call about this case (null for NA or none yet). */
  builder_contact_id?: number | null;
  /** Filled on a single case (GET /api/cases/{id} and writes), not on lists. */
  builder_contact?: BuilderContact | null;
  /** The project's type (null for NA and untyped); single case only. Drives the unit label. */
  project_type?: ProjectType | null;
};

export type Stage = {
  id: number;
  code: string;
  name: string;
  sort_order: number;
  requires_document: boolean;
};

export type StageEvent = {
  id: number;
  reached_at: string;
  stage_code: string;
  stage_name: string;
  changed_by: string;
};

export type Customer = {
  id: number;
  name: string;
  mobile: string;
  email: string;
  status: "active" | "dropped";
  cases?: CaseRow[];
  leads?: LeadRow[];
};

export type Bank = { id: number; name: string; has_own_stages?: boolean };
/** A builder project. NA (is_system) is the fixed entry for loans without one. */
export type ProjectType = "high_rise" | "apartments" | "plots" | "villas";
/** A builder contact of one project. `removed` is set on a lead's or case's contact that admin took off the list. */
export type BuilderContact = { id: number; name: string; phone: string; email: string; label: string; removed?: boolean };
export type Project = {
  id: number; name: string; is_system: boolean; open_cases: number; total_cases: number;
  type: ProjectType | null; address: string; active_contacts: number;
  /** Active contacts; only on GET /api/config/projects/{id} and admin saves. */
  contacts?: BuilderContact[];
};
export type DropReason = { id: number; value: string };
export type StaffUser = { id: number; full_name: string; username: string };

export type SearchGroup = {
  customer: { id: number; name: string; mobile: string; status: string };
  cases: Case[];
  leads?: LeadRow[];
};

export type LeadStatus = "open" | "converted" | "dropped";
export type LeadResponse = "interested" | "call_back" | "not_reachable" | "not_interested";

export const responseLabel: Record<LeadResponse, string> = {
  interested: "Interested",
  call_back: "Call back later",
  not_reachable: "Not reachable",
  not_interested: "Not interested",
};

/** An enquiry for one project, before the customer comes in (spec 2026-11-23). */
export type LeadRow = {
  id: number;
  customer_id: number;
  customer_name: string;
  customer_mobile: string;
  customer_email: string;
  project_id: number;
  project_name: string;
  assignee_id: number;
  assignee_name: string;
  status: LeadStatus;
  next_call_date: string | null;
  last_response: LeadResponse | null;
  last_contact_at: string | null;
  call_due: boolean;
  overdue_days: number;
  created_at: string;
  converted_at: string | null;
  case_id: number | null;
  builder_contact_id?: number | null;
  builder_contact?: BuilderContact | null;
};

export type LeadContact = {
  id: number;
  response: LeadResponse;
  remark: string;
  next_call_date: string | null;
  created_at: string;
  created_by: string;
};

export type DocumentInfo = {
  id: number;
  label: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  stage_id: number;
  uploaded_at: string;
};

/** A row of GET /api/cases: a case with names and stage progress. */
export type CaseRow = Case & {
  customer_name: string;
  customer_mobile: string;
  bank_name: string;
  project_name: string;
  stage_name: string;
  stage_code: string;
  stage_order: number;
  stage_count: number;
  assignee_name: string;
  stage_since: string;
  needs_document: boolean;
};

/** A case from GET /api/cases/attention with why it needs attention. */
export type AttentionItem = CaseRow & { reason: string };

/** A user on the admin Team tab. */
export type TeamMember = {
  id: number;
  username: string;
  full_name: string;
  role: Role;
  email: string;
  is_active: boolean;
  open_cases: number;
};

export type StageCount = { count: number; amount: string; dropped: number };

/** GET /api/reports/dashboard. */
export type DashboardData = {
  new_customers_today: number;
  new_cases_today: number;
  new_customers: number;
  new_cases: number;
  stage_moves_today: number;
  cases_by_stage: Record<string, number>;
  sanctions: StageCount;
  disbursements: StageCount;
};
