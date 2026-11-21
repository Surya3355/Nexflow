import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import App from "./App";
import { bodyOf, mockApi } from "./test/fetchMock";

afterEach(() => vi.restoreAllMocks());

function renderAs(role: "admin" | "manager" | "employee") {
  mockApi({ "GET /api/me": { body: { id: 1, username: "u", full_name: "U", role } } });
  render(<MemoryRouter><App /></MemoryRouter>);
}

describe("navigation by role", () => {
  it("shows the Admin link to an admin", async () => {
    renderAs("admin");
    expect(await screen.findByRole("link", { name: "Admin" })).toBeInTheDocument();
  });

  it("does not show the Admin link to a manager", async () => {
    renderAs("manager");
    await screen.findByRole("navigation", { name: "Main" });
    expect(screen.queryByRole("link", { name: "Admin" })).not.toBeInTheDocument();
  });

  it("does not show the Admin link to an employee", async () => {
    renderAs("employee");
    await screen.findByRole("navigation", { name: "Main" });
    expect(screen.queryByRole("link", { name: "Admin" })).not.toBeInTheDocument();
  });
});

describe("pages by role", () => {
  async function signInAs(role: "admin" | "manager" | "employee") {
    const spy = mockApi({
      "GET /api/me": { status: 401, body: { error: "not signed in" } },
      "POST /api/login": { body: { id: 1, username: "u", full_name: "Priya Sharma", role } },
      "GET /api/config/stages": { body: [] },
      "GET /api/admin/archive": { body: [] },
      "GET /api/admin/users": { body: [] },
      "GET /api/config/banks": { body: [] },
      "GET /api/config/dropdowns/drop_reason": { body: [] },
      "GET /api/cases": { body: [] },
      "GET /api/cases/attention": { body: [] },
      "GET /api/users": { body: [] },
      "GET /api/reports/dashboard": {
        body: {
          new_customers_today: 0, new_cases_today: 0, new_customers: 0, new_cases: 0, stage_moves_today: 0,
          cases_by_stage: {}, sanctions: { count: 0, amount: "0", dropped: 0 }, disbursements: { count: 0, amount: "0", dropped: 0 },
        },
      },
    });
    render(<MemoryRouter initialEntries={["/"]}><App /></MemoryRouter>);
    await userEvent.type(await screen.findByLabelText("Username"), "u");
    await userEvent.type(screen.getByLabelText("Password"), "a-long-enough-password");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    return spy;
  }

  it("admin lands on Admin after sign-in", async () => {
    await signInAs("admin");
    expect(await screen.findByRole("heading", { name: "Admin" })).toBeInTheDocument();
  });

  it("admin can still open Home", async () => {
    await signInAs("admin");
    await screen.findByRole("heading", { name: "Admin" });
    await userEvent.click(screen.getByRole("link", { name: "Home" }));
    expect(await screen.findByRole("searchbox")).toBeInTheDocument();
  });

  it("manager and employee see Home, Projects and Reports only, and land on Home", async () => {
    for (const role of ["manager", "employee"] as const) {
      await signInAs(role);
      const nav = await screen.findByRole("navigation", { name: "Main" });
      const links = within(nav).getAllByRole("link").map((a) => a.textContent);
      expect(links).toEqual(["Home", "Projects", "Reports"]);
      expect(await screen.findByRole("searchbox")).toBeInTheDocument();
      cleanup();
      vi.restoreAllMocks();
    }
  });

  it("sends a non-admin who opens /admin to Home", async () => {
    mockApi({
      "GET /api/me": { body: { id: 1, username: "u", full_name: "U", role: "employee" } },
      "GET /api/cases": { body: [] },
      "GET /api/cases/attention": { body: [] },
      "GET /api/config/banks": { body: [] },
      "GET /api/users": { body: [] },
      "GET /api/reports/dashboard": { body: {} },
    });
    render(<MemoryRouter initialEntries={["/admin"]}><App /></MemoryRouter>);
    expect(await screen.findByRole("searchbox")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Admin" })).not.toBeInTheDocument();
  });

  it("Change password posts the current and new password", async () => {
    const spy = mockApi({
      "GET /api/me": { body: { id: 1, username: "u", full_name: "U", role: "employee" } },
      "GET /api/cases": { body: [] },
      "GET /api/cases/attention": { body: [] },
      "GET /api/config/banks": { body: [] },
      "GET /api/users": { body: [] },
      "GET /api/reports/dashboard": { body: {} },
      "POST /api/me/password": { status: 204 },
    });
    render(<MemoryRouter><App /></MemoryRouter>);
    await userEvent.click(await screen.findByRole("button", { name: "Account" }));
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    const dialog = screen.getByRole("dialog", { name: "Change password" });
    await userEvent.type(within(dialog).getByLabelText("Current password"), "old-password-1");
    await userEvent.type(within(dialog).getByLabelText("New password (8+ characters)"), "new-password-1");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Password changed.");
    expect(bodyOf(spy, "POST /api/me/password")).toEqual({ current: "old-password-1", new: "new-password-1" });
  });
});

describe("ended sessions", () => {
  it("shows the sign-in page when a request says the session has ended", async () => {
    mockApi({
      "GET /api/me": { body: { id: 1, username: "u", full_name: "U", role: "employee" } },
      "GET /api/cases": { status: 401, body: { error: "not signed in" } },
      "GET /api/cases/attention": { body: [] },
      "GET /api/config/banks": { body: [] },
      "GET /api/users": { body: [] },
      "GET /api/reports/dashboard": { body: {} },
    });
    render(<MemoryRouter><App /></MemoryRouter>);
    expect(await screen.findByRole("button", { name: "Sign in" })).toBeInTheDocument();
  });

  it("a wrong current password does not sign the person out", async () => {
    mockApi({
      "GET /api/me": { body: { id: 1, username: "u", full_name: "U", role: "employee" } },
      "GET /api/cases": { body: [] },
      "GET /api/cases/attention": { body: [] },
      "GET /api/config/banks": { body: [] },
      "GET /api/users": { body: [] },
      "GET /api/reports/dashboard": { body: {} },
      "POST /api/me/password": { status: 401, body: { error: "current password is wrong" } },
    });
    render(<MemoryRouter><App /></MemoryRouter>);
    await userEvent.click(await screen.findByRole("button", { name: "Account" }));
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    const dialog = screen.getByRole("dialog", { name: "Change password" });
    await userEvent.type(within(dialog).getByLabelText("Current password"), "wrong-password");
    await userEvent.type(within(dialog).getByLabelText("New password (8+ characters)"), "new-password-1");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("current password is wrong");
    expect(screen.queryByRole("button", { name: "Sign in" })).not.toBeInTheDocument();
  });
});
