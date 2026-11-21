import type { Role } from "./api/types";

// What each role sees (UI redesign spec §3). The server still checks every action.

export type MenuItem = { to: string; label: string };

export function menuFor(role: Role): MenuItem[] {
  const items: MenuItem[] = [
    { to: "/", label: "Home" },
    { to: "/projects", label: "Projects" },
    { to: "/reports", label: "Reports" },
  ];
  if (role === "admin") items.push({ to: "/admin", label: "Admin" });
  return items;
}

/** Where a person goes right after signing in. */
export function landingFor(role: Role): "/" | "/admin" {
  return role === "admin" ? "/admin" : "/";
}

/** Which case list Home opens on: employees start on their own cases. */
export function defaultScope(role: Role): "mine" | "all" {
  return role === "employee" ? "mine" : "all";
}
