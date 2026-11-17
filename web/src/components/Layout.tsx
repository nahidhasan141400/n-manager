import { NavLink, Outlet } from "react-router-dom";
import { useEffect, useRef, useState } from "react";
import { Beaker, CircleUser, FolderInput, Globe, LayoutDashboard, LogOut, Moon, Server, Settings, Sun } from "lucide-react";
import { api } from "../api";
import logo from "../assets/logo.png";
import { Button } from "./ui";

const links = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/applications", label: "Apps", icon: Server },
  { to: "/nginx", label: "Nginx", icon: Globe },
  { to: "/ftp", label: "FTP", icon: FolderInput },
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
  const [online, setOnline] = useState(true);
  const [menu, setMenu] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const accountRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!menu) return;
    function onPointer(event: MouseEvent) {
      if (!menuRef.current?.contains(event.target as Node)) setMenu(false);
    }
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setMenu(false);
        accountRef.current?.focus();
      }
    }
    document.addEventListener("mousedown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [menu]);

  useEffect(() => {
    let stop = false;
    async function ping() {
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
  }, []);

  return (
    <div className="flex h-full min-h-screen">
      <aside className="nav-shell flex w-64 shrink-0 flex-col">
        <div className="px-4 pb-4 pt-5">
          <img src={logo} alt="N Manager" className="h-9 w-auto max-w-full" />
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
            <span className={`h-2 w-2 rounded-full ${online ? "bg-[#3fb950]" : "bg-[var(--danger)]"}`} />
            {online ? "Online" : "Offline"}
          </div>
          {dryRun && (
            <div className="mt-2 flex items-center gap-2 text-[var(--warning)]">
              <Beaker size={14} />
              Dry run
            </div>
          )}
        </div>
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="relative z-20 flex h-12 items-center justify-end border-b border-[var(--border)] py-0 pl-4 pr-5">
          <div className="flex items-center gap-2">
            <Button iconOnly label={theme === "dark" ? "Light theme" : "Dark theme"} onClick={onTheme}>
              {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
            </Button>
            <div className="relative" ref={menuRef}>
              <button
                ref={accountRef}
                type="button"
                aria-label="Account"
                aria-haspopup="menu"
                aria-expanded={menu}
                title="Account"
                onClick={() => setMenu((open) => !open)}
                className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--card)] text-[var(--text)] hover:bg-[var(--hover)] [&_svg]:block [&_svg]:size-4"
              >
                <CircleUser size={16} />
              </button>
              {menu && (
                <div
                  role="menu"
                  aria-label="Account"
                  className="absolute right-0 z-30 mt-1.5 w-44 max-w-[calc(100vw-1.5rem)] rounded-lg border border-[var(--border)] bg-[var(--card)] p-1 shadow-[var(--shadow)]"
                >
                  <button
                    role="menuitem"
                    type="button"
                    className="flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-sm leading-none text-[var(--text)] hover:bg-[var(--hover)] [&_svg]:block [&_svg]:size-4 [&_svg]:shrink-0"
                    onClick={() => {
                      setMenu(false);
                      onLogout();
                    }}
                  >
                    <LogOut size={16} />
                    Log out
                  </button>
                </div>
              )}
            </div>
          </div>
        </header>
        <main className="min-w-0 flex-1 overflow-auto p-4">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
