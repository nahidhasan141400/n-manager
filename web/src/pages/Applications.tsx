import { useCallback, useEffect, useState } from "react";
import { Check, Cylinder, Database, Download, Hexagon, Play, Power, PowerOff, RotateCw, ScrollText, Square, Star, Trash2, Workflow, X } from "lucide-react";
import { api } from "../api";
import { Badge, Button, Card, Field, inputClass, useConfirm, useToast } from "../components/ui";
import { formatBytes, formatUptime } from "../format";
import type { ActionResult, Engine, NodeStatus, PM2Status } from "../types";
import { MySQLPanel } from "./MySQLPanel";

const apps = [
  { id: "mysql", label: "MySQL", icon: Database, tone: "bg-[color-mix(in_srgb,var(--accent)_16%,transparent)] text-[var(--accent)]" },
  { id: "postgres", label: "PostgreSQL", icon: Cylinder, tone: "bg-[color-mix(in_srgb,var(--link)_16%,transparent)] text-[var(--link)]" },
  { id: "node", label: "Node.js", icon: Hexagon, tone: "bg-[color-mix(in_srgb,var(--warning)_16%,transparent)] text-[var(--warning)]" },
  { id: "pm2", label: "PM2", icon: Workflow, tone: "bg-[var(--hover)] text-[var(--text)]" },
] as const;

export function Applications() {
  const [tab, setTab] = useState<(typeof apps)[number]["id"]>("mysql");
  const [mysql, setMysql] = useState<Engine | null>(null);
  const [postgres, setPostgres] = useState<Engine | null>(null);
  const [node, setNode] = useState<NodeStatus | null>(null);
  const [pm2, setPm2] = useState<PM2Status | null>(null);
  const toast = useToast();

  const refresh = useCallback(async () => {
    const [nextMysql, nextPostgres, nextNode, nextPm2] = await Promise.all([
      api<Engine>("/api/apps/mysql"),
      api<Engine>("/api/apps/postgres"),
      api<NodeStatus>("/api/apps/node"),
      api<PM2Status>("/api/apps/pm2"),
    ]);
    setMysql(nextMysql);
    setPostgres(nextPostgres);
    setNode(nextNode);
    setPm2(nextPm2);
  }, []);

  useEffect(() => {
    refresh().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"));
  }, [refresh]);

  const engines = { mysql, postgres };

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {apps.map((app) => {
          const Icon = app.icon;
          const selected = tab === app.id;
          const summary = app.id === "node" ? nodeSummary(node) : app.id === "pm2" ? pm2Summary(pm2) : engineSummary(engines[app.id]);
          const live = app.id === "node" ? Boolean(node?.node_version) : app.id === "pm2" ? Boolean(pm2?.processes.some((item) => item.status === "online")) : engines[app.id]?.active === "active";
          return (
            <button
              key={app.id}
              type="button"
              aria-pressed={selected}
              onClick={() => setTab(app.id)}
              className={`flex items-center gap-3 rounded-xl border px-3 py-3 text-left transition-colors ${selected ? "border-[var(--accent)] bg-[color-mix(in_srgb,var(--accent)_8%,var(--card))]" : "border-[var(--border)] bg-[var(--card)] hover:bg-[var(--hover)]"}`}
            >
              <span className={`grid h-10 w-10 shrink-0 place-items-center rounded-lg ${app.tone}`}>
                <Icon size={18} />
              </span>
              <span className="min-w-0">
                <span className="flex items-center gap-2 font-medium">
                  {app.label}
                  <span className={`h-2 w-2 rounded-full ${live ? "bg-[var(--accent)]" : "bg-[var(--muted)]"}`} />
                </span>
                <span className="block truncate text-xs text-[var(--muted)]">{summary}</span>
              </span>
            </button>
          );
        })}
      </div>
      {tab === "node" ? <NodePanel status={node} reload={refresh} /> : tab === "pm2" ? <PM2Panel status={pm2} reload={refresh} /> : (
        <div className="space-y-4">
          <DatabasePanel id={tab} engine={engines[tab]} reload={refresh} />
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

function Fact({ label, value }: { label: string; value: string }) {
  const text = shown(value);
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg)] px-3 py-2" title={text === "—" ? undefined : text}>
      <div className="text-xs text-[var(--muted)]">{label}</div>
      <div className="truncate text-sm">{text}</div>
    </div>
  );
}

