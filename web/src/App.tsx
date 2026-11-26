import { useEffect, useState } from "react";
import { Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, onSessionEnded } from "./api/client";
import { TopBar } from "./components/TopBar";
import { Login, type SignedInUser } from "./pages/Login";
import { Home } from "./pages/Home";
import { CaseDetail } from "./pages/CaseDetail";
import { LeadDetail } from "./pages/LeadDetail";
import { CustomerDetail } from "./pages/CustomerDetail";
import { AdminHome } from "./pages/admin/AdminHome";
import { Reports } from "./pages/Reports";
import { Projects } from "./pages/Projects";
import { ProjectDetail } from "./pages/ProjectDetail";
import { landingFor } from "./roles";

/** Loads the signed-in user from the server. A 401 means not signed in. */
function useSession() {
  const [user, setUser] = useState<SignedInUser | null>(null);
  const [checked, setChecked] = useState(false);
  useEffect(() => {
    api
      .request<SignedInUser>("GET", "/api/me")
      .then(setUser)
      .catch(() => {
        // Not signed in (401) or the server is unreachable: show the sign-in page.
        setUser(null);
      })
      .finally(() => setChecked(true));
  }, []);
  return { user, setUser, checked };
}

export default function App() {
  const { user, setUser, checked } = useSession();
  const navigate = useNavigate();

  // Any request that finds the session ended (password reset, deactivated) returns to sign-in.
  useEffect(() => {
    onSessionEnded(() => setUser(null));
    return () => onSessionEnded(null);
  }, [setUser]);

  async function signOut() {
    await api.request("POST", "/api/logout").catch(() => undefined);
    setUser(null);
  }

  function signedIn(u: SignedInUser) {
    setUser(u);
    navigate(landingFor(u.role), { replace: true });
  }

  if (!checked) {
    return <p className="p-4 text-sm text-slate-500">Loading…</p>;
  }
  if (!user) {
    return <Login onSignedIn={signedIn} />;
  }
  return (
    <div className="min-h-screen">
      <TopBar user={user} onSignOut={signOut} />
      <Routes>
        <Route path="/" element={<Home user={user} />} />
        <Route path="/projects" element={<Projects />} />
        <Route path="/projects/:id" element={<ProjectDetail />} />
        <Route path="/reports" element={<Reports />} />
        <Route path="/cases/:id" element={<CaseDetail role={user.role} />} />
        <Route path="/customers/:id" element={<CustomerDetail />} />
        <Route path="/leads/:id" element={<LeadDetail />} />
        <Route
          path="/admin"
          element={user.role === "admin" ? <AdminHome role={user.role} /> : <Navigate to="/" replace />}
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </div>
  );
}
