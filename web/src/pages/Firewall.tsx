import { useEffect, useRef, useState } from "react";
import { Download, Pencil, Plus, Power, PowerOff, Save, Trash2, X } from "lucide-react";
import { api } from "../api";
import { Badge, Button, Card, Field, Modal, Placeholder, RowActions, SelectBox, useConfirm, useToast, TextInput } from "../components/ui";
import type { ActionResult } from "../types";

type Rule = {
  number: number;
  to: string;
  action: string;
  from: string;
  comment: string;
  v6: boolean;
};

type FirewallStatus = {
  installed: boolean;
  active: boolean;
  logging: string;
  default_incoming: string;
  default_outgoing: string;
  ssh_port: number;
  ipv6: boolean;
  rules: Rule[];
};

const empty: FirewallStatus = { installed: false, active: false, logging: "", default_incoming: "", default_outgoing: "", ssh_port: 22, ipv6: true, rules: [] };

function keepsSSH(rule: Rule, port: number) {
  if (rule.v6) return false;
  const action = rule.action.toUpperCase();
  if (!action.startsWith("ALLOW") || action.includes("OUT")) return false;
  const from = rule.from.trim().toLowerCase();
  if (from !== "anywhere" && from !== "any" && from !== "") return false;
  const [raw, proto] = rule.to.split("/");
  if (Number(raw) !== port) return false;
  return !proto || proto === "tcp" || proto === "any";
}

