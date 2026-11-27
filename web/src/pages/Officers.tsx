import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import type { Bank, BankOfficer, Stage } from "../api/types";
import { contactError } from "../api/officers";
import { Dialog } from "../components/Dialog";
import { alertBox, btn, btnPrimary, btnSmall, card, input, label, muted, page } from "../components/ui";

/**
 * Bank officers: every bank's officers by team. Any staff member can add,
 * edit and remove them (spec 2026-11-26 §5.4). Removed officers stay on cases.
 */
export function Officers() {
  const [list, setList] = useState<BankOfficer[] | null>(null);
  const [banks, setBanks] = useState<Bank[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<BankOfficer | null>(null);

  function reload() {
    api.request<BankOfficer[]>("GET", "/api/officers").then(setList).catch(() => setError("Could not load officers."));
  }
  useEffect(() => {
    reload();
    api.request<Bank[]>("GET", "/api/config/banks").then(setBanks).catch(() => setBanks([]));
  }, []);

  async function remove(o: BankOfficer) {
    if (!window.confirm(`Remove ${o.name}? Cases that have this officer keep them.`)) return;
    setError(null);
    try {
      await api.request("DELETE", `/api/officers/${o.id}`);
      reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not remove the officer.");
    }
  }

  // Banks in list order, each with its teams in list order.
  const groups: { bank: string; teams: { team: string; officers: BankOfficer[] }[] }[] = [];
  for (const o of list ?? []) {
    let g = groups.find((x) => x.bank === o.bank_name);
    if (!g) groups.push((g = { bank: o.bank_name, teams: [] }));
    let t = g.teams.find((x) => x.team === o.team_name);
    if (!t) g.teams.push((t = { team: o.team_name, officers: [] }));
    t.officers.push(o);
  }

  return (
    <main className={`${page} max-w-[900px]`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-semibold">Bank officers</h1>
        <button type="button" className={btnPrimary} onClick={() => setAdding(true)}>Add officer</button>
      </div>
      {error && <p role="alert" className={alertBox}>{error}</p>}
      {list === null ? <p className={muted}>Loading…</p> : list.length === 0 ? <p className={muted}>No officers yet. Add the first one.</p> : (
        groups.map((g) => (
          <section key={g.bank} aria-label={g.bank} className={card}>
            <h2 className="mb-2 text-lg font-semibold">{g.bank}</h2>
            {g.teams.map((t) => (
              <div key={t.team} className="mb-3 last:mb-0">
                <h3 className="text-sm font-medium text-slate-500">{t.team}</h3>
                <ul className="mt-1 divide-y divide-slate-100">
                  {t.officers.map((o) => (
                    <li key={o.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2">
                      <span className="font-semibold">{o.name}</span>
                      {o.phone && <a href={`tel:${o.phone}`} className="text-blue-700">{o.phone}</a>}
                      {o.email && <a href={`mailto:${o.email}`} className="break-all text-blue-700">{o.email}</a>}
                      <span className="ml-auto flex gap-2">
                        <button type="button" aria-label={`Edit ${o.name}`} className={btnSmall} onClick={() => setEditing(o)}>Edit</button>
                        <button type="button" aria-label={`Remove ${o.name}`} className={btnSmall} onClick={() => remove(o)}>Remove</button>
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </section>
        ))
      )}
      {adding && <OfficerDialog banks={banks} onClose={() => setAdding(false)} onSaved={() => { setAdding(false); reload(); }} />}
      {editing && <OfficerDialog banks={banks} officer={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); reload(); }} />}
    </main>
  );
}

function OfficerDialog({ banks, officer, onClose, onSaved }: { banks: Bank[]; officer?: BankOfficer; onClose: () => void; onSaved: () => void }) {
  const [bankId, setBankId] = useState("");
  const [team, setTeam] = useState("");
  const [teams, setTeams] = useState<Stage[]>([]);
  const [name, setName] = useState(officer?.name ?? "");
  const [phone, setPhone] = useState(officer?.phone ?? "");
  const [email, setEmail] = useState(officer?.email ?? "");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const sending = useRef(false);

  useEffect(() => {
    setTeams([]);
    setTeam("");
    if (officer || !bankId) return;
    let live = true;
    api.request<Stage[]>("GET", `/api/config/banks/${bankId}/stages`)
      .then((st) => {
        if (!live) return;
        const withTeam = [...st].sort((a, b) => a.sort_order - b.sort_order).filter((s) => s.officer_team);
        setTeams(withTeam);
        if (withTeam.length === 1) setTeam(withTeam[0].officer_team ?? "");
      })
      .catch(() => live && setTeams([]));
    return () => { live = false; };
  }, [bankId, officer]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (sending.current) return;
    setError(null);
    if (!officer && (!bankId || !team)) {
      setError("Choose the bank and team.");
      return;
    }
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
    try {
      if (officer) await api.request("PUT", `/api/officers/${officer.id}`, { name: name.trim(), phone: phone.trim(), email: email.trim() });
      else await api.request("POST", "/api/officers", { bank_id: Number(bankId), team, name: name.trim(), phone: phone.trim(), email: email.trim() });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the officer.");
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  return (
    <Dialog open title={officer ? "Edit officer" : "Add officer"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        {officer ? (
          <p className={muted}>{officer.bank_name} · {officer.team_name}</p>
        ) : (
          <>
            <label className={label}>Bank
              <select className={input} value={bankId} onChange={(e) => setBankId(e.target.value)}>
                <option value="">Choose a bank</option>
                {banks.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
              </select>
            </label>
            <label className={label}>Team
              <select className={input} value={team} onChange={(e) => setTeam(e.target.value)} disabled={teams.length === 0}>
                <option value="">Choose a team</option>
                {teams.map((s) => <option key={s.officer_team} value={s.officer_team}>{s.officer_team_name}</option>)}
              </select>
            </label>
          </>
        )}
        <label className={label}>Name<input className={input} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className={label}>Mobile<input className={input} inputMode="tel" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="10 digits" /></label>
        <label className={label}>Email<input className={input} type="email" inputMode="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="If the officer has no mobile" /></label>
        <p className={muted}>Enter a mobile number, an email, or both.</p>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={busy}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
