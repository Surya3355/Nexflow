import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { AttentionItem, Bank, CaseRow, DashboardData, LeadRow, Project, SearchGroup, StaffUser } from "../api/types";
import { btnPrimary, btnSmall, card, istToday, muted, page, rupees } from "../components/ui";
import { defaultScope } from "../roles";
import type { SignedInUser } from "./Login";
import { FilterPanel } from "./home/FilterPanel";
import { countFilters, filterLeads, noFilters, type Filters } from "./home/leadFilter";
import { CaseTable, type ListRow } from "./home/CaseTable";
import { LeadList, ResponseBadge } from "./home/LeadList";
import { NewCustomerPanel } from "./home/NewCustomerPanel";
import { TodayPanel } from "./home/TodayPanel";

const PAGE = 50;
const SEARCH_DELAY_MS = 300;

type Scope = "leads" | "mine" | "all" | "attention";

function greeting(name: string, now = new Date()): string {
  const hour = Number(now.toLocaleString("en-IN", { timeZone: "Asia/Kolkata", hour: "numeric", hour12: false }));
  const part = hour < 12 ? "Good morning" : hour < 17 ? "Good afternoon" : "Good evening";
  return `${part}, ${name.trim().split(/\s+/)[0]}`;
}

const tabs: { id: Scope; label: string }[] = [
  { id: "leads", label: "Leads" },
  { id: "mine", label: "My cases" },
  { id: "all", label: "All" },
  { id: "attention", label: "Needs attention" },
];

/**
 * Home: search, four list tabs and one Filter button, then the list, with
 * today's numbers beside it (spec §5.2; tabs and Filter: spec 2026-11-24).
 */
