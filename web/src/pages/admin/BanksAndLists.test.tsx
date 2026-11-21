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
      { id: 1, name: "NA", is_system: true, open_cases: 4, total_cases: 6 },
      { id: 2, name: "Prestige", is_system: false, open_cases: 2, total_cases: 3 },
    ],
  },
};

describe("BanksAndLists projects", () => {
  it("lists projects with NA first and no rename for NA", async () => {
    mockApi(base);
    render(<BanksAndLists />);
    const section = await screen.findByRole("region", { name: "Projects" });
    const rows = await within(section).findAllByRole("row");
    expect(rows[0]).toHaveTextContent("NA");
    expect(rows[0]).toHaveTextContent("4 open");
    expect(within(section).queryByRole("button", { name: "Rename NA" })).toBeNull();
    expect(within(section).getByRole("button", { name: "Rename Prestige" })).toBeInTheDocument();
  });

  it("adds a project and shows a duplicate error", async () => {
    const spy = mockApi({
      ...base,
      "POST /api/admin/projects": { status: 409, body: { error: "workflows: a project with this name already exists" } },
    });
    render(<BanksAndLists />);
    await userEvent.click(await screen.findByRole("button", { name: "+ Add project" }));
    await userEvent.type(screen.getByLabelText("Project name"), "prestige");
    await userEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/admin/projects")).toEqual({ name: "prestige" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("A project with this name already exists.");
  });
});
