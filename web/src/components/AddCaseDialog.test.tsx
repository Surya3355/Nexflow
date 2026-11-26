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
      fromLead={fromLead ? { id: 9, projectId: 4, projectName: "Green Villas", assigneeId: 2, builderContactId: null } : undefined} />,
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

const villas = {
  id: 4, name: "Green Villas", is_system: false, open_cases: 0, total_cases: 0, type: "villas", address: "Road", active_contacts: 2,
  contacts: [
    { id: 21, name: "Ravi", phone: "9000000021", email: "", label: "Sales office" },
    { id: 22, name: "Suresh", phone: "9000000022", email: "", label: "Site office" },
  ],
};

describe("AddCaseDialog builder contact", () => {
  function setupContacts(builderContactId: number | null) {
    const spy = mockApi({
      "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
      "GET /api/users": { body: [{ id: 2, full_name: "Anil", username: "anil" }] },
      "GET /api/config/projects/4": { body: villas },
      "POST /api/leads/9/case": { status: 201, body: saved },
    });
    const onSaved = vi.fn();
    render(
      <AddCaseDialog open onClose={() => {}} onSaved={onSaved} customerName="Ravi Kumar"
        fromLead={{ id: 9, projectId: 4, projectName: "Green Villas", assigneeId: 2, builderContactId }} />,
    );
    return { spy, onSaved, dialog: screen.getByRole("dialog") };
  }

  async function fillAndSave(dialog: HTMLElement) {
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.type(await within(dialog).findByLabelText("Villa no."), "V-12");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
  }

  it("prefills the lead's contact and sends it", async () => {
    const { spy, dialog, onSaved } = setupContacts(22);
    const select = await within(dialog).findByLabelText("Builder contact");
    await waitFor(() => expect(select).toHaveValue("22"));
    await fillAndSave(dialog);
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/leads/9/case")).toMatchObject({ builder_contact_id: 22 });
  });

  it("uses the first active contact when the lead's contact was removed", async () => {
    const { spy, dialog, onSaved } = setupContacts(99);
    const select = await within(dialog).findByLabelText("Builder contact");
    await waitFor(() => expect(select).toHaveValue("21"));
    await fillAndSave(dialog);
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/leads/9/case")).toMatchObject({ builder_contact_id: 21 });
  });

  it("has no contact select on NA and sends no contact", async () => {
    const { spy, dialog, onSaved } = setup(false);
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.type(within(dialog).getByRole("combobox", { name: "Project" }), "NA");
    await userEvent.click(within(dialog).getByRole("option", { name: "NA" }));
    expect(within(dialog).queryByLabelText("Builder contact")).not.toBeInTheDocument();
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.selectOptions(within(dialog).getByLabelText("Assigned to for new case"), "2");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/cases")).not.toHaveProperty("builder_contact_id");
  });
});

describe("AddCaseDialog unit label", () => {
  function setupType(type: string | null, isSystem = false) {
    const project = { ...villas, type, is_system: isSystem, contacts: isSystem ? [] : villas.contacts };
    const spy = mockApi({
      "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
      "GET /api/users": { body: [{ id: 2, full_name: "Anil", username: "anil" }] },
      "GET /api/config/projects/4": { body: project },
      "POST /api/leads/9/case": { status: 201, body: saved },
    });
    const onSaved = vi.fn();
    render(
      <AddCaseDialog open onClose={() => {}} onSaved={onSaved} customerName="Ravi Kumar"
        fromLead={{ id: 9, projectId: 4, projectName: "Green Villas", assigneeId: 2, builderContactId: 21 }} />,
    );
    return { spy, onSaved, dialog: screen.getByRole("dialog") };
  }

  it("villas: labels Villa no. and requires it", async () => {
    const { spy, dialog } = setupType("villas");
    const unit = await within(dialog).findByLabelText("Villa no.");
    expect(unit).toHaveAttribute("placeholder", "e.g. A-1012");
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Enter the villa no.");
    expect(bodyOf(spy, "POST /api/leads/9/case")).toBeUndefined();
    await userEvent.type(unit, "a-1012");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/leads/9/case")).toMatchObject({ unit_number: "A-1012" }));
  });

  it("high-rise and apartments label Flat no., plots Plot no.", async () => {
    setupType("high_rise");
    expect(await screen.findByLabelText("Flat no.")).toBeInTheDocument();
  });

  it("plots label Plot no.", async () => {
    setupType("plots");
    expect(await screen.findByLabelText("Plot no.")).toBeInTheDocument();
  });

  it("untyped project keeps an optional Unit no.", async () => {
    const { spy, dialog, onSaved } = setupType(null);
    await within(dialog).findByLabelText("Builder contact");
    expect(within(dialog).getByLabelText("Unit no.")).toBeInTheDocument();
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/leads/9/case")).toMatchObject({ unit_number: "" });
  });

  it("a bank on the HDFC stage list has no Start at choice and starts at its first stage", async () => {
    const spy = mockApi({
      "GET /api/config/banks": { body: [{ id: 1, name: "HDFC", stage_list: "hdfc", part_payments: true }, { id: 2, name: "SBI", stage_list: "standard" }] },
      "GET /api/users": { body: [{ id: 2, full_name: "Anil", username: "anil" }] },
      "GET /api/config/projects": { body: [{ id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 }] },
      "POST /api/leads/9/case": { status: 201, body: saved },
    });
    const onSaved = vi.fn();
    render(<AddCaseDialog open onClose={() => {}} onSaved={onSaved} customerName="Ravi Kumar" customerId={5}
      fromLead={{ id: 9, projectId: 4, projectName: "Green Villas", assigneeId: 2, builderContactId: null }} />);
    const dialog = screen.getByRole("dialog");
    await within(dialog).findByRole("option", { name: "HDFC" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "2");
    expect(within(dialog).getByLabelText("Start at")).toBeInTheDocument();
    await userEvent.selectOptions(within(dialog).getByLabelText("Bank for new case"), "1");
    expect(within(dialog).queryByLabelText("Start at")).not.toBeInTheDocument();
    await userEvent.type(within(dialog).getByLabelText("Loan amount (₹)"), "100000");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open case" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/leads/9/case")).toMatchObject({ start_at: "", tracking_number: "" });
  });
});
