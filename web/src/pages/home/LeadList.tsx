import { Link } from "react-router-dom";
import { responseLabel, type LeadRow } from "../../api/types";
import { Badge } from "../../components/Badge";
import { istDate, istToday } from "../../components/ui";

/** "Call due today" or "Call overdue by N days" for a lead whose call is due. */
export function callReason(l: LeadRow): string {
  if (l.overdue_days <= 0) return "Call due today";
  return `Call overdue by ${l.overdue_days} ${l.overdue_days === 1 ? "day" : "days"}`;
}

/** Day and month, e.g. "24 Nov", for a YYYY-MM-DD date. */
export function shortDay(ymd: string): string {
  return new Date(`${ymd}T00:00:00Z`).toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "UTC" });
}

/** What the right side of a lead row says about calls. */
function callLine(l: LeadRow): string {
  if (l.next_call_date) return `Next call ${shortDay(l.next_call_date)}`;
  if (l.last_contact_at) return `Last call ${istDate(l.last_contact_at)}`;
  return istDate(l.created_at) === istDate(`${istToday()}T12:00:00+05:30`) ? "Added today" : `Added ${istDate(l.created_at)}`;
}

export function ResponseBadge({ lead }: { lead: LeadRow }) {
  if (!lead.last_response) return <Badge tone="grey">Not contacted</Badge>;
  const tone = lead.last_response === "interested" ? "info" : lead.last_response === "not_interested" ? "bad" : "warn";
  return <Badge tone={tone}>{responseLabel[lead.last_response]}</Badge>;
}

/** Open leads with basic details only (mockup round15/home.html). */
export function LeadList({ leads }: { leads: LeadRow[] }) {
  return (
    <ul aria-label="Leads" className="divide-y divide-slate-200 overflow-hidden rounded-xl border border-slate-200 bg-white">
      {leads.map((l) => (
        <li key={l.id}>
          <Link to={`/leads/${l.id}`} className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1 px-4 py-3 text-sm text-slate-900 hover:bg-slate-50">
            <b className="text-base">{l.customer_name}</b>
            <span className="text-right"><ResponseBadge lead={l} /></span>
            <span className="text-slate-500">{l.customer_mobile} · {l.project_name} · {l.assignee_name}</span>
            <span className={`text-right ${l.call_due ? "text-amber-800" : "text-slate-500"}`}>{l.call_due ? callReason(l) : callLine(l)}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
