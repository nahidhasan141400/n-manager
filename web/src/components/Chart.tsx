import { useEffect, useRef } from "react";
import * as echarts from "echarts";

export function Chart({ option }: { option: echarts.EChartsOption }) {
  const el = useRef<HTMLDivElement>(null);
  const chart = useRef<echarts.ECharts | null>(null);

  useEffect(() => {
    if (!el.current) return;
    const instance = echarts.init(el.current);
    chart.current = instance;
    const observer = new ResizeObserver(() => instance.resize());
    observer.observe(el.current);
    return () => {
      observer.disconnect();
      instance.dispose();
      chart.current = null;
    };
  }, []);

  useEffect(() => {
    chart.current?.setOption(option, true);
  }, [option]);

  return <div ref={el} className="h-56 w-full" />;
}

export function chartTheme() {
  const style = getComputedStyle(document.documentElement);
  const pick = (name: string) => style.getPropertyValue(name).trim();
  return {
    text: pick("--muted") || "#9198a1",
    line: pick("--border") || "#30363d",
    accent: pick("--link") || "#58a6ff",
    green: pick("--accent") || "#238636",
    warning: pick("--warning") || "#d29922",
  };
}
