import type { ReactNode } from "react";

export type Tone = "info" | "ok" | "warn" | "bad" | "grey";

const tones: Record<Tone, string> = {
  info: "bg-blue-50 text-blue-700",
  ok: "bg-green-50 text-green-700",
  warn: "bg-amber-50 text-amber-800",
  bad: "bg-red-50 text-red-700",
  grey: "bg-slate-100 text-slate-600",
};

export function Badge({ tone = "info", children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-semibold ${tones[tone]}`}>{children}</span>;
}
