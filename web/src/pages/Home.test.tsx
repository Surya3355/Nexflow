import { afterEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { Home } from "./Home";
import { bodyOf, mockApi } from "../test/fetchMock";
import type { CaseRow } from "../api/types";
import type { SignedInUser } from "./Login";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

const me: SignedInUser = { id: 7, username: "priya", full_name: "Priya Sharma", role: "employee" };

function row(over: Partial<CaseRow>): CaseRow {
  return {
    id: 1042, customer_id: 1, bank_id: 1, project_id: 1, project_name: "NA", loan_amount: "4500000.00", current_stage_id: 3, assignee_id: 7,
    status: "active", created_at: "2026-11-03T04:44:00Z", customer_name: "Ravi Kumar", customer_mobile: "9848012345",
    bank_name: "HDFC", stage_name: "Under sanction", stage_code: "under_sanction", stage_order: 3, stage_count: 6,
    assignee_name: "Priya Sharma", stage_since: "2026-11-12T04:00:00Z", needs_document: false, ...over,
  };
}

const projects = [
  { id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 },
  { id: 4, name: "Prestige", is_system: false, open_cases: 0, total_cases: 0 },
];

/** Picks a project in the project box inside container. */
async function pickProject(container: HTMLElement, name: string) {
  await userEvent.type(within(container).getByRole("combobox", { name: "Project" }), name.slice(0, 3));
  await userEvent.click(within(container).getByRole("option", { name }));
}

const dashboard = {
  new_customers_today: 4, new_cases_today: 5, new_customers: 30, new_cases: 41, stage_moves_today: 7,
  cases_by_stage: {}, sanctions: { count: 12, amount: "0", dropped: 0 }, disbursements: { count: 0, amount: "0", dropped: 0 },
};

function setup(user: SignedInUser, rows: CaseRow[], extra: Record<string, { status?: number; body?: unknown }> = {}) {
  const spy = mockApi({
    "GET /api/cases": { body: rows },
    "GET /api/cases/attention": { body: [] },
    "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }, { id: 2, name: "SBI" }] },
    "GET /api/config/projects": { body: projects },
    "GET /api/users": { body: [{ id: 7, full_name: "Priya Sharma", username: "priya" }, { id: 8, full_name: "Suresh N", username: "suresh" }] },
    "GET /api/reports/dashboard": { body: dashboard },
    ...extra,
  });
  render(
    <MemoryRouter>
      <Routes>
        <Route path="/" element={<Home user={user} />} />
        <Route path="/cases/:id" element={<p>Case page</p>} />
        <Route path="/customers/:id" element={<p>Customer page</p>} />
      </Routes>
    </MemoryRouter>,
  );
  return spy;
}

const mine = row({ id: 1, customer_name: "Ravi Kumar", assignee_id: 7 });
const theirs = row({ id: 2, customer_name: "Lakshmi Reddy", assignee_id: 8, assignee_name: "Suresh N" });

describe("Home", () => {
  it("employee starts on My cases", async () => {
    setup(me, [mine, theirs]);
    expect(await screen.findByText("Ravi Kumar")).toBeInTheDocument();
    expect(screen.queryByText("Lakshmi Reddy")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "My cases · 1" })).toHaveAttribute("aria-pressed", "true");
  });

  it("manager starts on All cases", async () => {
    setup({ ...me, role: "manager" }, [mine, theirs]);
    expect(await screen.findByText("Lakshmi Reddy")).toBeInTheDocument();
    expect(screen.getByText("Ravi Kumar")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "All cases · 2" })).toHaveAttribute("aria-pressed", "true");
  });

  it("shows customer, bank, stage and amount in each row", async () => {
    setup(me, [mine]);
    const link = await screen.findByRole("link", { name: /Ravi Kumar/ });
    expect(link).toHaveAttribute("href", "/cases/1");
    expect(link).toHaveTextContent("HDFC");
    expect(link).toHaveTextContent("Under sanction");
    expect(link).toHaveTextContent("₹45,00,000");
    expect(within(link).getByRole("img", { name: "Stage 3 of 6" })).toBeInTheDocument();
  });

  it("marks a row whose stage needs documents", async () => {
    setup(me, [row({ id: 1, needs_document: true })]);
    expect(await screen.findByRole("link", { name: /Ravi Kumar/ })).toHaveTextContent("No documents");
  });

  it("Needs attention chip lists attention cases with their reason", async () => {
    setup(me, [mine], {
      "GET /api/cases/attention": { body: [{ ...row({ id: 9, customer_name: "Geetha V" }), reason: "In Received for 9 days" }] },
    });
    await userEvent.click(await screen.findByRole("button", { name: "Needs attention · 1" }));
    const list = screen.getByRole("list", { name: "Cases" });
    expect(within(list).getByText("Geetha V")).toBeInTheDocument();
    expect(within(list).getByText("In Received for 9 days")).toBeInTheDocument();
    expect(within(list).queryByText("Ravi Kumar")).not.toBeInTheDocument();
  });

  it("typing searches after a pause and shows grouped results", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const spy = setup(me, [mine], {
      "GET /api/search": {
        body: [{ customer: { id: 5, name: "Anitha S", mobile: "9988776655", status: "active" }, cases: [row({ id: 1019, stage_name: "Disbursed" })] }],
      },
    });
    await screen.findByText("Ravi Kumar");
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "anitha" } });
    expect(spy.mock.calls.some(([u]) => String(u).startsWith("/api/search"))).toBe(false);
    await act(async () => { vi.advanceTimersByTime(300); });
    const group = await screen.findByRole("region", { name: "Anitha S" });
    expect(within(group).getByRole("link", { name: /Case #1019/ })).toHaveAttribute("href", "/cases/1019");
    expect(spy.mock.calls.some(([u]) => String(u) === "/api/search?q=anitha")).toBe(true);
  });

  it("Show more reveals the next 50", async () => {
    const rows = Array.from({ length: 60 }, (_, i) => row({ id: i + 1, customer_name: `Customer ${i + 1}` }));
    setup(me, rows);
    const list = await screen.findByRole("list", { name: "Cases" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(50);
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(within(list).getAllByRole("listitem")).toHaveLength(60);
  });

  it("filters the case list by project and shows each case's project", async () => {
    const spy = setup(me, [row({ id: 1, customer_name: "Ravi Kumar", assignee_id: 7, project_name: "Prestige" })]);
    const list = await screen.findByRole("list", { name: "Cases" });
    expect(within(list).getByRole("link", { name: /Ravi Kumar/ })).toHaveTextContent("Prestige");
    await userEvent.type(screen.getByRole("combobox", { name: "Project" }), "Pre");
    await userEvent.click(screen.getByRole("option", { name: "Prestige" }));
    await waitFor(() => expect(spy.mock.calls.some(([u]) => String(u).startsWith("/api/cases?") && String(u).includes("project_id=4"))).toBe(true));
  });

  it("shows today's numbers", async () => {
    setup(me, [mine]);
    const today = await screen.findByRole("region", { name: "Today" });
    await waitFor(() => expect(within(today).getByText("Moved stage").nextSibling).toHaveTextContent("7"));
    expect(within(today).getByText("New customers").nextSibling).toHaveTextContent("4");
  });
});

describe("New customer panel", () => {
  async function open(extra: Record<string, { status?: number; body?: unknown }>) {
    const spy = setup(me, [], extra);
    await userEvent.click(await screen.findByRole("button", { name: "+ New customer" }));
    const panel = screen.getByRole("dialog", { name: "New customer" });
    await userEvent.type(within(panel).getByLabelText("Name"), "Imran");
    await userEvent.type(within(panel).getByLabelText("Mobile"), "7702088990");
    return { spy, panel };
  }

  it("creates the customer then the case and opens the case", async () => {
    const { spy, panel } = await open({
      "POST /api/customers": { status: 201, body: { customer: { id: 31, name: "Imran", mobile: "7702088990", email: "", status: "active" } } },
      "POST /api/cases": { status: 201, body: row({ id: 1051, customer_id: 31 }) },
    });
    await userEvent.selectOptions(within(panel).getByLabelText("Bank"), "1");
    await pickProject(panel, "Prestige");
    await userEvent.type(within(panel).getByLabelText("Loan amount (₹)"), "1800000");
    await userEvent.click(within(panel).getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Case page")).toBeInTheDocument();
    expect(bodyOf(spy, "POST /api/cases")).toEqual({ customer_id: 31, bank_id: 1, project_id: 4, loan_amount: "1800000", assignee_id: 7, bring_back: false });
  });

  it("asks for a project when a bank is chosen", async () => {
    const { spy, panel } = await open({});
    await userEvent.selectOptions(within(panel).getByLabelText("Bank"), "1");
    await userEvent.type(within(panel).getByLabelText("Loan amount (₹)"), "1800000");
    await userEvent.click(within(panel).getByRole("button", { name: "Save" }));
    expect(await within(panel).findByRole("alert")).toHaveTextContent("Choose a project, or NA.");
    expect(bodyOf(spy, "POST /api/customers")).toBeUndefined();
  });

  it("keeps the customer when the case fails", async () => {
    const { panel } = await open({
      "POST /api/customers": { status: 201, body: { customer: { id: 31, name: "Imran", mobile: "7702088990", email: "", status: "active" } } },
      "POST /api/cases": { status: 400, body: { error: "cases: loan amount must be a positive number" } },
    });
    await userEvent.selectOptions(within(panel).getByLabelText("Bank"), "1");
    await pickProject(panel, "NA");
    await userEvent.type(within(panel).getByLabelText("Loan amount (₹)"), "abc");
    await userEvent.click(within(panel).getByRole("button", { name: "Save" }));
    const msg = await within(panel).findByRole("alert");
    expect(msg).toHaveTextContent("Imran was saved, but the case was not opened");
    expect(within(msg).getByRole("link", { name: "Open Imran" })).toHaveAttribute("href", "/customers/31");
  });

  async function existing(status: "active" | "dropped", withBank: boolean, extra: Record<string, { status?: number; body?: unknown }> = {}) {
    const r = await open({
      "POST /api/customers": { body: { existing: { id: 3, name: "Ravi Kumar", mobile: "7702088990", email: "", status } } },
      ...extra,
    });
    if (withBank) {
      await userEvent.selectOptions(within(r.panel).getByLabelText("Bank"), "1");
      await pickProject(r.panel, "Prestige");
      await userEvent.type(within(r.panel).getByLabelText("Loan amount (₹)"), "1800000");
    }
    await userEvent.click(within(r.panel).getByRole("button", { name: "Save" }));
    return r;
  }

  it("an existing mobile offers to add the case to that customer", async () => {
    const { spy, panel } = await existing("active", true, { "POST /api/cases": { status: 201, body: row({ id: 1060, customer_id: 3 }) } });
    const note = await within(panel).findByRole("status");
    expect(note).toHaveTextContent("This mobile already belongs to Ravi Kumar");
    expect(within(note).getByRole("link", { name: "Ravi Kumar" })).toHaveAttribute("href", "/customers/3");
    expect(bodyOf(spy, "POST /api/cases")).toBeUndefined();
    await userEvent.click(within(panel).getByRole("button", { name: "Add this case to Ravi Kumar" }));
    expect(await screen.findByText("Case page")).toBeInTheDocument();
    expect(bodyOf(spy, "POST /api/cases")).toEqual({ customer_id: 3, bank_id: 1, project_id: 4, loan_amount: "1800000", assignee_id: 7, bring_back: false });
  });

  it("an existing mobile without a bank links to the customer", async () => {
    const { panel } = await existing("active", false);
    expect(await within(panel).findByRole("link", { name: "Open Ravi Kumar" })).toHaveAttribute("href", "/customers/3");
  });

  it("a dropped customer can be brought back with the case", async () => {
    const { spy, panel } = await existing("dropped", true, { "POST /api/cases": { status: 201, body: row({ id: 1061, customer_id: 3 }) } });
    await userEvent.click(await within(panel).findByRole("button", { name: "Bring back Ravi Kumar and add this case" }));
    expect(await screen.findByText("Case page")).toBeInTheDocument();
    expect(bodyOf(spy, "POST /api/cases")).toMatchObject({ customer_id: 3, bring_back: true });
  });

  it("a dropped customer can be brought back without a case", async () => {
    const { spy, panel } = await existing("dropped", false, {
      "POST /api/customers/3/reactivate": { body: { id: 3, name: "Ravi Kumar", mobile: "7702088990", email: "", status: "active" } },
    });
    await userEvent.click(await within(panel).findByRole("button", { name: "Bring back Ravi Kumar" }));
    expect(await screen.findByText("Customer page")).toBeInTheDocument();
    expect(spy.mock.calls.some(([u, init]) => String(u) === "/api/customers/3/reactivate" && init?.method === "POST")).toBe(true);
  });

  it("keeps what was typed when adding to the existing customer fails, and blocks double taps", async () => {
    let release: () => void = () => {};
    const { spy, panel } = await existing("active", true);
    spy.mockImplementationOnce(() => new Promise<Response>((resolve) => {
      release = () => resolve(new Response(JSON.stringify({ error: "cases: bank has no Received stage" }), { status: 400, headers: { "Content-Type": "application/json" } }));
    }));
    const add = await within(panel).findByRole("button", { name: "Add this case to Ravi Kumar" });
    await userEvent.click(add);
    expect(add).toBeDisabled();
    release();
    expect(await within(panel).findByRole("alert")).toHaveTextContent("Bank has no Received stage.");
    expect(within(panel).getByLabelText("Loan amount (₹)")).toHaveValue("1800000");
  });

  it("saves just the customer when bank is empty", async () => {
    const { spy, panel } = await open({
      "POST /api/customers": { status: 201, body: { customer: { id: 31, name: "Imran", mobile: "7702088990", email: "", status: "active" } } },
    });
    await userEvent.click(within(panel).getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Customer page")).toBeInTheDocument();
    expect(bodyOf(spy, "POST /api/cases")).toBeUndefined();
  });
});

describe("Home filters and search (carried from the old Cases and Search pages)", () => {
  it("applies the bank and assignee filters together and keeps dropped cases visible", async () => {
    const spy = setup({ ...me, role: "manager" }, [row({ id: 21, status: "dropped", customer_name: "Venkat Rao" })]);
    expect(await screen.findByRole("link", { name: /Venkat Rao/ })).toHaveTextContent("Dropped");
    await userEvent.selectOptions(screen.getByLabelText("Bank"), "1");
    await userEvent.selectOptions(screen.getByLabelText("Assigned to"), "8");
    await waitFor(() => {
      const last = spy.mock.calls.map(([u]) => String(u)).filter((u) => u.startsWith("/api/cases?")).pop();
      expect(last).toContain("bank_id=1");
      expect(last).toContain("assignee_id=8");
    });
  });

  it("explains when a search matches nothing", async () => {
    setup(me, [mine], { "GET /api/search": { body: [] } });
    await screen.findByText("Ravi Kumar");
    await userEvent.type(screen.getByRole("searchbox"), "zzz");
    expect(await screen.findByText("No customers or cases match.")).toBeInTheDocument();
  });
});
