import { FormEvent, useEffect, useState } from "react";
import { Download, FilePen, Globe, Play, Power, PowerOff, RefreshCw, RotateCw, Save, ScrollText, ShieldCheck, Square, Trash2 } from "lucide-react";
import { api } from "../api";
import { CodeEditor } from "../components/CodeEditor";
import { Badge, Button, Card, CheckField, Field, Modal, Placeholder, RowActions, SelectBox, TabBar, useConfirm, useToast, TextInput, TextAreaInput } from "../components/ui";
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
  const [siteOpen, setSiteOpen] = useState(false);
  const [logs, setLogs] = useState({ access: "", error: "" });
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
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
  useEffect(() => {
    load()
      .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
      .finally(() => setReady(true));
  }, []);

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
    if (!(await run("/api/nginx/sites", "POST", payload()))) return false;
    setForm(empty);
    return true;
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
      <TabBar
        value={tab}
        onValueChange={(value) => {
          setTab(value as typeof tab);
          if (value === "logs") refreshLogs().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"));
        }}
        items={[
          { value: "service", label: "Service", icon: <Power size={16} /> },
          { value: "sites", label: "Sites", icon: <Globe size={16} /> },
          { value: "certificates", label: "Certificates", icon: <ShieldCheck size={16} /> },
          { value: "logs", label: "Logs", icon: <ScrollText size={16} /> },
        ]}
      />
      {tab === "service" && (
        <>
          <Card loading={busy || !ready} title="Nginx" action={<Badge value={status?.installed ? status.active : "off"} />}>
            <div className="mb-4 grid gap-2 sm:grid-cols-3">
              <Fact pending={!ready} label="Version" value={status?.version || "—"} />
              <Fact pending={!ready} label="Boot" value={status?.enabled || "—"} />
              <Fact pending={!ready} label="Layout" value={status?.layout || "—"} />
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
          <Card loading={busy || !ready} title="nginx.conf">
            {mainError ? <p className="text-sm text-[var(--warning)]">{mainError}</p> : <CodeEditor value={mainConfig} readOnly />}
          </Card>
        </>
      )}
      {tab === "sites" && (
        <>
          <Card
            loading={busy || !ready}
            title="Sites"
            action={
              <span className="flex items-center gap-2">
                <Badge value={String(sites.length)} />
                <Button icon kind="primary" label="New site" disabled={busy} onClick={() => { setForm(empty); setSiteOpen(true); }}><Globe size={16} /></Button>
              </span>
            }
          >
            {siteError && <p className="mb-3 text-sm text-[var(--warning)]">{siteError}</p>}
            <ul className="space-y-2">
              {sites.map((site) => (
                <li key={site.name} className="rounded-lg border border-[var(--border)] px-3 py-2">
                  <RowActions
                    actions={[
                      { label: "Edit", icon: <FilePen size={16} />, onSelect: () => setEditor({ name: site.name, config: site.config, enabled: site.enabled }) },
                      { label: site.enabled ? "Disable" : "Enable", icon: site.enabled ? <PowerOff size={16} /> : <Power size={16} />, disabled: busy, onSelect: () => run(`/api/nginx/sites/${site.name}/${site.enabled ? "disable" : "enable"}`, "POST") },
                      { label: "Delete", icon: <Trash2 size={16} />, danger: true, disabled: busy, onSelect: async () => { if (await confirm(`Delete ${site.name}?`)) run(`/api/nginx/sites/${site.name}`, "DELETE", { confirm: true }); } },
                    ]}
                  >
                    <span className="min-w-0">
                      <span className="flex items-center gap-2 font-medium">
                        <span className="truncate">{site.name}</span>
                        <Badge value={site.enabled ? "on" : "off"} />
                      </span>
                      <span className="block truncate text-xs text-[var(--muted)]">{site.domains || "—"}</span>
                    </span>
                  </RowActions>
                </li>
              ))}
            </ul>
          </Card>
          <Modal open={siteOpen} wide title="New site" onOpenChange={setSiteOpen}>
            <form className="grid items-end gap-3 md:grid-cols-2" onSubmit={async (event) => { if (await create(event)) setSiteOpen(false); }}>
              <Field label="Name"><TextInput value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></Field>
              <Field label="Domains"><TextInput value={form.domains} onChange={(e) => setForm({ ...form, domains: e.target.value })} placeholder="example.com, www.example.com" required /></Field>
              <Field label="Mode">
                <SelectBox label="Mode" value={form.mode} onValueChange={(mode) => setForm({ ...form, mode })} options={[{ value: "proxy", label: "Proxy" }, { value: "root", label: "Root" }]} />
              </Field>
              {form.mode === "proxy" ? (
                <Field label="Proxy"><TextInput value={form.proxy_pass} onChange={(e) => setForm({ ...form, proxy_pass: e.target.value })} placeholder="http://127.0.0.1:3000" /></Field>
              ) : (
                <Field label="Root"><TextInput value={form.web_root} onChange={(e) => setForm({ ...form, web_root: e.target.value })} placeholder="/var/www/html" /></Field>
              )}
              <Field label="Certificate">
                <SelectBox
                  label="Certificate"
                  value={certs.find((item) => item.cert === form.ssl_certificate)?.name || ""}
                  onValueChange={chooseCert}
                  options={[{ value: "", label: "None" }, ...certs.map((item) => ({ value: item.name, label: item.name }))]}
                />
              </Field>
              <Field label="Body size"><TextInput value={form.body_size} onChange={(e) => setForm({ ...form, body_size: e.target.value })} placeholder="20m" /></Field>
              <Field label="Certificate path"><TextInput value={form.ssl_certificate} onChange={(e) => setForm({ ...form, ssl_certificate: e.target.value })} placeholder="/etc/ssl/certs/site.pem" /></Field>
              <Field label="Key path"><TextInput value={form.ssl_certificate_key} onChange={(e) => setForm({ ...form, ssl_certificate_key: e.target.value })} placeholder="/etc/ssl/private/site.key" /></Field>
              <CheckField label="HTTPS redirect" checked={form.redirect} onCheckedChange={(redirect) => setForm({ ...form, redirect })} />
              <CheckField label="Websocket" checked={form.websocket} onCheckedChange={(websocket) => setForm({ ...form, websocket })} />
              <CheckField label="Enable" checked={form.enabled} onCheckedChange={(enabled) => setForm({ ...form, enabled })} />
              <div className="flex justify-end gap-2 md:col-span-2">
                <Button icon label="Editor" disabled={busy} onClick={openPreview}><FilePen size={16} /></Button>
                <Button type="submit" icon kind="primary" label="Create" disabled={busy}><Download size={16} /></Button>
              </div>
            </form>
          </Modal>
          <Modal open={editor !== null} wide title={editor?.name || "Config"} onOpenChange={(open) => { if (!open) setEditor(null); }}>
            {editor && (
              <>
                <CodeEditor value={editor.config} onChange={(config) => setEditor({ ...editor, config })} />
                <div className="mt-3 flex justify-end">
                  <Button icon kind="primary" label="Save" disabled={busy || !editor.name} onClick={async () => {
                    if (await run(`/api/nginx/sites/${editor.name}`, "PUT", { name: editor.name, config: editor.config, enabled: editor.enabled })) setEditor(null);
                  }}><Save size={16} /></Button>
                </div>
              </>
            )}
          </Modal>
        </>
      )}
      {tab === "certificates" && (
        <>
          <Card loading={busy || !ready} title="Certbot" action={<Badge value={certbot ? certbotVersion || "installed" : "off"} />}>
            <form
              className="grid gap-3 md:grid-cols-2"
              onSubmit={async (event) => {
                event.preventDefault();
                if (!(await confirm("Issue a certificate with Certbot?"))) return;
                const domains = issueDomains.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean);
                await run("/api/nginx/certificates/issue", "POST", { site: issueSite, email: issueEmail, domains, confirm: true });
              }}
            >
              <Field label="Email"><TextInput type="email" value={issueEmail} onChange={(e) => setIssueEmail(e.target.value)} placeholder="admin@example.com" required /></Field>
              <Field label="Site">
                <SelectBox
                  label="Site"
                  value={issueSite}
                  onValueChange={(name) => {
                    setIssueSite(name);
                    setIssueDomains(sites.find((site) => site.name === name)?.domains || "");
                  }}
                  options={[{ value: "", label: "Domains only" }, ...sites.map((site) => ({ value: site.name, label: site.name }))]}
                />
              </Field>
              <Field label="Domains" className="md:col-span-2"><TextInput value={issueDomains} onChange={(e) => setIssueDomains(e.target.value)} placeholder="example.com www.example.com" required={!issueSite} /></Field>
              <div className="flex gap-2 md:col-span-2">
                <Button type="submit" icon kind="primary" label="Issue" disabled={busy}><ShieldCheck size={16} /></Button>
                <Button icon label="Renew" disabled={busy} onClick={async () => {
                  if (await confirm("Renew certificates with Certbot?")) run("/api/nginx/certificates/renew", "POST", { confirm: true });
                }}><RefreshCw size={16} /></Button>
              </div>
            </form>
          </Card>
          <Card loading={busy || !ready} title="Upload">
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
              <Field label="Name"><TextInput value={certName} onChange={(e) => setCertName(e.target.value)} placeholder="app" required /></Field>
              <Field label="Certificate"><TextAreaInput className="min-h-28 font-mono text-xs" value={certBody} onChange={(e) => setCertBody(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" required /></Field>
              <Field label="Private key"><TextAreaInput className="min-h-28 font-mono text-xs" value={certKey} onChange={(e) => setCertKey(e.target.value)} placeholder="-----BEGIN PRIVATE KEY-----" required /></Field>
              <div><Button type="submit" icon kind="primary" label="Upload" disabled={busy}><ShieldCheck size={16} /></Button></div>
            </form>
          </Card>
          <Card loading={busy || !ready} title="Certificates" action={<Badge value={String(certs.length)} />}>
            <ul className="space-y-2">
              {certs.map((item) => (
                <li key={item.cert} className="rounded-lg border border-[var(--border)] px-3 py-2">
                  <RowActions
                    actions={item.managed ? [{
                      label: "Delete",
                      icon: <Trash2 size={16} />,
                      danger: true,
                      disabled: busy,
                      onSelect: async () => {
                        if (await confirm(`Delete ${item.name}?`)) run("/api/nginx/certificates", "DELETE", { name: item.name, confirm: true });
                      },
                    }] : []}
                  >
                    <span className="min-w-0">
                      <span className="flex items-center gap-2 font-medium">
                        <span className="truncate">{item.name}</span>
                        <Badge value={item.days < 0 ? "expired" : `${item.days}d`} />
                      </span>
                      <span className="block truncate text-xs text-[var(--muted)]">{item.subject || item.cert}</span>
                    </span>
                  </RowActions>
                </li>
              ))}
            </ul>
          </Card>
        </>
      )}
      {tab === "logs" && (
        <div className="grid gap-3 xl:grid-cols-2">
          <Card loading={busy || !ready} title="Access" action={<Button icon label="Refresh" onClick={() => refreshLogs()}><RefreshCw size={16} /></Button>}><pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs">{logs.access || "—"}</pre></Card>
          <Card loading={busy || !ready} title="Error"><pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs">{logs.error || "—"}</pre></Card>
        </div>
      )}
    </div>
  );
}

function Fact({ label, value, pending }: { label: string; value: string; pending?: boolean }) {
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg)] px-3 py-2" title={pending ? undefined : value}>
      <div className="text-xs text-[var(--muted)]">{label}</div>
      {pending ? <Placeholder className="mt-1 h-4 w-16" /> : <div className="truncate text-sm">{value || "—"}</div>}
    </div>
  );
}
