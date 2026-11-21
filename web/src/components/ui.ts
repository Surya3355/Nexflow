// Shared Tailwind class strings, so pages look the same without a component library.
export const card = "rounded-xl border border-slate-200 bg-white p-4";
export const input = "mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900";
export const label = "block text-sm text-slate-600";
export const btn = "inline-block rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm text-slate-800 hover:border-slate-400 disabled:opacity-50";
export const btnPrimary = "inline-block rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50";
export const btnDanger = "inline-block rounded-lg border border-red-200 bg-white px-3 py-1.5 text-sm text-red-700 hover:border-red-400";
export const btnSmall = "inline-block rounded-lg border border-slate-200 bg-white px-3 py-1 text-sm text-slate-800 hover:border-slate-400";
export const muted = "text-sm text-slate-500";
export const alertBox = "rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700";
export const okBox = "rounded-lg bg-green-50 px-3 py-2 text-sm text-green-800";
export const warnBox = "rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800";
export const page = "mx-auto w-full max-w-[1320px] space-y-4 px-4 pb-16 pt-5";

/** Money as Indian rupees with Indian digit grouping, e.g. ₹45,00,000. */
export function rupees(amount: string | number | undefined): string {
  const n = Number(amount ?? 0);
  if (!Number.isFinite(n)) return `₹${amount}`;
  return `₹${n.toLocaleString("en-IN", { maximumFractionDigits: 2 })}`;
}

/** A date or time shown in India time. */
export function istDate(iso: string | undefined, withTime = false): string {
  if (!iso) return "";
  return new Date(iso).toLocaleString("en-IN", {
    timeZone: "Asia/Kolkata",
    day: "numeric",
    month: "short",
    year: "numeric",
    ...(withTime ? { hour: "2-digit", minute: "2-digit" } : {}),
  });
}

/** "Today", "Yesterday", "6 days ago" for a past time. */
export function ago(iso: string | undefined, now = new Date()): string {
  if (!iso) return "";
  const days = Math.floor((now.getTime() - new Date(iso).getTime()) / 86_400_000);
  if (days <= 0) return "Today";
  if (days === 1) return "Yesterday";
  return `${days} days ago`;
}

/** Today's date in India as YYYY-MM-DD. */
export function istToday(now = new Date()): string {
  return now.toLocaleDateString("en-CA", { timeZone: "Asia/Kolkata" });
}