export function Home({ user }: { user: SignedInUser }) {
  const [scope, setScope] = useState<Scope>(defaultScope(user.role));
  const [query, setQuery] = useState("");
  const [groups, setGroups] = useState<SearchGroup[] | null>(null);
  const [rows, setRows] = useState<CaseRow[]>([]);
  const [attention, setAttention] = useState<AttentionItem[]>([]);
  const [leads, setLeads] = useState<LeadRow[]>([]);
  const [today, setToday] = useState<DashboardData | null>(null);
  const [banks, setBanks] = useState<Bank[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [filters, setFilters] = useState<Filters>(noFilters);
  const [filtering, setFiltering] = useState(false);
  const [shown, setShown] = useState(PAGE);
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
    api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
    api.request<StaffUser[]>("GET", "/api/users").then(setUsers).catch(() => setUsers([]));
    api.request<AttentionItem[]>("GET", "/api/cases/attention").then(setAttention).catch(() => setAttention([]));
    api.request<LeadRow[]>("GET", "/api/leads").then(setLeads).catch(() => setLeads([]));
    const month = `${istToday().slice(0, 8)}01`;
    api.request<DashboardData>("GET", `/api/reports/dashboard?from=${month}&to=${istToday()}`).then(setToday).catch(() => setToday(null));
  }, []);

  const { bankId, projectId, stageId, assigneeId, from, to } = filters;
  useEffect(() => {
    const params = new URLSearchParams();
    if (bankId) params.set("bank_id", bankId);
    if (projectId !== null) params.set("project_id", String(projectId));
    if (stageId) params.set("stage_id", stageId);
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const qs = params.toString();
    api.request<CaseRow[]>("GET", `/api/cases${qs ? `?${qs}` : ""}`)
      .then((r) => { setRows(r); setError(null); })
      .catch(() => { setRows([]); setError("Could not load cases."); });
  }, [bankId, projectId, stageId, from, to]);

  // Search waits for a pause in typing; an empty box shows the case list again.
  useEffect(() => {
    const q = query.trim();
    if (!q) {
      setGroups(null);
      return;
    }
    const timer = setTimeout(() => {
      api.request<SearchGroup[]>("GET", `/api/search?q=${encodeURIComponent(q)}`)
        .then(setGroups)
        .catch(() => setError("Search failed. Try again."));
    }, SEARCH_DELAY_MS);
    return () => clearTimeout(timer);
  }, [query]);

  // Assigned to is applied here, not in the request, so My cases always holds
  // your own cases even when All is filtered to someone else.
  const mineRows = useMemo(() => rows.filter((r) => r.assignee_id === user.id), [rows, user.id]);
  const allRows = useMemo(() => (assigneeId ? rows.filter((r) => String(r.assignee_id) === assigneeId) : rows), [rows, assigneeId]);
  const scopedAttention = useMemo(
    () => (defaultScope(user.role) === "mine" ? attention.filter((a) => a.assignee_id === user.id) : attention),
    [attention, user.id, user.role],
  );
  // Lead calls due today or earlier also need attention, scoped like cases.
  const leadAttention = useMemo(
    () => leads.filter((l) => l.call_due && (defaultScope(user.role) !== "mine" || l.assignee_id === user.id)),
    [leads, user.id, user.role],
  );
  const attentionCount = scopedAttention.length + leadAttention.length;
  const shownLeads = useMemo(() => filterLeads(leads, filters), [leads, filters]);
  const filterCount = scope === "attention" ? 0 : countFilters(filters, scope);
  const listed: ListRow[] = scope === "mine" ? mineRows : scope === "all" ? allRows : scope === "attention" ? scopedAttention : [];

  function pick(s: Scope) {
    setScope(s);
    setShown(PAGE);
  }

  return (
    <main className={page}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">{greeting(user.full_name || user.username)}</h1>
        <button type="button" className={btnPrimary} onClick={() => setAdding(true)}>+ New customer</button>
      </div>

      <div className="grid items-start gap-5 min-[1100px]:grid-cols-[1fr_300px]">
        <div className="min-w-0 space-y-4">
          <input
            type="search"
            aria-label="Search"
            placeholder="Search name, mobile, tracking number or bank…"
            className="w-full rounded-lg border border-slate-200 bg-white px-4 py-3 text-base"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />

          {groups === null && (
            <div className="flex items-center gap-2">
              <div role="tablist" aria-label="Lists" className="flex min-w-0 flex-1 gap-1 overflow-x-auto">
                {tabs.map((t) => {
                  const n = t.id === "leads" ? shownLeads.length : t.id === "mine" ? mineRows.length : t.id === "all" ? allRows.length : attentionCount;
                  const on = scope === t.id;
                  return (
                    <button key={t.id} type="button" role="tab" aria-selected={on} onClick={() => pick(t.id)}
                      className={`whitespace-nowrap rounded-full border px-3 py-1 text-sm ${on ? "border-slate-900 bg-slate-900 text-white" : "border-slate-200 bg-white text-slate-700 hover:border-slate-400"}`}>
                      {`${t.label} · ${n}`}
                    </button>
                  );
                })}
              </div>
              <button type="button" className={`${btnSmall} shrink-0 disabled:opacity-50`} disabled={scope === "attention"}
                aria-label={filterCount > 0 ? `Filter, ${filterCount} on` : "Filter"} onClick={() => setFiltering(true)}>
                ⚙ Filter{filterCount > 0 ? ` · ${filterCount}` : ""}
              </button>
            </div>
          )}

          {error && <p role="alert" className="text-sm text-red-700">{error}</p>}

          {groups !== null ? (
            <SearchResults groups={groups} />
          ) : (
            scope === "leads" ? (
              shownLeads.length === 0 ? <p className={`${card} ${muted}`}>{leads.length === 0 ? "No open leads." : "No leads match."}</p> : <LeadList leads={shownLeads} />
            ) : <>
              {listed.length === 0 && (scope !== "attention" || leadAttention.length === 0) ? (
                <p className={`${card} ${muted}`}>{scope === "attention" ? "Nothing needs attention." : "No cases match."}</p>
              ) : (
                <CaseTable rows={listed.slice(0, shown)} leads={scope === "attention" ? leadAttention : []} />
              )}
              {listed.length > shown && (
                <p className={muted}>
                  Showing {shown} of {listed.length} ·{" "}
                  <button type="button" className="text-blue-700" onClick={() => setShown((n) => n + PAGE)}>Show more</button>
                </p>
              )}
            </>
          )}
        </div>
        <div className="order-first min-[1100px]:order-none">
          <TodayPanel data={today} attention={scopedAttention} leads={leadAttention} />
        </div>
      </div>

      {scope !== "attention" && (
        <FilterPanel open={filtering} scope={scope} value={filters} banks={banks} projects={projects} users={users}
          onClose={() => setFiltering(false)}
          onApply={(f) => { setFilters(f); setFiltering(false); setShown(PAGE); }} />
      )}
      <NewCustomerPanel open={adding} onClose={() => setAdding(false)} projects={projects} users={users} meId={user.id} />
    </main>
  );
}

function SearchResults({ groups }: { groups: SearchGroup[] }) {
  if (groups.length === 0) return <p className={`${card} ${muted}`}>No customers or cases match.</p>;
  return (
    <div className="space-y-3">
      {groups.map((g) => (
        <section key={g.customer.id} aria-label={g.customer.name} className={card}>
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <Link to={`/customers/${g.customer.id}`} className="font-semibold text-slate-900">{g.customer.name}</Link>
            <span className={muted}>{g.customer.mobile}{g.customer.status === "dropped" ? " · Dropped" : ""}</span>
          </div>
          <ul className="mt-2 space-y-1 text-sm">
            {(g.leads ?? []).map((l) => (
              <li key={`lead-${l.id}`}>
                <Link to={`/leads/${l.id}`} className={`${btnSmall} flex w-full items-center justify-between gap-2`}>
                  <span>Lead · {l.project_name}</span><ResponseBadge lead={l} />
                </Link>
              </li>
            ))}
            {g.cases.map((c) => (
              <li key={c.id}>
                <Link to={`/cases/${c.id}`} className={`${btnSmall} w-full`}>
                  Case #{c.id} · {rupees(c.loan_amount)} · {c.status}{c.tracking_number ? ` · ${c.tracking_number}` : ""}
                </Link>
              </li>
            ))}
            {g.cases.length === 0 && (g.leads ?? []).length === 0 && <li className={muted}>No cases yet.</li>}
          </ul>
        </section>
      ))}
    </div>
  );
}
