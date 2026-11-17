import { Check, CircleUser, Ellipsis, LogOut, X } from "lucide-react";
import { Button as FormButton, Checkbox as FormCheckbox, ContextMenu, Dialog, DropdownMenu as ThemesMenu, Select as FormSelect, TextArea as FormTextArea, TextField } from "@radix-ui/themes";
import { AlertDialog, DropdownMenu, Label, Progress, Tabs, Toast, Tooltip } from "radix-ui";
import { createContext, useContext, useState, type ComponentProps, type ReactNode, type Ref } from "react";

const emptyValue = "__empty";

export function Placeholder({ className = "h-4 w-24" }: { className?: string }) {
  return <span className={`skeleton ${className}`} aria-hidden="true" />;
}

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
  ref,
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
  ref?: Ref<HTMLButtonElement>;
}) {
  const control = (
    <FormButton
      ref={ref}
      type={type}
      size="2"
      variant={kind === "primary" ? "solid" : kind === "danger" ? "soft" : "surface"}
      color={kind === "danger" ? "red" : kind === "primary" ? "green" : "gray"}
      disabled={disabled}
      onClick={onClick}
      aria-label={label}
      aria-expanded={expanded}
      className={iconOnly ? "icon-button" : undefined}
    >
      {children}
      {!iconOnly && label}
    </FormButton>
  );
  const tip = title || (iconOnly ? label : "");
  if (!tip) return control;
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{control}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content className="z-50 rounded-md bg-[var(--text)] px-2 py-1 text-xs text-[var(--bg)]" sideOffset={6}>
          {tip}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
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
    <Label.Root className={`block min-w-0 text-sm ${className}`}>
      <span className="mb-1 block text-xs text-[var(--muted)]">{label}</span>
      {children}
    </Label.Root>
  );
}

export function CheckField({
  label,
  checked,
  onCheckedChange,
}: {
  label: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex h-8 items-center gap-2 text-sm">
      <FormCheckbox size="2" checked={checked} onCheckedChange={(value) => onCheckedChange(value === true)} />
      {label}
    </label>
  );
}

export type SelectOption = { value: string; label: string };

export function SelectBox({
  value,
  onValueChange,
  options,
  placeholder = "Select",
  tone = "field",
  label,
}: {
  value: string;
  onValueChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  tone?: "field" | "sidebar";
  label?: string;
}) {
  const stored = options.some((item) => item.value === (value || emptyValue)) ? value || emptyValue : undefined;
  return (
    <FormSelect.Root size="2" value={stored} onValueChange={(next) => onValueChange(next === emptyValue ? "" : next)}>
      <FormSelect.Trigger className={tone === "sidebar" ? "mt-1 w-full" : "w-full"} variant={tone === "sidebar" ? "soft" : "surface"} aria-label={label} placeholder={placeholder} />
      <FormSelect.Content position="popper" sideOffset={4}>
        {options.map((item) => (
          <FormSelect.Item key={item.value || emptyValue} value={item.value || emptyValue}>
            {item.label}
          </FormSelect.Item>
        ))}
      </FormSelect.Content>
    </FormSelect.Root>
  );
}

export function TabBar({
  value,
  onValueChange,
  items,
}: {
  value: string;
  onValueChange: (value: string) => void;
  items: { value: string; label: string; icon?: ReactNode }[];
}) {
  return (
    <Tabs.Root value={value} onValueChange={onValueChange}>
      <Tabs.List className="flex flex-wrap gap-2">
        {items.map((item) => (
          <Tabs.Trigger
            key={item.value}
            value={item.value}
            className="inline-flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-sm leading-none outline-none data-[state=active]:border-transparent data-[state=active]:bg-[var(--accent)] data-[state=active]:text-white data-[state=inactive]:border-[var(--border)] data-[state=inactive]:bg-[var(--card)] data-[state=inactive]:hover:bg-[var(--hover)] [&_svg]:size-4"
          >
            {item.icon}
            {item.label}
          </Tabs.Trigger>
        ))}
      </Tabs.List>
    </Tabs.Root>
  );
}

