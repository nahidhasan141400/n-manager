import { Tabs } from "radix-ui";
import { useCallback, useEffect, useState } from "react";
import { Check, Copy, Cylinder, Database, Download, GitBranch, Hexagon, KeyRound, Play, Power, PowerOff, RotateCw, Save, ScrollText, Square, Star, Trash2, Workflow } from "lucide-react";
import { api } from "../api";
import { Badge, Button, Card, Field, Modal, Placeholder, RowActions, useConfirm, useToast, TextInput } from "../components/ui";
import { formatBytes, formatUptime } from "../format";
import type { ActionResult, Engine, GitStatus, NodeStatus, PM2Status } from "../types";
import { MySQLPanel } from "./MySQLPanel";

const apps = [
  { id: "mysql", label: "MySQL", icon: Database, tone: "bg-[color-mix(in_srgb,var(--accent)_16%,transparent)] text-[var(--accent)]" },
  { id: "postgres", label: "PostgreSQL", icon: Cylinder, tone: "bg-[color-mix(in_srgb,var(--link)_16%,transparent)] text-[var(--link)]" },
  { id: "node", label: "Node.js", icon: Hexagon, tone: "bg-[color-mix(in_srgb,var(--warning)_16%,transparent)] text-[var(--warning)]" },
  { id: "pm2", label: "PM2", icon: Workflow, tone: "bg-[var(--hover)] text-[var(--text)]" },
  { id: "git", label: "Git", icon: GitBranch, tone: "bg-[color-mix(in_srgb,var(--link)_16%,transparent)] text-[var(--link)]" },
] as const;

export function Applications() {
  const [tab, setTab] = useState<(typeof apps)[number]["id"]>("mysql");
  const [mysql, setMysql] = useState<Engine | null>(null);
  const [postgres, setPostgres] = useState<Engine | null>(null);
  const [node, setNode] = useState<NodeStatus | null>(null);
  const [pm2, setPm2] = useState<PM2Status | null>(null);
  const [git, setGit] = useState<GitStatus | null>(null);
  const [ready, setReady] = useState(false);
  const toast = useToast();

  const refresh = useCallback(async () => {
    const [nextMysql, nextPostgres, nextNode, nextPm2, nextGit] = await Promise.all([
      api<Engine>("/api/apps/mysql"),
      api<Engine>("/api/apps/postgres"),
      api<NodeStatus>("/api/apps/node"),
      api<PM2Status>("/api/apps/pm2"),
      api<GitStatus>("/api/apps/git"),
    ]);
    setMysql(nextMysql);
    setPostgres(nextPostgres);
    setNode(nextNode);
    setPm2(nextPm2);
    setGit(nextGit);
  }, []);

  useEffect(() => {
    refresh()
      .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
      .finally(() => setReady(true));
  }, [refresh]);

  const engines = { mysql, postgres };

  return (
    <div className="space-y-4">
      <Tabs.Root value={tab} onValueChange={(value) => setTab(value as typeof tab)}>
      <Tabs.List className="grid gap-3 grid-cols-2 md:grid-cols-5">
        {apps.map((app) => {
          const Icon = app.icon;
          const summary = app.id === "node" ? nodeSummary(node) : app.id === "pm2" ? pm2Summary(pm2) : app.id === "git" ? gitSummary(git) : engineSummary(engines[app.id]);
          const live = app.id === "node" ? Boolean(node?.node_version) : app.id === "pm2" ? Boolean(pm2?.processes.some((item) => item.status === "online")) : app.id === "git" ? Boolean(git?.installed) : engines[app.id]?.active === "active";
          return (
            <Tabs.Trigger
              key={app.id}
              value={app.id}
              className={`flex items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors ${!ready ? "gradient-frame" : "border data-[state=active]:border-[var(--accent)] data-[state=active]:bg-[color-mix(in_srgb,var(--accent)_8%,var(--card))] data-[state=inactive]:border-[var(--border)] data-[state=inactive]:bg-[var(--card)] data-[state=inactive]:hover:bg-[var(--hover)]"}`}
            >
              <span className={`grid h-10 w-10 shrink-0 place-items-center rounded-lg ${app.tone}`}>
                <Icon size={18} />
              </span>
              <span className="min-w-0">
                <span className="flex items-center gap-2 font-medium">
                  {app.label}
                  {!ready ? null : <span className={`h-2 w-2 rounded-full ${live ? "bg-[var(--accent)]" : "bg-[var(--muted)]"}`} />}
                </span>
                {!ready ? <Placeholder className="mt-1 h-3 w-20" /> : <span className="block truncate text-xs text-[var(--muted)]">{summary}</span>}
              </span>
            </Tabs.Trigger>
          );
        })}
      </Tabs.List>
      </Tabs.Root>
      {tab === "node" ? <NodePanel status={node} reload={refresh} pending={!ready} /> : tab === "pm2" ? <PM2Panel status={pm2} reload={refresh} pending={!ready} /> : tab === "git" ? <GitPanel status={git} reload={refresh} pending={!ready} /> : (
        <div className="space-y-4">
          <DatabasePanel id={tab} engine={engines[tab]} reload={refresh} pending={!ready} />
          {tab === "mysql" && mysql?.installed && <MySQLPanel />}
        </div>
      )}
    </div>
  );
}

