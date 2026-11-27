import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Officers } from "./Officers";
import { bodyOf, mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const list = [
  { id: 1, bank_id: 1, bank_name: "HDFC", team: "tdep", team_name: "TDEP", name: "Asha", phone: "9876543210" },
  { id: 2, bank_id: 1, bank_name: "HDFC", team: "dovr", team_name: "DOVR", name: "Ravi", phone: "9876543211" },
  { id: 3, bank_id: 2, bank_name: "SBI", team: "officer", team_name: "Officer", name: "Sita", phone: "9876543212" },
];
const hdfcStages = [
  { id: 31, code: "link_sent", name: "Link sent", sort_order: 2, requires_document: false, officer_team: "tdep", officer_team_name: "TDEP" },
  { id: 34, code: "dovr", name: "DOVR", sort_order: 4, requires_document: false, officer_team: "dovr", officer_team_name: "DOVR" },
  { id: 40, code: "signing", name: "Signing", sort_order: 10, requires_document: false, officer_team: "" },
];

function setup(extra: Record<string, { status?: number; body?: unknown }> = {}) {
  const spy = mockApi({
    "GET /api/officers": { body: list },
    "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }, { id: 2, name: "SBI" }] },
    "GET /api/config/banks/1/stages": { body: hdfcStages },
    ...extra,
  });
  render(<Officers />);
  return spy;
}

describe("Officers page", () => {
  it("groups officers by bank and team with call links", async () => {
    setup();
    expect(await screen.findByRole("heading", { level: 1, name: "Bank officers" })).toBeInTheDocument();
    const hdfc = await screen.findByRole("region", { name: "HDFC" });
    expect(within(hdfc).getByRole("heading", { name: "TDEP" })).toBeInTheDocument();
    expect(within(hdfc).getByRole("link", { name: "9876543211" })).toHaveAttribute("href", "tel:9876543211");
    expect(within(screen.getByRole("region", { name: "SBI" })).getByText("Sita")).toBeInTheDocument();
  });

  it("adds an officer with a team from the bank's stages", async () => {
    const spy = setup({ "POST /api/officers": { status: 201, body: { ...list[1], id: 9, name: "Kiran" } } });
    await screen.findByRole("region", { name: "HDFC" });
    await userEvent.click(screen.getByRole("button", { name: "Add officer" }));
    const dialog = screen.getByRole("dialog", { name: "Add officer" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank"), "1");
    await within(dialog).findByRole("option", { name: "DOVR" });
    expect(within(dialog).queryByRole("option", { name: "Signing" })).not.toBeInTheDocument();
    await userEvent.selectOptions(within(dialog).getByLabelText("Team"), "dovr");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Kiran");
    await userEvent.type(within(dialog).getByLabelText("Mobile"), "9000000009");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/officers")).toEqual({ bank_id: 1, team: "dovr", name: "Kiran", phone: "9000000009", email: "" }));
  });

  it("edits an officer's name and mobile", async () => {
    const spy = setup({ "PUT /api/officers/2": { body: { ...list[1], name: "Ravi K" } } });
    await userEvent.click(await screen.findByRole("button", { name: "Edit Ravi" }));
    const dialog = screen.getByRole("dialog", { name: "Edit officer" });
    const name = within(dialog).getByLabelText("Name");
    expect(name).toHaveValue("Ravi");
    await userEvent.clear(name);
    await userEvent.type(name, "Ravi K");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "PUT /api/officers/2")).toEqual({ name: "Ravi K", phone: "9876543211", email: "" }));
  });

  it("removes an officer after a confirm", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const spy = setup({ "DELETE /api/officers/2": { status: 204 } });
    await userEvent.click(await screen.findByRole("button", { name: "Remove Ravi" }));
    expect(confirm).toHaveBeenCalledWith("Remove Ravi? Cases that have this officer keep them.");
    await waitFor(() => expect(spy.mock.calls.some(([u, i]) => String(u) === "/api/officers/2" && i?.method === "DELETE")).toBe(true));
  });

  it("shows a hint when there are no officers", async () => {
    mockApi({ "GET /api/officers": { body: [] }, "GET /api/config/banks": { body: [] } });
    render(<Officers />);
    expect(await screen.findByText("No officers yet. Add the first one.")).toBeInTheDocument();
  });
});

describe("Officers page email", () => {
  it("shows an email link and adds an officer with only an email", async () => {
    const spy = mockApi({
      "GET /api/officers": { body: [{ id: 4, bank_id: 1, bank_name: "HDFC", team: "dovr", team_name: "DOVR", name: "Mail", phone: "", email: "mail@hdfc.example" }] },
      "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
      "GET /api/config/banks/1/stages": { body: hdfcStages },
      "POST /api/officers": { status: 201, body: {} },
    });
    render(<Officers />);
    expect(await screen.findByRole("link", { name: "mail@hdfc.example" })).toHaveAttribute("href", "mailto:mail@hdfc.example");
    await userEvent.click(screen.getByRole("button", { name: "Add officer" }));
    const dialog = screen.getByRole("dialog", { name: "Add officer" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank"), "1");
    await within(dialog).findByRole("option", { name: "DOVR" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Team"), "dovr");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Kiran");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Enter a mobile number or an email.");
    await userEvent.type(within(dialog).getByLabelText("Email"), "kiran@hdfc.example");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/officers")).toEqual({ bank_id: 1, team: "dovr", name: "Kiran", phone: "", email: "kiran@hdfc.example" }));
  });
});
