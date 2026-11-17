import { useCallback, useEffect, useRef, useState } from "react";
import { Copy, Download, KeyRound, Plus, Shield, Trash2, Upload } from "lucide-react";
import { api, download, serverId } from "../api";
import { Badge, Button, Card, Field, Modal, Placeholder, RowActions, SelectBox, useConfirm, useToast, TextInput } from "../components/ui";
import type { ActionResult, MySQLAdminView, MySQLGrant } from "../types";

const privileges = ["ALL", "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "INDEX", "ALTER", "CREATE VIEW", "SHOW VIEW"];

const emptyAdmin: MySQLAdminView = {
  databases: [],
  users: [],
  grants: [],
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
  const [dbOpen, setDbOpen] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  const [grantOpen, setGrantOpen] = useState(false);
  const [passwordFor, setPasswordFor] = useState<{ name: string; host: string } | null>(null);
  const [nextPassword, setNextPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
  const [importTarget, setImportTarget] = useState("");
  const [ssh, setSsh] = useState<{ host: string; port: number; username: string } | null>(null);
  const importFile = useRef<HTMLInputElement>(null);
  const toast = useToast();
  const confirm = useConfirm();

  const load = useCallback(async () => {
    const next = await api<MySQLAdminView>("/api/apps/mysql/admin");
    setAdmin({
      databases: next.databases ?? [],
      users: next.users ?? [],
      grants: next.grants ?? [],
      error: next.error,
    });
  }, []);

  useEffect(() => {
    load()
      .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
      .finally(() => setReady(true));
  }, [load]);

  useEffect(() => {
    api<{ servers: { id: string; host: string; port: number; username: string }[] }>("/api/servers")
      .then((res) => {
        const item = (res.servers || []).find((server) => server.id === serverId());
        if (item) setSsh({ host: item.host, port: item.port || 22, username: item.username });
      })
      .catch(() => undefined);
  }, []);

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
    setGrantOpen(true);
  }

  function openGrant() {
    setGrantUser("");
    setGrantHost("localhost");
    setGrantDb("");
    setSelected(["SELECT"]);
    setGrantOpen(true);
  }

  const manageable = admin.databases.filter((item) => !item.system);
  const accounts = admin.users.filter((item) => !item.system);
  const tunnel = ssh ? `ssh -p ${ssh.port} -L 3306:127.0.0.1:3306 ${ssh.username}@${ssh.host}` : "";

  return (
    <div className="space-y-4">
      <Card loading={!ready} title="Connect a local database tool">
        <p className="text-sm text-[var(--muted)]">MySQL on this server accepts connections from the server itself. DataGrip and other local tools reach it through SSH.</p>
        <ol className="mt-3 list-decimal space-y-2 pl-5 text-sm">
          <li>Create a user below with host <span className="font-medium">127.0.0.1</span> and a password. Use that account in the tool.</li>
          <li>Set the MySQL connection to host <span className="font-medium">127.0.0.1</span>, port <span className="font-medium">3306</span>, that user, and the database name.</li>
          <li>Turn on the tool’s SSH connection, or run the tunnel below and leave it open.</li>
        </ol>
        <div className="mt-3 grid gap-2 sm:grid-cols-3">
          <div className="rounded-lg border border-[var(--border)] px-3 py-2">
            <div className="text-xs text-[var(--muted)]">SSH host</div>
            {!ready ? <Placeholder className="mt-1 h-4 w-28" /> : <div className="truncate text-sm">{ssh?.host || "—"}</div>}
          </div>
          <div className="rounded-lg border border-[var(--border)] px-3 py-2">
            <div className="text-xs text-[var(--muted)]">SSH port</div>
            {!ready ? <Placeholder className="mt-1 h-4 w-10" /> : <div className="text-sm">{ssh?.port || "—"}</div>}
          </div>
          <div className="rounded-lg border border-[var(--border)] px-3 py-2">
            <div className="text-xs text-[var(--muted)]">SSH user</div>
            {!ready ? <Placeholder className="mt-1 h-4 w-16" /> : <div className="truncate text-sm">{ssh?.username || "—"}</div>}
          </div>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <code className="min-w-0 flex-1 overflow-x-auto rounded-md border border-[var(--border)] bg-[var(--bg)] px-3 py-2 text-xs">{!ready ? <Placeholder className="h-4 w-full" /> : tunnel || "Select a server"}</code>
          <Button
            icon
            label="Copy"
            disabled={!ready || !tunnel}
            onClick={() => {
              navigator.clipboard.writeText(tunnel).then(
                () => toast("ok", "Copied the SSH tunnel"),
                () => toast("err", "Could not copy the SSH tunnel"),
              );
            }}
          >
            <Copy size={16} />
          </Button>
        </div>
        <p className="mt-3 text-sm text-[var(--muted)]">If port 3306 is already used on your computer, change the first 3306 in the command to 3307 and use port 3307 in the tool.</p>
      </Card>
      {admin.error && (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] px-3 py-2 text-sm text-[var(--muted)]">
          {admin.error}. Create, drop, and privilege changes still go through the panel helper.
        </div>
      )}
      <Card
        loading={busy || !ready}
        title="Databases"
        action={<Button icon kind="primary" label="Create" disabled={busy} onClick={() => setDbOpen(true)}><Plus size={16} /></Button>}
      >
        <p className="mb-3 text-sm text-[var(--muted)]">Export downloads a SQL file. Import runs a SQL file, up to 32 MB, inside that database.</p>
        <input
          ref={importFile}
          type="file"
          accept=".sql,text/plain"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            const name = importTarget;
            event.target.value = "";
            if (!file || !name) return;
            void (async () => {
              if (file.size > 32 * 1024 * 1024) {
                toast("err", "SQL file must be 32 MB or smaller");
                return;
              }
              if (!(await confirm(`Import ${file.name} into ${name}?`))) return;
              const body = new FormData();
              body.set("name", name);
              body.set("confirm", "true");
              body.set("file", file);
              setBusy(true);
              try {
                const res = await api<ActionResult>("/api/apps/mysql/databases/import", { method: "POST", body });
                toast("ok", res.message);
                await load();
              } catch (err) {
                toast("err", err instanceof Error ? err.message : "request failed");
              } finally {
                setBusy(false);
              }
            })();
          }}
        />
        <ul className="space-y-2">
          {!ready && <li><Placeholder className="h-10 w-full" /></li>}
          {ready && admin.databases.length === 0 && !admin.error && <li className="text-sm text-[var(--muted)]">No databases</li>}
          {admin.databases.map((item) => (
            <li key={item.name} className="rounded-lg border border-[var(--border)] px-3 py-2">
              <RowActions
                actions={item.system ? [] : [
                  {
                    label: "Export",
                    icon: <Download size={16} />,
                    disabled: busy,
                    onSelect: () => {
                      setBusy(true);
                      download(`/api/apps/mysql/databases/export?name=${encodeURIComponent(item.name)}`, `${item.name}.sql`)
                        .then(() => toast("ok", `Exported ${item.name}.sql`))
                        .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
                        .finally(() => setBusy(false));
                    },
                  },
                  { label: "Import", icon: <Upload size={16} />, disabled: busy, onSelect: () => { setImportTarget(item.name); importFile.current?.click(); } },
                  {
                    label: "Drop",
                    icon: <Trash2 size={16} />,
                    danger: true,
                    disabled: busy,
                    onSelect: async () => {
                      if (!(await confirm(`Drop database ${item.name}?`))) return;
                      await run("/api/apps/mysql/databases", "DELETE", { name: item.name, confirm: true });
                    },
                  },
                ]}
              >
                <span className="flex items-center gap-2">
                  <span className="truncate text-sm">{item.name}</span>
                  {item.system && <Badge value="system" />}
                </span>
              </RowActions>
            </li>
          ))}
        </ul>
        <Modal open={dbOpen} title="Create database" onOpenChange={setDbOpen}>
          <form
            className="grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              run("/api/apps/mysql/databases", "POST", { name: dbName.trim() }).then((ok) => {
                if (!ok) return;
                setDbName("");
                setDbOpen(false);
              });
            }}
          >
            <Field label="Database">
              <TextInput value={dbName} onChange={(event) => setDbName(event.target.value)} placeholder="app_db" autoComplete="off" />
            </Field>
            <div className="flex justify-end">
              <Button icon kind="primary" type="submit" label="Create" disabled={busy || !dbName.trim()}><Plus size={16} /></Button>
            </div>
          </form>
        </Modal>
      </Card>
      <Card
        loading={busy || !ready}
        title="Users"
        action={<Button icon kind="primary" label="Create" disabled={busy} onClick={() => setUserOpen(true)}><Plus size={16} /></Button>}
      >
        <ul className="space-y-2">
          {!ready && <li><Placeholder className="h-10 w-full" /></li>}
          {ready && admin.users.length === 0 && !admin.error && <li className="text-sm text-[var(--muted)]">No users</li>}
          {admin.users.map((item) => {
            const key = `${item.name}@${item.host}`;
            return (
              <li key={key} className="rounded-lg border border-[var(--border)] px-3 py-2">
                <RowActions
                  actions={item.system ? [] : [
                    { label: "Password", icon: <KeyRound size={16} />, disabled: busy, onSelect: () => { setPasswordFor({ name: item.name, host: item.host }); setNextPassword(""); } },
                    {
                      label: "Drop",
                      icon: <Trash2 size={16} />,
                      danger: true,
                      disabled: busy,
                      onSelect: async () => {
                        if (!(await confirm(`Drop user ${key}?`))) return;
                        await run("/api/apps/mysql/users", "DELETE", { name: item.name, host: item.host, confirm: true });
                      },
                    },
                  ]}
                >
                  <span className="flex items-center gap-2 text-sm">
                    <span>{item.name}@{item.host}</span>
                    {item.system && <Badge value="system" />}
                  </span>
                </RowActions>
              </li>
            );
          })}
        </ul>
        <Modal open={userOpen} title="Create user" onOpenChange={setUserOpen}>
          <form
            className="grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              run("/api/apps/mysql/users", "POST", { name: userName.trim(), host, password }).then((ok) => {
                if (!ok) return;
                setUserName("");
                setPassword("");
                setUserOpen(false);
              });
            }}
          >
            <Field label="Username">
              <TextInput value={userName} onChange={(event) => setUserName(event.target.value)} placeholder="app" autoComplete="off" />
            </Field>
            <Field label="Host">
              <SelectBox
                label="Host"
                value={host}
                onValueChange={setHost}
                options={[
                  { value: "localhost", label: "localhost" },
                  { value: "127.0.0.1", label: "127.0.0.1" },
                  { value: "%", label: "any local client (%)" },
                ]}
              />
            </Field>
            <Field label="Password">
              <TextInput type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="new-password" />
            </Field>
            <div className="flex justify-end">
              <Button icon kind="primary" type="submit" label="Create" disabled={busy || !userName.trim() || password.length < 8}><Plus size={16} /></Button>
            </div>
          </form>
        </Modal>
        <Modal
          open={passwordFor !== null}
          title={passwordFor ? `Password for ${passwordFor.name}` : "Password"}
          onOpenChange={(open) => { if (!open) setPasswordFor(null); }}
        >
          <form
            className="grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              if (!passwordFor) return;
              run("/api/apps/mysql/users/password", "PUT", { name: passwordFor.name, host: passwordFor.host, password: nextPassword }).then((ok) => {
                if (!ok) return;
                setNextPassword("");
                setPasswordFor(null);
              });
            }}
          >
            <Field label="New password">
              <TextInput type="password" value={nextPassword} autoComplete="new-password" onChange={(event) => setNextPassword(event.target.value)} />
            </Field>
            <div className="flex justify-end">
              <Button icon kind="primary" type="submit" label="Save" disabled={busy || nextPassword.length < 8}><KeyRound size={16} /></Button>
            </div>
          </form>
        </Modal>
      </Card>
      <Card
        loading={busy || !ready}
        title="Permissions"
        action={<Button icon kind="primary" label="Grant" disabled={busy} onClick={openGrant}><Shield size={16} /></Button>}
      >
        <ul className="space-y-2">
          {!ready && <li><Placeholder className="h-10 w-full" /></li>}
          {ready && admin.grants.length === 0 && !admin.error && <li className="text-sm text-[var(--muted)]">No privileges</li>}
          {admin.grants.map((grant) => (
            <li key={`${grant.user}@${grant.host}/${grant.database}`} className="rounded-lg border border-[var(--border)] px-3 py-2">
              <RowActions
                actions={grant.system ? [] : [
                  { label: "Edit", icon: <Shield size={16} />, disabled: busy, onSelect: () => editGrant(grant) },
                  {
                    label: "Revoke",
                    icon: <Trash2 size={16} />,
                    danger: true,
                    disabled: busy,
                    onSelect: async () => {
                      if (!(await confirm(`Revoke privileges of ${grant.user}@${grant.host} on ${grant.database}?`))) return;
                      await run("/api/apps/mysql/grants", "DELETE", { name: grant.user, host: grant.host, database: grant.database, confirm: true });
                    },
                  },
                ]}
              >
                <span className="min-w-0 text-sm">
                  <span className="font-medium">{grant.user}@{grant.host}</span>
                  <span className="text-[var(--muted)]"> on {grant.database}</span>
                  <span className="mt-0.5 block text-xs text-[var(--muted)]">{grant.privileges.join(", ")}</span>
                </span>
              </RowActions>
            </li>
          ))}
        </ul>
        <Modal open={grantOpen} wide title="Grant privileges" onOpenChange={setGrantOpen}>
          <form
            className="space-y-3"
            onSubmit={(event) => {
              event.preventDefault();
              run("/api/apps/mysql/grants", "POST", { name: grantUser, host: grantHost, database: grantDb, privileges: selected }).then((ok) => {
                if (ok) setGrantOpen(false);
              });
            }}
          >
            <div className="grid items-end gap-2 md:grid-cols-3">
              <Field label="User">
                <TextInput list="mysql-users" value={grantUser} onChange={(event) => setGrantUser(event.target.value)} placeholder="app" autoComplete="off" />
                <datalist id="mysql-users">
                  {accounts.map((item) => <option key={`${item.name}@${item.host}`} value={item.name} />)}
                </datalist>
              </Field>
              <Field label="Host">
                <SelectBox
                  label="Host"
                  value={grantHost}
                  onValueChange={setGrantHost}
                  options={[
                    { value: "localhost", label: "localhost" },
                    { value: "127.0.0.1", label: "127.0.0.1" },
                    { value: "%", label: "any local client (%)" },
                  ]}
                />
              </Field>
              <Field label="Database">
                <TextInput list="mysql-databases" value={grantDb} onChange={(event) => setGrantDb(event.target.value)} placeholder="app_db" autoComplete="off" />
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
            <div className="flex justify-end">
              <Button icon kind="primary" type="submit" label="Save" disabled={busy || !grantUser || !grantDb || selected.length === 0}><Shield size={16} /></Button>
            </div>
          </form>
        </Modal>
      </Card>
    </div>
  );
}
