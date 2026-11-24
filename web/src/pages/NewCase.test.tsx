import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { CustomerDetail } from "./CustomerDetail";
import { bodyOf, mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

describe("New case from a customer", () => {
  it("opens a case for this customer with the chosen bank, amount and assignee", async () => {
    const spy = mockApi({
      "GET /api/customers/5": { body: { id: 5, name: "Ravi", mobile: "9876543210", email: "", status: "active", cases: [] } },
      "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
      "GET /api/users": { body: [{ id: 3, full_name: "Asha", username: "asha" }] },
      "GET /api/config/projects": { body: [{ id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 }, { id: 4, name: "Prestige", is_system: false, open_cases: 0, total_cases: 0 }] },
      "POST /api/cases": { status: 201, body: { id: 21, customer_id: 5, bank_id: 1, loan_amount: "2500000.00", current_stage_id: 1, assignee_id: 3, status: "active", created_at: "2026-11-17T10:00:00Z" } },
    });
    render(
      <MemoryRouter initialEntries={["/customers/5"]}>
        <Routes>
          <Route path="/customers/:id" element={<CustomerDetail />} />
          <Route path="/cases/21" element={<p>Case 21 page</p>} />
        </Routes>
      </MemoryRouter>,
    );
    await userEvent.click(await screen.findByRole("button", { name: "+ New case" }));
    await userEvent.selectOptions(screen.getByLabelText("Bank for new case"), "1");
    await userEvent.type(screen.getByLabelText("Loan amount (₹)"), "2500000");
    await userEvent.selectOptions(screen.getByLabelText("Assigned to for new case"), "3");
    await userEvent.click(screen.getByRole("button", { name: "Open case" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose a project, or NA.");
    expect(bodyOf(spy, "POST /api/cases")).toBeUndefined();

    const dialog = screen.getByRole("dialog", { name: "New case" });
    await userEvent.type(within(dialog).getByRole("combobox", { name: "Project" }), "Pre");
    await userEvent.click(within(dialog).getByRole("option", { name: "Prestige" }));
    await userEvent.click(screen.getByRole("button", { name: "Open case" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases")).toEqual({
      customer_id: 5, bank_id: 1, project_id: 4, loan_amount: "2500000", assignee_id: 3, unit_number: "", start_at: "received", tracking_number: "",
    }));
    expect(await screen.findByText("Case 21 page")).toBeInTheDocument();
  });
});
