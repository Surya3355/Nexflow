import { useEffect, useId, useRef, useState } from "react";
import { api, ApiError } from "../api/client";
import type { BankOfficer } from "../api/types";
import { contactError, officerLabel, officerOption } from "../api/officers";
import { alertBox, btnPrimary, btnSmall, input, label } from "./ui";

type Props = {
  bankId: number;
  team: string;
  teamName: string;
  /** The chosen officer id; null = not known yet. */
  value: number | null;
  onChange: (id: number | null) => void;
};

/**
 * Pick a bank officer of one team, or add a new one in place (spec 2026-11-26
 * §5.3). It sits inside other forms, so the new-officer fields are not a form.
 */
export function OfficerSelect({ bankId, team, teamName, value, onChange }: Props) {
  const [list, setList] = useState<BankOfficer[]>([]);
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const sending = useRef(false);
  const id = useId();

  useEffect(() => {
    let live = true;
    api.request<BankOfficer[]>("GET", "/api/officers")
      .then((all) => live && setList(all.filter((o) => o.bank_id === bankId && o.team === team)))
      .catch(() => live && setList([]));
    return () => { live = false; };
  }, [bankId, team]);

  async function save() {
    if (sending.current) return;
    if (!name.trim()) {
      setError("Enter the officer's name.");
      return;
    }
    const missing = contactError(phone, email);
    if (missing) {
      setError(missing);
      return;
    }
    sending.current = true;
    setBusy(true);
    setError(null);
    try {
      const o = await api.request<BankOfficer>("POST", "/api/officers", { bank_id: bankId, team, name: name.trim(), phone: phone.trim(), email: email.trim() });
      setList((l) => [...l, o]);
      onChange(o.id);
      setAdding(false);
      setName("");
      setPhone("");
      setEmail("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add the officer.");
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  return (
    <div className="space-y-2">
      <label className={label} htmlFor={id}>{officerLabel(teamName)} (optional)</label>
      <div className="flex gap-2">
        <select id={id} className={input} value={value ?? ""} onChange={(e) => onChange(e.target.value ? Number(e.target.value) : null)}>
          <option value="">Not known yet</option>
          {list.map((o) => <option key={o.id} value={o.id}>{officerOption(o)}</option>)}
        </select>
        {!adding && <button type="button" className={`${btnSmall} mt-1 whitespace-nowrap`} onClick={() => setAdding(true)}>+ New officer</button>}
      </div>
      {adding && (
        <div className="space-y-2 rounded-lg border border-slate-200 p-3">
          <input aria-label="New officer name" placeholder="Name" className={input} value={name} onChange={(e) => setName(e.target.value)} />
          <input aria-label="New officer mobile" placeholder="10-digit mobile" inputMode="tel" className={input} value={phone} onChange={(e) => setPhone(e.target.value)} />
          <input aria-label="New officer email" placeholder="Email (if no mobile)" type="email" inputMode="email" className={input} value={email} onChange={(e) => setEmail(e.target.value)} />
          {error && <p role="alert" className={alertBox}>{error}</p>}
          <div className="flex gap-2">
            <button type="button" className={btnPrimary} disabled={busy} onClick={save}>Save officer</button>
            <button type="button" className={btnSmall} onClick={() => { setAdding(false); setError(null); }}>Cancel</button>
          </div>
        </div>
      )}
    </div>
  );
}
