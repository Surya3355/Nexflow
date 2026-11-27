import { useState } from "react";
import { NavLink } from "react-router-dom";
import type { SignedInUser } from "../pages/Login";
import { menuFor } from "../roles";
import { ChangePasswordDialog } from "./ChangePasswordDialog";
import { useEscape } from "./Dialog";

const linkClass = ({ isActive }: { isActive: boolean }) =>
  `whitespace-nowrap rounded-md px-2.5 py-1.5 text-sm ${isActive ? "bg-blue-50 font-semibold text-blue-700" : "text-slate-500 hover:text-slate-900"}`;

/**
 * Logo, the menu for the person's role, and an account menu. On a phone the
 * page links are behind a ☰ button, so none sit off-screen (round 16).
 */
export function TopBar({ user, onSignOut }: { user: SignedInUser; onSignOut: () => void }) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [phoneOpen, setPhoneOpen] = useState(false);
  const [changing, setChanging] = useState(false);
  const initial = (user.full_name || user.username).trim().charAt(0).toUpperCase();
  const items = menuFor(user.role);
  useEscape(phoneOpen, () => setPhoneOpen(false));

  return (
    <header className="relative flex items-center gap-4 border-b border-slate-200 bg-white px-4 py-2.5">
      <span className="font-bold">Nexflow</span>
      <nav aria-label="Main" className="hidden flex-1 gap-1 sm:flex">
        {items.map((m) => (
          <NavLink key={m.to} to={m.to} end={m.to === "/"} className={linkClass}>
            {m.label}
          </NavLink>
        ))}
      </nav>
      <span className="flex-1 sm:hidden" />
      <button
        type="button"
        aria-label="Menu"
        aria-expanded={phoneOpen}
        onClick={() => setPhoneOpen((o) => !o)}
        className="rounded-md px-2 py-1 text-xl leading-none text-slate-600 sm:hidden"
      >
        ☰
      </button>
      {phoneOpen && (
        <nav aria-label="Phone menu" className="absolute inset-x-0 top-full z-20 flex flex-col border-b border-slate-200 bg-white px-4 py-2 shadow-lg sm:hidden">
          {items.map((m) => (
            <NavLink key={m.to} to={m.to} end={m.to === "/"} className={linkClass} onClick={() => setPhoneOpen(false)}>
              {m.label}
            </NavLink>
          ))}
          <button type="button" className="rounded-md px-2.5 py-1.5 text-left text-sm text-slate-500" onClick={onSignOut}>
            Sign out
          </button>
        </nav>
      )}
      <div className="relative">
        <button
          type="button"
          aria-label="Account"
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen((o) => !o)}
          className="grid h-8 w-8 place-items-center rounded-full bg-blue-600 text-sm font-semibold text-white"
        >
          {initial}
        </button>
        {menuOpen && (
          <div className="absolute right-0 z-10 mt-2 w-48 rounded-lg border border-slate-200 bg-white py-1 text-sm shadow-lg">
            <p className="truncate px-3 py-2 text-slate-500">{user.full_name}</p>
            <button type="button" className="block w-full px-3 py-2 text-left hover:bg-slate-50" onClick={() => { setMenuOpen(false); setChanging(true); }}>
              Change password
            </button>
            <button type="button" className="block w-full px-3 py-2 text-left hover:bg-slate-50" onClick={onSignOut}>
              Sign out
            </button>
          </div>
        )}
      </div>
      <ChangePasswordDialog open={changing} onClose={() => setChanging(false)} />
    </header>
  );
}
