import { useEffect, useState } from "react";
import { Play, Power, PowerOff, RotateCw, Square } from "lucide-react";
import { api } from "../api";
import { Badge, Card, Placeholder, RowActions, useConfirm, useToast } from "../components/ui";

type ServiceStatus = {
  unit: string;
  active: string;
  enabled: string;
  exists: boolean;
};

export function ServicesPage() {
  const [items, setItems] = useState<ServiceStatus[]>([]);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState("");
  const toast = useToast();
  const confirm = useConfirm();

  async function load() {
    const res = await api<{ services: ServiceStatus[] }>("/api/services");
    setItems(res.services || []);
  }

  useEffect(() => {
    load()
      .catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))
      .finally(() => setReady(true));
  }, []);

  async function act(unit: string, action: string) {
    const destructive = action === "stop" || action === "disable";
    if (destructive && !(await confirm(`${action} ${unit}?`))) return;
    setBusy(unit);
    try {
      const res = await api<{ message?: string }>("/api/services/action", {
        method: "POST",
        body: JSON.stringify({ unit, action, confirm: destructive }),
      });
      toast("ok", res.message || "completed");
      await load();
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "request failed");
    } finally {
      setBusy("");
    }
  }

  const apache = items.find((item) => item.unit === "apache2" || item.unit === "httpd");
  const nginx = items.find((item) => item.unit === "nginx");
  const blocked = apache?.active === "active" && nginx?.active !== "active";

  return (
    <div className="space-y-4">
      <Card loading={!ready || busy !== ""} title="Services" action={<Badge value={ready ? String(items.length) : "…"} />}>
        {blocked && (
          <p className="mb-4 text-sm text-[var(--muted)]">
            {apache?.unit} is running and Nginx is not. Stop {apache?.unit} so Nginx can use port 80.
          </p>
        )}
        {!ready && (
          <div className="space-y-3">
            <Placeholder className="h-10 w-full" />
            <Placeholder className="h-10 w-full" />
          </div>
        )}
        {ready && items.length === 0 && <p className="text-sm text-[var(--muted)]">No managed services were found on this server.</p>}
        <ul className="space-y-2">
          {items.map((item) => (
            <li key={item.unit} className="rounded-lg border border-[var(--border)] px-3 py-2">
              <RowActions
                actions={[
                  { label: "Start", icon: <Play size={16} />, disabled: busy !== "", onSelect: () => act(item.unit, "start") },
                  { label: "Stop", icon: <Square size={16} />, disabled: busy !== "", onSelect: () => act(item.unit, "stop") },
                  { label: "Restart", icon: <RotateCw size={16} />, disabled: busy !== "", onSelect: () => act(item.unit, "restart") },
                  { label: "Enable", icon: <Power size={16} />, disabled: busy !== "", onSelect: () => act(item.unit, "enable") },
                  { label: "Disable", icon: <PowerOff size={16} />, danger: true, disabled: busy !== "", onSelect: () => act(item.unit, "disable") },
                ]}
              >
                <span className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium">{item.unit}</span>
                  <Badge value={item.active || "unknown"} />
                  <span className="text-xs text-[var(--muted)]">{item.enabled}</span>
                </span>
              </RowActions>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
