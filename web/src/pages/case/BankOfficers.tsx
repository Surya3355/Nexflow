import { useState } from "react";
import { api, ApiError } from "../../api/client";
import type { CaseOfficer } from "../../api/types";
import { officerLabel } from "../../api/officers";
import { Dialog } from "../../components/Dialog";
import { OfficerSelect } from "../../components/OfficerSelect";
import { alertBox, btn, btnPrimary, card } from "../../components/ui";

type Props = {
  caseId: number;
  bankId: number;
  /** The teams the case has reached, in stage order. */
  officers: CaseOfficer[];
  readOnly: boolean;
  onChanged: () => void;
};

/** Who handles the case at the bank, per team (spec 2026-11-26 §5.3). */
export function BankOfficers({ caseId, bankId, officers, readOnly, onChanged }: Props) {
  const [editing, setEditing] = useState<CaseOfficer | null>(null);
  if (officers.length === 0) return null;
  return (
    <section aria-label="Bank officers" className={card}>
      <h2 className="mb-2 font-semibold">Bank officers</h2>
      <ul className="space-y-2 text-base">
        {officers.map((t) => (
          <li key={t.team} className="flex flex-wrap items-baseline gap-x-2">
            <span className="text-slate-500">{t.team_name}</span>
            {t.officer ? (
              <>
                <span className="font-semibold">{t.officer.name}</span>
                {t.officer.removed && <span className="text-sm text-slate-500">(removed)</span>}
                {t.officer.phone && <a href={`tel:${t.officer.phone}`} className="text-blue-700">{t.officer.phone}</a>}
                {t.officer.email && <a href={`mailto:${t.officer.email}`} className="break-all text-blue-700">{t.officer.email}</a>}
              </>
            ) : (
              <span className="text-amber-700">Officer not set</span>
            )}
            {!readOnly && (
              <button type="button" aria-label={`Change ${officerLabel(t.team_name)}`} className="ml-auto text-sm text-blue-700" onClick={() => setEditing(t)}>change</button>
            )}
          </li>
        ))}
      </ul>
      {editing && (
        <ChangeOfficerDialog caseId={caseId} bankId={bankId} team={editing} onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); onChanged(); }} />
      )}
    </section>
  );
}

function ChangeOfficerDialog({ caseId, bankId, team, onClose, onSaved }: {
  caseId: number; bankId: number; team: CaseOfficer; onClose: () => void; onSaved: () => void;
}) {
  const [chosen, setChosen] = useState<number | null>(team.officer && !team.officer.removed ? team.officer.id : null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const unchanged = chosen === null || chosen === team.officer?.id;

  async function save() {
    if (busy || chosen === null) return;
    setBusy(true);
    setError(null);
    try {
      await api.request("POST", `/api/cases/${caseId}/officers`, { team: team.team, officer_id: chosen });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open title={officerLabel(team.team_name)} onClose={onClose}>
      <div className="space-y-3">
        <OfficerSelect bankId={bankId} team={team.team} teamName={team.team_name} value={chosen} onChange={setChosen} />
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="button" className={btnPrimary} disabled={busy || unchanged} onClick={save}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </div>
    </Dialog>
  );
}
