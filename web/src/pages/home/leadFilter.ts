import type { LeadRow } from "../../api/types";

/** The Home filters. Empty strings and null mean "any". */
export type Filters = { bankId: string; projectId: number | null; stageId: string; assigneeId: string; from: string; to: string };
export type FilterScope = "leads" | "mine" | "all";

export const noFilters: Filters = { bankId: "", projectId: null, stageId: "", assigneeId: "", from: "", to: "" };

/**
 * How many filters are on for a tab: Leads counts project and assigned only,
 * My cases leaves out assigned (it is always you), and a date range counts once.
 */
export function countFilters(f: Filters, scope: FilterScope): number {
  const project = f.projectId !== null ? 1 : 0;
  const assigned = f.assigneeId ? 1 : 0;
  if (scope === "leads") return project + assigned;
  const rest = (f.bankId ? 1 : 0) + project + (f.stageId ? 1 : 0) + (f.from || f.to ? 1 : 0);
  return scope === "mine" ? rest : rest + assigned;
}

/**
 * Open leads narrowed by project and assignee. Order: calls due (in the
 * server's order), then leads nobody has contacted yet, then the rest.
 */
export function filterLeads(leads: LeadRow[], f: Filters): LeadRow[] {
  const kept = leads.filter((l) =>
    (f.projectId === null || l.project_id === f.projectId) && (!f.assigneeId || String(l.assignee_id) === f.assigneeId));
  const rank = (l: LeadRow) => (l.call_due ? 0 : l.last_contact_at === null ? 1 : 2);
  return kept.map((l, i) => ({ l, i })).sort((a, b) => rank(a.l) - rank(b.l) || a.i - b.i).map((x) => x.l);
}
