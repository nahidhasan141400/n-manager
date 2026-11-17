import { FormEvent, useState } from "react";
import { LoaderCircle, LogIn } from "lucide-react";
import { api, setCsrf } from "../api";
import logo from "../assets/logo.png";
import { Button, Field, inputClass } from "../components/ui";

export function Login({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await api<{ csrf_token: string }>("/api/login", {
        method: "POST",
        body: JSON.stringify({ password }),
      });
      setCsrf(res.csrf_token);
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--bg)] p-4">
      <form onSubmit={submit} className="w-full max-w-xs rounded-md border border-[var(--border)] bg-[var(--card)] p-5">
        <div className="mb-4 rounded-lg bg-[#10261c] px-3 py-2">
          <img src={logo} alt="N Manager" className="h-9 w-auto max-w-full" />
        </div>
        <Field label="Password">
          <input
            className={inputClass}
            type="password"
            autoFocus
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        {error && <p className="mt-3 text-sm text-[var(--danger)]">{error}</p>}
        <div className="mt-4">
          <Button type="submit" kind="primary" icon label="Sign in" disabled={busy}>
            {busy ? <LoaderCircle size={16} className="animate-spin" /> : <LogIn size={16} />}
          </Button>
        </div>
      </form>
    </div>
  );
}
