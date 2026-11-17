export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

let csrf = "";

export function setCsrf(token: string) {
  csrf = token;
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const method = (init.method ?? "GET").toUpperCase();
  if (csrf && method !== "GET" && method !== "HEAD") headers.set("X-CSRF-Token", csrf);
  const res = await fetch(path, { ...init, headers, credentials: "same-origin" });
  const text = await res.text();
  let data: { error?: string } = {};
  if (text) {
    try {
      data = JSON.parse(text) as { error?: string };
    } catch {
      data = { error: text };
    }
  }
  if (res.status === 401 && path !== "/api/login") {
    window.dispatchEvent(new Event("nmanager:unauthorized"));
  }
  if (!res.ok) throw new ApiError(data.error || res.statusText, res.status);
  return data as T;
}
