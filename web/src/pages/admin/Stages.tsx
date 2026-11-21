import { useEffect, useState } from "react";
import { api, ApiError } from "../../api/client";
import type { Stage } from "../../api/types";

/** The standard stages. Admin can mark a stage as requiring documents (round 11). */
export function Stages() {
  const [stages, setStages] = useState<Stage[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<Stage[]>("GET", "/api/config/stages").then(setStages).catch(() => setStages([]));
  }, []);

  async function setRequired(stage: Stage, required: boolean) {
    setError(null);
    try {
      await api.request<void>("PATCH", `/api/admin/stages/${stage.id}`, { requires_document: required });
      setStages((list) => list.map((s) => (s.id === stage.id ? { ...s, requires_document: required } : s)));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the stage.");
    }
  }

  return (
    <section aria-label="Stages" className="space-y-2">
      <h2 className="font-medium">Stages</h2>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <ul className="space-y-2 text-sm">
        {stages.map((s) => (
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
  );
}
