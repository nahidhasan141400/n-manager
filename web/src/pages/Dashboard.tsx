import { useCallback, useEffect, useState, type ReactNode } from "react";
import type { EChartsOption } from "echarts";
import { ArrowDown, ArrowUp, Clock, Cpu, Globe, HardDrive, MemoryStick, RefreshCw, Server } from "lucide-react";
import { api } from "../api";
import { Chart, chartTheme } from "../components/Chart";
import { Button, Card, Meter } from "../components/ui";
import { formatBytes, formatTime, formatUptime, pct } from "../format";
import type { Metrics, SystemInfo } from "../types";

export function Dashboard({ theme }: { theme: string }) {
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const [nextInfo, nextMetrics] = await Promise.all([
        api<SystemInfo>("/api/system/info"),
        api<Metrics>("/api/system/metrics"),
      ]);
      setInfo(nextInfo);
      setMetrics(nextMetrics);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load metrics");
    }
  }, []);

  useEffect(() => {
    load();
    const seconds = Math.max(1, metrics?.refresh_interval || 5);
    const timer = window.setInterval(load, seconds * 1000);
    return () => window.clearInterval(timer);
  }, [load, metrics?.refresh_interval]);

  const colors = chartTheme();
  const history = metrics?.history ?? [];
  const root = metrics?.current.disks.find((disk) => disk.mount === "/") ?? metrics?.current.disks[0];

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button icon label="Refresh" title={metrics ? formatTime(metrics.updated_at) : "Refresh"} onClick={load}><RefreshCw size={16} /></Button>
      </div>
      {error && <p className="text-sm text-[var(--danger)]">{error}</p>}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <Stat icon={<Cpu size={16} />} title="CPU" value={pct(metrics?.current.cpu_percent ?? 0)} detail={`${metrics?.current.cpu_cores ?? info?.cpu_cores ?? 0} · ${metrics?.current.load1.toFixed(2) ?? "0"}`} meter={metrics?.current.cpu_percent} />
        <Stat icon={<MemoryStick size={16} />} title="RAM" value={pct(metrics?.current.mem_percent ?? 0)} detail={`${formatBytes(metrics?.current.mem_used ?? 0)} / ${formatBytes(metrics?.current.mem_total ?? 0)}`} meter={metrics?.current.mem_percent} />
        <Stat icon={<HardDrive size={16} />} title="Disk" value={pct(root?.percent ?? 0)} detail={root ? `${formatBytes(root.used)} / ${formatBytes(root.total)}` : "—"} meter={root?.percent} />
        <Stat icon={<Globe size={16} />} title="Net" value={formatBytes(metrics?.current.net_rx_bps ?? 0) + "/s"} detail={`${formatBytes(metrics?.current.net_tx_bps ?? 0)}/s`} />
      </div>
      {info && (
        <Card>
          <dl className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Info icon={<Server size={14} />} label="Host" value={info.hostname} />
            <Info icon={<Cpu size={14} />} label="CPU" value={info.cpu_model || "—"} />
            <Info icon={<Clock size={14} />} label="Up" value={formatUptime(info.uptime_seconds)} />
            <Info icon={<Globe size={14} />} label="OS" value={info.os || info.distro} />
            <Info label="Kernel" value={info.kernel} />
            <Info label="LAN" value={info.private_ips[0] || "—"} />
            <Info label="WAN" value={info.public_ip || "—"} />
          </dl>
        </Card>
      )}
      <div className="grid gap-3 xl:grid-cols-2" data-theme={theme}>
        <Card title="CPU"><Chart option={lineOption("CPU %", history.map((p) => [p.t, p.cpu]), colors.accent, colors)} /></Card>
        <Card title="RAM"><Chart option={lineOption("Memory %", history.map((p) => [p.t, p.mem]), colors.green, colors)} /></Card>
        <Card title="Network"><Chart option={networkOption(history, colors)} /></Card>
        <Card title="Disk"><Chart option={diskOption(metrics?.current.disks ?? [], colors)} /></Card>
        <Card title="Trends"><Chart option={trendOption(history, colors)} /></Card>
      </div>
      <Card title="Interfaces">
        <table className="w-full text-left text-sm">
          <thead className="text-[var(--muted)]">
            <tr>
              <th className="py-2" />
              <th><ArrowDown size={14} aria-label="Received" /></th>
              <th><ArrowUp size={14} aria-label="Sent" /></th>
              <th><ArrowDown size={14} aria-label="Download" /></th>
              <th><ArrowUp size={14} aria-label="Upload" /></th>
            </tr>
          </thead>
          <tbody>
            {(metrics?.current.net ?? []).map((iface) => (
              <tr key={iface.name} className="border-t border-[var(--border)]">
                <td className="py-2">{iface.name}</td>
                <td>{formatBytes(iface.rx)}</td>
                <td>{formatBytes(iface.tx)}</td>
                <td>{formatBytes(iface.rx_bps)}/s</td>
                <td>{formatBytes(iface.tx_bps)}/s</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}

function Stat({ icon, title, value, detail, meter }: { icon: ReactNode; title: string; value: string; detail: string; meter?: number }) {
  return (
    <section className="rounded-md border border-[var(--border)] bg-[var(--card)] p-4">
      <div className="flex items-center gap-1.5 text-sm text-[var(--muted)]">{icon}{title}</div>
      <div className="my-1 text-xl font-semibold">{value}</div>
      <div className="mb-3 text-xs text-[var(--muted)]">{detail}</div>
      {meter !== undefined && <Meter value={meter} />}
    </section>
  );
}

function Info({ icon, label, value }: { icon?: ReactNode; label: string; value: string }) {
  return (
    <div title={label}>
      <dt className="flex items-center gap-1 text-xs text-[var(--muted)]">{icon}{label}</dt>
      <dd className="truncate">{value || "—"}</dd>
    </div>
  );
}

function base(colors: ReturnType<typeof chartTheme>, legend = false): EChartsOption {
  return {
    animation: false,
    grid: { left: 8, right: 12, top: legend ? 32 : 12, bottom: 4, containLabel: true },
    textStyle: { color: colors.text, fontSize: 12 },
    tooltip: { trigger: "axis" },
  };
}

function legend(colors: ReturnType<typeof chartTheme>): EChartsOption["legend"] {
  return {
    top: 0,
    left: "center",
    icon: "circle",
    itemWidth: 8,
    itemHeight: 8,
    itemGap: 16,
    textStyle: { color: colors.text, fontSize: 12 },
  };
}

function lineOption(name: string, data: number[][], color: string, colors: ReturnType<typeof chartTheme>): EChartsOption {
  return {
    ...base(colors),
    xAxis: { type: "time", axisLabel: { color: colors.text, hideOverlap: true }, axisLine: { lineStyle: { color: colors.line } } },
    yAxis: { type: "value", max: 100, axisLabel: { color: colors.text, formatter: "{value}%" }, splitLine: { lineStyle: { color: colors.line } } },
    series: [{ name, type: "line", showSymbol: false, data, lineStyle: { color }, areaStyle: { opacity: 0.12 } }],
  };
}

function networkOption(history: Metrics["history"], colors: ReturnType<typeof chartTheme>): EChartsOption {
  const rate = (value: unknown) => `${formatBytes(Number(value))}/s`;
  return {
    ...base(colors, true),
    legend: legend(colors),
    tooltip: { trigger: "axis", valueFormatter: rate },
    xAxis: { type: "time", axisLabel: { color: colors.text, hideOverlap: true } },
    yAxis: { type: "value", axisLabel: { color: colors.text, formatter: (value: number) => formatBytes(value) }, splitLine: { lineStyle: { color: colors.line } } },
    series: [
      { name: "Download", type: "line", showSymbol: false, data: history.map((p) => [p.t, p.rx]), itemStyle: { color: colors.accent }, lineStyle: { color: colors.accent } },
      { name: "Upload", type: "line", showSymbol: false, data: history.map((p) => [p.t, p.tx]), itemStyle: { color: colors.warning }, lineStyle: { color: colors.warning } },
    ],
  };
}

function diskOption(disks: Metrics["current"]["disks"], colors: ReturnType<typeof chartTheme>): EChartsOption {
  return {
    ...base(colors),
    tooltip: { trigger: "axis", valueFormatter: (value) => `${value}%` },
    xAxis: { type: "category", data: disks.map((d) => d.mount), axisLabel: { color: colors.text, interval: 0, hideOverlap: true, width: 72, overflow: "truncate" } },
    yAxis: { type: "value", max: 100, axisLabel: { color: colors.text, formatter: "{value}%" }, splitLine: { lineStyle: { color: colors.line } } },
    series: [{ type: "bar", barMaxWidth: 28, data: disks.map((d) => Number(d.percent.toFixed(1))), itemStyle: { color: colors.green, borderRadius: [3, 3, 0, 0] } }],
  };
}

function trendOption(history: Metrics["history"], colors: ReturnType<typeof chartTheme>): EChartsOption {
  return {
    ...base(colors, true),
    legend: legend(colors),
    tooltip: { trigger: "axis", valueFormatter: (value) => `${Number(value).toFixed(1)}%` },
    xAxis: { type: "time", axisLabel: { color: colors.text, hideOverlap: true } },
    yAxis: { type: "value", max: 100, axisLabel: { color: colors.text, formatter: "{value}%" }, splitLine: { lineStyle: { color: colors.line } } },
    series: [
      { name: "CPU", type: "line", showSymbol: false, data: history.map((p) => [p.t, p.cpu]), itemStyle: { color: colors.accent }, lineStyle: { color: colors.accent } },
      { name: "Memory", type: "line", showSymbol: false, data: history.map((p) => [p.t, p.mem]), itemStyle: { color: colors.green }, lineStyle: { color: colors.green } },
    ],
  };
}
