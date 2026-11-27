import type { PaymentKind } from "../../api/types";
import { input, label } from "../../components/ui";
import { todayIST } from "./BankQueries";

type Props = {
  amount: string;
  date: string;
  kind: PaymentKind;
  /** The bank pays in parts; otherwise only Full is offered. */
  partPayments: boolean;
  onAmount: (v: string) => void;
  onDate: (v: string) => void;
  onKind: (k: PaymentKind) => void;
};

/** Amount, date and Part/Full for one disbursement payment (spec 2026-11-25 §5.3). */
export function PaymentFields({ amount, date, kind, partPayments, onAmount, onDate, onKind }: Props) {
  return (
    <>
      {partPayments && (
        <fieldset className="flex gap-4 text-sm">
          <legend className="text-slate-600">Payment type</legend>
          {(["part", "full"] as const).map((k) => (
            <label key={k} className="flex items-center gap-2">
              <input type="radio" name="payment-kind" checked={kind === k} onChange={() => onKind(k)} />
              {k === "part" ? "Part" : "Full"}
            </label>
          ))}
        </fieldset>
      )}
      <div className="grid grid-cols-2 gap-3">
        <label className={label}>Amount paid (₹)<input className={input} inputMode="decimal" value={amount} onChange={(e) => onAmount(e.target.value)} /></label>
        <label className={label}>Payment date<input type="date" className={input} max={todayIST()} value={date} onChange={(e) => onDate(e.target.value)} /></label>
      </div>
    </>
  );
}
