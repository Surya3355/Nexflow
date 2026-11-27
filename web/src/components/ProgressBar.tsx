/** Stage progress as small segments: `done` of `total` filled. */
export function ProgressBar({ done, total }: { done: number; total: number }) {
  const segments = Array.from({ length: Math.max(total, 0) }, (_, i) => i < done);
  return (
    <div role="img" aria-label={`Stage ${done} of ${total}`} className="flex gap-1">
      {segments.map((filled, i) => (
        <span key={i} className={`h-1.5 flex-1 rounded ${filled ? "bg-blue-600" : "bg-slate-200"}`} />
      ))}
    </div>
  );
}