export function AccountMenu({ onLogout }: { onLogout: () => void }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <FormButton size="2" variant="surface" color="gray" aria-label="Account" className="icon-button">
          <CircleUser size={16} />
        </FormButton>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="z-30 w-44 rounded-lg border border-[var(--border)] bg-[var(--card)] p-1 text-[var(--text)] shadow-[var(--shadow)]" align="end" sideOffset={6}>
          <DropdownMenu.Item
            className="flex h-9 cursor-pointer items-center gap-2 rounded-md px-2 text-sm leading-none outline-none data-[highlighted]:bg-[var(--hover)] [&_svg]:size-4"
            onSelect={onLogout}
          >
            <LogOut size={16} />
            Log out
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export type MenuAction = {
  label: string;
  icon?: ReactNode;
  onSelect: () => void;
  danger?: boolean;
  disabled?: boolean;
};

function ActionItems({ actions, Item }: { actions: MenuAction[]; Item: typeof ContextMenu.Item }) {
  return actions.map((action) => (
    <Item key={action.label} color={action.danger ? "red" : undefined} disabled={action.disabled} onSelect={action.onSelect}>
      <span className="inline-flex items-center gap-2 [&_svg]:size-4">
        {action.icon}
        {action.label}
      </span>
    </Item>
  ));
}

export function RowActions({ children, actions }: { children: ReactNode; actions: MenuAction[] }) {
  if (actions.length === 0) return <div className="min-w-0 flex-1">{children}</div>;
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger>
        <div className="flex min-w-0 flex-1 items-center justify-between gap-2">
          <div className="min-w-0 flex-1">{children}</div>
          <ThemesMenu.Root>
            <ThemesMenu.Trigger>
              <FormButton type="button" size="2" variant="surface" color="gray" aria-label="Actions" className="icon-button">
                <Ellipsis size={16} />
              </FormButton>
            </ThemesMenu.Trigger>
            <ThemesMenu.Content align="end" sideOffset={4}>
              <ActionItems actions={actions} Item={ThemesMenu.Item} />
            </ThemesMenu.Content>
          </ThemesMenu.Root>
        </div>
      </ContextMenu.Trigger>
      <ContextMenu.Content>
        <ActionItems actions={actions} Item={ContextMenu.Item} />
      </ContextMenu.Content>
    </ContextMenu.Root>
  );
}

export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  wide = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Content maxWidth={wide ? "760px" : "460px"} size="2">
        <Dialog.Title>{title}</Dialog.Title>
        {description ? <Dialog.Description className="mb-3 mt-1 text-sm text-[var(--muted)]">{description}</Dialog.Description> : <div className="mt-3" />}
        {children}
      </Dialog.Content>
    </Dialog.Root>
  );
}

export function TextInput(props: ComponentProps<typeof TextField.Root>) {
  return <TextField.Root size="2" {...props} />;
}

export function TextAreaInput(props: ComponentProps<typeof FormTextArea>) {
  return <FormTextArea size="2" {...props} />;
}

type ToastItem = { id: number; kind: "ok" | "err"; text: string };
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
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [confirm, setConfirm] = useState<ConfirmState>(null);

  function push(kind: "ok" | "err", text: string) {
    const id = Date.now() + Math.random();
    setToasts((items) => [...items, { id, kind, text }]);
  }

  function ask(message: string) {
    return new Promise<boolean>((resolve) => setConfirm({ message, resolve }));
  }

  function finish(value: boolean) {
    setConfirm((current) => {
      current?.resolve(value);
      return null;
    });
  }

  return (
    <Tooltip.Provider delayDuration={300}>
      <ToastContext.Provider value={push}>
        <ConfirmContext.Provider value={ask}>
          <Toast.Provider swipeDirection="right" duration={4500}>
            {children}
            {toasts.map((toast) => (
              <Toast.Root
                key={toast.id}
                className={`rounded-md border bg-[var(--card)] px-3 py-2 shadow-[var(--shadow)] ${toast.kind === "err" ? "border-[var(--danger)]" : "border-[var(--border)]"}`}
                onOpenChange={(open) => {
                  if (!open) setToasts((items) => items.filter((item) => item.id !== toast.id));
                }}
              >
                <Toast.Description>{toast.text}</Toast.Description>
              </Toast.Root>
            ))}
            <Toast.Viewport className="fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2 outline-none" />
          </Toast.Provider>
          <AlertDialog.Root open={confirm !== null} onOpenChange={(open) => { if (!open) finish(false); }}>
            <AlertDialog.Portal>
              <AlertDialog.Overlay className="fixed inset-0 z-50 bg-black/50" />
              <AlertDialog.Content className="fixed left-1/2 top-1/2 z-50 w-[calc(100%-2rem)] max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-md border border-[var(--border)] bg-[var(--card)] p-4 shadow-[var(--shadow)]">
                <AlertDialog.Title className="mb-1 font-medium">Confirm</AlertDialog.Title>
                <AlertDialog.Description className="mb-4 text-sm text-[var(--muted)]">{confirm?.message}</AlertDialog.Description>
                <div className="flex justify-end gap-2">
                  <AlertDialog.Cancel asChild>
                    <Button label="Cancel"><X size={16} /></Button>
                  </AlertDialog.Cancel>
                  <AlertDialog.Action asChild>
                    <Button kind="danger" label="Confirm" onClick={() => finish(true)}><Check size={16} /></Button>
                  </AlertDialog.Action>
                </div>
              </AlertDialog.Content>
            </AlertDialog.Portal>
          </AlertDialog.Root>
        </ConfirmContext.Provider>
      </ToastContext.Provider>
    </Tooltip.Provider>
  );
}

export function Card({ title, children, action, loading = false }: { title?: string; children: ReactNode; action?: ReactNode; loading?: boolean }) {
  return (
    <section className={`rounded-md ${loading ? "gradient-frame" : "border border-[var(--border)] bg-[var(--card)]"}`}>
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
    <Progress.Root className="relative h-1.5 overflow-hidden rounded-full bg-[var(--bg)]" value={width}>
      <Progress.Indicator className="h-full bg-[var(--accent)]" style={{ width: `${width}%` }} />
    </Progress.Root>
  );
}
