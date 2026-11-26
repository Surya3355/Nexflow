import { useEffect, useState } from "react";
import { api, ApiError } from "../../api/client";
import type { Stage } from "../../api/types";

type StageList = { bank_id: number | null; name: string; stages: Stage[] };

/** Every stage list: the standard one and each bank's own (HDFC). Admin can mark a stage as requiring documents (round 11). */
export function Stages() {
  const [lists, setLists] = useState<StageList[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<StageList[]>("GET", "/api/admin/stage-lists").then(setLists).catch(() => setLists([]));
  }, []);

  async function setRequired(stage: Stage, required: boolean) {
    setError(null);
    try {
      await api.request<void>("PATCH", `/api/admin/stages/${stage.id}`, { requires_document: required });
      setLists((all) => all.map((l) => ({ ...l, stages: l.stages.map((s) => (s.id === stage.id ? { ...s, requires_document: required } : s)) })));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the stage.");
    }
  }

  return (
    <div className="space-y-4">
      <h2 className="font-medium">Stages</h2>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      {lists.map((l) => (
        <section key={l.bank_id ?? "standard"} aria-label={l.name} className="space-y-2">
          <h3 className="text-sm font-semibold text-slate-600">{l.name}</h3>
          <ul className="space-y-2 text-sm">
            {l.stages.map((s) => (
              <li key={s.id} className="flex items-center justify-between rounded border bg-white px-3 py-2">
                <span>{s.name}</span>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    aria-label={`Requires documents: ${s.name}`}
                    checked={s.requires_document}
                    onChange={(e) => setRequired(s, e.target.checked)}
                  />
                  Requires documents
                </label>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
