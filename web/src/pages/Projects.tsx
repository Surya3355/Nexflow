import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Project } from "../api/types";
import { muted, page } from "../components/ui";

/** Projects: every builder project with its case counts, NA first (spec 2026-11-21 §4.5). */
export function Projects() {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [q, setQ] = useState("");

  useEffect(() => {
    api.request<Project[]>("GET", "/api/config/projects")
      .then(setProjects)
      .catch(() => setError("Could not load projects."));
  }, []);

  const needle = q.trim().toLowerCase();
  const shown = (projects ?? []).filter((p) => !needle || p.name.toLowerCase().includes(needle));

  return (
    <main className={`${page} max-w-[900px]`}>
      <h1 className="text-xl font-semibold">Projects</h1>
      <input
        type="search"
        aria-label="Find a project"
        placeholder="Find a project…"
        className="w-full rounded-lg border border-slate-200 bg-white px-4 py-3 text-base"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      {projects === null && !error ? <p className={muted}>Loading…</p> : shown.length === 0 ? (
        <p className={muted}>No project matches “{q}”.</p>
      ) : (
        <ul aria-label="Projects" className="divide-y divide-slate-200 overflow-hidden rounded-xl border border-slate-200 bg-white">
          {shown.map((p) => (
            <li key={p.id}>
              <Link to={`/projects/${p.id}`} className="flex items-center justify-between gap-3 px-4 py-3 text-sm text-slate-900 hover:bg-slate-50">
                <span className="min-w-0 break-words font-semibold">{p.name}</span>
                <span className="flex-none text-slate-500">{p.open_cases} open · {p.total_cases} total</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}
