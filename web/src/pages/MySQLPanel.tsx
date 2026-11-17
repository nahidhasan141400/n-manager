import { useCallback, useEffect, useState } from "react";
import { Download, KeyRound, Plus, Power, PowerOff, Shield, Trash2 } from "lucide-react";
import { api } from "../api";
import { Badge, Button, Card, Field, inputClass, useConfirm, useToast } from "../components/ui";
import type { ActionResult, MySQLAdminView, MySQLGrant } from "../types";

const privileges = ["ALL", "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "INDEX", "ALTER", "CREATE VIEW", "SHOW VIEW"];

const emptyAdmin: MySQLAdminView = {
  databases: [],
  users: [],
  grants: [],
  phpmyadmin: { installed: false, version: "", enabled: false, mode: "" },
};

export function MySQLPanel() {
  const [admin, setAdmin] = useState<MySQLAdminView>(emptyAdmin);
  const [dbName, setDbName] = useState("");
  const [userName, setUserName] = useState("");
  const [host, setHost] = useState("localhost");
  const [password, setPassword] = useState("");
  const [grantUser, setGrantUser] = useState("");
  const [grantHost, setGrantHost] = useState("localhost");
  const [grantDb, setGrantDb] = useState("");
  const [selected, setSelected] = useState<string[]>(["SELECT"]);
  const [passwords, setPasswords] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  const load = useCallback(async () => {
    const next = await api<MySQLAdminView>("/api/apps/mysql/admin");
    setAdmin({
      databases: next.databases ?? [],
      users: next.users ?? [],
      grants: next.grants ?? [],
      phpmyadmin: next.phpmyadmin ?? emptyAdmin.phpmyadmin,
      error: next.error,
    });
  }, []);

  useEffect(() => {
    load().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"));
  }, [load]);

  async function run(path: string, method: string, body: unknown) {
    setBusy(true);
    try {
      const res = await api<ActionResult>(path, { method, body: JSON.stringify(body) });
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

  function togglePrivilege(priv: string) {
    setSelected((current) => {
      if (priv === "ALL") return current.includes("ALL") ? [] : ["ALL"];
      const withoutAll = current.filter((item) => item !== "ALL");
      return withoutAll.includes(priv) ? withoutAll.filter((item) => item !== priv) : [...withoutAll, priv];
    });
  }

  function editGrant(grant: MySQLGrant) {
    setGrantUser(grant.user);
    if (grant.host === "localhost" || grant.host === "127.0.0.1" || grant.host === "%") setGrantHost(grant.host);
    setGrantDb(grant.database);
    const known = grant.privileges.filter((item) => privileges.includes(item));
    setSelected(known.length === privileges.length - 1 ? ["ALL"] : known);
  }

  const manageable = admin.databases.filter((item) => !item.system);
  const accounts = admin.users.filter((item) => !item.system);

  return (
    <div className="space-y-4">
      {admin.error && (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] px-3 py-2 text-sm text-[var(--muted)]">
          {admin.error}. Create, drop, and privilege changes still go through the panel helper.
        </div>
      )}
      <Card title="Databases">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            run("/api/apps/mysql/databases", "POST", { name: dbName.trim() }).then((ok) => { if (ok) setDbName(""); });
          }}
        >
          <Field label="Database" className="w-56">
            <input className={inputClass} value={dbName} onChange={(event) => setDbName(event.target.value)} placeholder="app_db" autoComplete="off" />
          </Field>
          <Button icon kind="primary" type="submit" label="Create" disabled={busy || !dbName.trim()}><Plus size={16} /></Button>
        </form>
        <ul className="mt-4 space-y-2">
          {admin.databases.length === 0 && !admin.error && <li className="text-sm text-[var(--muted)]">No databases</li>}
          {admin.databases.map((item) => (
            <li key={item.name} className="flex items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2">
              <span className="truncate text-sm">{item.name}</span>
              {item.system ? <Badge value="system" /> : (
                <Button
                  icon
                  kind="danger"
                  label="Drop"
                  disabled={busy}
                  onClick={async () => {
                    if (!(await confirm(`Drop database ${item.name}?`))) return;
                    await run("/api/apps/mysql/databases", "DELETE", { name: item.name, confirm: true });
                  }}
                >
                  <Trash2 size={16} />
                </Button>
              )}
            </li>
          ))}
        </ul>
      </Card>
      <Card title="Users">
        <form
          className="grid items-end gap-2 md:grid-cols-[minmax(0,1fr)_10rem_minmax(0,1fr)_auto]"
          onSubmit={(event) => {
            event.preventDefault();
            run("/api/apps/mysql/users", "POST", { name: userName.trim(), host, password }).then((ok) => {
              if (!ok) return;
              setUserName("");
              setPassword("");
            });
          }}
        >
          <Field label="Username">
            <input className={inputClass} value={userName} onChange={(event) => setUserName(event.target.value)} placeholder="app" autoComplete="off" />
          </Field>
          <Field label="Host">
            <select className={inputClass} value={host} onChange={(event) => setHost(event.target.value)}>
              <option value="localhost">localhost</option>
              <option value="127.0.0.1">127.0.0.1</option>
              <option value="%">any local client (%)</option>
            </select>
          </Field>
          <Field label="Password">
            <input className={inputClass} type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="new-password" />
          </Field>
          <Button icon kind="primary" type="submit" label="Create" disabled={busy || !userName.trim() || password.length < 8}><Plus size={16} /></Button>
        </form>
        <ul className="mt-4 space-y-2">
          {admin.users.length === 0 && !admin.error && <li className="text-sm text-[var(--muted)]">No users</li>}
          {admin.users.map((item) => {
            const key = `${item.name}@${item.host}`;
            return (
              <li key={key} className="flex flex-wrap items-end justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2">
                <span className="pb-2 text-sm">{item.name}@{item.host}</span>
                {item.system ? <Badge value="system" /> : (
                  <div className="flex flex-wrap items-end gap-2">
                    <Field label="New password">
                      <input
                        className={`${inputClass} w-44`}
                        type="password"
                        value={passwords[key] ?? ""}
                        autoComplete="new-password"
                        onChange={(event) => setPasswords((current) => ({ ...current, [key]: event.target.value }))}
                      />
                    </Field>
                    <Button
                      icon
                      label="Password"
                      disabled={busy || (passwords[key] ?? "").length < 8}
                      onClick={() => run("/api/apps/mysql/users/password", "PUT", { name: item.name, host: item.host, password: passwords[key] ?? "" }).then(() => setPasswords((current) => ({ ...current, [key]: "" })))}
                    >
                      <KeyRound size={16} />
                    </Button>
                    <Button
                      icon
                      kind="danger"
                      label="Drop"
                      disabled={busy}
                      onClick={async () => {
                        if (!(await confirm(`Drop user ${key}?`))) return;
                        await run("/api/apps/mysql/users", "DELETE", { name: item.name, host: item.host, confirm: true });
                      }}
                    >
                      <Trash2 size={16} />
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      </Card>
      <Card title="Permissions">
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            run("/api/apps/mysql/grants", "POST", { name: grantUser, host: grantHost, database: grantDb, privileges: selected });
          }}
        >
          <div className="grid items-end gap-2 md:grid-cols-3">
            <Field label="User">
              <input className={inputClass} list="mysql-users" value={grantUser} onChange={(event) => setGrantUser(event.target.value)} placeholder="app" autoComplete="off" />
              <datalist id="mysql-users">
                {accounts.map((item) => <option key={`${item.name}@${item.host}`} value={item.name} />)}
              </datalist>
            </Field>
            <Field label="Host">
              <select className={inputClass} value={grantHost} onChange={(event) => setGrantHost(event.target.value)}>
                <option value="localhost">localhost</option>
                <option value="127.0.0.1">127.0.0.1</option>
                <option value="%">any local client (%)</option>
              </select>
            </Field>
            <Field label="Database">
              <input className={inputClass} list="mysql-databases" value={grantDb} onChange={(event) => setGrantDb(event.target.value)} placeholder="app_db" autoComplete="off" />
              <datalist id="mysql-databases">
                {manageable.map((item) => <option key={item.name} value={item.name} />)}
              </datalist>
            </Field>
          </div>
          <div>
            <div className="mb-1 text-xs text-[var(--muted)]">Privileges</div>
            <div className="flex flex-wrap gap-2">
              {privileges.map((priv) => (
                <label key={priv} className="flex items-center gap-1.5 rounded-md border border-[var(--border)] px-2 py-1 text-xs">
                  <input type="checkbox" checked={selected.includes(priv)} onChange={() => togglePrivilege(priv)} />
                  {priv}
                </label>
              ))}
            </div>
          </div>
          <Button icon kind="primary" type="submit" label="Save" disabled={busy || !grantUser || !grantDb || selected.length === 0}><Shield size={16} /></Button>
        </form>
        <ul className="mt-4 space-y-2">
          {admin.grants.length === 0 && !admin.error && <li className="text-sm text-[var(--muted)]">No privileges</li>}
          {admin.grants.map((grant) => (
            <li key={`${grant.user}@${grant.host}/${grant.database}`} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-2">
              <span className="min-w-0 text-sm">
                <span className="font-medium">{grant.user}@{grant.host}</span>
                <span className="text-[var(--muted)]"> on {grant.database}</span>
                <span className="mt-0.5 block text-xs text-[var(--muted)]">{grant.privileges.join(", ")}</span>
              </span>
              {!grant.system && (
                <span className="flex gap-2">
                  <Button icon label="Edit" disabled={busy} onClick={() => editGrant(grant)}><Shield size={16} /></Button>
                  <Button
                    icon
                    kind="danger"
                    label="Revoke"
                    disabled={busy}
                    onClick={async () => {
                      if (!(await confirm(`Revoke privileges of ${grant.user}@${grant.host} on ${grant.database}?`))) return;
                      await run("/api/apps/mysql/grants", "DELETE", { name: grant.user, host: grant.host, database: grant.database, confirm: true });
                    }}
                  >
                    <Trash2 size={16} />
                  </Button>
                </span>
              )}
            </li>
          ))}
        </ul>
      </Card>
      <Card
        title="phpMyAdmin"
        action={<Badge value={admin.phpmyadmin.enabled ? "enabled" : admin.phpmyadmin.installed ? "installed" : "off"} />}
      >
        <div className="mb-4 grid gap-2 sm:grid-cols-3">
          <Fact label="Version" value={admin.phpmyadmin.version} />
          <Fact label="Mode" value={admin.phpmyadmin.mode} />
          <Fact label="Address" value={admin.phpmyadmin.mode === "nginx" && admin.phpmyadmin.enabled ? "127.0.0.1:8088" : ""} />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button icon kind="primary" label="Install" disabled={busy || admin.phpmyadmin.installed} onClick={() => run("/api/apps/mysql/phpmyadmin/install", "POST", {})}><Download size={16} /></Button>
          <Button icon label="Enable" disabled={busy || !admin.phpmyadmin.installed || admin.phpmyadmin.enabled} onClick={() => run("/api/apps/mysql/phpmyadmin", "POST", { action: "enable" })}><Power size={16} /></Button>
          <Button
            icon
            kind="danger"
            label="Disable"
            disabled={busy || !admin.phpmyadmin.enabled}
            onClick={async () => {
              if (!(await confirm("Disable phpMyAdmin?"))) return;
              await run("/api/apps/mysql/phpmyadmin", "POST", { action: "disable", confirm: true });
            }}
          >
            <PowerOff size={16} />
          </Button>
        </div>
      </Card>
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  const text = value.trim() || "—";
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg)] px-3 py-2">
      <div className="text-xs text-[var(--muted)]">{label}</div>
      <div className="truncate text-sm">{text}</div>
    </div>
  );
}