function DatabasePanel({ id, engine, reload }: { id: "mysql" | "postgres"; engine: Engine | null; reload: () => Promise<void> }) {
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
    if (logs !== null) {
      setLogs(null);
      return;
    }
    const res = await api<{ logs: string; error?: string }>(`/api/apps/${id}/logs`);
    setLogs(res.logs || res.error || "");
  }

  const bind = engine?.config.bind_address || engine?.config.listen_addresses || "";

  return (
    <Card
      title={engine?.label || app.label}
      action={<Badge value={engine?.installed ? engine.active : "off"} />}
    >
      <div className="mb-4 flex items-center gap-3">
        <span className={`grid h-11 w-11 place-items-center rounded-xl ${app.tone}`}>
          <Icon size={20} />
        </span>
        <div className="grid flex-1 gap-2 sm:grid-cols-3">
          <Fact label="Version" value={engine?.version || "—"} />
          <Fact label="Bind" value={bind} />
          <Fact label="Boot" value={engine?.installed ? engine.enabled : "—"} />
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {!engine?.installed && (
          <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run(`/api/apps/${id}/install`, {}, "install")}>
            <Download size={16} />
          </Button>
        )}
        {engine?.installed && (
          <>
            <Button icon kind={running ? "default" : "primary"} label="Start" disabled={busy || running} onClick={() => service("start")}><Play size={16} /></Button>
            <Button icon kind="danger" label="Stop" disabled={busy || !running} onClick={() => service("stop")}><Square size={16} /></Button>
            <Button icon label="Restart" disabled={busy || !running} onClick={() => service("restart")}><RotateCw size={16} /></Button>
            <span className="mx-1 hidden h-6 w-px bg-[var(--border)] sm:block" />
            <Button icon label="Enable" disabled={busy || enabled} onClick={() => service("enable")}><Power size={16} /></Button>
            <Button icon kind="danger" label="Disable" disabled={busy || !enabled} onClick={() => service("disable")}><PowerOff size={16} /></Button>
            <span className="mx-1 hidden h-6 w-px bg-[var(--border)] sm:block" />
            <Button icon label={logs !== null ? "Hide" : "Logs"} disabled={busy} onClick={() => toggleLogs().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))}><ScrollText size={16} /></Button>
          </>
        )}
      </div>
      {logs !== null && <LogView text={logs} onClose={() => setLogs(null)} />}
      {id === "mysql" && engine?.installed && (
        <form
          className="mt-4 flex flex-wrap items-end gap-2 border-t border-[var(--border)] pt-4"
          onSubmit={(event) => {
            event.preventDefault();
            run("/api/apps/mysql/settings", { max_connections: Number(maxConn), wait_timeout: Number(waitTimeout) }, "settings");
          }}
        >
          <Field label="Max connections" className="w-40">
            <input className={inputClass} inputMode="numeric" value={maxConn} onChange={(event) => setMaxConn(event.target.value)} />
          </Field>
          <Field label="Wait timeout" className="w-40">
            <input className={inputClass} inputMode="numeric" value={waitTimeout} onChange={(event) => setWaitTimeout(event.target.value)} />
          </Field>
          <Button icon kind="primary" type="submit" label="Save" disabled={busy}><Check size={16} /></Button>
        </form>
      )}
    </Card>
  );
}

function LogView({ text, onClose }: { text: string; onClose: () => void }) {
  return (
    <div className="mt-4 overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--input)]">
      <div className="flex items-center justify-between border-b border-[var(--border)] px-3 py-2">
        <span className="text-xs text-[var(--muted)]">Logs</span>
        <button type="button" aria-label="Close" className="grid h-7 w-7 place-items-center rounded-md hover:bg-[var(--hover)]" onClick={onClose}>
          <X size={14} />
        </button>
      </div>
      <pre className="max-h-80 overflow-auto p-3 text-xs leading-5">{text || "—"}</pre>
    </div>
  );
}

