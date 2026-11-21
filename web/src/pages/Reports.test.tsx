import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { Reports } from "./Reports";
import { mockApi } from "../test/fetchMock";
import { istToday } from "../components/ui";

afterEach(() => vi.restoreAllMocks());

const dashboard = {
  new_customers_today: 4, new_cases_today: 5, new_customers: 34, new_cases: 41, stage_moves_today: 7,
  cases_by_stage: { received: 9, logged_in: 14 },
  sanctions: { count: 12, amount: "46000000.00", dropped: 1 },
  disbursements: { count: 7, amount: "29000000.00", dropped: 0 },
};

function setup() {
  return mockApi({
    "GET /api/reports/rows": { body: { columns: ["Case ID", "Customer"], rows: [["1042", "Ravi Kumar"]] } },
    "GET /api/reports/dashboard": { body: dashboard },
    "GET /api/config/banks": { body: [{ id: 2, name: "SBI" }] },
    "GET /api/config/projects": { body: [{ id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 }, { id: 4, name: "Prestige", is_system: false, open_cases: 0, total_cases: 0 }] },
    "GET /api/config/stages": { body: [
      { id: 1, code: "received", name: "Received", sort_order: 1, requires_document: false },
      { id: 2, code: "logged_in", name: "Logged in", sort_order: 2, requires_document: false },
    ] },
  });
}

function lastCall(spy: ReturnType<typeof mockApi>, prefix: string) {
  return spy.mock.calls.map(([u]) => String(u)).filter((u) => u.startsWith(prefix)).pop() ?? "";
}

describe("Reports", () => {
  it("defaults to This month and sends from/to to the tiles and the table", async () => {
    const spy = setup();
    render(<MemoryRouter><Reports /></MemoryRouter>);
    expect(await screen.findByRole("button", { name: "This month" })).toHaveAttribute("aria-pressed", "true");
    const today = istToday();
    const monthStart = `${today.slice(0, 8)}01`;
    await waitFor(() => {
      expect(lastCall(spy, "/api/reports/dashboard")).toContain(`from=${monthStart}`);
      expect(lastCall(spy, "/api/reports/dashboard")).toContain(`to=${today}`);
      expect(lastCall(spy, "/api/reports/rows")).toContain(`from=${monthStart}`);
    });
  });

  it("tiles show period counts with today below", async () => {
    setup();
    render(<MemoryRouter><Reports /></MemoryRouter>);
    const tile = await screen.findByRole("group", { name: "New customers" });
    expect(tile).toHaveTextContent("34");
    expect(tile).toHaveTextContent("4 today");
    expect(screen.getByRole("group", { name: "Sanctioned" })).toHaveTextContent("12");
    expect(screen.getByRole("group", { name: "Sanctioned" })).toHaveTextContent("1 dropped since");
    expect(screen.getByRole("group", { name: "Disbursed" })).toHaveTextContent("7");
  });

  it("lists open cases by stage with stage names", async () => {
    setup();
    render(<MemoryRouter><Reports /></MemoryRouter>);
    const section = await screen.findByRole("region", { name: "Open cases by stage" });
    await waitFor(() => expect(within(section).getByText("Logged in")).toBeInTheDocument());
    expect(within(section).getByText("14")).toBeInTheDocument();
  });

  it("links the Case ID cell to the case", async () => {
    setup();
    render(<MemoryRouter><Reports /></MemoryRouter>);
    expect(await screen.findByRole("link", { name: "1042" })).toHaveAttribute("href", "/cases/1042");
  });

  it("exports CSV with the current filters in the link", async () => {
    setup();
    render(<MemoryRouter><Reports /></MemoryRouter>);
    await userEvent.selectOptions(await screen.findByLabelText("Bank"), "2");
    await userEvent.click(screen.getByRole("button", { name: "Custom…" }));
    const from = screen.getByLabelText("From");
    await userEvent.clear(from);
    await userEvent.type(from, "2026-11-01");
    await waitFor(() => {
      const link = screen.getByRole("link", { name: "⬇ Export CSV" });
      expect(link.getAttribute("href")).toContain("/api/reports/export.csv?");
      expect(link.getAttribute("href")).toContain("bank_id=2");
      expect(link.getAttribute("href")).toContain("from=2026-11-01");
    });
  });

  it("a project filter goes to the tiles, the table and the CSV", async () => {
    const spy = setup();
    render(<MemoryRouter><Reports /></MemoryRouter>);
    await userEvent.type(await screen.findByRole("combobox", { name: "Project" }), "Pre");
    await userEvent.click(screen.getByRole("option", { name: "Prestige" }));
    await waitFor(() => expect(lastCall(spy, "/api/reports/dashboard")).toContain("project_id=4"));
    expect(lastCall(spy, "/api/reports/rows")).toContain("project_id=4");
    expect(screen.getByRole("link", { name: "⬇ Export CSV" }).getAttribute("href")).toContain("project_id=4");
  });

  it("Custom shows date inputs, presets hide them", async () => {
    setup();
    render(<MemoryRouter><Reports /></MemoryRouter>);
    await screen.findByRole("button", { name: "This month" });
    expect(screen.queryByLabelText("From")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Custom…" }));
    expect(screen.getByLabelText("From")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Today" }));
    expect(screen.queryByLabelText("From")).not.toBeInTheDocument();
  });
});

describe("presetRange", () => {
  it("This week starts on Monday, including when today is Sunday", async () => {
    const { presetRange } = await import("./Reports");
    expect(presetRange("week", "2026-11-18")).toEqual({ from: "2026-11-16", to: "2026-11-18" });
    expect(presetRange("week", "2026-11-22")).toEqual({ from: "2026-11-16", to: "2026-11-22" });
    expect(presetRange("week", "2026-11-16")).toEqual({ from: "2026-11-16", to: "2026-11-16" });
  });
});
