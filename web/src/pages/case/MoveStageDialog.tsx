import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import type { Case, Stage } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { alertBox, btn, btnPrimary, input, istToday, label } from "../../components/ui";

type Props = {
  open: boolean;
  caseData: Case;
  stages: Stage[];
  /** The stage to move to; null lets the person pick any stage. */
  target: Stage | null;
  onClose: () => void;
  onMoved: (updated: Case & { warning?: string }) => void;
};

/** Moves a case, asking only what the target stage needs (spec §5.4). */
export function MoveStageDialog({ open, caseData, stages, target, onClose, onMoved }: Props) {
  const [pickedId, setPickedId] = useState<number | "">("");
  const [tracking, setTracking] = useState("");
  const [amount, setAmount] = useState(caseData.loan_amount);
  const [rate, setRate] = useState("");
  const [rateType, setRateType] = useState("floating");
  const [tenure, setTenure] = useState("");
  const [date, setDate] = useState(istToday());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setPickedId("");
    setTracking(caseData.tracking_number ?? "");
    setAmount(caseData.loan_amount);
    setRate("");
    setTenure("");
    setDate(istToday());
    setError(null);
  }, [open, caseData]);

  const to = target ?? stages.find((s) => s.id === pickedId) ?? null;
  // Letters and digits only, in capitals (spec 2026-11-21 §4.7). A tracking
  // number already stored on the case is kept as it is and not re-checked.
  const trackingValue = tracking.trim();
  const trackingChanged = trackingValue !== (caseData.tracking_number ?? "");
  const trackingBad = to?.code === "logged_in" && trackingChanged && trackingValue !== "" && !/^[A-Z0-9]+$/.test(trackingValue);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!to) {
      setError("Choose a stage.");
      return;
    }
    const body: Record<string, unknown> = { stage_id: to.id };
    if (trackingBad) return;
    if (to.code === "logged_in" && trackingValue && trackingChanged) body.tracking_number = trackingValue;
    if (to.code === "sanctioned") {
      if (!rate.trim()) {
        setError("Enter the interest rate.");
        return;
      }
      body.sanction = { amount: amount.trim(), rate_percent: rate.trim(), interest_type: rateType, tenure_months: Number(tenure) || 0, date };
    }
    if (to.code === "disbursed") body.disbursement = { amount: amount.trim(), date };
    setError(null);
    try {
      onMoved(await api.request<Case & { warning?: string }>("POST", `/api/cases/${caseData.id}/stage`, body));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not move the case.");
    }
  }

  return (
    <Dialog open={open} title={target ? `Move to ${target.name}` : "Move to another stage"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        {!target && (
          <label className={label}>Move to stage
            <select className={input} value={pickedId} onChange={(e) => setPickedId(e.target.value ? Number(e.target.value) : "")}>
              <option value="">Choose a stage</option>
              {stages.filter((s) => s.id !== caseData.current_stage_id).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </label>
        )}
        {to?.code === "logged_in" && (
          <>
            <label className={label}>Tracking number
              <input className={input} autoCapitalize="characters" aria-invalid={trackingBad} value={tracking} onChange={(e) => setTracking(e.target.value.toUpperCase())} />
            </label>
            {trackingBad && <p className="text-sm text-red-700">Tracking number can contain only letters and numbers.</p>}
          </>
        )}
        {to?.code === "sanctioned" && (
          <>
            <label className={label}>Sanctioned amount (₹)<input className={input} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} /></label>
            <div className="grid grid-cols-2 gap-3">
              <label className={label}>Interest rate (%)<input className={input} inputMode="decimal" value={rate} onChange={(e) => setRate(e.target.value)} /></label>
              <label className={label}>Rate type
                <select className={input} value={rateType} onChange={(e) => setRateType(e.target.value)}>
                  <option value="floating">Floating</option>
                  <option value="fixed">Fixed</option>
                </select>
              </label>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <label className={label}>Tenure (months)<input className={input} inputMode="numeric" value={tenure} onChange={(e) => setTenure(e.target.value)} /></label>
              <label className={label}>Sanction date<input type="date" className={input} value={date} onChange={(e) => setDate(e.target.value)} /></label>
            </div>
          </>
        )}
        {to?.code === "disbursed" && (
          <div className="grid grid-cols-2 gap-3">
            <label className={label}>Disbursed amount (₹)<input className={input} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} /></label>
            <label className={label}>Disbursement date<input type="date" className={input} value={date} onChange={(e) => setDate(e.target.value)} /></label>
          </div>
        )}
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={trackingBad}>Confirm move</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
