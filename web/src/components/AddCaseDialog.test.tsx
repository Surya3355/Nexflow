import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AddCaseDialog } from "./AddCaseDialog";
import { bodyOf, mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const saved = { id: 77, customer_id: 5, bank_id: 1, project_id: 4, loan_amount: "1", current_stage_id: 2, assignee_id: 2, status: "active", created_at: "" };

function setup(fromLead: boolean, post: { status?: number; body?: unknown } = { status: 201, body: saved }) {
  const spy = mockApi({
    "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
    "GET /api/users": { body: [{ id: 2, full_name: "Anil", username: "anil" }] },
    "GET /api/config/projects": { body: [{ id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 }] },
    "POST /api/leads/9/case": post,
    "POST /api/cases": post,
  });
  const onSaved = vi.fn();
  render(
    <AddCaseDialog open onClose={() => {}} onSaved={onSaved} customerName="Ravi Kumar" customerId={5}
      fromLead={fromLead ? { id: 9, projectName: "Green Villas", assigneeId: 2 } : undefined} />,
  );
  return { spy, onSaved, dialog: screen.getByRole("dialog") };
}

describe("AddCaseDialog", () => {
  it("from lead shows project as text, not a picker", () => {
    const { dialog } = setup(true);
    expect(within(dialog).getByText("Green Villas")).toBeInTheDocument();
    expect(within(dialog).queryByRole("combobox", { name: "Project" })).not.toBeInTheDocument();
  });

  it("Logged in asks for tracking number", async () => {
    const { spy, dialog, onSaved } = setup(true);
    await within(dialog).findByRole("option", { name: "HDFC" });
    expect(within(dialog).queryByLabelText("Tracking number")).not.toBeInTheDocument();
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.selectOptions(within(dialog).getByLabelText("Start at"), "logged_in");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Enter the tracking number.");
    await userEvent.type(within(dialog).getByLabelText("Tracking number"), "hl12");
    expect(within(dialog).getByLabelText("Tracking number")).toHaveValue("HL12");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/leads/9/case")).toMatchObject({ start_at: "logged_in", tracking_number: "HL12" });
  });

  it("double tap sends one request", async () => {
    let release: () => void = () => {};
    const { spy, dialog } = setup(true);
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    spy.mockImplementationOnce(() => new Promise<Response>((resolve) => {
      release = () => resolve(new Response(JSON.stringify(saved), { status: 201, headers: { "Content-Type": "application/json" } }));
    }));
    const save = within(dialog).getByRole("button", { name: "Open case" });
    await userEvent.click(save);
    await userEvent.click(save);
    expect(save).toBeDisabled();
    release();
    const posts = spy.mock.calls.filter(([u, init]) => String(u) === "/api/leads/9/case" && init?.method === "POST");
    expect(posts).toHaveLength(1);
  });

  it("without a lead asks for a project and posts to cases", async () => {
    const { spy, dialog } = setup(false);
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.selectOptions(within(dialog).getByLabelText("Assigned to for new case"), "2");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Choose a project, or NA.");
    await userEvent.type(within(dialog).getByRole("combobox", { name: "Project" }), "NA");
    await userEvent.click(within(dialog).getByRole("option", { name: "NA" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases")).toEqual({
      customer_id: 5, bank_id: 1, project_id: 1, loan_amount: "100000", assignee_id: 2, unit_number: "", start_at: "received", tracking_number: "",
    }));
  });

  it("asks for a bank and a person before sending", async () => {
    const { spy, dialog } = setup(false);
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.type(within(dialog).getByRole("combobox", { name: "Project" }), "NA");
    await userEvent.click(within(dialog).getByRole("option", { name: "NA" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Choose a bank.");
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Choose who it is assigned to.");
    expect(bodyOf(spy, "POST /api/cases")).toBeUndefined();
  });
});
