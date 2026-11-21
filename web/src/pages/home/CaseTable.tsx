import { Link } from "react-router-dom";
import type { CaseRow } from "../../api/types";
import { Badge } from "../../components/Badge";
import { ProgressBar } from "../../components/ProgressBar";
import { ago, rupees } from "../../components/ui";

export type ListRow = CaseRow & { reason?: string };

// One grid for every width: columns on desktop, a compact two-column row on phones.
const grid =
  "grid grid-cols-[1fr_auto] gap-x-3 gap-y-1 px-4 py-3 min-[760px]:grid-cols-[1.5fr_0.7fr_1fr_1.5fr_0.9fr_0.8fr_0.9fr] min-[760px]:items-center";

function StageCell({ r }: { r: ListRow }) {
  const done = r.status === "completed" || r.stage_code === "disbursed";
  return (
    <span className="col-span-2 min-[760px]:col-span-1 min-[760px]:order-none order-3">
      <span className="mb-1 block"><ProgressBar done={r.stage_order} total={r.stage_count} /></span>
      <span className="flex flex-wrap items-center gap-1.5 text-sm">
        {r.status === "dropped" ? <Badge tone="bad">Dropped</Badge> : done ? <Badge tone="ok">{r.stage_name}</Badge> : <span className="text-slate-500">{r.stage_name}</span>}
        {r.needs_document && r.status === "active" && <Badge tone="warn">No documents</Badge>}
        {r.reason && <span className="text-amber-800">{r.reason}</span>}
      </span>
    </span>
  );
}

/** The case list: header row on desktop, one link per case. */
export function CaseTable({ rows }: { rows: ListRow[] }) {
  return (
    <div className="overflow-hidden rounded-xl border border-slate-200 bg-white">
      <div className={`${grid} hidden bg-slate-50 py-2.5 text-xs uppercase tracking-wide text-slate-500 min-[760px]:grid`} aria-hidden="true">
        <span>Customer</span><span>Bank</span><span>Project</span><span>Stage</span><span>Assigned</span><span>Updated</span><span className="text-right">Amount</span>
      </div>
      <ul aria-label="Cases" className="divide-y divide-slate-200">
        {rows.map((r) => (
          <li key={r.id}>
            <Link to={`/cases/${r.id}`} className={`${grid} text-sm text-slate-900 hover:bg-slate-50`}>
              <span>
                <b>{r.customer_name}</b>
                <span className="block text-slate-500">{r.customer_mobile} · #{r.id}<span className="min-[760px]:hidden"> · {r.bank_name} · {r.project_name}</span></span>
              </span>
              <span className="hidden min-[760px]:inline">{r.bank_name}</span>
              <span className="hidden break-words min-[760px]:inline">{r.project_name}</span>
              <StageCell r={r} />
              <span className="hidden min-[760px]:inline">{r.assignee_name}</span>
              <span className="hidden text-slate-500 min-[760px]:inline">{ago(r.stage_since)}</span>
              <span className="text-right font-semibold min-[760px]:order-none order-2">{rupees(r.loan_amount)}</span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
