import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, request } from "./client";

afterEach(() => {
  vi.restoreAllMocks();
});

function mockFetch(status: number, body: unknown) {
  return vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(body === undefined ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

describe("request", () => {
  it("throws ApiError with the status on a 403", async () => {
    mockFetch(403, { error: "not allowed for this role" });
    const err = await request("POST", "/api/admin/banks", { name: "X" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiErr = err as ApiError;
    expect(apiErr.status).toBe(403);
    expect(apiErr.message).toBe("not allowed for this role");
  });

  it("sends the session cookie and a JSON body", async () => {
    const spy = mockFetch(201, { id: 7 });
    const out = await request<{ id: number }>("POST", "/api/customers", { name: "Ravi" });
    expect(out).toEqual({ id: 7 });
    const [url, init] = spy.mock.calls[0];
    expect(url).toBe("/api/customers");
    expect(init?.credentials).toBe("same-origin");
    expect(init?.body).toBe(JSON.stringify({ name: "Ravi" }));
  });

  it("returns undefined for 204", async () => {
    mockFetch(204, undefined);
    await expect(request("DELETE", "/api/cases/1")).resolves.toBeUndefined();
  });
});

describe("server error text", () => {
  it("drops the package prefix and reads as a sentence", async () => {
    mockFetch(409, { error: "workflows: a project with this name already exists" });
    const err = (await request("POST", "/api/admin/projects", { name: "X" }).catch((e: unknown) => e)) as ApiError;
    expect(err.message).toBe("A project with this name already exists.");
  });

  it("keeps a sentence that already ends with a full stop", async () => {
    mockFetch(409, { error: "cases: this customer was dropped. Bring them back first" });
    const err = (await request("POST", "/api/cases", {}).catch((e: unknown) => e)) as ApiError;
    expect(err.message).toBe("This customer was dropped. Bring them back first.");
  });
});
