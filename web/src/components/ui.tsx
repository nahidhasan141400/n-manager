import { Check, X } from "lucide-react";
import { createContext, useContext, useState, type ReactNode } from "react";

export function Badge({ value }: { value: string }) {
  const v = (value || "unknown").toLowerCase();
  const tone =
    v === "active" || v === "enabled" || v === "installed" || v === "running" || v === "online"
      ? "text-[var(--accent)] border-[var(--accent)]"
      : v === "failed" || v === "error"
        ? "text-[var(--danger)] border-[var(--danger)]"
        : "text-[var(--muted)] border-[var(--border)]";
  return <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs ${tone}`}>{value || "unknown"}</span>;
}

export function Button({
  children,
  onClick,
  kind = "default",
  type = "button",
  disabled,
  label,
  title,
  iconOnly = false,
  expanded,
}: {
  children: ReactNode;
  onClick?: () => void;
  kind?: "default" | "primary" | "danger";
  type?: "button" | "submit";
  disabled?: boolean;
  label?: string;
  title?: string;
  icon?: boolean;
  iconOnly?: boolean;
  expanded?: boolean;
}) {
  const tone =
    kind === "primary"
      ? "bg-[var(--accent)] text-white border-transparent hover:bg-[var(--accent-hover)]"
      : kind === "danger"
        ? "bg-transparent text-[var(--danger)] border-[var(--danger)] hover:bg-[var(--hover)]"
        : "bg-[var(--card)] text-[var(--text)] border-[var(--border)] hover:bg-[var(--hover)]";
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      aria-label={label}
      aria-expanded={expanded}
      title={title ?? label}
      className={`inline-flex h-8 items-center justify-center gap-1.5 whitespace-nowrap rounded-md border text-sm leading-none disabled:cursor-not-allowed disabled:opacity-50 [&_svg]:block [&_svg]:size-4 [&_svg]:shrink-0 ${iconOnly ? "w-8 px-0" : "px-2.5"} ${tone}`}
    >
      {children}
      {!iconOnly && label && <span className="leading-none">{label}</span>}
    </button>
  );
}

export function Field({
  label,
  children,
  className = "",
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <label className={`block min-w-0 text-sm ${className}`}>
      <span className="mb-1 block text-xs text-[var(--muted)]">{label}</span>
      {children}
    </label>
  );
}

export const inputClass =
  "w-full rounded-md border border-[var(--border)] bg-[var(--input)] px-3 py-2 outline-none";

type Toast = { id: number; kind: "ok" | "err"; text: string };
type ConfirmState = { message: string; resolve: (value: boolean) => void } | null;

const ToastContext = createContext<(kind: "ok" | "err", text: string) => void>(() => undefined);
const ConfirmContext = createContext<(message: string) => Promise<boolean>>(async () => false);

export function useToast() {
  return useContext(ToastContext);
}

export function useConfirm() {
  return useContext(ConfirmContext);
}

export function Feedback({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [confirm, setConfirm] = useState<ConfirmState>(null);

  function push(kind: "ok" | "err", text: string) {
    const id = Date.now() + Math.random();
    setToasts((items) => [...items, { id, kind, text }]);
    setTimeout(() => setToasts((items) => items.filter((item) => item.id !== id)), 4500);
  }

  function ask(message: string) {
    return new Promise<boolean>((resolve) => setConfirm({ message, resolve }));
  }

  return (
    <ToastContext.Provider value={push}>
      <ConfirmContext.Provider value={ask}>
        {children}
        <div className="fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
          {toasts.map((toast) => (
            <div
              key={toast.id}
              className={`rounded-md border bg-[var(--card)] px-3 py-2 shadow-[var(--shadow)] ${toast.kind === "err" ? "border-[var(--danger)]" : "border-[var(--border)]"}`}
            >
              {toast.text}
            </div>
          ))}
        </div>
        {confirm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
            <div className="w-full max-w-sm rounded-md border border-[var(--border)] bg-[var(--card)] p-4 shadow-[var(--shadow)]">
              <p className="mb-4">{confirm.message}</p>
              <div className="flex justify-end gap-2">
                <Button icon label="Cancel" onClick={() => { confirm.resolve(false); setConfirm(null); }}><X size={16} /></Button>
                <Button icon kind="danger" label="Confirm" onClick={() => { confirm.resolve(true); setConfirm(null); }}><Check size={16} /></Button>
              </div>
            </div>
          </div>
        )}
      </ConfirmContext.Provider>
    </ToastContext.Provider>
  );
}

export function Card({ title, children, action }: { title?: string; children: ReactNode; action?: ReactNode }) {
  return (
    <section className="rounded-md border border-[var(--border)] bg-[var(--card)]">
      {title && (
        <header className="flex items-center justify-between border-b border-[var(--border)] px-4 py-3">
          <h2 className="font-medium">{title}</h2>
          {action}
        </header>
      )}
      <div className="p-4">{children}</div>
    </section>
  );
}

export function Meter({ value }: { value: number }) {
  const width = Math.max(0, Math.min(100, value));
  return (
    <div className="h-1.5 overflow-hidden rounded-full bg-[var(--bg)]" title={`${width.toFixed(1)}%`}>
      <div className="h-full bg-[var(--accent)]" style={{ width: `${width}%` }} />
    </div>
  );
}
