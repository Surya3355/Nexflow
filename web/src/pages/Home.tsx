import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { AttentionItem, Bank, CaseRow, DashboardData, Project, SearchGroup, Stage, StaffUser } from "../api/types";
import { Chip } from "../components/Chip";
import { btnPrimary, btnSmall, card, istToday, muted, page, rupees } from "../components/ui";
import { defaultScope } from "../roles";
import type { SignedInUser } from "./Login";
import { ProjectPicker } from "../components/ProjectPicker";
import { CaseTable, type ListRow } from "./home/CaseTable";
import { NewCustomerPanel } from "./home/NewCustomerPanel";
import { TodayPanel } from "./home/TodayPanel";

const PAGE = 50;
const SEARCH_DELAY_MS = 300;

type Scope = "mine" | "all" | "attention";

function greeting(name: string, now = new Date()): string {
  const hour = Number(now.toLocaleString("en-IN", { timeZone: "Asia/Kolkata", hour: "numeric", hour12: false }));
  const part = hour < 12 ? "Good morning" : hour < 17 ? "Good afternoon" : "Good evening";
  return `${part}, ${name.trim().split(/\s+/)[0]}`;
}

const chipSelect =
  "rounded-full border border-slate-200 bg-white px-3 py-1 text-sm text-slate-700 hover:border-slate-400 disabled:opacity-50";

/** Home: search, filter chips and the case list, with today's numbers beside it (spec §5.2). */
export function Home({ user }: { user: SignedInUser }) {
  const [scope, setScope] = useState<Scope>(defaultScope(user.role));
  const [query, setQuery] = useState("");
  const [groups, setGroups] = useState<SearchGroup[] | null>(null);
  const [rows, setRows] = useState<CaseRow[]>([]);
  const [attention, setAttention] = useState<AttentionItem[]>([]);
  const [today, setToday] = useState<DashboardData | null>(null);
  const [banks, setBanks] = useState<Bank[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [stages, setStages] = useState<Stage[]>([]);
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [bankId, setBankId] = useState("");
  const [projectId, setProjectId] = useState<number | null>(null);
  const [stageId, setStageId] = useState("");
  const [assigneeId, setAssigneeId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [showDates, setShowDates] = useState(false);
  const [shown, setShown] = useState(PAGE);
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
    api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
    api.request<StaffUser[]>("GET", "/api/users").then(setUsers).catch(() => setUsers([]));
    api.request<AttentionItem[]>("GET", "/api/cases/attention").then(setAttention).catch(() => setAttention([]));
    const month = `${istToday().slice(0, 8)}01`;
    api.request<DashboardData>("GET", `/api/reports/dashboard?from=${month}&to=${istToday()}`).then(setToday).catch(() => setToday(null));
  }, []);

  useEffect(() => {
    if (!bankId) {
      setStages([]);
      return;
    }
    api.request<Stage[]>("GET", `/api/config/banks/${bankId}/stages`).then(setStages).catch(() => setStages([]));
  }, [bankId]);

  useEffect(() => {
    const params = new URLSearchParams();
    if (bankId) params.set("bank_id", bankId);
    if (projectId !== null) params.set("project_id", String(projectId));
    if (stageId) params.set("stage_id", stageId);
    if (assigneeId) params.set("assignee_id", assigneeId);
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const qs = params.toString();
    api.request<CaseRow[]>("GET", `/api/cases${qs ? `?${qs}` : ""}`)
      .then((r) => { setRows(r); setError(null); })
      .catch(() => { setRows([]); setError("Could not load cases."); });
  }, [bankId, projectId, stageId, assigneeId, from, to]);

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

  const mineRows = useMemo(() => rows.filter((r) => r.assignee_id === user.id), [rows, user.id]);
  const scopedAttention = useMemo(
    () => (defaultScope(user.role) === "mine" ? attention.filter((a) => a.assignee_id === user.id) : attention),
    [attention, user.id, user.role],
  );
  const listed: ListRow[] = scope === "mine" ? mineRows : scope === "all" ? rows : scopedAttention;

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
        <div className="space-y-4">
          <input
            type="search"
            aria-label="Search"
            placeholder="Search name, mobile, tracking number or bank…"
            className="w-full rounded-lg border border-slate-200 bg-white px-4 py-3 text-base"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />

          {groups === null && (
            <div className="flex flex-wrap gap-1.5">
              <Chip active={scope === "mine"} onClick={() => pick("mine")}>{`My cases · ${mineRows.length}`}</Chip>
              <Chip active={scope === "all"} onClick={() => pick("all")}>{`All cases · ${rows.length}`}</Chip>
              <Chip active={scope === "attention"} onClick={() => pick("attention")}>{`Needs attention · ${scopedAttention.length}`}</Chip>
              <select aria-label="Bank" className={chipSelect} value={bankId} onChange={(e) => { setBankId(e.target.value); setStageId(""); }}>
                <option value="">Bank: all</option>
                {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
              </select>
              <div className="w-44">
                <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} allowAll aria-label="Project" className={`${chipSelect} w-full`} placeholder="Project: all" />
              </div>
              <select aria-label="Stage" className={chipSelect} value={stageId} onChange={(e) => setStageId(e.target.value)} disabled={!bankId} title={bankId ? undefined : "Choose a bank first"}>
                <option value="">Stage: all</option>
                {stages.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
              <select aria-label="Assigned to" className={chipSelect} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)}>
                <option value="">Assigned: anyone</option>
                {users.map((u) => <option key={u.id} value={u.id}>{u.full_name}</option>)}
              </select>
              <Chip active={showDates || Boolean(from || to)} onClick={() => setShowDates((v) => !v)}>Date ▾</Chip>
            </div>
          )}
          {groups === null && showDates && (
            <div className="flex flex-wrap gap-3 text-sm">
              <label className="text-slate-600">From <input type="date" aria-label="From" className="ml-1 rounded-lg border border-slate-300 px-2 py-1" value={from} onChange={(e) => setFrom(e.target.value)} /></label>
              <label className="text-slate-600">To <input type="date" aria-label="To" className="ml-1 rounded-lg border border-slate-300 px-2 py-1" value={to} onChange={(e) => setTo(e.target.value)} /></label>
            </div>
          )}

          {error && <p role="alert" className="text-sm text-red-700">{error}</p>}

          {groups !== null ? (
            <SearchResults groups={groups} />
          ) : (
            <>
              {listed.length === 0 ? (
                <p className={`${card} ${muted}`}>{scope === "attention" ? "Nothing needs attention." : "No cases match."}</p>
              ) : (
                <CaseTable rows={listed.slice(0, shown)} />
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
          <TodayPanel data={today} attention={scopedAttention} />
        </div>
      </div>

      <NewCustomerPanel open={adding} onClose={() => setAdding(false)} banks={banks} projects={projects} users={users} meId={user.id} />
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
            {g.cases.map((c) => (
              <li key={c.id}>
                <Link to={`/cases/${c.id}`} className={`${btnSmall} w-full`}>
                  Case #{c.id} · {rupees(c.loan_amount)} · {c.status}{c.tracking_number ? ` · ${c.tracking_number}` : ""}
                </Link>
              </li>
            ))}
            {g.cases.length === 0 && <li className={muted}>No cases yet.</li>}
          </ul>
        </section>
      ))}
    </div>
  );
}
