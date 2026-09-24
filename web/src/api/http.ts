import type { ApiOk } from "./types";
import { i18n } from "../i18n";

const TOKEN_KEY = "tdload_token";

type TicketKind = "media" | "sse";

type TicketCache = {
  token: string;
  expiresAt: number;
};

const tickets: Record<TicketKind, TicketCache | null> = {
  media: null,
  sse: null,
};

let ticketInflight: Partial<Record<TicketKind, Promise<string | null>>> = {};

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string | null): void {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
  clearTickets();
}

export function clearTickets(): void {
  tickets.media = null;
  tickets.sse = null;
  ticketInflight = {};
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(path, { ...init, headers });
  const body = (await res.json().catch(() => ({}))) as ApiOk<T>;

  if (res.status === 401) {
    setToken(null);
    if (!path.includes("/api/auth/login") && location.pathname !== "/login") {
      location.href = "/login";
    }
  }

  if (!res.ok || body.ok === false) {
    throw new Error(
      body.message || i18n.global.t("http.requestFailed", { status: res.status }),
    );
  }
  return body.data as T;
}

export async function ensureTicket(kind: TicketKind): Promise<string | null> {
  if (!getToken()) {
    clearTickets();
    return null;
  }
  const cached = tickets[kind];
  const now = Math.floor(Date.now() / 1000);
  if (cached && cached.expiresAt > now + 90) {
    return cached.token;
  }
  if (!ticketInflight[kind]) {
    ticketInflight[kind] = (async () => {
      try {
        const data = await api<{ token: string; expiresAt: number; kind: string }>(
          "/api/auth/ticket",
          {
            method: "POST",
            body: JSON.stringify({ kind }),
          },
        );
        tickets[kind] = { token: data.token, expiresAt: data.expiresAt };
        return data.token;
      } catch {
        tickets[kind] = null;
        return null;
      } finally {
        delete ticketInflight[kind];
      }
    })();
  }
  return ticketInflight[kind]!;
}

export async function refreshTickets(): Promise<void> {
  await Promise.all([ensureTicket("media"), ensureTicket("sse")]);
}

export async function openEventSource(path: string): Promise<EventSource> {
  const token = await ensureTicket("sse");
  const url = token
    ? `${path}${path.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}`
    : path;
  return new EventSource(url);
}
