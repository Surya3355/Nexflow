import { useLocation, useNavigate } from "react-router-dom";
import type { Role } from "../../api/types";
import { page } from "../../components/ui";
import { Archive } from "./Archive";
import { BanksAndLists } from "./BanksAndLists";
import { Stages } from "./Stages";
import { Team } from "./Team";

const tabs = [
  { id: "team", label: "Team" },
  { id: "banks", label: "Banks & lists" },
  { id: "stages", label: "Stages" },
  { id: "archive", label: "Archive" },
] as const;

type TabId = (typeof tabs)[number]["id"];

/** Admin, in tabs that list what exists (spec §5.6). The URL hash keeps the tab. Every action is also checked by the server. */
export function AdminHome({ role }: { role: Role }) {
  const location = useLocation();
  const navigate = useNavigate();
  const fromHash = location.hash.replace("#", "");
  const current: TabId = tabs.some((t) => t.id === fromHash) ? (fromHash as TabId) : "team";

  return (
    <main className={`${page} max-w-[1100px]`}>
      <h1 className="text-xl font-semibold">Admin</h1>
      <div role="tablist" aria-label="Admin sections" className="flex gap-1 overflow-x-auto border-b border-slate-200">
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={current === t.id}
            onClick={() => navigate({ hash: t.id }, { replace: true })}
            className={`whitespace-nowrap border-b-2 px-3.5 py-2 text-sm ${current === t.id ? "border-blue-600 font-semibold text-slate-900" : "border-transparent text-slate-500"}`}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div role="tabpanel">
        {current === "team" && <Team />}
        {current === "banks" && <BanksAndLists />}
        {current === "stages" && <div className="rounded-xl border border-slate-200 bg-white p-4"><Stages /></div>}
        {current === "archive" && <div className="rounded-xl border border-slate-200 bg-white p-4"><Archive role={role} /></div>}
      </div>
    </main>
  );
}
