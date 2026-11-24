import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { LeadDetail } from "./LeadDetail";
import { bodyOf, mockApi } from "../test/fetchMock";
import type { LeadRow } from "../api/types";

afterEach(() => vi.restoreAllMocks());

function lead(over: Partial<LeadRow> = {}): LeadRow {
  return {
    id: 9, customer_id: 5, customer_name: "Ravi Kumar", customer_mobile: "9845011212", customer_email: "ravi@example.com",
    project_id: 4, project_name: "Green Villas", assignee_id: 2, assignee_name: "Anil", status: "open",
    next_call_date: "2026-11-24", last_response: null, last_contact_at: null, call_due: false, overdue_days: 0,
    created_at: "2026-11-22T04:00:00Z", converted_at: null, case_id: null, ...over,
  };
}

const contacts = [
  { id: 2, response: "call_back", remark: "Busy, call Monday", next_call_date: "2026-11-24", created_at: "2026-11-22T10:40:00Z", created_by: "Anil" },
  { id: 1, response: "not_reachable", remark: "Switched off", next_call_date: null, created_at: "2026-11-22T05:32:00Z", created_by: "Anil" },
];

function renderLead(routes: Record<string, { status?: number; body?: unknown }> = {}, l = lead()) {
  const spy = mockApi({
    "GET /api/leads/9": { body: { lead: l, contacts } },
    "GET /api/users": { body: [{ id: 2, full_name: "Anil", username: "anil" }, { id: 3, full_name: "Priya", username: "priya" }] },
    "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
    "GET /api/config/projects": { body: [{ id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 }, { id: 4, name: "Green Villas", is_system: false, open_cases: 0, total_cases: 0 }] },
    "GET /api/config/dropdowns/drop_reason": { body: [{ id: 7, value: "Customer not interested" }] },
    ...routes,
  });
  render(
    <MemoryRouter initialEntries={["/leads/9"]}>
      <Routes>
        <Route path="/leads/:id" element={<LeadDetail />} />
        <Route path="/cases/:id" element={<p>Case page</p>} />
      </Routes>
    </MemoryRouter>,
  );
  return spy;
}

describe("LeadDetail", () => {
  it("shows customer name as the heading", async () => {
    renderLead();
    const h = await screen.findByRole("heading", { level: 1, name: "Ravi Kumar" });
    expect(h.className).toContain("text-2xl");
    expect(screen.getByRole("link", { name: "9845011212" })).toHaveAttribute("href", "tel:9845011212");
    expect(screen.getByText("Green Villas")).toBeInTheDocument();
  });

  it("lists the contact log newest first", async () => {
    renderLead();
    const log = await screen.findByRole("list", { name: "Contact log" });
    const items = within(log).getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Call back later");
    expect(items[0]).toHaveTextContent("Busy, call Monday");
    expect(items[1]).toHaveTextContent("Not reachable");
  });

  it("logs a contact and shows it first", async () => {
    const spy = renderLead({ "POST /api/leads/9/contacts": { status: 201, body: lead({ last_response: "interested" }) } });
    await userEvent.click(await screen.findByRole("button", { name: "Log contact" }));
    const dialog = screen.getByRole("dialog", { name: "Log contact" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Interested" }));
    await userEvent.type(within(dialog).getByLabelText("Remark"), "Coming tomorrow");
    await userEvent.type(within(dialog).getByLabelText("Next call (optional)"), "2026-11-25");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/leads/9/contacts")).toEqual({ response: "interested", remark: "Coming tomorrow", next_call_date: "2026-11-25" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Log contact" })).not.toBeInTheDocument());
  });

  it("needs a response and a remark", async () => {
    const spy = renderLead();
    await userEvent.click(await screen.findByRole("button", { name: "Log contact" }));
    const dialog = screen.getByRole("dialog", { name: "Log contact" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Choose a response and write a remark.");
    expect(bodyOf(spy, "POST /api/leads/9/contacts")).toBeUndefined();
  });

  it("not interested offers drop and can be declined", async () => {
    const spy = renderLead({ "POST /api/leads/9/contacts": { status: 201, body: lead({ last_response: "not_interested" }) } });
    await userEvent.click(await screen.findByRole("button", { name: "Log contact" }));
    const dialog = screen.getByRole("dialog", { name: "Log contact" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Not interested" }));
    await userEvent.type(within(dialog).getByLabelText("Remark"), "Bought elsewhere");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    const drop = await screen.findByRole("dialog", { name: "Drop this lead?" });
    await userEvent.click(within(drop).getByRole("button", { name: "Cancel" }));
    expect(bodyOf(spy, "POST /api/leads/9/drop")).toBeUndefined();
    expect(screen.queryByRole("dialog", { name: "Drop this lead?" })).not.toBeInTheDocument();
  });

  it("Add case converts and opens the case", async () => {
    const spy = renderLead({ "POST /api/leads/9/case": { status: 201, body: { id: 77, customer_id: 5, bank_id: 1, project_id: 4, loan_amount: "4500000", current_stage_id: 1, assignee_id: 2, status: "active", created_at: "" } } });
    await userEvent.click(await screen.findByRole("button", { name: "Customer is in: Add case" }));
    const dialog = screen.getByRole("dialog", { name: "Add case for Ravi Kumar" });
    await userEvent.type(within(dialog).getByLabelText("Unit no."), "B-14");
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "4500000");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    expect(await screen.findByText("Case page")).toBeInTheDocument();
    expect(bodyOf(spy, "POST /api/leads/9/case")).toEqual({
      bank_id: 1, loan_amount: "4500000", assignee_id: 2, unit_number: "B-14", start_at: "received", tracking_number: "",
    });
  });

  it("New project adds another lead for the customer", async () => {
    const spy = renderLead({ "POST /api/customers/5/leads": { status: 201, body: lead({ id: 10, project_id: 1, project_name: "NA" }) } });
    await userEvent.click(await screen.findByRole("button", { name: "+ New project" }));
    const dialog = screen.getByRole("dialog", { name: "New project for Ravi Kumar" });
    await userEvent.type(within(dialog).getByRole("combobox", { name: "Project" }), "NA");
    await userEvent.click(within(dialog).getByRole("option", { name: "NA" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Save lead" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/customers/5/leads")).toEqual({ project_id: 1, assignee_id: 2, bring_back: false }));
  });

  it("dropped lead shows Bring back", async () => {
    const spy = renderLead({ "POST /api/leads/9/bring-back": { body: lead() } }, lead({ status: "dropped" }));
    await userEvent.click(await screen.findByRole("button", { name: "Bring back" }));
    await waitFor(() => expect(spy.mock.calls.some(([u, init]) => String(u) === "/api/leads/9/bring-back" && init?.method === "POST")).toBe(true));
    expect(screen.queryByRole("button", { name: "Log contact" })).not.toBeInTheDocument();
  });

  it("converted lead links to its case", async () => {
    renderLead({}, lead({ status: "converted", case_id: 77 }));
    expect(await screen.findByRole("link", { name: "Moved to case #77" })).toHaveAttribute("href", "/cases/77");
    expect(screen.queryByRole("button", { name: "Customer is in: Add case" })).not.toBeInTheDocument();
  });

  it("converted lead whose case was deleted says so", async () => {
    renderLead({}, lead({ status: "converted", case_id: null }));
    expect(await screen.findByText("Its case was deleted.")).toBeInTheDocument();
  });
});
