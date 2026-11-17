import { FormEvent, useEffect, useState } from "react";
import { Database, Plus, Save, Server, Shield, SlidersHorizontal, Trash2 } from "lucide-react";
import { ApiError, api, setServer } from "../api";
import { Badge, Button, Card, Field, Modal, Placeholder, RowActions, TabBar, useConfirm, useToast, TextInput } from "../components/ui";
import type { AuditEntry, Settings } from "../types";

export function SettingsPage({ onInterval }: { onInterval: (seconds: number) => void }) {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [tab, setTab] = useState<"general" | "security" | "database" | "servers">("servers");
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

  const waiting = !settings;
  const waitingFields = tab === "database" ? ["Path", "Version"] : tab === "security" ? ["Session minutes", "Current password", "New password"] : ["Host", "Port", "TLS certificate", "TLS key", "Refresh seconds"];

  return (
    <div className="space-y-4">
      <TabBar
        value={tab}
        onValueChange={(value) => setTab(value as typeof tab)}
        items={[
          { value: "general", label: "General", icon: <SlidersHorizontal size={16} /> },
          { value: "security", label: "Security", icon: <Shield size={16} /> },
          { value: "database", label: "Database", icon: <Database size={16} /> },
          { value: "servers", label: "Servers", icon: <Server size={16} /> },
        ]}
      />
      {waiting && tab !== "servers" && (
        <Card loading title={tab === "general" ? "General" : tab === "security" ? "Session" : "SQLite"}>
          <div className="grid max-w-xl gap-3">
            {waitingFields.map((label) => (
              <Field key={label} label={label}><Placeholder className="h-10 w-full" /></Field>
            ))}
          </div>
        </Card>
      )}
      {!waiting && tab === "general" && (
        <Card title={settings.version}>
          <form className="grid max-w-xl gap-3" onSubmit={save}>
            <Field label="Host"><TextInput value={settings.host} onChange={(e) => setSettings({ ...settings, host: e.target.value })} /></Field>
            <Field label="Port"><TextInput type="number" value={settings.port} onChange={(e) => setSettings({ ...settings, port: Number(e.target.value) })} /></Field>
            <Field label="TLS certificate"><TextInput value={settings.tls_cert} onChange={(e) => setSettings({ ...settings, tls_cert: e.target.value })} placeholder="/etc/ssl/certs/n-manager.pem" /></Field>
            <Field label="TLS key"><TextInput value={settings.tls_key} onChange={(e) => setSettings({ ...settings, tls_key: e.target.value })} placeholder="/etc/ssl/private/n-manager.key" /></Field>
            <Field label="Refresh seconds"><TextInput type="number" min={1} max={300} value={settings.refresh_interval} onChange={(e) => setSettings({ ...settings, refresh_interval: Number(e.target.value) })} /></Field>
            <Button type="submit" icon kind="primary" label="Save"><Save size={16} /></Button>
          </form>
        </Card>
      )}
      {!waiting && tab === "security" && (
        <div className="space-y-4">
          <Card title="Session">
            <form className="grid max-w-xl gap-3" onSubmit={save}>
              <Field label="Session minutes"><TextInput type="number" value={settings.session_ttl_minutes} onChange={(e) => setSettings({ ...settings, session_ttl_minutes: Number(e.target.value) })} /></Field>
              <Button type="submit" icon kind="primary" label="Save"><Save size={16} /></Button>
            </form>
          </Card>
          <Card title="Password">
            <form className="grid max-w-xl gap-3" onSubmit={changePassword}>
              <Field label="Current password"><TextInput type="password" value={current} onChange={(e) => setCurrent(e.target.value)} /></Field>
              <Field label="New password"><TextInput type="password" value={next} onChange={(e) => setNext(e.target.value)} /></Field>
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
      {!waiting && tab === "database" && (
        <Card title="SQLite" action={<Badge value={settings.database.ready ? "ready" : "off"} />}>
          <p className="break-all text-sm text-[var(--muted)]">{settings.database.path}</p>
          <p className="mt-2 text-sm">{settings.database.version}/{settings.database.latest}</p>
        </Card>
      )}
      {tab === "servers" && <ServersPanel />}
    </div>
  );
}

type ManagedServer = { id: string; name: string; host: string; port: number; username: string; trusted: boolean };

function ServersPanel() {
  const toast = useToast();
  const confirm = useConfirm();
  const [items, setItems] = useState<ManagedServer[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState(22);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [editing, setEditing] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [trust, setTrust] = useState<{ id: string; fingerprint: string } | null>(null);

  async function load() {
    const res = await api<{ servers: ManagedServer[] }>("/api/servers");
    setItems(res.servers || []);
    window.dispatchEvent(new Event("nmanager:servers"));
  }
  useEffect(() => {
    load()
      .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
      .finally(() => setLoaded(true));
  }, []);

  async function save(event: FormEvent) {
    event.preventDefault();
    const body = { name, host, port: Number(port), username, password };
    try {
      const item = editing
        ? await api<ManagedServer>(`/api/servers/${editing}`, { method: "PUT", body: JSON.stringify(body) })
        : await api<ManagedServer>("/api/servers", { method: "POST", body: JSON.stringify(body) });
      setName("");
      setHost("");
      setPort(22);
      setUsername("");
      setPassword("");
      setEditing("");
      setFormOpen(false);
      toast("ok", editing ? "Server updated." : "Server added.");
      if (!editing) setServer(item.id);
      await load();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "Unable to save the server");
    }
  }

  function openAdd() {
    setEditing("");
    setName("");
    setHost("");
    setPort(22);
    setUsername("");
    setPassword("");
    setFormOpen(true);
  }

  function edit(item: ManagedServer) {
    setEditing(item.id);
    setName(item.name);
    setHost(item.host);
    setPort(item.port);
    setUsername(item.username);
    setPassword("");
    setFormOpen(true);
  }

  async function test(id: string) {
    try {
      await api(`/api/servers/${id}/test`, { method: "POST" });
      setTrust(null);
      toast("ok", "Connected.");
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.body.needs_trust && err.body.fingerprint) {
        setTrust({ id, fingerprint: err.body.fingerprint });
        toast("err", "Host key changed. Confirm it before connecting.");
        return;
      }
      toast("err", err instanceof Error ? err.message : "Connection failed");
    }
  }

  async function confirmTrust() {
    if (!trust) return;
    try {
      await api(`/api/servers/${trust.id}/trust`, { method: "POST", body: JSON.stringify({ confirm: true }) });
      setTrust(null);
      toast("ok", "Host key saved.");
      await load();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "Unable to save the host key");
    }
  }

  async function remove(item: ManagedServer) {
    if (!(await confirm(`Remove ${item.name}?`))) return;
    try {
      await api(`/api/servers/${item.id}`, { method: "DELETE", body: JSON.stringify({ confirm: true }) });
      toast("ok", "Server removed.");
      await load();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "Unable to remove the server");
    }
  }

  return (
    <div className="space-y-4">
      <Card
        title="Servers"
        loading={!loaded}
        action={<Button kind="primary" label="Add" onClick={openAdd}><Plus size={16} /></Button>}
      >
        <div className="space-y-2">
          {!loaded && Array.from({ length: 2 }, (_, index) => (
            <div key={index} className="space-y-2 rounded-lg border border-[var(--border)] px-3 py-3">
              <Placeholder className="h-4 w-32" />
              <Placeholder className="h-3 w-48" />
            </div>
          ))}
          {loaded && items.map((item) => (
            <div key={item.id} className="rounded-lg border border-[var(--border)] px-3 py-2">
              <RowActions
                actions={[
                  { label: "Use", icon: <Server size={16} />, onSelect: () => { setServer(item.id); toast("ok", `${item.name} selected.`); } },
                  { label: "Test", icon: <Shield size={16} />, onSelect: () => test(item.id) },
                  { label: "Edit", icon: <SlidersHorizontal size={16} />, onSelect: () => edit(item) },
                  { label: "Remove", icon: <Trash2 size={16} />, danger: true, onSelect: () => remove(item) },
                ]}
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0">
                    <span className="block font-medium">{item.name}</span>
                    <span className="block truncate text-sm text-[var(--muted)]">{item.username}@{item.host}:{item.port}</span>
                  </span>
                  <Badge value={item.trusted ? "trusted" : "new"} />
                </span>
              </RowActions>
            </div>
          ))}
          {loaded && items.length === 0 && <p className="text-sm text-[var(--muted)]">No servers yet.</p>}
        </div>
      </Card>
      <Modal
        open={formOpen}
        title={editing ? "Edit server" : "Add server"}
        onOpenChange={(open) => {
          setFormOpen(open);
          if (!open) setPassword("");
        }}
      >
        <form className="grid gap-3" onSubmit={save}>
          <Field label="Name"><TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder="web-1" /></Field>
          <Field label="IP or hostname"><TextInput value={host} onChange={(e) => setHost(e.target.value)} placeholder="203.0.113.10" /></Field>
          <Field label="Port"><TextInput type="number" min={1} max={65535} value={port} onChange={(e) => setPort(Number(e.target.value))} /></Field>
          <Field label="User"><TextInput value={username} onChange={(e) => setUsername(e.target.value)} placeholder="root" /></Field>
          <Field label="Password"><TextInput type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={editing ? "Leave blank to keep the current password" : ""} /></Field>
          <div className="flex justify-end">
            <Button type="submit" kind="primary" label={editing ? "Save" : "Add"}><Plus size={16} /></Button>
          </div>
        </form>
      </Modal>
      <Modal open={trust !== null} title="Host key" description="Confirm this fingerprint before the panel connects." onOpenChange={(open) => { if (!open) setTrust(null); }}>
        <p className="break-all text-sm">{trust?.fingerprint}</p>
        <div className="mt-3 flex justify-end"><Button kind="primary" label="Trust" onClick={confirmTrust}><Shield size={16} /></Button></div>
      </Modal>
    </div>
  );
}
