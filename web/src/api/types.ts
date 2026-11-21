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
};

export type Bank = { id: number; name: string; has_own_stages?: boolean };
/** A builder project. NA (is_system) is the fixed entry for loans without one. */
export type Project = { id: number; name: string; is_system: boolean; open_cases: number; total_cases: number };
export type DropReason = { id: number; value: string };
export type StaffUser = { id: number; full_name: string; username: string };

export type SearchGroup = {
  customer: { id: number; name: string; mobile: string; status: string };
  cases: Case[];
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
