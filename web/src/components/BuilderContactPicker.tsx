import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import type { BuilderContact, Project } from "../api/types";
import { Dialog } from "./Dialog";
import { alertBox, btn, btnPrimary, muted } from "./ui";

type Props = {
  open: boolean;
  projectId: number;
  current: number | null | undefined;
  onClose: () => void;
  onPick: (contactId: number) => Promise<void>;
};

/** Pick one of the project's current contacts for a lead or case. */
export function BuilderContactPicker({ open, projectId, current, onClose, onPick }: Props) {
  const [contacts, setContacts] = useState<BuilderContact[] | null>(null);
  const [chosen, setChosen] = useState<number | null>(current ?? null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setChosen(current ?? null);
    api.request<Project>("GET", `/api/config/projects/${projectId}`)
      .then((p) => setContacts(p.contacts ?? []))
      .catch(() => setContacts([]));
  }, [open, projectId, current]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (chosen === null || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onPick(chosen);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} title="Builder contact" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        {contacts === null ? <p className={muted}>Loading…</p> : contacts.length === 0 ? (
          <p className={muted}>This project has no contacts yet. Ask admin to add one.</p>
        ) : (
          <fieldset className="space-y-2">
            {contacts.map((c) => (
              <label key={c.id} className="flex items-start gap-2 text-sm text-slate-800">
                <input type="radio" name="builder-contact" className="mt-1" checked={chosen === c.id} onChange={() => setChosen(c.id)} />
                <span className="min-w-0 break-words">{[c.label, c.name, c.phone].filter(Boolean).join(" · ")}</span>
              </label>
            ))}
          </fieldset>
        )}
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary} disabled={busy || chosen === null}>Save</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}
