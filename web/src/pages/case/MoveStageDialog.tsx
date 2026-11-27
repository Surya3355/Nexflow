import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import type { Case, CaseOfficer, PaymentKind, Stage } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { alertBox, btn, btnPrimary, input, istToday, label } from "../../components/ui";
import { OfficerSelect } from "../../components/OfficerSelect";
import { PaymentFields } from "./PaymentFields";

type Props = {
  open: boolean;
  caseData: Case;
  stages: Stage[];
  /** The stage to move to; null lets the person pick any stage. */
  target: Stage | null;
  onClose: () => void;
  onMoved: (updated: Case & { warning?: string }) => void;
  /** The case's current officers, to prefill the target stage's team. */
  officers?: CaseOfficer[];
};

/** Moves a case, asking only what the target stage's role needs (spec 2026-11-25 §5.3). */
export function MoveStageDialog({ open, caseData, stages, target, onClose, onMoved, officers = [] }: Props) {
  const [pickedId, setPickedId] = useState<number | "">("");
  const [tracking, setTracking] = useState("");
  const [amount, setAmount] = useState(caseData.loan_amount);
  const [rate, setRate] = useState("");
  const [rateType, setRateType] = useState("floating");
  const [tenure, setTenure] = useState("");
  const [date, setDate] = useState(istToday());
  const [paid, setPaid] = useState("");
  const [kind, setKind] = useState<PaymentKind>("full");
  const [remark, setRemark] = useState("");
  const [officerId, setOfficerId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setPickedId("");
    setTracking(caseData.tracking_number ?? "");
    setAmount(caseData.loan_amount);
    setRate("");
    setTenure("");
    setDate(istToday());
    // A case that already has payments is not given another one by default (moving back in by mistake).
    const paidBefore = (caseData.payments?.length ?? 0) > 0;
    setPaid(paidBefore ? "" : caseData.loan_amount);
    setKind(caseData.part_payments ? "part" : "full");
    setRemark("");
    setError(null);
  }, [open, caseData]);

  const to = target ?? stages.find((s) => s.id === pickedId) ?? null;
  // Letters and digits only, in capitals (spec 2026-11-21 §4.7). A tracking
  // number already stored on the case is kept as it is and not re-checked.
  const trackingValue = tracking.trim();
  const trackingChanged = trackingValue !== (caseData.tracking_number ?? "");
  const role = to?.role ?? "";
  const team = to?.officer_team ?? "";
  // Prefill the team's current officer while it is still on the bank's list.
  const currentOfficer = officers.find((o) => o.team === team)?.officer;
  const prefill = currentOfficer && !currentOfficer.removed ? currentOfficer.id : null;
  useEffect(() => { setOfficerId(prefill); }, [team, prefill, open]);
  const hasPayments = (caseData.payments?.length ?? 0) > 0;
  const trackingBad = role === "tracking" && trackingChanged && trackingValue !== "" && !/^[A-Z0-9]+$/.test(trackingValue);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!to) {
      setError("Choose a stage.");
      return;
    }
    const body: Record<string, unknown> = { stage_id: to.id };
    if (trackingBad) return;
    if (role === "tracking" && trackingValue && trackingChanged) body.tracking_number = trackingValue;
    if (role === "sanction") {
      if (!amount.trim()) {
        setError("Enter the sanctioned amount.");
        return;
      }
      if (!rate.trim()) {
        setError("Enter the interest rate.");
        return;
      }
      body.sanction = { amount: amount.trim(), rate_percent: rate.trim(), interest_type: rateType, tenure_months: Number(tenure) || 0, date };
    }
    if (role === "disbursement" && !caseData.fully_paid) {
      // The first payment is required; a case that already has payments may skip it.
      if (paid.trim()) body.payment = { amount: paid.trim(), date, kind: caseData.part_payments ? kind : "full" };
      else if (!hasPayments) {
        setError("Enter the amount paid.");
        return;
      }
    }
    if (remark.trim()) body.remark = remark.trim();
    // An empty pick keeps the current officer; the same officer is not sent again.
    if (team && officerId !== null && officerId !== currentOfficer?.id) body.officer_id = officerId;
    if (busy) return;
    setError(null);
    setBusy(true);
    try {
      onMoved(await api.request<Case & { warning?: string }>("POST", `/api/cases/${caseData.id}/stage`, body));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not move the case.");
    } finally {
      setBusy(false);
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
        {role === "tracking" && (
          <>
            <label className={label}>Tracking number
              <input className={input} autoCapitalize="characters" aria-invalid={trackingBad} value={tracking} onChange={(e) => setTracking(e.target.value.toUpperCase())} />
            </label>
            {trackingBad && <p className="text-sm text-red-700">Tracking number can contain only letters and numbers.</p>}
          </>
        )}
        {role === "sanction" && (
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
        {role === "disbursement" && !caseData.fully_paid && (
          <PaymentFields amount={paid} date={date} kind={kind} partPayments={Boolean(caseData.part_payments)}
            onAmount={setPaid} onDate={setDate} onKind={setKind} />
        )}
        {team && (
          <OfficerSelect bankId={caseData.bank_id} team={team} teamName={to?.officer_team_name ?? team} value={officerId} onChange={setOfficerId} />
        )}
        {to && (
          <label className={label}>Remark (optional)
            <textarea className={input} rows={2} maxLength={500} value={remark} onChange={(e) => setRemark(e.target.value)} />
          </label>
        )}
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={trackingBad || busy}>Confirm move</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
