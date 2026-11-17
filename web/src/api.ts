export class ApiError extends Error {
  status: number;
  body: { error?: string; needs_trust?: boolean; fingerprint?: string };
  constructor(message: string, status: number, body: ApiError["body"] = {}) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

const serverKey = "nmanager-server";

export function serverId() {
  return localStorage.getItem(serverKey) || "";
}

export function setServer(id: string) {
  if (id) localStorage.setItem(serverKey, id);
  else localStorage.removeItem(serverKey);
  window.dispatchEvent(new Event("nmanager:server"));
}

let csrf = "";

export function setCsrf(token: string) {
  csrf = token;
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const method = (init.method ?? "GET").toUpperCase();
  if (csrf && method !== "GET" && method !== "HEAD") headers.set("X-CSRF-Token", csrf);
  const selected = serverId();
  if (selected) headers.set("X-Server-Id", selected);
  const res = await fetch(path, { ...init, headers, credentials: "same-origin" });
  const text = await res.text();
  let data: { error?: string; needs_trust?: boolean; fingerprint?: string } = {};
  if (text) {
    try {
      data = JSON.parse(text) as typeof data;
    } catch {
      data = { error: text };
    }
  }
  if (res.status === 401 && path !== "/api/login") {
    window.dispatchEvent(new Event("nmanager:unauthorized"));
  }
  if (!res.ok) throw new ApiError(data.error || res.statusText, res.status, data);
  return data as T;
}

export async function download(path: string, filename: string) {
  const headers = new Headers();
  const selected = serverId();
  if (selected) headers.set("X-Server-Id", selected);
  const res = await fetch(path, { headers, credentials: "same-origin" });
  if (!res.ok) {
    const text = await res.text();
    let message = res.statusText;
    try {
      const data = JSON.parse(text) as { error?: string };
      if (data.error) message = data.error;
    } catch {
      if (text) message = text;
    }
    if (res.status === 401) window.dispatchEvent(new Event("nmanager:unauthorized"));
    throw new ApiError(message, res.status);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}