function engineSummary(engine: Engine | null) {
  if (!engine) return "…";
  if (!engine.installed) return "not installed";
  return engine.version ? `${engine.active} · ${engine.version}` : engine.active;
}

function nodeSummary(status: NodeStatus | null) {
  if (!status) return "…";
  if (!status.nvm_installed) return "not installed";
  return status.node_version || "no version";
}

function gitSummary(status: GitStatus | null) {
  if (!status) return "…";
  if (!status.installed) return "not installed";
  return status.version || "installed";
}

function pm2Summary(status: PM2Status | null) {
  if (!status) return "…";
  if (!status.installed) return "not installed";
  const online = status.processes.filter((item) => item.status === "online").length;
  return `${online} online`;
}

function shown(value: string) {
  const text = value.trim();
  return text && text !== "unknown" ? text : "—";
}

function Fact({ label, value, pending }: { label: string; value: string; pending?: boolean }) {
  const text = shown(value);
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg)] px-3 py-2" title={pending || text === "—" ? undefined : text}>
      <div className="text-xs text-[var(--muted)]">{label}</div>
      {pending ? <Placeholder className="mt-1 h-4 w-16" /> : <div className="truncate text-sm">{text}</div>}
    </div>
  );
}

function DatabasePanel({ id, engine, reload, pending }: { id: "mysql" | "postgres"; engine: Engine | null; reload: () => Promise<void>; pending?: boolean }) {
  const [logs, setLogs] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [maxConn, setMaxConn] = useState("151");
  const [waitTimeout, setWaitTimeout] = useState("28800");
  const toast = useToast();
  const confirm = useConfirm();
  const running = engine?.active === "active";
  const enabled = engine?.enabled === "enabled";
  const app = apps.find((item) => item.id === id)!;
  const Icon = app.icon;

  useEffect(() => {
    setLogs(null);
  }, [id]);

  useEffect(() => {
    if (id !== "mysql" || !engine) return;
    setMaxConn(engine.config.max_connections || "151");
    setWaitTimeout(engine.config.wait_timeout || "28800");
  }, [id, engine]);

  async function run(path: string, body: unknown, message: string) {
    setBusy(true);
    try {
      const res = await api<ActionResult>(path, { method: "POST", body: JSON.stringify(body) });
      toast("ok", res.message);
      await reload();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : message);
    } finally {
      setBusy(false);
    }
  }

  async function service(action: string) {
    const destructive = action === "stop" || action === "disable";
    if (destructive && !(await confirm(`${action} ${engine?.label}?`))) return;
    await run(`/api/apps/${id}/service`, { action, confirm: destructive }, action);
  }

  async function toggleLogs() {
    const res = await api<{ logs: string; error?: string }>(`/api/apps/${id}/logs`);
    setLogs(res.logs || res.error || "");
  }

  const bind = engine?.config.bind_address || engine?.config.listen_addresses || "";

  return (
    <Card
      title={engine?.label || app.label}
      loading={pending || busy}
      action={pending ? undefined : <Badge value={engine?.installed ? engine.active : "off"} />}
    >
      <div className="mb-4 flex items-center gap-3">
        <span className={`grid h-11 w-11 place-items-center rounded-xl ${app.tone}`}>
          <Icon size={20} />
        </span>
        <div className="grid flex-1 gap-2 sm:grid-cols-3">
          <Fact pending={pending} label="Version" value={engine?.version || "—"} />
          <Fact pending={pending} label="Bind" value={bind} />
          <Fact pending={pending} label="Boot" value={engine?.installed ? engine.enabled : "—"} />
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {!pending && !engine?.installed && (
          <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run(`/api/apps/${id}/install`, {}, "install")}>
            <Download size={16} />
          </Button>
        )}
        {!pending && engine?.installed && (
          <>
            <Button icon kind={running ? "default" : "primary"} label="Start" disabled={busy || running} onClick={() => service("start")}><Play size={16} /></Button>
            <Button icon kind="danger" label="Stop" disabled={busy || !running} onClick={() => service("stop")}><Square size={16} /></Button>
            <Button icon label="Restart" disabled={busy || !running} onClick={() => service("restart")}><RotateCw size={16} /></Button>
            <span className="mx-1 hidden h-6 w-px bg-[var(--border)] sm:block" />
            <Button icon label="Enable" disabled={busy || enabled} onClick={() => service("enable")}><Power size={16} /></Button>
            <Button icon kind="danger" label="Disable" disabled={busy || !enabled} onClick={() => service("disable")}><PowerOff size={16} /></Button>
            <span className="mx-1 hidden h-6 w-px bg-[var(--border)] sm:block" />
            <Button icon label="Logs" disabled={busy} onClick={() => toggleLogs().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))}><ScrollText size={16} /></Button>
          </>
        )}
      </div>
      <Modal open={logs !== null} wide title={`${engine?.label || app.label} logs`} onOpenChange={(open) => { if (!open) setLogs(null); }}>
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs leading-5">{logs || "—"}</pre>
      </Modal>
      {id === "mysql" && engine?.installed && (
        <form
          className="mt-4 flex flex-wrap items-end gap-2 border-t border-[var(--border)] pt-4"
          onSubmit={(event) => {
            event.preventDefault();
            run("/api/apps/mysql/settings", { max_connections: Number(maxConn), wait_timeout: Number(waitTimeout) }, "settings");
          }}
        >
          <Field label="Max connections" className="w-40">
            <TextInput inputMode="numeric" value={maxConn} onChange={(event) => setMaxConn(event.target.value)} />
          </Field>
          <Field label="Wait timeout" className="w-40">
            <TextInput inputMode="numeric" value={waitTimeout} onChange={(event) => setWaitTimeout(event.target.value)} />
          </Field>
          <Button icon kind="primary" type="submit" label="Save" disabled={busy}><Check size={16} /></Button>
        </form>
      )}
    </Card>
  );
}

