import { Link } from "react-router-dom";
import type { AttentionItem, DashboardData } from "../../api/types";
import { card } from "../../components/ui";

function Stat({ label, value }: { label: string; value: number | undefined }) {
  return (
    <div className="flex justify-between py-1.5 text-sm">
      <span>{label}</span>
      <b className="text-base">{value ?? "–"}</b>
    </div>
  );
}

/** Today's numbers and the cases that need attention, beside the list on wide screens. */
export function TodayPanel({ data, attention }: { data: DashboardData | null; attention: AttentionItem[] }) {
  return (
    <aside className="space-y-4 min-[1100px]:sticky min-[1100px]:top-4">
      <section aria-label="Today" className={`${card} hidden min-[760px]:block`}>
        <h2 className="mb-2 font-semibold">Today</h2>
        <Stat label="New customers" value={data?.new_customers_today} />
        <Stat label="New cases" value={data?.new_cases_today} />
        <Stat label="Moved stage" value={data?.stage_moves_today} />
        <Stat label="Sanctioned this month" value={data?.sanctions?.count} />
        <Link to="/reports" className="mt-2 inline-block text-sm text-blue-700">Full reports →</Link>
      </section>
      <section aria-label="Needs attention" className={`${card} hidden min-[760px]:block`}>
        <h2 className="mb-1 font-semibold">Needs attention</h2>
        {attention.length === 0 ? (
          <p className="text-sm text-slate-500">Nothing waiting.</p>
        ) : (
          <ul className="divide-y divide-slate-200">
            {attention.slice(0, 5).map((a) => (
              <li key={a.id}>
                <Link to={`/cases/${a.id}`} className="block py-2 text-sm text-slate-900">
                  <b>{a.customer_name}</b>
                  <span className="block text-slate-500">{a.reason}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </aside>
  );
}
