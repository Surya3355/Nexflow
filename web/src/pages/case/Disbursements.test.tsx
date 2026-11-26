import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Disbursements } from "./Disbursements";
import { bodyOf, mockApi } from "../../test/fetchMock";
import type { Case } from "../../api/types";

afterEach(() => vi.restoreAllMocks());

const base = {
  id: 11, customer_id: 5, bank_id: 1, project_id: 1, loan_amount: "3500000.00", current_stage_id: 41, assignee_id: 2,
  status: "active", created_at: "2026-11-01T10:00:00Z", part_payments: true,
  payments: [
    { id: 1, amount: "1000000.00", paid_on: "2026-11-05", kind: "part" },
    { id: 2, amount: "1500000.00", paid_on: "2026-11-18", kind: "part" },
  ],
  total_paid: "2500000.00", remaining: "1000000.00", fully_paid: false, over_paid: false,
} as Case;

function show(c: Case, atDisbursement = true, canCorrect = false, onChanged = vi.fn()) {
  render(<Disbursements caseData={c} atDisbursement={atDisbursement} canCorrect={canCorrect} onChanged={onChanged} />);
  return onChanged;
}

describe("Disbursements", () => {
  it("lists payments with total paid and remaining", () => {
    show(base);
    const block = screen.getByRole("region", { name: "Disbursements" });
    expect(within(block).getAllByRole("listitem")).toHaveLength(2);
    expect(block).toHaveTextContent("Part");
    expect(block).toHaveTextContent("₹10,00,000");
    expect(within(block).getByText("Total paid").nextSibling).toHaveTextContent("₹25,00,000");
    expect(within(block).getByText("Remaining").nextSibling).toHaveTextContent("₹10,00,000");
  });

  it("adds a payment at the disbursement stage", async () => {
    const spy = mockApi({ "POST /api/cases/11/payments": { body: base } });
    const changed = show(base);
    await userEvent.click(screen.getByRole("button", { name: "Add payment" }));
    await userEvent.click(screen.getByRole("radio", { name: "Full" }));
    await userEvent.type(screen.getByLabelText("Amount paid (₹)"), "1000000");
    const date = screen.getByLabelText("Payment date");
    await userEvent.clear(date);
    await userEvent.type(date, "2026-11-25");
    await userEvent.click(screen.getByRole("button", { name: "Save payment" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/payments")).toEqual({ amount: "1000000", date: "2026-11-25", kind: "full" }));
    expect(changed).toHaveBeenCalled();
  });

  it("hides Add payment once fully paid or away from the disbursement stage", () => {
    const { unmount } = render(<Disbursements caseData={{ ...base, fully_paid: true, status: "completed" }} atDisbursement canCorrect={false} onChanged={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Add payment" })).not.toBeInTheDocument();
    unmount();
    show(base, false);
    expect(screen.queryByRole("button", { name: "Add payment" })).not.toBeInTheDocument();
  });

  it("offers only Full when the bank has no part payments", async () => {
    show({ ...base, part_payments: false });
    await userEvent.click(screen.getByRole("button", { name: "Add payment" }));
    expect(screen.queryByRole("radio", { name: "Part" })).not.toBeInTheDocument();
  });

  it("edit and delete only for admin or manager", async () => {
    show(base, true, false);
    expect(screen.queryByRole("button", { name: /^Edit payment/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Delete payment/ })).not.toBeInTheDocument();
  });

  it("manager edits and deletes a payment", async () => {
    const spy = mockApi({
      "PATCH /api/cases/11/payments/1": { body: base },
      "DELETE /api/cases/11/payments/2": { body: base },
    });
    vi.spyOn(window, "confirm").mockReturnValue(true);
    show(base, true, true);
    await userEvent.click(screen.getByRole("button", { name: "Edit payment of 5 Nov 2026" }));
    const amount = screen.getByLabelText("Amount paid (₹)");
    await userEvent.clear(amount);
    await userEvent.type(amount, "900000");
    await userEvent.click(screen.getByRole("button", { name: "Save payment" }));
    await waitFor(() => expect(bodyOf(spy, "PATCH /api/cases/11/payments/1")).toEqual({ amount: "900000", date: "2026-11-05", kind: "part" }));
    await userEvent.click(screen.getByRole("button", { name: "Delete payment of 18 Nov 2026" }));
    await waitFor(() => expect(spy.mock.calls.some(([u, i]) => String(u).endsWith("/api/cases/11/payments/2") && i?.method === "DELETE")).toBe(true));
  });

  it("warns when total paid is more than the sanctioned amount", () => {
    show({ ...base, over_paid: true, remaining: "0.00" });
    expect(screen.getByText("Total paid is more than the sanctioned amount.")).toBeInTheDocument();
  });
});
