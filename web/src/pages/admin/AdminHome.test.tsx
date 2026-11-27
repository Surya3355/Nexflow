import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { AdminHome } from "./AdminHome";
import { bodyOf, mockApi } from "../../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const team = [
  { id: 1, username: "priya", full_name: "Priya Sharma", role: "admin", email: "", is_active: true, open_cases: 12 },
  { id: 2, username: "divya", full_name: "Divya K", role: "employee", email: "d@example.com", is_active: true, open_cases: 15 },
];

function setup(hash = "", extra: Record<string, { status?: number; body?: unknown }> = {}) {
  const spy = mockApi({
    "GET /api/admin/users": { body: team },
    "GET /api/config/banks": { body: [{ id: 1, name: "HDFC", has_own_stages: true }, { id: 2, name: "SBI", has_own_stages: false }] },
    "GET /api/config/dropdowns/drop_reason": { body: [{ id: 1, value: "Customer not interested" }] },
    "GET /api/config/stages": { body: [] },
    "GET /api/admin/archive": { body: [] },
    ...extra,
  });
  render(<MemoryRouter initialEntries={[`/admin${hash}`]}><AdminHome role="admin" /></MemoryRouter>);
  return spy;
}

describe("Admin", () => {
  it("Team lists users with role and open cases", async () => {
    setup();
    const row = (await screen.findByText("Divya K")).closest("tr")!;
    expect(row).toHaveTextContent("divya");
    expect(row).toHaveTextContent("Employee");
    expect(row).toHaveTextContent("15");
  });

  it("Add person dialog posts and refreshes the list", async () => {
    const spy = setup("", { "POST /api/admin/users": { status: 201, body: { id: 3, username: "suresh", role: "manager" } } });
    await screen.findByText("Divya K");
    await userEvent.click(screen.getByRole("button", { name: "+ Add person" }));
    const dialog = screen.getByRole("dialog", { name: "Add a person" });
    await userEvent.type(within(dialog).getByLabelText("Full name"), "Suresh N");
    await userEvent.type(within(dialog).getByLabelText("Username"), "suresh");
    await userEvent.selectOptions(within(dialog).getByLabelText("Role"), "manager");
    await userEvent.type(within(dialog).getByLabelText("Email (for case alerts)"), "s@example.com");
    await userEvent.type(within(dialog).getByLabelText("First password (8+ characters)"), "first-password");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/admin/users")).toEqual({
      username: "suresh", full_name: "Suresh N", role: "manager", email: "s@example.com", password: "first-password",
    }));
    await waitFor(() => expect(spy.mock.calls.filter(([u]) => String(u) === "/api/admin/users").length).toBeGreaterThan(2));
  });

  it("Reset password posts the new password and confirms", async () => {
    const spy = setup("", { "POST /api/admin/users/2/password": { status: 204 } });
    await screen.findByText("Divya K");
    await userEvent.click(screen.getByRole("button", { name: "Reset password for Divya K" }));
    const dialog = screen.getByRole("dialog", { name: "Reset password for Divya K" });
    await userEvent.type(within(dialog).getByLabelText("New password (8+ characters)"), "temporary-pass");
    await userEvent.click(within(dialog).getByRole("button", { name: "Reset" }));
    expect(await within(dialog).findByRole("status")).toHaveTextContent("Password reset. Divya K is signed out everywhere.");
    expect(bodyOf(spy, "POST /api/admin/users/2/password")).toEqual({ password: "temporary-pass" });
  });

  it("Banks tab shows which banks use their own stages", async () => {
    setup("#banks");
    const hdfc = (await screen.findByText("HDFC")).closest("tr")!;
    expect(hdfc).toHaveTextContent("HDFC stages");
    expect(screen.getByText("SBI").closest("tr")!).toHaveTextContent("Standard stages");
  });

  it("renames a bank", async () => {
    const spy = setup("#banks", { "PUT /api/admin/banks/2": { body: { id: 2, name: "State Bank" } } });
    await screen.findByText("SBI");
    await userEvent.click(screen.getByRole("button", { name: "Rename SBI" }));
    const dialog = screen.getByRole("dialog", { name: "Rename SBI" });
    const name = within(dialog).getByLabelText("Bank name");
    await userEvent.clear(name);
    await userEvent.type(name, "State Bank");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "PUT /api/admin/banks/2")).toEqual({ name: "State Bank" }));
  });

  it("adds a drop reason", async () => {
    const spy = setup("#banks", { "POST /api/admin/dropdowns": { status: 201, body: { id: 5, category: "drop_reason", value: "Moved city" } } });
    await screen.findByText("Customer not interested");
    await userEvent.click(screen.getByRole("button", { name: "+ Add reason" }));
    const dialog = screen.getByRole("dialog", { name: "Add a drop reason" });
    await userEvent.type(within(dialog).getByLabelText("Reason"), "Moved city");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/admin/dropdowns")).toEqual({ category: "drop_reason", value: "Moved city", sort_order: 100 }));
  });

  it("tab follows the URL hash", async () => {
    setup("#archive");
    expect(await screen.findByRole("tab", { name: "Archive" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("heading", { name: "Archive" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "Stages" }));
    expect(screen.getByRole("tab", { name: "Stages" })).toHaveAttribute("aria-selected", "true");
  });
});
