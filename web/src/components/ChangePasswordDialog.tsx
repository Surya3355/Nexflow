import { useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import { Dialog } from "./Dialog";
import { alertBox, btn, btnPrimary, input, label, okBox } from "./ui";

/** Lets the signed-in person change their own password. */
export function ChangePasswordDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await api.request("POST", "/api/me/password", { current, new: next });
      setDone(true);
      setCurrent("");
      setNext("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not change the password.");
    }
  }

  function close() {
    setDone(false);
    setError(null);
    onClose();
  }

  return (
    <Dialog open={open} title="Change password" onClose={close}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Current password
          <input type="password" autoComplete="current-password" className={input} value={current} onChange={(e) => setCurrent(e.target.value)} />
        </label>
        <label className={label}>New password (8+ characters)
          <input type="password" autoComplete="new-password" className={input} value={next} onChange={(e) => setNext(e.target.value)} />
        </label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        {done && <p role="status" className={okBox}>Password changed.</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Save</button>
          <button type="button" className={btn} onClick={close}>Close</button>
        </div>
      </form>
    </Dialog>
  );
}
