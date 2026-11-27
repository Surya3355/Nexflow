import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Team } from "./Team";
import { bodyOf, mockApi } from "../../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const team = [
  { id: 1, username: "owner", full_name: "Owner", role: "admin", email: "", is_active: true, open_cases: 0 },
  { id: 2, username: "priya", full_name: "Priya Sharma", role: "employee", email: "", is_active: true, open_cases: 3 },
  { id: 3, username: "ravi", full_name: "Ravi Kumar", role: "manager", email: "", is_active: true, open_cases: 0 },
  { id: 4, username: "gone", full_name: "Old Hand", role: "employee", email: "", is_active: false, open_cases: 0 },
];

function row(name: string) {
  return screen.getByRole("row", { name: new RegExp(name) });
}

describe("Team", () => {
  it("offers Switch off for active people and Switch on for switched-off ones", async () => {
    mockApi({ "GET /api/admin/users": { body: team } });
    render(<Team />);
    await screen.findByText("Priya Sharma");
    expect(within(row("Priya Sharma")).getByRole("button", { name: "Switch off Priya Sharma" })).toBeInTheDocument();
    expect(within(row("Old Hand")).getByText("Switched off")).toBeInTheDocument();
    expect(within(row("Old Hand")).getByRole("button", { name: "Switch on Old Hand" })).toBeInTheDocument();
  });

  it("asks who takes over open work, and sends that person", async () => {
    const spy = mockApi({
      "GET /api/admin/users": { body: team },
      "GET /api/admin/users/2/open-work": { body: { leads: 2, cases: 3, queries: 1 } },
      "POST /api/admin/users/2/deactivate": { status: 204 },
    });
    render(<Team />);
    await userEvent.click(await screen.findByRole("button", { name: "Switch off Priya Sharma" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Open leads 2 · Open cases 3 · Open bank queries 1")).toBeInTheDocument();
    const picker = within(dialog).getByLabelText("Who takes over?");
    // Only other active people can take over.
    const options = within(picker).getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["Choose a person", "Owner", "Ravi Kumar"]);
    await userEvent.click(within(dialog).getByRole("button", { name: "Switch off" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Choose who takes over.");
    await userEvent.selectOptions(picker, "3");
    await userEvent.click(within(dialog).getByRole("button", { name: "Switch off" }));
    expect(bodyOf(spy, "POST /api/admin/users/2/deactivate")).toEqual({ takeover_id: 3 });
  });

  it("switches off without a takeover when nothing is open", async () => {
    const spy = mockApi({
      "GET /api/admin/users": { body: team },
      "GET /api/admin/users/3/open-work": { body: { leads: 0, cases: 0, queries: 0 } },
      "POST /api/admin/users/3/deactivate": { status: 204 },
    });
    render(<Team />);
    await userEvent.click(await screen.findByRole("button", { name: "Switch off Ravi Kumar" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Switch off Ravi Kumar? Nothing is open.")).toBeInTheDocument();
    expect(within(dialog).queryByLabelText("Who takes over?")).not.toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Switch off" }));
    expect(bodyOf(spy, "POST /api/admin/users/3/deactivate")).toEqual({});
  });

  it("shows the server's reason when a switch off is refused", async () => {
    mockApi({
      "GET /api/admin/users": { body: team },
      "GET /api/admin/users/1/open-work": { body: { leads: 0, cases: 0, queries: 0 } },
      "POST /api/admin/users/1/deactivate": { status: 409, body: { error: "access: the last active admin can't be switched off or changed to another role" } },
    });
    render(<Team />);
    await userEvent.click(await screen.findByRole("button", { name: "Switch off Owner" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(/Nothing is open/);
    await userEvent.click(within(dialog).getByRole("button", { name: "Switch off" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The last active admin can't be switched off");
  });

  it("changes a role", async () => {
    const spy = mockApi({
      "GET /api/admin/users": { body: team },
      "PUT /api/admin/users/2/role": { status: 204 },
    });
    render(<Team />);
    await userEvent.click(await screen.findByRole("button", { name: "Change role for Priya Sharma" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.selectOptions(within(dialog).getByLabelText("Role"), "manager");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(bodyOf(spy, "PUT /api/admin/users/2/role")).toEqual({ role: "manager" });
  });

  it("switches a person back on", async () => {
    const spy = mockApi({
      "GET /api/admin/users": { body: team },
      "POST /api/admin/users/4/reactivate": { status: 204 },
    });
    render(<Team />);
    await userEvent.click(await screen.findByRole("button", { name: "Switch on Old Hand" }));
    expect(spy.mock.calls.some(([u, i]) => String(u).endsWith("/api/admin/users/4/reactivate") && i?.method === "POST")).toBe(true);
  });
});
