// Thin wrapper over fetch for the Nexflow JSON API. The session is a cookie;
// the browser sends it with same-origin requests. The server decides every
// permission: this client never trusts or checks roles for access control.

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

/**
 * The server writes errors as "<area>: <message>" (for example
 * "workflows: a project with this name already exists"). Staff see the
 * message as a sentence: no area prefix, a capital first letter, a full stop.
 */
export function friendlyMessage(raw: string): string {
  const m = /^[a-z]+: (.+)$/s.exec(raw);
  if (!m) return raw;
  const text = m[1].charAt(0).toUpperCase() + m[1].slice(1);
  return /[.!?]$/.test(text) ? text : `${text}.`;
}

// A 401 from these paths means a wrong password, not an ended session.
const passwordChecks = new Set(["/api/login", "/api/me/password", "/api/me"]);
let sessionEnded: (() => void) | null = null;

/** Registers what to do when the server says the session has ended (signed out, password reset, deactivated). */
export function onSessionEnded(fn: (() => void) | null) {
  sessionEnded = fn;
}

function checkSession(path: string, status: number) {
  if (status === 401 && !passwordChecks.has(path.split("?")[0])) sessionEnded?.();
}

/** Sends a request and returns the parsed JSON body (undefined for 204). Throws ApiError on non-2xx. */
export async function request<T>(method: Method, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  let payload: BodyInit | undefined;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  const res = await fetch(path, { method, headers, body: payload, credentials: "same-origin" });
  if (res.status === 204) {
    return undefined as T;
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    checkSession(path, res.status);
    const message = typeof data?.error === "string" ? friendlyMessage(data.error) : `request failed (${res.status})`;
    throw new ApiError(res.status, message);
  }
  return data as T;
}

/** Sends a multipart form (file upload). The browser sets the boundary. */
export async function upload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(path, { method: "POST", body: form, credentials: "same-origin" });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    checkSession(path, res.status);
    const message = typeof data?.error === "string" ? friendlyMessage(data.error) : `upload failed (${res.status})`;
    throw new ApiError(res.status, message);
  }
  return data as T;
}

export const api = { request, upload };
