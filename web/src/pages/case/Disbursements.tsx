import { useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import type { Case, Payment, PaymentKind } from "../../api/types";
import { alertBox, btnPrimary, btnSmall, card, istDate, istToday, muted, rupees, warnBox } from "../../components/ui";
import { PaymentFields } from "./PaymentFields";

type Props = {
  caseData: Case;
  /** The case is at its bank's disbursement stage. */
  atDisbursement: boolean;
  /** Admin or manager: may edit and delete payments. */
  canCorrect: boolean;
  onChanged: (updated: Case) => void;
};

type Editing = { payment: Payment | null } | null;

/** Payments, total paid and remaining for one case (spec 2026-11-25 §5.4). */
export function Disbursements({ caseData: c, atDisbursement, canCorrect, onChanged }: Props) {
  const [editing, setEditing] = useState<Editing>(null);
  const [error, setError] = useState<string | null>(null);
  const payments = c.payments ?? [];
  if (payments.length === 0 && !atDisbursement) return null;
  const canAdd = atDisbursement && !c.fully_paid && c.status !== "dropped";

  async function remove(p: Payment) {
    if (!window.confirm(`Delete the payment of ${rupees(p.amount)} on ${istDate(p.paid_on)}?`)) return;
    setError(null);
    try {
      onChanged(await api.request<Case>("DELETE", `/api/cases/${c.id}/payments/${p.id}`));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not delete the payment.");
    }
  }

  return (
    <section aria-label="Disbursements" className={card}>
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Disbursements</h2>
        {canAdd && !editing && <button type="button" className={btnSmall} onClick={() => setEditing({ payment: null })}>Add payment</button>}
      </div>
      {payments.length === 0 ? <p className={`${muted} mt-2`}>No payments yet.</p> : (
        <ul className="mt-2 space-y-1 text-sm">
          {payments.map((p) => (
            <li key={p.id} className="flex items-center justify-between gap-2 rounded-lg bg-slate-50 px-3 py-1.5">
              <span>{istDate(p.paid_on)} · <b>{rupees(p.amount)}</b> · {p.kind === "full" ? "Full" : "Part"}</span>
              {canCorrect && (
                <span className="flex gap-2">
                  <button type="button" className="text-blue-700" aria-label={`Edit payment of ${istDate(p.paid_on)}`} onClick={() => setEditing({ payment: p })}>edit</button>
                  <button type="button" className="text-red-700" aria-label={`Delete payment of ${istDate(p.paid_on)}`} onClick={() => remove(p)}>delete</button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-base">
        <dt className="text-slate-500">Total paid</dt><dd className="font-semibold">{rupees(c.total_paid)}</dd>
        <dt className="text-slate-500">Remaining</dt><dd className="font-semibold">{rupees(c.remaining)}</dd>
      </dl>
      {c.over_paid && <p className={`${warnBox} mt-2`}>Total paid is more than the sanctioned amount.</p>}
      {error && <p role="alert" className={`${alertBox} mt-2`}>{error}</p>}
      {editing && (
        <PaymentForm caseData={c} payment={editing.payment} onCancel={() => setEditing(null)}
          onSaved={(u) => { setEditing(null); onChanged(u); }} />
      )}
    </section>
  );
}

function PaymentForm({ caseData: c, payment, onCancel, onSaved }: { caseData: Case; payment: Payment | null; onCancel: () => void; onSaved: (c: Case) => void }) {
  const [amount, setAmount] = useState(payment?.amount.replace(/\.00$/, "") ?? "");
  const [date, setDate] = useState(payment?.paid_on ?? istToday());
  const [kind, setKind] = useState<PaymentKind>(payment?.kind ?? (c.part_payments ? "part" : "full"));
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    if (!amount.trim()) {
      setError("Enter the amount paid.");
      return;
    }
    setError(null);
    setBusy(true);
    const body = { amount: amount.trim(), date, kind: c.part_payments || payment ? kind : "full" };
    try {
      onSaved(payment
        ? await api.request<Case>("PATCH", `/api/cases/${c.id}/payments/${payment.id}`, body)
        : await api.request<Case>("POST", `/api/cases/${c.id}/payments`, body));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the payment.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="mt-3 space-y-3 border-t pt-3">
      <PaymentFields amount={amount} date={date} kind={kind} partPayments={Boolean(c.part_payments)}
        onAmount={setAmount} onDate={setDate} onKind={setKind} />
      {error && <p role="alert" className={alertBox}>{error}</p>}
      <div className="flex gap-2">
        <button type="submit" className={btnPrimary} disabled={busy}>Save payment</button>
        <button type="button" className={btnSmall} onClick={onCancel}>Cancel</button>
      </div>
    </form>
  );
}
