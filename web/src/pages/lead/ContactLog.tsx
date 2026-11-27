import { responseLabel, type LeadContact } from "../../api/types";
import { istDate } from "../../components/ui";
import { shortDay } from "../home/LeadList";

/** A lead's calls, newest first. Read-only. */
export function ContactLog({ contacts }: { contacts: LeadContact[] }) {
  if (contacts.length === 0) return <p className="text-sm text-slate-500">No contacts logged yet.</p>;
  return (
    <ol aria-label="Contact log" className="divide-y divide-slate-200">
      {contacts.map((c) => (
        <li key={c.id} className="py-2.5 text-base">
          <b>{responseLabel[c.response]}</b>, “{c.remark}”
          <span className="block text-sm text-slate-500">
            {istDate(c.created_at, true)} · {c.created_by}{c.next_call_date ? ` · next call ${shortDay(c.next_call_date)}` : ""}
          </span>
        </li>
      ))}
    </ol>
  );
}