export function FirewallPage() {
  const [status, setStatus] = useState<FirewallStatus>(empty);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [port, setPort] = useState("");
  const [action, setAction] = useState("allow");
  const [direction, setDirection] = useState("in");
  const [protocol, setProtocol] = useState("tcp");
  const [from, setFrom] = useState("any");
  const [comment, setComment] = useState("");
  const [incoming, setIncoming] = useState("deny");
  const [outgoing, setOutgoing] = useState("allow");
  const [logging, setLogging] = useState("low");
  const [ipv6, setIpv6] = useState("yes");
  const [editing, setEditing] = useState<number | null>(null);
  const [ruleOpen, setRuleOpen] = useState(false);
  const ruleOpenRef = useRef(false);
  const toast = useToast();
  const confirm = useConfirm();

  async function load() {
    const next = await api<FirewallStatus>("/api/firewall");
    setStatus({ ...empty, ...next, rules: next.rules || [] });
    setIncoming(next.default_incoming || "deny");
    setOutgoing(next.default_outgoing || "allow");
    setLogging(next.logging || "low");
    setIpv6(next.ipv6 ? "yes" : "no");
  }

  useEffect(() => {
    load()
      .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
      .finally(() => setReady(true));
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

  function resetRule() {
    setEditing(null);
    setPort("");
    setAction("allow");
    setDirection("in");
    setProtocol("tcp");
    setFrom("any");
    setComment("");
  }

  function setRuleDialog(open: boolean) {
    ruleOpenRef.current = open;
    setRuleOpen(open);
    if (!open) window.setTimeout(() => { if (!ruleOpenRef.current) resetRule(); }, 200);
  }

  function openAdd() {
    resetRule();
    setRuleDialog(true);
  }

  function editRule(rule: Rule) {
    const actionText = rule.action.toUpperCase();
    const [raw, proto] = rule.to.split("/");
    if (!Number.isInteger(Number(raw)) || Number(raw) < 1) {
      toast("err", "This rule cannot be edited");
      return;
    }
    const from = rule.from.trim().toLowerCase();
    setEditing(rule.number);
    setAction(actionText.startsWith("DENY") ? "deny" : actionText.startsWith("REJECT") ? "reject" : actionText.startsWith("LIMIT") ? "limit" : "allow");
    setDirection(actionText.includes("OUT") ? "out" : "in");
    setPort(raw);
    setProtocol(proto || "any");
    setFrom(from === "anywhere" || from === "any" || from === "" ? "any" : rule.from);
    setComment(rule.comment);
    setRuleDialog(true);
  }

  async function allow(nextPort: number, nextComment: string) {
    await run("/api/firewall/rules", "POST", { port: nextPort, protocol: "tcp", from: "any", comment: nextComment });
  }

  return (
    <div className="space-y-4">
      <Card
        loading={!ready || busy}
        title="Firewall"
        action={<Badge value={!ready ? "…" : !status.installed ? "not installed" : status.active ? "active" : "inactive"} />}
      >
        <p className="text-sm text-[var(--muted)]">
          UFW filters incoming traffic. Enabling it denies new connections except the rules below. SSH on port {status.ssh_port || 22} stays allowed from anywhere, and that rule cannot be deleted. MySQL port 3306 is not opened.
        </p>
        {ready && (
          <div className="mt-4 flex flex-wrap gap-2">
            {!status.installed && <Button icon kind="primary" label="Install" disabled={busy} onClick={() => run("/api/firewall/install", "POST", {})}><Download size={16} /></Button>}
            {status.installed && !status.active && (
              <Button icon kind="primary" label="Enable" disabled={busy} onClick={async () => {
                if (!(await confirm(`Enable the firewall? Incoming traffic is denied except the rules below. SSH on port ${status.ssh_port || 22} stays allowed from anywhere.`))) return;
                await run("/api/firewall/enable", "POST", { confirm: true });
              }}><Power size={16} /></Button>
            )}
            {status.installed && status.active && (
              <Button icon kind="danger" label="Disable" disabled={busy} onClick={async () => {
                if (!(await confirm("Disable the firewall? Incoming traffic will no longer be filtered."))) return;
                await run("/api/firewall/disable", "POST", { confirm: true });
              }}><PowerOff size={16} /></Button>
            )}
          </div>
        )}
        {ready && status.installed && (
          <p className="mt-3 text-sm text-[var(--muted)]">
            Incoming {status.default_incoming || "—"}, outgoing {status.default_outgoing || "—"}, logging {status.logging || "—"}, IPv6 {status.ipv6 ? "on" : "off"}.
          </p>
        )}
      </Card>
      {ready && status.installed && (
        <Card loading={busy} title="Configuration">
          <p className="mb-4 text-sm text-[var(--muted)]">
            Policies apply to traffic that does not match a rule. SSH on port {status.ssh_port || 22} stays allowed when incoming traffic is denied or rejected.
          </p>
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              const restrictive = status.active && (incoming === "deny" || incoming === "reject" || outgoing === "deny" || outgoing === "reject");
              void (async () => {
                if (restrictive && !(await confirm(`Save this firewall configuration? Restrictive policies apply immediately. SSH on port ${status.ssh_port || 22} stays allowed.`))) return;
                await run("/api/firewall/config", "POST", { incoming, outgoing, logging, ipv6: ipv6 === "yes" });
              })();
            }}
          >
            <Field label="Incoming" className="w-36">
              <SelectBox label="Incoming" value={incoming} onValueChange={setIncoming} options={[{ value: "deny", label: "Deny" }, { value: "reject", label: "Reject" }, { value: "allow", label: "Allow" }]} />
            </Field>
            <Field label="Outgoing" className="w-36">
              <SelectBox label="Outgoing" value={outgoing} onValueChange={setOutgoing} options={[{ value: "allow", label: "Allow" }, { value: "deny", label: "Deny" }, { value: "reject", label: "Reject" }]} />
            </Field>
            <Field label="Logging" className="w-36">
              <SelectBox label="Logging" value={logging} onValueChange={setLogging} options={[{ value: "off", label: "Off" }, { value: "low", label: "Low" }, { value: "medium", label: "Medium" }, { value: "high", label: "High" }, { value: "full", label: "Full" }]} />
            </Field>
            <Field label="IPv6" className="w-32">
              <SelectBox label="IPv6" value={ipv6} onValueChange={setIpv6} options={[{ value: "yes", label: "On" }, { value: "no", label: "Off" }]} />
            </Field>
            <Button icon kind="primary" type="submit" label="Save" disabled={busy}><Save size={16} /></Button>
          </form>
        </Card>
      )}
      <Card
        loading={!ready || busy}
        title="Rules"
        action={ready && status.installed ? (
          <span className="flex flex-wrap justify-end gap-2">
            <Button icon label="HTTP" disabled={busy} onClick={() => allow(80, "HTTP")}><Plus size={16} /></Button>
            <Button icon label="HTTPS" disabled={busy} onClick={() => allow(443, "HTTPS")}><Plus size={16} /></Button>
            <Button icon kind="primary" label="Add rule" disabled={busy} onClick={openAdd}><Plus size={16} /></Button>
          </span>
        ) : undefined}
      >
        {!ready && <Placeholder className="h-10 w-full" />}
        {ready && !status.installed && <p className="text-sm text-[var(--muted)]">Install the firewall to manage rules.</p>}
        {ready && status.installed && status.rules.length === 0 && <p className="text-sm text-[var(--muted)]">No rules yet.</p>}
        <ul className="space-y-2">
          {status.rules.map((rule) => {
            const protectedSSH = keepsSSH(rule, status.ssh_port || 22) && status.rules.filter((item) => keepsSSH(item, status.ssh_port || 22)).length === 1;
            return (
            <li key={rule.number} className="rounded-lg border border-[var(--border)] px-3 py-2">
              <RowActions
                actions={[
                  ...(!rule.v6 ? [{ label: "Edit", icon: <Pencil size={16} />, disabled: busy, onSelect: () => editRule(rule) }] : []),
                  {
                    label: "Delete",
                    icon: <Trash2 size={16} />,
                    danger: true,
                    disabled: busy || protectedSSH,
                    onSelect: async () => {
                      if (!(await confirm(`Delete firewall rule ${rule.number} for ${rule.to}?`))) return;
                      await run("/api/firewall/rules", "DELETE", { number: rule.number, confirm: true });
                    },
                  },
                ]}
              >
                <span className="min-w-0 text-sm">
                  <span className="font-medium">{rule.number}. {rule.to}</span>
                  <span className="text-[var(--muted)]"> {rule.action} from {rule.from}{rule.v6 ? " (IPv6)" : ""}</span>
                  {rule.comment && <span className="mt-0.5 block text-xs text-[var(--muted)]">{rule.comment}</span>}
                  {protectedSSH && <span className="mt-0.5 block text-xs text-[var(--muted)]">Kept so SSH stays reachable</span>}
                </span>
              </RowActions>
            </li>
            );
          })}
        </ul>
        <Modal
          open={ruleOpen}
          title={editing !== null ? `Edit rule ${editing}` : "Add rule"}
          description="SSH on the stored port stays allowed from anywhere."
          onOpenChange={setRuleDialog}
        >
          <form
            className="grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              const next = Number(port);
              if (!Number.isInteger(next) || next < 1 || next > 65535) {
                toast("err", "Enter a port from 1 to 65535");
                return;
              }
              void (async () => {
                const risky = action === "deny" || action === "reject" || action === "limit";
                if (editing !== null) {
                  if (!(await confirm(`Update firewall rule ${editing}?`))) return;
                  if (await run("/api/firewall/rules", "PUT", { number: editing, action, direction, port: next, protocol, from: from.trim() || "any", comment: comment.trim() })) setRuleDialog(false);
                  return;
                }
                if (risky && !(await confirm(`Add a ${action} rule for port ${next}?`))) return;
                if (await run("/api/firewall/rules", "POST", { action, direction, port: next, protocol, from: from.trim() || "any", comment: comment.trim() })) setRuleDialog(false);
              })();
            }}
          >
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Action">
                <SelectBox label="Action" value={action} onValueChange={setAction} options={[{ value: "allow", label: "Allow" }, { value: "deny", label: "Deny" }, { value: "reject", label: "Reject" }, { value: "limit", label: "Limit" }]} />
              </Field>
              <Field label="Direction">
                <SelectBox label="Direction" value={direction} onValueChange={setDirection} options={[{ value: "in", label: "Incoming" }, { value: "out", label: "Outgoing" }]} />
              </Field>
              <Field label="Port">
                <TextInput value={port} onChange={(event) => setPort(event.target.value)} placeholder="8088" inputMode="numeric" autoComplete="off" />
              </Field>
              <Field label="Protocol">
                <SelectBox label="Protocol" value={protocol} onValueChange={setProtocol} options={[{ value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }, { value: "any", label: "Any" }]} />
              </Field>
            </div>
            <Field label="From">
              <TextInput value={from} onChange={(event) => setFrom(event.target.value)} placeholder="any" autoComplete="off" />
            </Field>
            <Field label="Comment">
              <TextInput value={comment} onChange={(event) => setComment(event.target.value)} placeholder="HTTP" autoComplete="off" />
            </Field>
            <div className="flex justify-end gap-2">
              <Button icon label="Cancel" disabled={busy} onClick={() => setRuleDialog(false)}><X size={16} /></Button>
              <Button icon kind="primary" type="submit" label={editing !== null ? "Update" : "Save"} disabled={busy || !port.trim()}>{editing !== null ? <Save size={16} /> : <Plus size={16} />}</Button>
            </div>
          </form>
        </Modal>
      </Card>
    </div>
  );
}
