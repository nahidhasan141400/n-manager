import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useEffect, useState } from "react";
import { Beaker, Cog, Copy, FolderInput, Globe, LayoutDashboard, Moon, ScrollText, Server, Settings, Shield, Sun } from "lucide-react";
import { api, serverId, setServer } from "../api";
import logo from "../assets/logo.png";
import { AccountMenu, Button, Card, SelectBox, useToast } from "./ui";

type ManagedServer = { id: string; name: string; host: string; trusted: boolean };

async function copyHost(host: string, toast: (kind: "ok" | "err", text: string) => void) {
  try {
    await Promise.race([
      navigator.clipboard.writeText(host),
      new Promise<never>((_, reject) => window.setTimeout(() => reject(new Error("timeout")), 700)),
    ]);
    toast("ok", `Copied ${host}`);
    return;
  } catch {
    const field = document.createElement("textarea");
    field.value = host;
    field.setAttribute("readonly", "");
    field.style.position = "fixed";
    field.style.left = "-9999px";
    document.body.appendChild(field);
    field.select();
    const copied = document.execCommand("copy");
    field.remove();
    toast(copied ? "ok" : "err", copied ? `Copied ${host}` : "Could not copy the IP");
  }
}

const links = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/applications", label: "Apps", icon: Server },
  { to: "/nginx", label: "Nginx", icon: Globe },
  { to: "/services", label: "Services", icon: Cog },
  { to: "/firewall", label: "Firewall", icon: Shield },
  { to: "/ftp", label: "FTP", icon: FolderInput },
  { to: "/logs", label: "Logs", icon: ScrollText },
  { to: "/settings", label: "Settings", icon: Settings },
];

export function Layout({
  theme,
  onTheme,
  onLogout,
  dryRun,
}: {
  theme: "dark" | "light";
  onTheme: () => void;
  onLogout: () => void;
  dryRun: boolean;
}) {
  const [online, setOnline] = useState(false);
  const [servers, setServers] = useState<ManagedServer[]>([]);
  const [current, setCurrent] = useState(serverId);
  const location = useLocation();
  const toast = useToast();
  const host = servers.find((item) => item.id === current)?.host || "";

  useEffect(() => {
    function refresh() {
      setCurrent(serverId());
    }
    window.addEventListener("nmanager:server", refresh);
    return () => window.removeEventListener("nmanager:server", refresh);
  }, []);

  useEffect(() => {
    let stop = false;
    function refresh() {
      api<{ servers: ManagedServer[] }>("/api/servers")
        .then((res) => {
          if (stop) return;
          const next = res.servers || [];
          setServers(next);
          if (serverId() && !next.some((item) => item.id === serverId())) setServer("");
        })
        .catch(() => undefined);
    }
    refresh();
    window.addEventListener("nmanager:servers", refresh);
    return () => {
      stop = true;
      window.removeEventListener("nmanager:servers", refresh);
    };
  }, []);

  useEffect(() => {
    let stop = false;
    async function ping() {
      if (!current) {
        if (!stop) setOnline(false);
        return;
      }
      try {
        await api("/api/system/info");
        if (!stop) setOnline(true);
      } catch {
        if (!stop) setOnline(false);
      }
    }
    ping();
    const timer = window.setInterval(ping, 15000);
    return () => {
      stop = true;
      window.clearInterval(timer);
    };
  }, [current]);

  return (
    <div className="flex h-dvh overflow-hidden">
      <aside className="nav-shell flex h-full w-64 min-w-64 shrink-0 flex-col overflow-y-auto">
        <div className="px-4 pb-4 pt-5">
          <img src={logo} alt="N Manager" className="h-9 w-auto max-w-full" />
        </div>
        <div className="px-3 pb-3">
          <label className="block text-xs text-white/80">
            Server
            <SelectBox
              label="Server"
              tone="sidebar"
              value={current}
              onValueChange={setServer}
              placeholder="Select a server"
              options={[{ value: "", label: "Select a server" }, ...servers.map((item) => ({ value: item.id, label: item.name }))]}
            />
          </label>
        </div>
        <nav className="flex flex-1 flex-col gap-1 px-3">
          {links.map((link) => {
            const Icon = link.icon;
            return (
              <NavLink
                key={link.to}
                to={link.to}
                end={link.to === "/"}
                className={({ isActive }) =>
                  `flex h-10 items-center gap-2.5 rounded-lg px-2 text-sm leading-none text-white [&_svg]:block [&_svg]:shrink-0 ${isActive ? "bg-[var(--sidebar-active)]" : "hover:bg-[var(--sidebar-hover)]"}`
                }
              >
                {({ isActive }) => (
                  <>
                    <span className={`grid h-7 w-7 place-items-center rounded-md ${isActive ? "bg-white/15" : "bg-white/10"}`}>
                      <Icon size={16} />
                    </span>
                    {link.label}
                  </>
                )}
              </NavLink>
            );
          })}
        </nav>
        <div className="m-3 rounded-xl bg-white/10 px-3 py-2.5 text-xs">
          <div className="flex items-center gap-2">
            <span className={`h-2 w-2 rounded-full ${!current ? "bg-white/40" : online ? "bg-[#3fb950]" : "bg-[var(--danger)]"}`} />
            {!current ? "No server" : online ? "Online" : "Offline"}
          </div>
          {dryRun && (
            <div className="mt-2 flex items-center gap-2 text-[var(--warning)]">
              <Beaker size={14} />
              Dry run
            </div>
          )}
        </div>
      </aside>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <header className="relative z-20 flex h-12 shrink-0 items-center gap-2 border-b border-[var(--border)] py-0 pl-4 pr-5">
          {host && (
            <Button icon label={host} title="Copy IP" onClick={() => void copyHost(host, toast)}>
              <Copy size={16} />
            </Button>
          )}
          <div className="ml-auto flex items-center gap-2">
            <Button iconOnly label={theme === "dark" ? "Light theme" : "Dark theme"} onClick={onTheme}>
              {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
            </Button>
            <AccountMenu onLogout={onLogout} />
          </div>
        </header>
        <main className="min-h-0 min-w-0 flex-1 overflow-auto p-4">
          {location.pathname !== "/settings" && location.pathname !== "/logs" && !current ? (
            <Card title="Add a server">
              <p className="text-sm text-[var(--muted)]">Choose a server from the sidebar after you add its SSH login in Settings.</p>
              <NavLink to="/settings" className="mt-3 inline-flex text-sm">Open Settings</NavLink>
            </Card>
          ) : (
            <Outlet key={current || "none"} />
          )}
        </main>
      </div>
    </div>
  );
}
