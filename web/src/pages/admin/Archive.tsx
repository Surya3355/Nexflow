import { useEffect, useState } from "react";
import { api, ApiError } from "../../api/client";
import type { DropReason, Role } from "../../api/types";

type ArchiveItem = {
  id: number;
  entity_type: "customer" | "case";
  entity_id: number;
  archived_at: string;
  purge_after: string;
  restored: boolean;
};

/** Archived customers and cases. Only an admin can restore; the server enforces it too. */
export function Archive({ role }: { role: Role }) {
  const [items, setItems] = useState<ArchiveItem[]>([]);
  const [reasons, setReasons] = useState<DropReason[]>([]);
  const [restoring, setRestoring] = useState<number | null>(null);
  const [reasonId, setReasonId] = useState<number | "">("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.request<ArchiveItem[]>("GET", "/api/admin/archive").then(setItems).catch(() => setItems([]));
    api.request<DropReason[]>("GET", "/api/config/dropdowns/drop_reason").then(setReasons).catch(() => setReasons([]));
  }, []);

  async function restore(item: ArchiveItem) {
    setError(null);
    try {
      await api.request("POST", `/api/admin/archive/${item.id}/restore`, { reason_id: reasonId });
      setItems((list) => list.map((i) => (i.id === item.id ? { ...i, restored: true } : i)));
      setRestoring(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not restore.");
    }
  }

  return (
    <section aria-label="Archive" className="space-y-2">
      <h2 className="font-medium">Archive</h2>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <ul className="space-y-2 text-sm">
        {items.map((item) => (
          <li key={item.id} className="rounded border bg-white px-3 py-2">
            <span>{item.entity_type} {item.entity_id}</span>
            {item.restored && <span className="ml-2 text-slate-500">restored</span>}
            {role === "admin" && !item.restored && (
              restoring === item.id ? (
                <div className="mt-2 flex flex-wrap gap-2">
                  <select aria-label="Reason" className="rounded border px-2 py-1" value={reasonId} onChange={(e) => setReasonId(e.target.value ? Number(e.target.value) : "")}>
                    <option value="">Choose a reason</option>
                    {reasons.map((r) => <option key={r.id} value={r.id}>{r.value}</option>)}
                  </select>
                  <button type="button" className="rounded bg-slate-900 px-3 py-1 text-white" onClick={() => restore(item)}>Confirm restore</button>
                </div>
              ) : (
                <button type="button" className="mt-2 rounded border px-3 py-1" onClick={() => setRestoring(item.id)}>
                  Restore {item.entity_type} {item.entity_id}
                </button>
              )
            )}
          </li>
        ))}
      </ul>
      {items.length === 0 && <p className="text-sm text-slate-600">Nothing archived.</p>}
    </section>
  );
}
