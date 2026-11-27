import { useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import { responseLabel, type LeadResponse } from "../../api/types";
import { Dialog } from "../../components/Dialog";
import { alertBox, btn, btnPrimary, input, label, muted } from "../../components/ui";

type Props = { open: boolean; leadId: number; onClose: () => void; onSaved: (response: LeadResponse) => void };

const responses = Object.keys(responseLabel) as LeadResponse[];

/** Records one call: response, remark and the next call date (spec §4.4). */
export function LogContactDialog({ open, leadId, onClose, onSaved }: Props) {
  const [response, setResponse] = useState<LeadResponse | null>(null);
  const [remark, setRemark] = useState("");
  const [nextCall, setNextCall] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!response || !remark.trim()) {
      setError("Choose a response and write a remark.");
      return;
    }
    setError(null);
    setBusy(true);
    try {
      await api.request("POST", `/api/leads/${leadId}/contacts`, { response, remark: remark.trim(), next_call_date: nextCall || null });
      setResponse(null);
      setRemark("");
      setNextCall("");
      onSaved(response);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the contact.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} title="Log contact" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <div>
          <span className={muted}>Response</span>
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            {responses.map((r) => (
              <button key={r} type="button" aria-pressed={response === r} onClick={() => setResponse(r)}
                className={`rounded-full border px-3 py-1.5 text-sm ${response === r ? "border-slate-900 bg-slate-900 text-white" : "border-slate-200 bg-white text-slate-800"}`}>
                {responseLabel[r]}
              </button>
            ))}
          </div>
        </div>
        <label className={label}>Remark<textarea className={`${input} min-h-[70px]`} value={remark} onChange={(e) => setRemark(e.target.value)} /></label>
        <label className={label}>Next call (optional)<input type="date" className={input} value={nextCall} onChange={(e) => setNextCall(e.target.value)} /></label>
        <p className={muted}>Date, time and your name are saved automatically.</p>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={busy}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
