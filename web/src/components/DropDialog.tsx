import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import type { DropReason } from "../api/types";
import { Dialog } from "./Dialog";
import { alertBox, btn, input, label } from "./ui";

type Props = {
  open: boolean;
  title: string;
  confirmLabel: string;
  onClose: () => void;
  /** Sends the drop. Throw to show the error in the dialog. */
  onConfirm: (reasonId: number, remark: string) => Promise<void>;
};

/** Asks for a drop reason (and a remark when the reason is Other) before dropping. */
export function DropDialog({ open, title, confirmLabel, onClose, onConfirm }: Props) {
  const [reasons, setReasons] = useState<DropReason[]>([]);
  const [reasonId, setReasonId] = useState<number | "">("");
  const [remark, setRemark] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    api.request<DropReason[]>("GET", "/api/config/dropdowns/drop_reason").then(setReasons).catch(() => setReasons([]));
  }, [open]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const reason = reasons.find((r) => r.id === reasonId);
    if (!reason) {
      setError("Choose a reason.");
      return;
    }
    if (reason.value === "Other" && !remark.trim()) {
      setError("A remark is required when the reason is Other.");
      return;
    }
    setError(null);
    try {
      await onConfirm(reason.id, remark.trim());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not drop it.");
    }
  }

  return (
    <Dialog open={open} title={title} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Reason
          <select className={input} value={reasonId} onChange={(e) => setReasonId(e.target.value ? Number(e.target.value) : "")}>
            <option value="">Choose a reason</option>
            {reasons.map((r) => <option key={r.id} value={r.id}>{r.value}</option>)}
          </select>
        </label>
        <label className={label}>Remark<input className={input} placeholder="Required when the reason is Other" value={remark} onChange={(e) => setRemark(e.target.value)} /></label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className="rounded-lg bg-red-700 px-4 py-2 text-sm font-medium text-white">{confirmLabel}</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