function NodePanel({ status, reload }: { status: NodeStatus | null; reload: () => Promise<void> }) {
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
      <Card title="Node.js" action={<Badge value={status?.node_version || "off"} />}>
        <div className="mb-4 grid gap-2 sm:grid-cols-3">
          <Fact label="Node" value={status?.node_version || "—"} />
          <Fact label="npm" value={status?.npm_version || "—"} />
          <Fact label="NVM" value={status?.nvm_installed ? status.nvm_path : "—"} />
        </div>
        {!status?.nvm_installed && (
          <Button icon kind="primary" label="Install NVM" disabled={busy} onClick={() => run("/api/apps/node/nvm/install", {})}>
            <Download size={16} />
          </Button>
        )}
      </Card>
      <Card title="Versions" action={<Badge value={String(status?.versions.length ?? 0)} />}>
        <form className="mb-3 flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); run("/api/apps/node/versions", { version }); }}>
          <Field label="Version" className="flex-1"><input className={inputClass} value={version} onChange={(e) => setVersion(e.target.value)} placeholder="20 or lts" /></Field>
          <Button type="submit" icon kind="primary" label="Install" disabled={busy}><Download size={16} /></Button>
        </form>
        <ul className="space-y-2">
          {(status?.versions ?? []).map((item) => {
            const isDefault = status?.default === item;
            return (
              <li key={item} className="flex items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2">
                <span className="flex min-w-0 items-center gap-2">
                  <Hexagon size={14} className="shrink-0 text-[var(--warning)]" />
                  <span className="truncate">{item}</span>
                  {isDefault && <Badge value="default" />}
                </span>
                <span className="flex gap-2">
                  {!isDefault && (
                    <Button icon label="Set default" disabled={busy} onClick={() => run("/api/apps/node/default", { version: item })}><Star size={16} /></Button>
                  )}
                  <Button icon kind="danger" label="Uninstall" disabled={busy} onClick={async () => { if (await confirm(`Uninstall ${item}?`)) run("/api/apps/node/versions/remove", { version: item, confirm: true }); }}><Trash2 size={16} /></Button>
                </span>
              </li>
            );
          })}
        </ul>
      </Card>
      <Card title="Packages" action={<Badge value={String(status?.global_packages.length ?? 0)} />}>
        <form className="mb-3 flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); run("/api/apps/node/packages", { name: pkg }); }}>
          <Field label="Package" className="flex-1"><input className={inputClass} value={pkg} onChange={(e) => setPkg(e.target.value)} placeholder="pm2" /></Field>
          <Button type="submit" icon kind="primary" label="Install" disabled={busy}><Download size={16} /></Button>
        </form>
        <ul className="space-y-2">
          {(status?.global_packages ?? []).map((item) => (
            <li key={item} className="flex items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2 text-sm">
              <span className="truncate">{item}</span>
              <Button icon kind="danger" label="Remove" disabled={busy} onClick={async () => {
                const name = item.split("@")[0];
                if (await confirm(`Remove ${name}?`)) run("/api/apps/node/packages/remove", { name, confirm: true });
              }}><Trash2 size={16} /></Button>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

function PM2Panel({ status, reload }: { status: PM2Status | null; reload: () => Promise<void> }) {
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
    if (logs !== null && logName === processName) {
      setLogs(null);
      setLogName("");
      return;
    }
    const res = await api<{ logs: string; error?: string }>(`/api/apps/pm2/logs?name=${encodeURIComponent(processName)}`);
    setLogName(processName);
    setLogs(res.logs || res.error || "");
  }

  return (
    <div className="space-y-4">
      <Card title="PM2" action={<Badge value={status?.installed ? status.version || "installed" : "off"} />}>
        {!status?.installed && (
          <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run("install", "")}>
            <Download size={16} />
          </Button>
        )}
        {status?.installed && (
          <form
            className="flex flex-col gap-2 sm:flex-row sm:items-end"
            onSubmit={(event) => {
              event.preventDefault();
              run("start", name, script);
            }}
          >
            <Field label="Script" className="flex-1"><input className={inputClass} value={script} onChange={(e) => setScript(e.target.value)} placeholder="/home/app/server.js" /></Field>
            <Field label="Name" className="sm:w-40"><input className={inputClass} value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <Button type="submit" icon kind="primary" label="Start" disabled={busy}><Play size={16} /></Button>
          </form>
        )}
      </Card>
      {status?.installed && (
        <Card title="Processes" action={<Badge value={String(status.processes.length)} />}>
          <ul className="space-y-2">
            {status.processes.map((item) => {
              const online = item.status === "online";
              return (
                <li key={item.id} className="rounded-lg border border-[var(--border)] px-3 py-3">
                  <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="truncate font-medium">{item.name}</span>
                        <Badge value={item.status || "unknown"} />
                      </div>
                      <div className="mt-1 text-xs text-[var(--muted)]">
                        {item.cpu.toFixed(0)}% · {formatBytes(item.memory)} · {item.restarts} restarts · {online ? formatUptime(item.uptime) : "stopped"}
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {online ? (
                        <Button icon kind="danger" label="Stop" disabled={busy} onClick={async () => { if (await confirm(`Stop ${item.name}?`)) run("stop", item.name, "", true); }}><Square size={16} /></Button>
                      ) : (
                        <Button icon kind="primary" label="Start" disabled={busy} onClick={() => run("start", item.name)}><Play size={16} /></Button>
                      )}
                      <Button icon label="Restart" disabled={busy} onClick={() => run("restart", item.name)}><RotateCw size={16} /></Button>
                      <Button icon label={logs !== null && logName === item.name ? "Hide" : "Logs"} disabled={busy} onClick={() => showLogs(item.name).catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))}><ScrollText size={16} /></Button>
                      <Button icon kind="danger" label="Delete" disabled={busy} onClick={async () => { if (await confirm(`Delete ${item.name}?`)) run("delete", item.name, "", true); }}><Trash2 size={16} /></Button>
                    </div>
                  </div>
                  {logs !== null && logName === item.name && <LogView text={logs} onClose={() => { setLogs(null); setLogName(""); }} />}
                </li>
              );
            })}
          </ul>
        </Card>
      )}
    </div>
  );
}
