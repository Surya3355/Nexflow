import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { CustomerDetail } from "./CustomerDetail";
import { bodyOf, mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const caseRow = {
  id: 1042, customer_id: 5, bank_id: 1, loan_amount: "4500000.00", current_stage_id: 3, assignee_id: 7, status: "active",
  created_at: "2026-11-03T04:44:00Z", customer_name: "Ravi Kumar", customer_mobile: "9848012345", bank_name: "HDFC",
  stage_name: "Under sanction", stage_code: "under_sanction", stage_order: 3, stage_count: 6, assignee_name: "Priya",
  stage_since: "2026-11-12T04:00:00Z", needs_document: false, tracking_number: "HD88213", project_id: 4, project_name: "Prestige",
};

function setup(customer: Record<string, unknown>, extra: Record<string, { status?: number; body?: unknown }> = {}) {
  const spy = mockApi({
    "GET /api/customers/5": { body: customer },
    "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
    "GET /api/users": { body: [{ id: 7, full_name: "Priya", username: "priya" }] },
    "GET /api/config/projects": { body: [{ id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 }] },
    "GET /api/config/dropdowns/drop_reason": { body: [{ id: 1, value: "Customer not interested" }, { id: 9, value: "Other" }] },
    ...extra,
  });
  render(
    <MemoryRouter initialEntries={["/customers/5"]}>
      <Routes><Route path="/customers/:id" element={<CustomerDetail />} /></Routes>
    </MemoryRouter>,
  );
  return spy;
}

const ravi = { id: 5, name: "Ravi Kumar", mobile: "9848012345", email: "ravi@example.com", status: "active", cases: [caseRow] };

describe("Customer page", () => {
  it("shows name, contact details, a Call link and the cases", async () => {
    setup(ravi);
    expect(await screen.findByRole("heading", { name: "Ravi Kumar" })).toBeInTheDocument();
    expect(screen.getByText(/ravi@example.com/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Call" })).toHaveAttribute("href", "tel:9848012345");
    const link = screen.getByRole("link", { name: /HDFC · Prestige · Case #1042/ });
    expect(link).toHaveAttribute("href", "/cases/1042");
    expect(link).toHaveTextContent("Under sanction");
    expect(within(link).getByRole("img", { name: "Stage 3 of 6" })).toBeInTheDocument();
  });

  it("Edit saves name, mobile and email", async () => {
    const spy = setup(ravi, { "PUT /api/customers/5": { body: { ...ravi, name: "Ravi K" } } });
    await userEvent.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = screen.getByRole("dialog", { name: "Edit customer" });
    const name = within(dialog).getByLabelText("Name");
    await userEvent.clear(name);
    await userEvent.type(name, "Ravi K");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "PUT /api/customers/5")).toEqual({ name: "Ravi K", mobile: "9848012345", email: "ravi@example.com" }));
    expect(await screen.findByRole("heading", { name: "Ravi K" })).toBeInTheDocument();
  });

  it("Drop customer needs a reason, and a remark when the reason is Other", async () => {
    const spy = setup(ravi, { "POST /api/customers/5/drop": { body: { ...ravi, status: "dropped" } } });
    await userEvent.click(await screen.findByRole("button", { name: "Drop customer…" }));
    const dialog = screen.getByRole("dialog", { name: "Drop Ravi Kumar?" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Drop customer" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Choose a reason.");
    await userEvent.selectOptions(within(dialog).getByLabelText("Reason"), "9");
    await userEvent.click(within(dialog).getByRole("button", { name: "Drop customer" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("A remark is required when the reason is Other.");
    await userEvent.type(within(dialog).getByLabelText("Remark"), "Moved abroad");
    await userEvent.click(within(dialog).getByRole("button", { name: "Drop customer" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/customers/5/drop")).toEqual({ reason_id: 9, remark: "Moved abroad" }));
  });

  it("hides New case and Drop for a dropped customer", async () => {
    setup({ ...ravi, status: "dropped" });
    await screen.findByRole("heading", { name: "Ravi Kumar" });
    expect(screen.getByText("Dropped")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "+ New case" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Drop customer…" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bring back" })).toBeInTheDocument();
  });

  it("Bring back ignores a second tap while the first is running", async () => {
    let release: () => void = () => {};
    const spy = setup({ ...ravi, status: "dropped" });
    const button = await screen.findByRole("button", { name: "Bring back" });
    spy.mockImplementationOnce(() => new Promise<Response>((resolve) => {
      release = () => resolve(new Response(JSON.stringify({ ...ravi, status: "active" }), { status: 200, headers: { "Content-Type": "application/json" } }));
    }));
    await userEvent.click(button);
    expect(button).toBeDisabled();
    await userEvent.click(button);
    release();
    await waitFor(() => expect(spy.mock.calls.filter(([u]) => String(u) === "/api/customers/5/reactivate")).toHaveLength(1));
  });

  it("Bring back makes a dropped customer active again", async () => {
    const spy = setup({ ...ravi, status: "dropped" }, {
      "POST /api/customers/5/reactivate": { body: { ...ravi, status: "active" } },
    });
    await userEvent.click(await screen.findByRole("button", { name: "Bring back" }));
    await waitFor(() => expect(spy.mock.calls.some(([u, init]) => String(u) === "/api/customers/5/reactivate" && init?.method === "POST")).toBe(true));
  });
});
