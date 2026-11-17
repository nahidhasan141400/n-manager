import { Theme } from "@radix-ui/themes";
import { useEffect, useState } from "react";
import { Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, setCsrf } from "./api";
import logo from "./assets/logo.png";
import { Layout } from "./components/Layout";
import { Card, Feedback, Placeholder } from "./components/ui";
import { Applications } from "./pages/Applications";
import { Dashboard } from "./pages/Dashboard";
import { Login } from "./pages/Login";
import { FirewallPage } from "./pages/Firewall";
import { FtpPage } from "./pages/Ftp";
import { LogsPage } from "./pages/Logs";
import { NginxPage } from "./pages/Nginx";
import { ServicesPage } from "./pages/Services";
import { SettingsPage } from "./pages/Settings";
import type { Settings } from "./types";

export function App() {
  const [ready, setReady] = useState(false);
  const [authed, setAuthed] = useState(false);
  const [theme, setTheme] = useState<"dark" | "light">(() => (localStorage.getItem("nmanager-theme") === "light" ? "light" : "dark"));
  const [dryRun, setDryRun] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("nmanager-theme", theme);
  }, [theme]);

  useEffect(() => {
    let stop = false;
    api<{ username: string }>("/api/me")
      .then(() => api<{ csrf_token: string }>("/api/csrf"))
      .then((res) => {
        if (stop) return;
        setCsrf(res.csrf_token);
        setAuthed(true);
        return api<Settings>("/api/settings");
      })
      .then((settings) => {
        if (settings) setDryRun(settings.dry_run);
      })
      .catch(() => {
        if (!stop) setAuthed(false);
      })
      .finally(() => {
        if (!stop) setReady(true);
      });
    const onUnauthorized = () => setAuthed(false);
    window.addEventListener("nmanager:unauthorized", onUnauthorized);
    return () => {
      stop = true;
      window.removeEventListener("nmanager:unauthorized", onUnauthorized);
    };
  }, []);

  async function logout() {
    try {
      await api("/api/logout", { method: "POST" });
    } catch {
      /* session may already be gone */
    }
    setCsrf("");
    setAuthed(false);
    navigate("/login");
  }

  const shell = !ready ? (
      <div className="flex h-dvh overflow-hidden bg-[var(--bg)]">
        <aside className="nav-shell flex h-full w-64 min-w-64 shrink-0 flex-col px-3 py-5">
          <img src={logo} alt="N Manager" className="mb-4 h-9 w-auto px-1" />
          {["Dashboard", "Apps", "Nginx", "Services", "Firewall", "FTP", "Logs", "Settings"].map((label) => (
            <div key={label} className="flex h-10 items-center px-2 text-sm text-white">{label}</div>
          ))}
        </aside>
        <main className="min-w-0 flex-1 p-4">
          <Card loading>
            <Placeholder className="h-4 w-32" />
            <Placeholder className="mt-3 h-4 w-48" />
          </Card>
        </main>
      </div>
  ) : (
    <Feedback>
      <Routes>
        <Route path="/login" element={authed ? <Navigate to="/" replace /> : <Login onSuccess={() => {
          setAuthed(true);
          api<Settings>("/api/settings").then((settings) => setDryRun(settings.dry_run)).catch(() => undefined);
          navigate("/");
        }} />} />
        <Route element={authed ? <Layout theme={theme} dryRun={dryRun} onTheme={() => setTheme(theme === "dark" ? "light" : "dark")} onLogout={logout} /> : <Navigate to="/login" replace />}>
          <Route path="/" element={<Dashboard theme={theme} />} />
          <Route path="/applications" element={<Applications />} />
          <Route path="/nginx" element={<NginxPage />} />
          <Route path="/web" element={<Navigate to="/nginx" replace />} />
          <Route path="/services" element={<ServicesPage />} />
          <Route path="/firewall" element={<FirewallPage />} />
          <Route path="/ftp" element={<FtpPage />} />
          <Route path="/logs" element={<LogsPage />} />
          <Route path="/settings" element={<SettingsPage onInterval={() => undefined} />} />
        </Route>
      </Routes>
    </Feedback>
  );

  return (
    <Theme appearance={theme} accentColor="green" grayColor="slate" radius="medium" panelBackground="solid">
      {shell}
    </Theme>
  );
}
