import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Bank, DashboardData, Project, Stage } from "../api/types";
import { ProjectPicker } from "../components/ProjectPicker";
import { Chip } from "../components/Chip";
import { btn, card, istToday, muted, page, rupees } from "../components/ui";

type Preset = "today" | "week" | "month" | "custom";

/** The from/to (YYYY-MM-DD, India time) for a preset period. */
export function presetRange(p: Exclude<Preset, "custom">, today = istToday()): { from: string; to: string } {
  if (p === "today") return { from: today, to: today };
  if (p === "month") return { from: `${today.slice(0, 8)}01`, to: today };
  const d = new Date(`${today}T00:00:00Z`);
  const sinceMonday = (d.getUTCDay() + 6) % 7;
  d.setUTCDate(d.getUTCDate() - sinceMonday);
  return { from: d.toISOString().slice(0, 10), to: today };
}

function Tile({ label, value, sub }: { label: string; value: number | string; sub?: string }) {
  return (
    <div role="group" aria-label={label} className={card}>
      <p className={muted}>{label}</p>
      <p className="text-2xl font-bold">{value}</p>
      {sub && <p className={muted}>{sub}</p>}
    </div>
  );
}

const chipSelect = "rounded-full border border-slate-200 bg-white px-3 py-1 text-sm text-slate-700 hover:border-slate-400";

/** Reports: period, bank and project filters drive the tiles, stage bars, table and CSV (spec §5.5). */
export function Reports() {
  const [preset, setPreset] = useState<Preset>("month");
  const [custom, setCustom] = useState(presetRange("month"));
  const [bankId, setBankId] = useState("");
  const [projectId, setProjectId] = useState<number | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [banks, setBanks] = useState<Bank[]>([]);
  const [stages, setStages] = useState<Stage[]>([]);
  const [data, setData] = useState<DashboardData | null>(null);
  const [columns, setColumns] = useState<string[]>([]);
  const [rows, setRows] = useState<string[][]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
    api.request<Project[]>("GET", "/api/config/projects").then(setProjects).catch(() => setProjects([]));
    api.request<Stage[]>("GET", "/api/config/stages").then(setStages).catch(() => setStages([]));
  }, []);

  const range = preset === "custom" ? custom : presetRange(preset);
  const qs = useMemo(() => {
    const p = new URLSearchParams();
    if (range.from) p.set("from", range.from);
    if (range.to) p.set("to", range.to);
    if (bankId) p.set("bank_id", bankId);
    if (projectId !== null) p.set("project_id", String(projectId));
    return p.toString();
  }, [range.from, range.to, bankId, projectId]);

  useEffect(() => {
    setError(null);
    api.request<DashboardData>("GET", `/api/reports/dashboard?${qs}`).then(setData).catch(() => setError("Could not load the numbers."));
    api.request<{ columns: string[]; rows: string[][] }>("GET", `/api/reports/rows?${qs}`)
      .then((r) => { setColumns(r.columns); setRows(r.rows); })
      .catch(() => { setColumns([]); setRows([]); });
  }, [qs]);

  const byStage = useMemo(() => {
    const counts = data?.cases_by_stage ?? {};
    const known = stages.filter((s) => s.code in counts).map((s) => ({ name: s.name, n: counts[s.code] }));
    const extra = Object.keys(counts).filter((code) => !stages.some((s) => s.code === code)).map((code) => ({ name: code, n: counts[code] }));
    return [...known, ...extra];
  }, [data, stages]);
  const max = Math.max(1, ...byStage.map((s) => s.n));
  const caseCol = columns.indexOf("Case ID");

  return (
    <main className={page}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">Reports</h1>
        <a className={btn} href={`/api/reports/export.csv?${qs}`}>⬇ Export CSV</a>
      </div>

      <div className="flex flex-wrap items-center gap-1.5">
        <Chip active={preset === "today"} onClick={() => setPreset("today")}>Today</Chip>
        <Chip active={preset === "week"} onClick={() => setPreset("week")}>This week</Chip>
        <Chip active={preset === "month"} onClick={() => setPreset("month")}>This month</Chip>
        <Chip active={preset === "custom"} onClick={() => setPreset("custom")}>Custom…</Chip>
        <select aria-label="Bank" className={chipSelect} value={bankId} onChange={(e) => setBankId(e.target.value)}>
          <option value="">All banks</option>
          {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
        </select>
        <div className="w-44">
          <ProjectPicker projects={projects} value={projectId} onChange={setProjectId} allowAll aria-label="Project" className={`${chipSelect} w-full`} placeholder="All projects" />
        </div>
      </div>
      {preset === "custom" && (
        <div className="flex flex-wrap gap-3 text-sm">
          <label className="text-slate-600">From <input type="date" aria-label="From" className="ml-1 rounded-lg border border-slate-300 px-2 py-1" value={custom.from} onChange={(e) => setCustom({ ...custom, from: e.target.value })} /></label>
          <label className="text-slate-600">To <input type="date" aria-label="To" className="ml-1 rounded-lg border border-slate-300 px-2 py-1" value={custom.to} onChange={(e) => setCustom({ ...custom, to: e.target.value })} /></label>
        </div>
      )}
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}

      {data && (
        <div className="grid grid-cols-2 gap-3 min-[760px]:grid-cols-4">
          <Tile label="New customers" value={data.new_customers} sub={`${data.new_customers_today} today`} />
          <Tile label="New cases" value={data.new_cases} sub={`${data.new_cases_today} today`} />
          <Tile label="Sanctioned" value={data.sanctions.count} sub={`${rupees(data.sanctions.amount)}${data.sanctions.dropped ? ` · ${data.sanctions.dropped} dropped since` : ""}`} />
          <Tile label="Disbursed" value={data.disbursements.count} sub={rupees(data.disbursements.amount)} />
        </div>
      )}

      <section aria-label="Open cases by stage" className={card}>
        <h2 className="mb-3 font-semibold">Open cases by stage</h2>
        {byStage.length === 0 ? <p className={muted}>No open cases.</p> : (
          <ul className="space-y-3 text-sm">
            {byStage.map((s) => (
              <li key={s.name}>
                <div className="flex justify-between"><span>{s.name}</span><b>{s.n}</b></div>
                <div className="mt-1 h-1.5 rounded bg-slate-200"><div className="h-1.5 rounded bg-blue-600" style={{ width: `${(s.n / max) * 100}%` }} /></div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className={card}>
        <h2 className="mb-2 font-semibold">Cases opened in this period</h2>
        <div className="overflow-x-auto">
          <table className="min-w-full text-left text-sm">
            <thead>
              <tr>{columns.map((c) => <th key={c} className="whitespace-nowrap border-b border-slate-200 px-2 py-2 text-xs font-medium uppercase tracking-wide text-slate-500">{c}</th>)}</tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  {r.map((cell, j) => (
                    <td key={j} className="whitespace-nowrap border-b border-slate-100 px-2 py-2">
                      {j === caseCol ? <Link to={`/cases/${cell}`} className="text-blue-700">{cell}</Link> : cell}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          {rows.length === 0 && <p className={`${muted} py-2`}>No cases in this period.</p>}
        </div>
      </section>
    </main>
  );
}
