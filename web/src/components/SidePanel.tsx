import type { ReactNode } from "react";
import { Header, useEscape } from "./Dialog";

type Props = { open: boolean; title: string; onClose: () => void; children: ReactNode };

/** A panel that slides in from the right. Full width on phones. */
export function SidePanel({ open, title, onClose, children }: Props) {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-20 bg-slate-900/40" onClick={onClose}>
      <aside
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="absolute inset-y-0 right-0 w-full max-w-md overflow-y-auto bg-white p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <Header title={title} onClose={onClose} />
        {children}
      </aside>
    </div>
  );
}
