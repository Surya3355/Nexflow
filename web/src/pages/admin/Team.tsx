import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError } from "../../api/client";
import type { TeamMember } from "../../api/types";
import { Badge } from "../../components/Badge";
import { Dialog } from "../../components/Dialog";
import { alertBox, btn, btnPrimary, card, input, label, muted, okBox } from "../../components/ui";

const roleName = { admin: "Admin", manager: "Manager", employee: "Employee" } as const;

/** Admin Team tab: everyone with role and open cases; add a person; reset a password. */
export function Team() {
  const [team, setTeam] = useState<TeamMember[]>([]);
  const [adding, setAdding] = useState(false);
  const [resetting, setResetting] = useState<TeamMember | null>(null);
  const [error, setError] = useState<string | null>(null);

  function load() {
    api.request<TeamMember[]>("GET", "/api/admin/users").then(setTeam).catch(() => setError("Could not load the team."));
  }
  useEffect(load, []);

  return (
    <section className={card}>
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Team</h2>
        <button type="button" className={btnPrimary} onClick={() => setAdding(true)}>+ Add person</button>
      </div>
      {error && <p role="alert" className={`${alertBox} mt-3`}>{error}</p>}
      <div className="mt-3 overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="text-xs uppercase tracking-wide text-slate-500">
              <th className="px-2 py-2 font-medium">Name</th><th className="px-2 py-2 font-medium">Username</th>
              <th className="px-2 py-2 font-medium">Role</th><th className="px-2 py-2 font-medium">Open cases</th><th className="px-2 py-2" />
            </tr>
          </thead>
          <tbody>
            {team.map((m) => (
              <tr key={m.id} className="border-t border-slate-100">
                <td className="whitespace-nowrap px-2 py-2">{m.full_name}{!m.is_active && <span className={muted}> · inactive</span>}</td>
                <td className="px-2 py-2">{m.username}</td>
                <td className="px-2 py-2"><Badge tone={m.role === "admin" ? "info" : "grey"}>{roleName[m.role]}</Badge></td>
                <td className="px-2 py-2">{m.open_cases}</td>
                <td className="px-2 py-2 text-right">
                  <button type="button" aria-label={`Reset password for ${m.full_name}`} className="whitespace-nowrap text-blue-700" onClick={() => setResetting(m)}>
                    Reset password
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <AddPersonDialog open={adding} onClose={() => setAdding(false)} onAdded={() => { setAdding(false); load(); }} />
      <ResetPasswordDialog member={resetting} onClose={() => setResetting(null)} />
    </section>
  );
}

function AddPersonDialog({ open, onClose, onAdded }: { open: boolean; onClose: () => void; onAdded: () => void }) {
  const [fullName, setFullName] = useState("");
  const [username, setUsername] = useState("");
  const [role, setRole] = useState("employee");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await api.request("POST", "/api/admin/users", { username: username.trim(), full_name: fullName.trim(), role, email: email.trim(), password });
      setFullName(""); setUsername(""); setRole("employee"); setEmail(""); setPassword("");
      onAdded();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add the person.");
    }
  }

  return (
    <Dialog open={open} title="Add a person" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <label className={label}>Full name<input className={input} value={fullName} onChange={(e) => setFullName(e.target.value)} /></label>
        <label className={label}>Username<input className={input} autoComplete="off" value={username} onChange={(e) => setUsername(e.target.value)} /></label>
        <label className={label}>Role
          <select className={input} value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="employee">Employee</option>
            <option value="manager">Manager</option>
            <option value="admin">Admin</option>
          </select>
        </label>
        <label className={label}>Email (for case alerts)<input type="email" className={input} value={email} onChange={(e) => setEmail(e.target.value)} /></label>
        <label className={label}>First password (8+ characters)<input type="password" autoComplete="new-password" className={input} value={password} onChange={(e) => setPassword(e.target.value)} /></label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Add</button>
          <button type="button" className={btn} onClick={onClose}>Cancel</button>
        </div>
      </form>
    </Dialog>
  );
}

function ResetPasswordDialog({ member, onClose }: { member: TeamMember | null; onClose: () => void }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  useEffect(() => {
    setPassword("");
    setError(null);
    setDone(false);
  }, [member]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!member) return;
    setError(null);
    try {
      await api.request("POST", `/api/admin/users/${member.id}/password`, { password });
      setDone(true);
      setPassword("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reset the password.");
    }
  }

  const title = member ? `Reset password for ${member.full_name}` : "Reset password";
  return (
    <Dialog open={member !== null} title={title} onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <p className={muted}>They will be signed out on every device and must sign in with the new password.</p>
        <label className={label}>New password (8+ characters)<input type="password" autoComplete="new-password" className={input} value={password} onChange={(e) => setPassword(e.target.value)} /></label>
        {error && <p role="alert" className={alertBox}>{error}</p>}
        {done && member && <p role="status" className={okBox}>Password reset. {member.full_name} is signed out everywhere.</p>}
        <div className="flex gap-2">
          <button type="submit" className={btnPrimary}>Reset</button>
          <button type="button" className={btn} onClick={onClose}>Close</button>
        </div>
      </form>
    </Dialog>
  );
}
