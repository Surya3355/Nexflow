import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BanksAndLists } from "./BanksAndLists";
import { bodyOf, mockApi } from "../../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const base = {
  "GET /api/config/banks": { body: [] },
  "GET /api/config/dropdowns/drop_reason": { body: [] },
  "GET /api/config/projects": {
    body: [
      { id: 1, name: "NA", is_system: true, open_cases: 4, total_cases: 6, type: null, address: "", active_contacts: 0 },
      { id: 2, name: "Prestige", is_system: false, open_cases: 2, total_cases: 3, type: "villas", address: "Road", active_contacts: 2 },
      { id: 3, name: "Old Site", is_system: false, open_cases: 1, total_cases: 1, type: null, address: "", active_contacts: 0 },
    ],
  },
};

describe("BanksAndLists projects", () => {
  it("lists projects with type and contacts, NA first and not editable", async () => {
    mockApi(base);
    render(<BanksAndLists />);
    const section = await screen.findByRole("region", { name: "Projects" });
    const rows = await within(section).findAllByRole("row");
    expect(rows[0]).toHaveTextContent("NA");
    expect(rows[0]).toHaveTextContent("4 open");
    expect(rows[0]).not.toHaveTextContent("Add type and contact");
    expect(within(section).queryByRole("button", { name: "Edit NA" })).toBeNull();
    expect(rows[1]).toHaveTextContent("Villas");
    expect(rows[1]).toHaveTextContent("2 contacts");
    expect(rows[2]).toHaveTextContent("Add type and contact");
    expect(within(section).getByRole("button", { name: "Edit Prestige" })).toBeInTheDocument();
  });

  it("opens the project form from + Add project", async () => {
    mockApi(base);
    render(<BanksAndLists />);
    await userEvent.click(await screen.findByRole("button", { name: "+ Add project" }));
    expect(screen.getByRole("dialog", { name: "Add a project" })).toBeInTheDocument();
    expect(screen.getByRole("radiogroup", { name: "Type" })).toBeInTheDocument();
  });
});

describe("BanksAndLists bank settings", () => {
  const banks = [
    { id: 1, name: "HDFC", has_own_stages: true, stage_list: "hdfc", part_payments: true },
    { id: 2, name: "SBI", has_own_stages: false, stage_list: "standard", part_payments: false },
  ];

  it("shows each bank's stage list and offers HDFC stages only to a standard bank", async () => {
    mockApi({ ...base, "GET /api/config/banks": { body: banks } });
    render(<BanksAndLists />);
    const hdfc = await screen.findByRole("row", { name: /^HDFC/ });
    expect(hdfc).toHaveTextContent("HDFC stages");
    expect(within(hdfc).queryByRole("button", { name: /Use HDFC stages/ })).toBeNull();
    const sbi = screen.getByRole("row", { name: /SBI/ });
    expect(sbi).toHaveTextContent("Standard stages");
    expect(within(sbi).getByRole("button", { name: "Use HDFC stages for SBI" })).toBeInTheDocument();
  });

  it("switches a bank to HDFC stages after a confirm", async () => {
    const spy = mockApi({ ...base, "GET /api/config/banks": { body: banks }, "PATCH /api/admin/banks/2": { status: 204 } });
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<BanksAndLists />);
    await userEvent.click(await screen.findByRole("button", { name: "Use HDFC stages for SBI" }));
    expect(confirm).toHaveBeenCalledWith("Move this bank's cases to the HDFC stages? This cannot be undone.");
    await waitFor(() => expect(bodyOf(spy, "PATCH /api/admin/banks/2")).toEqual({ stage_list: "hdfc" }));
  });

  it("part payments checkbox saves per bank", async () => {
    const spy = mockApi({ ...base, "GET /api/config/banks": { body: banks }, "PATCH /api/admin/banks/2": { status: 204 } });
    render(<BanksAndLists />);
    const box = await screen.findByRole("checkbox", { name: "Part payments: SBI" });
    expect(box).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Part payments: HDFC" })).toBeChecked();
    await userEvent.click(box);
    await waitFor(() => expect(bodyOf(spy, "PATCH /api/admin/banks/2")).toEqual({ part_payments: true }));
  });
});
