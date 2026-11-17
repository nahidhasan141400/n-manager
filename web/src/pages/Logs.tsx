import { useEffect, useState } from "react";
import { ListFilter, RefreshCw, Trash2, TriangleAlert } from "lucide-react";
import { api } from "../api";
import { Badge, Button, Card, Placeholder, TabBar, useConfirm, useToast } from "../components/ui";
import type { DebugEntry } from "../types";

export function LogsPage() {
  const [entries, setEntries] = useState<DebugEntry[]>([]);
  const [ready, setReady] = useState(false);
  const [level, setLevel] = useState<"" | "error">("");
  const toast = useToast();
  const confirm = useConfirm();

  async function load(next = level) {
    const query = next ? `?level=${next}` : "";
    try {
      const res = await api<{ entries: DebugEntry[] }>(`/api/logs${query}`);
      setEntries(res.entries || []);
    } finally {
      setReady(true);
    }
  }

  useEffect(() => {
    load().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"));
    const timer = window.setInterval(() => {
      load().catch(() => undefined);
    }, 3000);
    return () => window.clearInterval(timer);
  }, [level]);

  async function clear() {
    if (!(await confirm("Clear the debug log?"))) return;
    try {
      await api("/api/logs", { method: "DELETE" });
      setEntries([]);
      toast("ok", "Log cleared.");
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "Unable to clear the log");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <TabBar
          value={level || "all"}
          onValueChange={(value) => setLevel(value === "error" ? "error" : "")}
          items={[
            { value: "all", label: "All", icon: <ListFilter size={16} /> },
            { value: "error", label: "Errors", icon: <TriangleAlert size={16} /> },
          ]}
        />
        <Button icon label="Refresh" onClick={() => load().catch((err: unknown) => toast("err", err instanceof Error ? err.message : "request failed"))}><RefreshCw size={16} /></Button>
        <Button icon kind="danger" label="Clear" onClick={() => clear().catch(() => undefined)}><Trash2 size={16} /></Button>
      </div>
      <Card title="Debug log" loading={!ready}>
        <p className="mb-3 text-sm text-[var(--muted)]">SSH connections, failed commands on the VPS, and failed panel requests. Passwords are left out. The same line is kept once every 30 seconds.</p>
        {!ready ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }, (_, index) => <Placeholder key={index} className="h-8 w-full" />)}
          </div>
        ) : entries.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No log entries yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.id} className="border-t border-[var(--border)] align-top">
                    <td className="whitespace-nowrap py-2 pr-3 text-[var(--muted)]">{new Date(entry.created_at * 1000).toLocaleString()}</td>
                    <td className="py-2 pr-3"><Badge value={entry.level} /></td>
                    <td className="py-2 pr-3">{entry.server || "panel"}</td>
                    <td className="py-2 pr-3 text-[var(--muted)]">{entry.kind}</td>
                    <td className="py-2">
                      <div className="break-all font-mono text-xs">{entry.message}</div>
                      {entry.detail && <div className="mt-1 break-all font-mono text-xs text-[var(--muted)]">{entry.detail}</div>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
