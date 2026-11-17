import { useCallback, useEffect, useState } from "react";
import { Download, Folder, KeyRound, Lock, LockOpen, Play, Power, PowerOff, RefreshCw, RotateCw, Save, ScrollText, Square, Trash2, Upload, UserPlus } from "lucide-react";
import { api } from "../api";
import { Badge, Button, Card, Field, Modal, Placeholder, RowActions, useConfirm, useToast, TextInput } from "../components/ui";
import type { ActionResult, FTPStatus } from "../types";

export function FtpPage() {
  const [status, setStatus] = useState<FTPStatus | null>(null);
  const [port, setPort] = useState(21);
  const [pasvMin, setPasvMin] = useState(40000);
  const [pasvMax, setPasvMax] = useState(40100);
  const [pasvAddress, setPasvAddress] = useState("");
  const [write, setWrite] = useState(true);
  const [umask, setUmask] = useState("022");
  const [idle, setIdle] = useState(600);
  const [dataTimeout, setDataTimeout] = useState(120);
  const [maxClients, setMaxClients] = useState(50);
  const [maxPerIP, setMaxPerIP] = useState(5);
  const [rateKB, setRateKB] = useState(0);
  const [banner, setBanner] = useState("");
  const [transferLog, setTransferLog] = useState(true);
  const [tls, setTls] = useState(false);
  const [tlsCert, setTlsCert] = useState("");
  const [tlsKey, setTlsKey] = useState("");
  const [name, setName] = useState("");
  const [home, setHome] = useState("");
  const [homes, setHomes] = useState<string[]>([]);
  const [password, setPassword] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [userOpen, setUserOpen] = useState(false);
  const [nextPassword, setNextPassword] = useState("");
  const [logs, setLogs] = useState<string | null>(null);
  const [logTitle, setLogTitle] = useState("Logs");
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  const load = useCallback(async () => {
    const next = await api<FTPStatus>("/api/ftp");
    setStatus(next);
    setPort(next.port || 21);
    setPasvMin(next.pasv_min || 40000);
    setPasvMax(next.pasv_max || 40100);
    setPasvAddress(next.pasv_address || "");
    setWrite(next.write);
    setUmask(next.umask || "022");
    setIdle(next.idle_timeout || 600);
    setDataTimeout(next.data_timeout || 120);
    setMaxClients(next.max_clients || 50);
    setMaxPerIP(next.max_per_ip || 5);
    setRateKB(next.max_rate ? Math.round(next.max_rate / 1024) : 0);
    setBanner(next.banner || "");
    setTransferLog(next.transfer_log);
    setTls(next.tls);
    setTlsCert(next.tls_cert || "");
    setTlsKey(next.tls_key || "");
  }, []);

  useEffect(() => {
    load()
      .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
      .finally(() => setReady(true));
    api<{ paths: string[] }>("/api/ftp/homes")
      .then((res) => setHomes(res.paths))
      .catch(() => setHomes([]));
  }, [load]);

  async function run(path: string, method: string, body?: unknown) {
    setBusy(true);
    try {
      const res = await api<ActionResult>(path, { method, body: body === undefined ? undefined : JSON.stringify(body) });
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
    if (destructive && !(await confirm(`${action} FTP?`))) return;
    await run("/api/ftp/service", "POST", { action, confirm: destructive });
  }

  async function showLog(kind: "logs" | "transfers") {
    const path = kind === "logs" ? "/api/ftp/logs" : "/api/ftp/transfers";
    try {
      const res = await api<{ logs: string; error?: string }>(path);
      setLogTitle(kind === "logs" ? "Logs" : "Transfers");
      setLogs(res.logs || res.error || "");
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "request failed");
    }
  }

  const running = status?.active === "active";
  const enabled = status?.enabled === "enabled";
  const rate = status?.max_rate ? `${Math.round(status.max_rate / 1024)} KB/s` : "unlimited";

  return (
    <div className="space-y-4">
      <Card loading={busy || !ready} title="FTP" action={<Badge value={status?.installed ? status.active : "off"} />}>
        <div className="mb-4 grid gap-2 sm:grid-cols-3 xl:grid-cols-6">
          <Fact pending={!ready} label="Version" value={status?.version || "—"} />
          <Fact pending={!ready} label="Port" value={String(status?.port || 21)} />
          <Fact pending={!ready} label="Passive" value={status ? `${status.pasv_min}–${status.pasv_max}` : "—"} />
          <Fact pending={!ready} label="Sessions" value={String(status?.sessions.length ?? 0)} />
          <Fact pending={!ready} label="Clients" value={status ? `${status.max_clients} / ${status.max_per_ip}` : "—"} />
          <Fact pending={!ready} label="Rate" value={status ? rate : "—"} />
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {!status?.installed && (
            <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run("/api/ftp/install", "POST", {})}>
              <Download size={16} />
            </Button>
          )}
          {status?.installed && (
            <>
              <Button icon kind={running ? "default" : "primary"} label="Start" disabled={busy || running} onClick={() => service("start")}><Play size={16} /></Button>
              <Button icon kind="danger" label="Stop" disabled={busy || !running} onClick={() => service("stop")}><Square size={16} /></Button>
              <Button icon label="Restart" disabled={busy || !running} onClick={() => service("restart")}><RotateCw size={16} /></Button>
              <span className="mx-1 hidden h-6 w-px bg-[var(--border)] sm:block" />
              <Button icon label="Enable" disabled={busy || enabled} onClick={() => service("enable")}><Power size={16} /></Button>
              <Button icon kind="danger" label="Disable" disabled={busy || !enabled} onClick={() => service("disable")}><PowerOff size={16} /></Button>
              <span className="mx-1 hidden h-6 w-px bg-[var(--border)] sm:block" />
              <Button icon label="Logs" disabled={busy} onClick={() => showLog("logs")}><ScrollText size={16} /></Button>
              <Button icon label="Transfers" disabled={busy} onClick={() => showLog("transfers")}><Upload size={16} /></Button>
            </>
          )}
        </div>
        <Modal open={logs !== null} wide title={logTitle} onOpenChange={(open) => { if (!open) setLogs(null); }}>
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs leading-5">{logs || "—"}</pre>
        </Modal>
      </Card>

      <Card loading={busy || !ready} title="Configuration">
        <form
          className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4"
          onSubmit={(event) => {
            event.preventDefault();
            run("/api/ftp", "PUT", {
              port,
              pasv_min: pasvMin,
              pasv_max: pasvMax,
              pasv_address: pasvAddress,
              write,
              umask,
              idle_timeout: idle,
              data_timeout: dataTimeout,
              max_clients: maxClients,
              max_per_ip: maxPerIP,
              max_rate: rateKB > 0 ? rateKB * 1024 : 0,
              banner,
              transfer_log: transferLog,
              tls,
              tls_cert: tlsCert,
              tls_key: tlsKey,
            });
          }}
        >
          <Field label="Port"><TextInput type="number" value={port} onChange={(e) => setPort(Number(e.target.value))} /></Field>
          <Field label="Passive address"><TextInput value={pasvAddress} onChange={(e) => setPasvAddress(e.target.value)} placeholder="203.0.113.10" /></Field>
          <Field label="Passive min"><TextInput type="number" value={pasvMin} onChange={(e) => setPasvMin(Number(e.target.value))} /></Field>
          <Field label="Passive max"><TextInput type="number" value={pasvMax} onChange={(e) => setPasvMax(Number(e.target.value))} /></Field>
          <Field label="Umask"><TextInput value={umask} onChange={(e) => setUmask(e.target.value)} maxLength={3} /></Field>
          <Field label="Banner"><TextInput value={banner} onChange={(e) => setBanner(e.target.value)} /></Field>
          <Field label="Idle timeout"><TextInput type="number" value={idle} onChange={(e) => setIdle(Number(e.target.value))} /></Field>
          <Field label="Data timeout"><TextInput type="number" value={dataTimeout} onChange={(e) => setDataTimeout(Number(e.target.value))} /></Field>
          <Field label="Max clients"><TextInput type="number" value={maxClients} onChange={(e) => setMaxClients(Number(e.target.value))} /></Field>
          <Field label="Max per IP"><TextInput type="number" value={maxPerIP} onChange={(e) => setMaxPerIP(Number(e.target.value))} /></Field>
          <Field label="Max rate KB/s"><TextInput type="number" value={rateKB} onChange={(e) => setRateKB(Number(e.target.value))} /></Field>
          <div className="flex flex-wrap items-center gap-4 px-1 sm:col-span-2 xl:col-span-4">
            <Check label="Uploads" checked={write} onChange={setWrite} />
            <Check label="Transfer log" checked={transferLog} onChange={setTransferLog} />
            <Check label="TLS" checked={tls} onChange={setTls} />
          </div>
          {tls && (
            <>
              <Field label="TLS certificate" className="sm:col-span-2"><TextInput value={tlsCert} onChange={(e) => setTlsCert(e.target.value)} placeholder="/etc/ssl/certs/ftp.pem" /></Field>
              <Field label="TLS key" className="sm:col-span-2"><TextInput value={tlsKey} onChange={(e) => setTlsKey(e.target.value)} placeholder="/etc/ssl/private/ftp.key" /></Field>
            </>
          )}
          <div className="sm:col-span-2 xl:col-span-4">
            <Button type="submit" icon kind="primary" label="Save" disabled={busy}><Save size={16} /></Button>
          </div>
        </form>
      </Card>

      <Card
        loading={busy || !ready}
        title="Users"
        action={
          <span className="flex items-center gap-2">
            <Badge value={String(status?.users.length ?? 0)} />
            <Button icon kind="primary" label="Add" disabled={busy} onClick={() => setUserOpen(true)}><UserPlus size={16} /></Button>
          </span>
        }
      >
        <ul className="space-y-2">
          {(status?.users ?? []).map((user) => (
            <li key={user.name} className="rounded-lg border border-[var(--border)] px-3 py-2">
              <RowActions
                actions={[
                  { label: "Password", icon: <KeyRound size={16} />, disabled: busy, onSelect: () => { setEditing(user.name); setNextPassword(""); } },
                  {
                    label: user.locked ? "Unlock" : "Lock",
                    icon: user.locked ? <LockOpen size={16} /> : <Lock size={16} />,
                    danger: !user.locked,
                    disabled: busy,
                    onSelect: async () => {
                      if (!user.locked && !(await confirm(`Lock ${user.name}?`))) return;
                      run("/api/ftp/users/lock", "POST", { name: user.name, lock: !user.locked, confirm: !user.locked });
                    },
                  },
                  { label: user.write ? "Read only" : "Uploads", icon: <Upload size={16} />, disabled: busy, onSelect: () => run("/api/ftp/users/write", "POST", { name: user.name, write: !user.write }) },
                  {
                    label: "Delete",
                    icon: <Trash2 size={16} />,
                    danger: true,
                    disabled: busy,
                    onSelect: async () => {
                      if (await confirm(`Delete ${user.name}?`)) run("/api/ftp/users", "DELETE", { name: user.name, confirm: true });
                    },
                  },
                ]}
              >
                <span className="min-w-0">
                  <span className="flex items-center gap-2 font-medium">
                    <span className="truncate">{user.name}</span>
                    <Badge value={user.locked ? "locked" : user.write ? "write" : "read only"} />
                  </span>
                  <span className="block truncate text-xs text-[var(--muted)]">{user.home || "—"}</span>
                </span>
              </RowActions>
            </li>
          ))}
        </ul>
        <Modal open={userOpen} title="Add FTP user" onOpenChange={setUserOpen}>
          <form
            className="grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              run("/api/ftp/users", "POST", { name, home, password }).then((ok) => {
                if (!ok) return;
                setPassword("");
                setUserOpen(false);
              });
            }}
          >
            <Field label="Username"><TextInput value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <HomeField value={home} paths={homes} onChange={setHome} />
            <Field label="Password"><TextInput type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
            <div className="flex justify-end"><Button type="submit" icon kind="primary" label="Add" disabled={busy}><UserPlus size={16} /></Button></div>
          </form>
        </Modal>
        <Modal open={editing !== null} title={editing ? `Password for ${editing}` : "Password"} onOpenChange={(open) => { if (!open) setEditing(null); }}>
          <form
            className="grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              run("/api/ftp/users/password", "PUT", { name: editing, password: nextPassword }).then((ok) => {
                if (!ok) return;
                setEditing(null);
                setNextPassword("");
              });
            }}
          >
            <Field label="New password"><TextInput type="password" value={nextPassword} onChange={(e) => setNextPassword(e.target.value)} /></Field>
            <div className="flex justify-end"><Button type="submit" icon kind="primary" label="Save" disabled={busy}><Save size={16} /></Button></div>
          </form>
        </Modal>
      </Card>

      <Card loading={busy || !ready}
        title="Sessions"
        action={
          <span className="flex items-center gap-2">
            <Badge value={String(status?.sessions.length ?? 0)} />
            <Button icon label="Refresh" disabled={busy} onClick={() => load().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))}><RefreshCw size={16} /></Button>
          </span>
        }
      >
        <ul className="space-y-2">
          {(status?.sessions ?? []).map((session) => (
            <li key={`${session.local}-${session.peer}`} className="flex items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2 text-sm">
              <span className="truncate">{session.peer}</span>
              <span className="truncate text-xs text-[var(--muted)]">{session.local}</span>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

function suggestHomes(paths: string[], query: string) {
  const q = query.trim().toLowerCase();
  return paths
    .filter((path) => !q || path.toLowerCase().includes(q))
    .sort((a, b) => a.split("/").length - b.split("/").length || a.localeCompare(b))
    .slice(0, 12);
}

function HomeField({ value, paths, onChange }: { value: string; paths: string[]; onChange: (value: string) => void }) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const matches = suggestHomes(paths, value);
  const shown = open && matches.length > 0;

  function choose(path: string) {
    onChange(path);
    setOpen(false);
  }

  return (
    <Field label="Home" className="relative">
      <TextInput
        role="combobox"
        aria-expanded={shown}
        aria-autocomplete="list"
        aria-controls="ftp-home-list"
        value={value}
        placeholder="/srv/ftp/username"
        onChange={(event) => {
          onChange(event.target.value);
          setActive(0);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => window.setTimeout(() => setOpen(false), 120)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown") {
            event.preventDefault();
            setOpen(true);
            setActive((index) => Math.min(index + 1, Math.max(matches.length - 1, 0)));
          } else if (event.key === "ArrowUp") {
            event.preventDefault();
            setActive((index) => Math.max(index - 1, 0));
          } else if (event.key === "Enter" && shown) {
            event.preventDefault();
            choose(matches[active] ?? matches[0]);
          } else if (event.key === "Escape") {
            setOpen(false);
          }
        }}
      />
      {shown && (
        <ul id="ftp-home-list" role="listbox" className="absolute left-0 right-0 top-full z-30 mt-1 max-h-52 overflow-auto rounded-md border border-[var(--border)] bg-[var(--card)] py-1 shadow-[var(--shadow)]">
          {matches.map((path, index) => (
            <li key={path} role="option" aria-selected={index === active}>
              <button
                type="button"
                className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm ${index === active ? "bg-[var(--hover)]" : ""}`}
                onMouseDown={(event) => {
                  event.preventDefault();
                  choose(path);
                }}
                onMouseEnter={() => setActive(index)}
              >
                <Folder size={14} className="shrink-0 text-[var(--muted)]" />
                <span className="truncate">{path}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </Field>
  );
}

function Check({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <label className="inline-flex h-8 items-center gap-2 text-sm">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      {label}
    </label>
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
