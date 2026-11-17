import { FormEvent, useEffect, useState } from "react";
import { Database, Save, Shield, SlidersHorizontal } from "lucide-react";
import { api } from "../api";
import { Badge, Button, Card, Field, inputClass, useToast } from "../components/ui";
import type { AuditEntry, Settings } from "../types";

export function SettingsPage({ onInterval }: { onInterval: (seconds: number) => void }) {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [tab, setTab] = useState<"general" | "security" | "database">("general");
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const toast = useToast();

  async function load() {
    const [nextSettings, nextAudit] = await Promise.all([
      api<Settings>("/api/settings"),
      api<{ entries: AuditEntry[] }>("/api/audit"),
    ]);
    setSettings(nextSettings);
    setAudit(nextAudit.entries);
    onInterval(nextSettings.refresh_interval);
  }
  useEffect(() => { load().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed")); }, []);

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!settings) return;
    try {
      const res = await api<{ restart_required: boolean }>("/api/settings", {
        method: "PUT",
        body: JSON.stringify({
          refresh_interval: Number(settings.refresh_interval),
          host: settings.host,
          port: Number(settings.port),
          tls_cert: settings.tls_cert,
          tls_key: settings.tls_key,
          session_ttl_minutes: Number(settings.session_ttl_minutes),
        }),
      });
      toast("ok", res.restart_required ? "Saved. Restart N Manager to apply the bind address." : "Settings saved.");
      onInterval(Number(settings.refresh_interval));
      await load();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "Unable to save");
    }
  }

  async function changePassword(event: FormEvent) {
    event.preventDefault();
    try {
      await api("/api/settings/password", { method: "POST", body: JSON.stringify({ current_password: current, new_password: next }) });
      setCurrent("");
      setNext("");
      toast("ok", "Password updated.");
      await load();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "Unable to change password");
    }
  }

  if (!settings) return null;

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        <Button icon kind={tab === "general" ? "primary" : "default"} label="General" onClick={() => setTab("general")}><SlidersHorizontal size={16} /></Button>
        <Button icon kind={tab === "security" ? "primary" : "default"} label="Security" onClick={() => setTab("security")}><Shield size={16} /></Button>
        <Button icon kind={tab === "database" ? "primary" : "default"} label="Database" onClick={() => setTab("database")}><Database size={16} /></Button>
      </div>
      {tab === "general" && (
        <Card title={settings.version}>
          <form className="grid max-w-xl gap-3" onSubmit={save}>
            <Field label="Host"><input className={inputClass} value={settings.host} onChange={(e) => setSettings({ ...settings, host: e.target.value })} /></Field>
            <Field label="Port"><input className={inputClass} type="number" value={settings.port} onChange={(e) => setSettings({ ...settings, port: Number(e.target.value) })} /></Field>
            <Field label="TLS certificate"><input className={inputClass} value={settings.tls_cert} onChange={(e) => setSettings({ ...settings, tls_cert: e.target.value })} placeholder="/etc/ssl/certs/n-manager.pem" /></Field>
            <Field label="TLS key"><input className={inputClass} value={settings.tls_key} onChange={(e) => setSettings({ ...settings, tls_key: e.target.value })} placeholder="/etc/ssl/private/n-manager.key" /></Field>
            <Field label="Refresh seconds"><input className={inputClass} type="number" min={1} max={300} value={settings.refresh_interval} onChange={(e) => setSettings({ ...settings, refresh_interval: Number(e.target.value) })} /></Field>
            <Button type="submit" icon kind="primary" label="Save"><Save size={16} /></Button>
          </form>
        </Card>
      )}
      {tab === "security" && (
        <div className="space-y-4">
          <Card title="Session">
            <form className="grid max-w-xl gap-3" onSubmit={save}>
              <Field label="Session minutes"><input className={inputClass} type="number" value={settings.session_ttl_minutes} onChange={(e) => setSettings({ ...settings, session_ttl_minutes: Number(e.target.value) })} /></Field>
              <Button type="submit" icon kind="primary" label="Save"><Save size={16} /></Button>
            </form>
          </Card>
          <Card title="Password">
            <form className="grid max-w-xl gap-3" onSubmit={changePassword}>
              <Field label="Current password"><input className={inputClass} type="password" value={current} onChange={(e) => setCurrent(e.target.value)} /></Field>
              <Field label="New password"><input className={inputClass} type="password" value={next} onChange={(e) => setNext(e.target.value)} /></Field>
              <Button type="submit" icon kind="primary" label="Update"><Save size={16} /></Button>
            </form>
          </Card>
          <Card title="Activity">
            <table className="w-full text-left text-sm">
              <tbody>
                {audit.map((entry) => (
                  <tr key={entry.id} className="border-t border-[var(--border)]">
                    <td className="py-2 text-[var(--muted)]">{new Date(entry.created_at * 1000).toLocaleString()}</td>
                    <td>{entry.action}</td>
                    <td>{entry.target}</td>
                    <td className="text-[var(--muted)]">{entry.detail}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>
        </div>
      )}
      {tab === "database" && (
        <Card title="SQLite" action={<Badge value={settings.database.ready ? "ready" : "off"} />}>
          <p className="break-all text-sm text-[var(--muted)]">{settings.database.path}</p>
          <p className="mt-2 text-sm">{settings.database.version}/{settings.database.latest}</p>
        </Card>
      )}
    </div>
  );
}
