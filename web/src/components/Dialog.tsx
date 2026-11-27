import { useEffect, type ReactNode } from "react";

type Props = { open: boolean; title: string; onClose: () => void; children: ReactNode };

/** A centred pop-up. Escape or the backdrop closes it. */
export function Dialog({ open, title, onClose, children }: Props) {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-20 bg-slate-900/40" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="mx-auto mt-[10vh] w-[min(440px,calc(100%-32px))] rounded-xl bg-white p-5 shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <Header title={title} onClose={onClose} />
        {children}
      </div>
    </div>
  );
}

export function Header({ title, onClose }: { title: string; onClose: () => void }) {
  return (
    <div className="mb-4 flex items-center justify-between">
      <h2 className="text-base font-semibold">{title}</h2>
      <button type="button" aria-label="Close" onClick={onClose} className="text-xl leading-none text-slate-400 hover:text-slate-700">
        ×
      </button>
    </div>
  );
}

export function useEscape(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
}
