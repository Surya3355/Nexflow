import { vi } from "vitest";

/** Routes fetch by "METHOD /path" (query string ignored). Unknown routes fail the test. */
export function mockApi(routes: Record<string, { status?: number; body?: unknown }>) {
  return vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = (init?.method ?? "GET").toUpperCase();
    const key = `${method} ${url.pathname}`;
    const hit = routes[key];
    if (!hit) {
      throw new Error(`unmocked request: ${key}`);
    }
    const status = hit.status ?? 200;
    if (status === 204) {
      return new Response(null, { status });
    }
    return new Response(JSON.stringify(hit.body ?? {}), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  });
}

/** Records request bodies by "METHOD /path" for assertions. */
export function bodyOf(spy: ReturnType<typeof mockApi>, key: string): unknown {
  const call = spy.mock.calls.find(([input, init]) => {
    const url = new URL(String(input), "http://localhost");
    return `${(init?.method ?? "GET").toUpperCase()} ${url.pathname}` === key;
  });
  const init = call?.[1];
  return init?.body ? JSON.parse(String(init.body)) : undefined;
}
