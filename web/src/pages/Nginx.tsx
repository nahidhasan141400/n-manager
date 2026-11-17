import { FormEvent, useEffect, useState } from "react";
import { Download, FilePen, Globe, Play, Power, PowerOff, RefreshCw, RotateCw, Save, ScrollText, ShieldCheck, Square, Trash2, X } from "lucide-react";
import { api } from "../api";
import { CodeEditor } from "../components/CodeEditor";
import { Badge, Button, Card, Field, inputClass, useConfirm, useToast } from "../components/ui";
import type { ActionResult, Certificate, NginxStatus, Site } from "../types";

const empty = {
  name: "",
  domains: "",
  mode: "proxy",
  proxy_pass: "http://127.0.0.1:3000",
  web_root: "/var/www/html",
  ssl_certificate: "",
  ssl_certificate_key: "",
  redirect: false,
  body_size: "",
  websocket: false,
  enabled: true,
};

export function NginxPage() {
  const [tab, setTab] = useState<"service" | "sites" | "certificates" | "logs">("service");
  const [status, setStatus] = useState<NginxStatus | null>(null);
  const [sites, setSites] = useState<Site[]>([]);
  const [siteError, setSiteError] = useState("");
  const [form, setForm] = useState(empty);
  const [certs, setCerts] = useState<Certificate[]>([]);
  const [certbot, setCertbot] = useState(false);
  const [certbotVersion, setCertbotVersion] = useState("");
  const [issueEmail, setIssueEmail] = useState("");
  const [issueSite, setIssueSite] = useState("");
  const [issueDomains, setIssueDomains] = useState("");
  const [certName, setCertName] = useState("");
  const [certBody, setCertBody] = useState("");
  const [certKey, setCertKey] = useState("");
  const [mainConfig, setMainConfig] = useState("");
  const [mainError, setMainError] = useState("");
  const [editor, setEditor] = useState<{ name: string; config: string; enabled: boolean } | null>(null);
  const [logs, setLogs] = useState({ access: "", error: "" });
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  async function load() {
    const [next, list, certificates, main] = await Promise.all([
      api<NginxStatus>("/api/nginx"),
      api<{ sites: Site[]; error?: string }>("/api/nginx/sites"),
      api<{ certificates: Certificate[]; certbot: boolean; certbot_version: string }>("/api/nginx/certificates"),
      api<{ config: string; error?: string }>("/api/nginx/config"),
    ]);
    setStatus(next);
    setSites(list.sites);
    setSiteError(list.error || "");
    setCerts(certificates.certificates || []);
    setCertbot(certificates.certbot);
    setCertbotVersion(certificates.certbot_version || "");
    setMainConfig(main.config || "");
    setMainError(main.error || "");
  }
  useEffect(() => { load().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed")); }, []);

  async function run(path: string, method: string, body?: unknown) {
    setBusy(true);
    try {
      const res = await api<ActionResult>(path, { method, body: body ? JSON.stringify(body) : undefined });
      toast("ok", res.message);
      await load();
      return true;
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "request failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function service(action: string) {
    const destructive = action === "stop" || action === "disable";
    if (destructive && !(await confirm(`${action} Nginx?`))) return;
    await run("/api/nginx/service", "POST", { action, confirm: destructive });
  }

  function payload() {
    return {
      name: form.name,
      domains: form.domains.split(",").map((item) => item.trim()).filter(Boolean),
      mode: form.mode,
      proxy_pass: form.proxy_pass,
      web_root: form.web_root,
      ssl_certificate: form.ssl_certificate,
      ssl_certificate_key: form.ssl_certificate_key,
      redirect: form.redirect,
      body_size: form.body_size,
      websocket: form.websocket,
      enabled: form.enabled,
    };
  }

  async function create(event: FormEvent) {
    event.preventDefault();
    if (await run("/api/nginx/sites", "POST", payload())) setForm(empty);
  }

  async function openPreview() {
    setBusy(true);
    try {
      const res = await api<{ config: string }>("/api/nginx/preview", { method: "POST", body: JSON.stringify(payload()) });
      setEditor({ name: form.name, config: res.config, enabled: form.enabled });
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "request failed");
    } finally {
      setBusy(false);
    }
  }

  async function refreshLogs() {
    setLogs(await api<{ access: string; error: string }>("/api/nginx/logs"));
  }

  function chooseCert(name: string) {
    const cert = certs.find((item) => item.name === name);
    setForm({ ...form, ssl_certificate: cert?.cert || "", ssl_certificate_key: cert?.key || "" });
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <Button icon kind={tab === "service" ? "primary" : "default"} label="Service" onClick={() => setTab("service")}><Power size={16} /></Button>
        <Button icon kind={tab === "sites" ? "primary" : "default"} label="Sites" onClick={() => setTab("sites")}><Globe size={16} /></Button>
        <Button icon kind={tab === "certificates" ? "primary" : "default"} label="Certificates" onClick={() => setTab("certificates")}><ShieldCheck size={16} /></Button>
        <Button icon kind={tab === "logs" ? "primary" : "default"} label="Logs" onClick={() => { setTab("logs"); refreshLogs().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed")); }}><ScrollText size={16} /></Button>
      </div>
      {tab === "service" && (
        <>
          <Card title="Nginx" action={<Badge value={status?.installed ? status.active : "off"} />}>
            <div className="mb-4 grid gap-2 sm:grid-cols-3">
              <Fact label="Version" value={status?.version || "—"} />
              <Fact label="Boot" value={status?.enabled || "—"} />
              <Fact label="Layout" value={status?.layout || "—"} />
            </div>
            <div className="flex flex-wrap gap-2">
              {!status?.installed && <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run("/api/nginx/install", "POST", {})}><Download size={16} /></Button>}
              <Button icon label="Start" disabled={busy} onClick={() => service("start")}><Play size={16} /></Button>
              <Button icon label="Stop" disabled={busy} onClick={() => service("stop")}><Square size={16} /></Button>
              <Button icon label="Restart" disabled={busy} onClick={() => service("restart")}><RotateCw size={16} /></Button>
              <Button icon label="Reload" disabled={busy} onClick={() => service("reload")}><RefreshCw size={16} /></Button>
              <Button icon label="Enable" disabled={busy} onClick={() => service("enable")}><Power size={16} /></Button>
              <Button icon label="Disable" disabled={busy} onClick={() => service("disable")}><PowerOff size={16} /></Button>
            </div>
          </Card>
          <Card title="nginx.conf">
            {mainError ? <p className="text-sm text-[var(--warning)]">{mainError}</p> : <CodeEditor value={mainConfig} readOnly />}
          </Card>
        </>
      )}
      {tab === "sites" && (
        <>
          <Card title="Site">
            <form className="grid items-end gap-3 md:grid-cols-2" onSubmit={create}>
              <Field label="Name"><input className={inputClass} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></Field>
              <Field label="Domains"><input className={inputClass} value={form.domains} onChange={(e) => setForm({ ...form, domains: e.target.value })} placeholder="example.com, www.example.com" required /></Field>
              <Field label="Mode">
                <select className={inputClass} value={form.mode} onChange={(e) => setForm({ ...form, mode: e.target.value })}>
                  <option value="proxy">Proxy</option>
                  <option value="root">Root</option>
                </select>
              </Field>
              {form.mode === "proxy" ? (
                <Field label="Proxy"><input className={inputClass} value={form.proxy_pass} onChange={(e) => setForm({ ...form, proxy_pass: e.target.value })} placeholder="http://127.0.0.1:3000" /></Field>
              ) : (
                <Field label="Root"><input className={inputClass} value={form.web_root} onChange={(e) => setForm({ ...form, web_root: e.target.value })} placeholder="/var/www/html" /></Field>
              )}
              <Field label="Certificate">
                <select className={inputClass} value={certs.find((item) => item.cert === form.ssl_certificate)?.name || ""} onChange={(e) => chooseCert(e.target.value)}>
                  <option value="">None</option>
                  {certs.map((item) => <option key={item.cert} value={item.name}>{item.name}</option>)}
                </select>
              </Field>
              <Field label="Body size"><input className={inputClass} value={form.body_size} onChange={(e) => setForm({ ...form, body_size: e.target.value })} placeholder="20m" /></Field>
              <Field label="Certificate path"><input className={inputClass} value={form.ssl_certificate} onChange={(e) => setForm({ ...form, ssl_certificate: e.target.value })} placeholder="/etc/ssl/certs/site.pem" /></Field>
              <Field label="Key path"><input className={inputClass} value={form.ssl_certificate_key} onChange={(e) => setForm({ ...form, ssl_certificate_key: e.target.value })} placeholder="/etc/ssl/private/site.key" /></Field>
              <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.redirect} onChange={(e) => setForm({ ...form, redirect: e.target.checked })} />HTTPS redirect</label>
              <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.websocket} onChange={(e) => setForm({ ...form, websocket: e.target.checked })} />Websocket</label>
              <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />Enable</label>
              <div className="flex gap-2">
                <Button type="submit" icon kind="primary" label="Create" disabled={busy}><Download size={16} /></Button>
                <Button icon label="Editor" disabled={busy} onClick={openPreview}><FilePen size={16} /></Button>
              </div>
            </form>
            {siteError && <p className="mt-3 text-sm text-[var(--warning)]">{siteError}</p>}
          </Card>
          <Card title="Sites" action={<Badge value={String(sites.length)} />}>
            <ul className="space-y-2">
              {sites.map((site) => (
                <li key={site.name} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2">
                  <span className="min-w-0">
                    <span className="flex items-center gap-2 font-medium">
                      <span className="truncate">{site.name}</span>
                      <Badge value={site.enabled ? "on" : "off"} />
                    </span>
                    <span className="block truncate text-xs text-[var(--muted)]">{site.domains || "—"}</span>
                  </span>
                  <span className="flex flex-wrap gap-2">
                    <Button icon label="Edit" onClick={() => setEditor({ name: site.name, config: site.config, enabled: site.enabled })}><FilePen size={16} /></Button>
                    <Button icon label={site.enabled ? "Disable" : "Enable"} disabled={busy} onClick={() => run(`/api/nginx/sites/${site.name}/${site.enabled ? "disable" : "enable"}`, "POST")}>{site.enabled ? <PowerOff size={16} /> : <Power size={16} />}</Button>
                    <Button icon kind="danger" label="Delete" disabled={busy} onClick={async () => { if (await confirm(`Delete ${site.name}?`)) run(`/api/nginx/sites/${site.name}`, "DELETE", { confirm: true }); }}><Trash2 size={16} /></Button>
                  </span>
                </li>
              ))}
            </ul>
          </Card>
          {editor && (
            <Card title={editor.name || "Config"} action={<Button icon label="Close" onClick={() => setEditor(null)}><X size={16} /></Button>}>
              <CodeEditor value={editor.config} onChange={(config) => setEditor({ ...editor, config })} />
              <div className="mt-3">
                <Button icon kind="primary" label="Save" disabled={busy || !editor.name} onClick={async () => {
                  if (await run(`/api/nginx/sites/${editor.name}`, "PUT", { name: editor.name, config: editor.config, enabled: editor.enabled })) setEditor(null);
                }}><Save size={16} /></Button>
              </div>
            </Card>
          )}
        </>
      )}
      {tab === "certificates" && (
        <>
          <Card title="Certbot" action={<Badge value={certbot ? certbotVersion || "installed" : "off"} />}>
            <form
              className="grid gap-3 md:grid-cols-2"
              onSubmit={async (event) => {
                event.preventDefault();
                if (!(await confirm("Issue a certificate with Certbot?"))) return;
                const domains = issueDomains.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean);
                await run("/api/nginx/certificates/issue", "POST", { site: issueSite, email: issueEmail, domains, confirm: true });
              }}
            >
              <Field label="Email"><input className={inputClass} type="email" value={issueEmail} onChange={(e) => setIssueEmail(e.target.value)} placeholder="admin@example.com" required /></Field>
              <Field label="Site">
                <select className={inputClass} value={issueSite} onChange={(e) => {
                  const name = e.target.value;
                  setIssueSite(name);
                  setIssueDomains(sites.find((site) => site.name === name)?.domains || "");
                }}>
                  <option value="">Domains only</option>
                  {sites.map((site) => <option key={site.name} value={site.name}>{site.name}</option>)}
                </select>
              </Field>
              <Field label="Domains" className="md:col-span-2"><input className={inputClass} value={issueDomains} onChange={(e) => setIssueDomains(e.target.value)} placeholder="example.com www.example.com" required={!issueSite} /></Field>
              <div className="flex gap-2 md:col-span-2">
                <Button type="submit" icon kind="primary" label="Issue" disabled={busy}><ShieldCheck size={16} /></Button>
                <Button icon label="Renew" disabled={busy} onClick={async () => {
                  if (await confirm("Renew certificates with Certbot?")) run("/api/nginx/certificates/renew", "POST", { confirm: true });
                }}><RefreshCw size={16} /></Button>
              </div>
            </form>
          </Card>
          <Card title="Upload">
            <form
              className="grid gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                run("/api/nginx/certificates", "POST", { name: certName, certificate: certBody, key: certKey }).then((ok) => {
                  if (ok) {
                    setCertBody("");
                    setCertKey("");
                  }
                });
              }}
            >
              <Field label="Name"><input className={inputClass} value={certName} onChange={(e) => setCertName(e.target.value)} placeholder="app" required /></Field>
              <Field label="Certificate"><textarea className={`${inputClass} min-h-28 font-mono text-xs`} value={certBody} onChange={(e) => setCertBody(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" required /></Field>
              <Field label="Private key"><textarea className={`${inputClass} min-h-28 font-mono text-xs`} value={certKey} onChange={(e) => setCertKey(e.target.value)} placeholder="-----BEGIN PRIVATE KEY-----" required /></Field>
              <div><Button type="submit" icon kind="primary" label="Upload" disabled={busy}><ShieldCheck size={16} /></Button></div>
            </form>
          </Card>
          <Card title="Certificates" action={<Badge value={String(certs.length)} />}>
            <ul className="space-y-2">
              {certs.map((item) => (
                <li key={item.cert} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2">
                  <span className="min-w-0">
                    <span className="flex items-center gap-2 font-medium">
                      <span className="truncate">{item.name}</span>
                      <Badge value={item.days < 0 ? "expired" : `${item.days}d`} />
                    </span>
                    <span className="block truncate text-xs text-[var(--muted)]">{item.subject || item.cert}</span>
                  </span>
                  {item.managed && (
                    <Button icon kind="danger" label="Delete" disabled={busy} onClick={async () => {
                      if (await confirm(`Delete ${item.name}?`)) run("/api/nginx/certificates", "DELETE", { name: item.name, confirm: true });
                    }}><Trash2 size={16} /></Button>
                  )}
                </li>
              ))}
            </ul>
          </Card>
        </>
      )}
      {tab === "logs" && (
        <div className="grid gap-3 xl:grid-cols-2">
          <Card title="Access" action={<Button icon label="Refresh" onClick={() => refreshLogs()}><RefreshCw size={16} /></Button>}><pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs">{logs.access || "—"}</pre></Card>
          <Card title="Error"><pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs">{logs.error || "—"}</pre></Card>
        </div>
      )}
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg)] px-3 py-2" title={value}>
      <div className="text-xs text-[var(--muted)]">{label}</div>
      <div className="truncate text-sm">{value || "—"}</div>
    </div>
  );
}