function NodePanel({ status, reload, pending }: { status: NodeStatus | null; reload: () => Promise<void>; pending?: boolean }) {
  const [version, setVersion] = useState("lts");
  const [pkg, setPkg] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  async function run(path: string, body: unknown) {
    setBusy(true);
    try {
      const res = await api<ActionResult>(path, { method: "POST", body: JSON.stringify(body) });
      toast("ok", res.message);
      await reload();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "request failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <Card title="Node.js" loading={pending || busy} action={pending ? undefined : <Badge value={status?.node_version || "off"} />}>
        <div className="mb-4 grid gap-2 sm:grid-cols-3">
          <Fact pending={pending} label="Node" value={status?.node_version || "—"} />
          <Fact pending={pending} label="npm" value={status?.npm_version || "—"} />
          <Fact pending={pending} label="NVM" value={status?.nvm_installed ? status.nvm_path : "—"} />
        </div>
        {!pending && !status?.nvm_installed && (
          <Button icon kind="primary" label="Install NVM" disabled={busy} onClick={() => run("/api/apps/node/nvm/install", {})}>
            <Download size={16} />
          </Button>
        )}
      </Card>
      <Card title="Versions" loading={pending || busy} action={pending ? undefined : <Badge value={String(status?.versions.length ?? 0)} />}>
        <form className="mb-3 flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); run("/api/apps/node/versions", { version }); }}>
          <Field label="Version" className="flex-1"><TextInput value={version} onChange={(e) => setVersion(e.target.value)} placeholder="20 or lts" /></Field>
          <Button type="submit" icon kind="primary" label="Install" disabled={busy}><Download size={16} /></Button>
        </form>
        <ul className="space-y-2">
          {pending && Array.from({ length: 2 }, (_, index) => <li key={index}><Placeholder className="h-10 w-full" /></li>)}
          {(status?.versions ?? []).map((item) => {
            const isDefault = status?.default === item;
            return (
              <li key={item} className="rounded-lg border border-[var(--border)] px-3 py-2">
                <RowActions
                  actions={[
                    ...(!isDefault ? [{ label: "Set default", icon: <Star size={16} />, disabled: busy, onSelect: () => run("/api/apps/node/default", { version: item }) }] : []),
                    { label: "Uninstall", icon: <Trash2 size={16} />, danger: true, disabled: busy, onSelect: async () => { if (await confirm(`Uninstall ${item}?`)) run("/api/apps/node/versions/remove", { version: item, confirm: true }); } },
                  ]}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <Hexagon size={14} className="shrink-0 text-[var(--warning)]" />
                    <span className="truncate">{item}</span>
                    {isDefault && <Badge value="default" />}
                  </span>
                </RowActions>
              </li>
            );
          })}
        </ul>
      </Card>
      <Card title="Packages" loading={pending || busy} action={pending ? undefined : <Badge value={String(status?.global_packages.length ?? 0)} />}>
        <form className="mb-3 flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); run("/api/apps/node/packages", { name: pkg }); }}>
          <Field label="Package" className="flex-1"><TextInput value={pkg} onChange={(e) => setPkg(e.target.value)} placeholder="pm2" /></Field>
          <Button type="submit" icon kind="primary" label="Install" disabled={busy}><Download size={16} /></Button>
        </form>
        <ul className="space-y-2">
          {(status?.global_packages ?? []).map((item) => (
            <li key={item} className="rounded-lg border border-[var(--border)] px-3 py-2 text-sm">
              <RowActions
                actions={[{
                  label: "Remove",
                  icon: <Trash2 size={16} />,
                  danger: true,
                  disabled: busy,
                  onSelect: async () => {
                    const name = item.split("@")[0];
                    if (await confirm(`Remove ${name}?`)) run("/api/apps/node/packages/remove", { name, confirm: true });
                  },
                }]}
              >
                <span className="truncate">{item}</span>
              </RowActions>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

function PM2Panel({ status, reload, pending }: { status: PM2Status | null; reload: () => Promise<void>; pending?: boolean }) {
  const [name, setName] = useState("");
  const [script, setScript] = useState("");
  const [logs, setLogs] = useState<string | null>(null);
  const [logName, setLogName] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  async function run(action: string, processName: string, processScript = "", confirmed = false) {
    setBusy(true);
    try {
      const res = await api<ActionResult>("/api/apps/pm2/action", {
        method: "POST",
        body: JSON.stringify({ action, name: processName, script: processScript, confirm: confirmed }),
      });
      toast("ok", res.message);
      await reload();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "request failed");
    } finally {
      setBusy(false);
    }
  }

  async function showLogs(processName: string) {
    const res = await api<{ logs: string; error?: string }>(`/api/apps/pm2/logs?name=${encodeURIComponent(processName)}`);
    setLogName(processName);
    setLogs(res.logs || res.error || "");
  }

  return (
    <div className="space-y-4">
      <Card title="PM2" loading={pending || busy} action={pending ? undefined : <Badge value={status?.installed ? status.version || "installed" : "off"} />}>
        {!pending && !status?.installed && (
          <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run("install", "")}>
            <Download size={16} />
          </Button>
        )}
        {!pending && status?.installed && (
          <form
            className="flex flex-col gap-2 sm:flex-row sm:items-end"
            onSubmit={(event) => {
              event.preventDefault();
              run("start", name, script);
            }}
          >
            <Field label="Script" className="flex-1"><TextInput value={script} onChange={(e) => setScript(e.target.value)} placeholder="/home/app/server.js" /></Field>
            <Field label="Name" className="sm:w-40"><TextInput value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <Button type="submit" icon kind="primary" label="Start" disabled={busy}><Play size={16} /></Button>
          </form>
        )}
      </Card>
      {status?.installed && (
        <Card title="Processes" loading={busy} action={<Badge value={String(status.processes.length)} />}>
          <ul className="space-y-2">
            {status.processes.map((item) => {
              const online = item.status === "online";
              return (
                <li key={item.id} className="rounded-lg border border-[var(--border)] px-3 py-2">
                  <RowActions
                    actions={[
                      online
                        ? { label: "Stop", icon: <Square size={16} />, danger: true, disabled: busy, onSelect: async () => { if (await confirm(`Stop ${item.name}?`)) run("stop", item.name, "", true); } }
                        : { label: "Start", icon: <Play size={16} />, disabled: busy, onSelect: () => run("start", item.name) },
                      { label: "Restart", icon: <RotateCw size={16} />, disabled: busy, onSelect: () => run("restart", item.name) },
                      { label: "Logs", icon: <ScrollText size={16} />, disabled: busy, onSelect: () => showLogs(item.name).catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed")) },
                      { label: "Delete", icon: <Trash2 size={16} />, danger: true, disabled: busy, onSelect: async () => { if (await confirm(`Delete ${item.name}?`)) run("delete", item.name, "", true); } },
                    ]}
                  >
                    <span className="min-w-0">
                      <span className="flex items-center gap-2">
                        <span className="truncate font-medium">{item.name}</span>
                        <Badge value={item.status || "unknown"} />
                      </span>
                      <span className="mt-1 block text-xs text-[var(--muted)]">
                        {item.cpu.toFixed(0)}% · {formatBytes(item.memory)} · {item.restarts} restarts · {online ? formatUptime(item.uptime) : "stopped"}
                      </span>
                    </span>
                  </RowActions>
                </li>
              );
            })}
          </ul>
          <Modal open={logs !== null} wide title={logName ? `${logName} logs` : "Logs"} onOpenChange={(open) => { if (!open) { setLogs(null); setLogName(""); } }}>
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs leading-5">{logs || "—"}</pre>
          </Modal>
        </Card>
      )}
    </div>
  );
}

async function copyText(value: string, toast: (kind: "ok" | "err", text: string) => void) {
  try {
    await navigator.clipboard.writeText(value);
    toast("ok", "Copied the public key");
  } catch {
    toast("err", "Could not copy the public key");
  }
}

function GitPanel({ status, reload, pending }: { status: GitStatus | null; reload: () => Promise<void>; pending?: boolean }) {
  const [name, setName] = useState(status?.name || "");
  const [email, setEmail] = useState(status?.email || "");
  const [url, setUrl] = useState("");
  const [dest, setDest] = useState("");
  const [cloneOpen, setCloneOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  useEffect(() => {
    setName(status?.name || "");
    setEmail(status?.email || "");
  }, [status?.name, status?.email]);

  async function run(path: string, body: unknown) {
    setBusy(true);
    try {
      const res = await api<ActionResult>(path, { method: "POST", body: JSON.stringify(body) });
      toast("ok", res.message);
      await reload();
      return true;
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "request failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <Card title="Git" loading={pending || busy} action={pending ? undefined : <Badge value={status?.installed ? status.version || "installed" : "off"} />}>
        <div className="mb-4 grid gap-2 sm:grid-cols-3">
          <Fact pending={pending} label="Version" value={status?.version || "—"} />
          <Fact pending={pending} label="Name" value={status?.name || "—"} />
          <Fact pending={pending} label="Email" value={status?.email || "—"} />
        </div>
        {!pending && !status?.installed && (
          <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run("/api/apps/git/install", {})}>
            <Download size={16} />
          </Button>
        )}
        {!pending && status?.installed && (
          <form
            className="flex flex-col gap-2 sm:flex-row sm:items-end"
            onSubmit={(event) => {
              event.preventDefault();
              void run("/api/apps/git/config", { name, email });
            }}
          >
            <Field label="Name" className="flex-1"><TextInput value={name} onChange={(event) => setName(event.target.value)} placeholder="Ada Lovelace" /></Field>
            <Field label="Email" className="flex-1"><TextInput type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="ada@example.com" /></Field>
            <Button type="submit" icon kind="primary" label="Save" disabled={busy}><Save size={16} /></Button>
          </form>
        )}
      </Card>
      <Card
        title="GitHub"
        loading={pending || busy}
        action={pending ? undefined : <Badge value={status?.github?.connected ? status.github.login || "connected" : status?.github?.ready ? "key" : "off"} />}
      >
        <p className="text-sm text-[var(--muted)]">
          {status?.github?.connected
            ? `GitHub accepts this server key as ${status.github.login}. Git uses it for github.com, including https addresses.`
            : "Create a key on this server, add it to GitHub, then test. Private repositories can then be cloned and pulled."}
        </p>
        {!pending && !status?.github?.ready && (
          <div className="mt-4">
            <Button icon kind="primary" label="Connect" disabled={busy} onClick={() => run("/api/apps/git/github", {})}><KeyRound size={16} /></Button>
          </div>
        )}
        {!pending && status?.github?.ready && (
          <div className="mt-4 space-y-3">
            {!status.github.connected && (
              <>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <a className="text-sm text-[var(--link)]" href="https://github.com/settings/ssh/new" target="_blank" rel="noreferrer">Add the key on GitHub</a>
                  <Button icon label="Copy" disabled={busy} onClick={() => copyText(status.github.public_key, toast)}><Copy size={16} /></Button>
                </div>
                <pre className="overflow-auto rounded-md border border-[var(--border)] bg-[var(--bg)] p-3 text-xs">{status.github.public_key}</pre>
              </>
            )}
            <Button icon label="Test" disabled={busy} onClick={() => run("/api/apps/git/github/test", {})}><KeyRound size={16} /></Button>
          </div>
        )}
      </Card>
      {status?.installed && (
        <Card
          title="Repositories"
          loading={busy}
          action={<Button icon kind="primary" label="Clone" disabled={busy} onClick={() => setCloneOpen(true)}><GitBranch size={16} /></Button>}
        >
          <p className="mb-3 text-sm text-[var(--muted)]">Repositories under /var/www, /opt, /srv, /root, and home directories.</p>
          <ul className="space-y-2">
            {(status.repos ?? []).length === 0 && <li className="text-sm text-[var(--muted)]">No repositories yet.</li>}
            {(status.repos ?? []).map((repo) => (
              <li key={repo.path} className="rounded-lg border border-[var(--border)] px-3 py-2">
                <RowActions
                  actions={[{
                    label: "Pull",
                    icon: <Download size={16} />,
                    disabled: busy,
                    onSelect: async () => {
                      if (await confirm(`Pull ${repo.path}?`)) run("/api/apps/git/pull", { path: repo.path, confirm: true });
                    },
                  }]}
                >
                  <span className="min-w-0">
                    <span className="flex items-center gap-2">
                      <span className="truncate font-medium">{repo.path}</span>
                      {repo.branch && <Badge value={repo.branch} />}
                    </span>
                    <span className="block truncate text-xs text-[var(--muted)]">{repo.remote || "no origin"}</span>
                  </span>
                </RowActions>
              </li>
            ))}
          </ul>
          <Modal open={cloneOpen} title="Clone repository" description="The directory is created under /var/www, /opt, /srv, /root, or a home directory." onOpenChange={setCloneOpen}>
            <form
              className="grid gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                void run("/api/apps/git/clone", { url, path: dest }).then((ok) => {
                  if (!ok) return;
                  setUrl("");
                  setCloneOpen(false);
                });
              }}
            >
              <Field label="Address"><TextInput value={url} onChange={(event) => setUrl(event.target.value)} placeholder="https://github.com/org/app.git" autoComplete="off" /></Field>
              <Field label="Directory"><TextInput value={dest} onChange={(event) => setDest(event.target.value)} placeholder="/var/www/app" autoComplete="off" /></Field>
              <div className="flex justify-end">
                <Button type="submit" icon kind="primary" label="Clone" disabled={busy || !url.trim() || !dest.trim()}><GitBranch size={16} /></Button>
              </div>
            </form>
          </Modal>
        </Card>
      )}
    </div>
  );
}
