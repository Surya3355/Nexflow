import { useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import { alertBox, btnPrimary, input, label } from "../components/ui";

export type SignedInUser = {
  id: number;
  username: string;
  full_name: string;
  role: "admin" | "manager" | "employee";
};

type Props = { onSignedIn: (user: SignedInUser) => void };

/** Username and password sign-in. Admin creates accounts; there is no sign-up. */
export function Login({ onSignedIn }: Props) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const user = await api.request<SignedInUser>("POST", "/api/login", { username, password });
      onSignedIn(user);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not sign in. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="grid min-h-screen place-items-center px-4">
      <div className="w-full max-w-sm rounded-xl border border-slate-200 bg-white p-6">
        <h1 className="text-xl font-semibold">Sign in to Nexflow</h1>
        <p className="mb-5 mt-1 text-sm text-slate-500">Home loan case tracking</p>
        <form onSubmit={submit} className="space-y-4" noValidate>
          <div>
            <label htmlFor="username" className={label}>Username</label>
            <input id="username" name="username" autoComplete="username" className={input} value={username} onChange={(e) => setUsername(e.target.value)} />
          </div>
          <div>
            <label htmlFor="password" className={label}>Password</label>
            <input id="password" name="password" type="password" autoComplete="current-password" className={input} value={password} onChange={(e) => setPassword(e.target.value)} />
          </div>
          {error && <p role="alert" className={alertBox}>{error}</p>}
          <button type="submit" disabled={busy} className={`${btnPrimary} w-full`}>Sign in</button>
          <p className="text-sm text-slate-500">Forgot your password? Ask your admin to reset it.</p>
        </form>
      </div>
    </main>
  );
}
