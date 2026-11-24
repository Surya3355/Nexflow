import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { CaseDetail } from "./CaseDetail";
import { bodyOf, mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const standard = [
  { id: 1, code: "received", name: "Received", sort_order: 1, requires_document: false },
  { id: 2, code: "logged_in", name: "Logged in", sort_order: 2, requires_document: false },
  { id: 3, code: "under_sanction", name: "Under sanction", sort_order: 3, requires_document: false },
  { id: 4, code: "sanctioned", name: "Sanctioned", sort_order: 4, requires_document: true },
  { id: 5, code: "documentation", name: "Documentation", sort_order: 5, requires_document: false },
  { id: 6, code: "disbursed", name: "Disbursed", sort_order: 6, requires_document: false },
];

const aCase: Record<string, unknown> & { id: number } = {
  id: 11, customer_id: 5, bank_id: 1, loan_amount: "2500000.00",
  current_stage_id: 1, assignee_id: 2, status: "active", created_at: "2026-11-12T10:00:00Z", project_id: 1,
};

const history = [
  { id: 1, reached_at: "2026-11-12T10:00:00Z", stage_code: "received", stage_name: "Received", changed_by: "Surya" },
  { id: 2, reached_at: "2026-11-13T09:30:00Z", stage_code: "logged_in", stage_name: "Logged in", changed_by: "Asha" },
  { id: 3, reached_at: "2026-11-14T16:45:00Z", stage_code: "received", stage_name: "Received", changed_by: "Asha" },
];

const customer = {
  id: 5, name: "Ravi Kumar", mobile: "9848012345", email: "", status: "active",
  cases: [{ ...aCase, bank_name: "HDFC", project_name: "NA", assignee_name: "Asha", customer_name: "Ravi Kumar" }],
};

function renderCase(routes: Record<string, { status?: number; body?: unknown }>, stages = standard, theCase = aCase) {
  const spy = mockApi({
    "GET /api/cases/11": { body: theCase },
    "GET /api/cases/11/history": { body: history },
    "GET /api/config/banks/1/stages": { body: stages },
    "GET /api/config/dropdowns/drop_reason": { body: [{ id: 7, value: "Not eligible" }, { id: 9, value: "Other" }] },
    "GET /api/cases/11/documents": { body: [] },
    "GET /api/customers/5": { body: customer },
    "GET /api/users": { body: [{ id: 2, full_name: "Asha", username: "asha" }, { id: 3, full_name: "Divya", username: "divya" }] },
    "GET /api/config/projects": { body: [{ id: 1, name: "NA", is_system: true, open_cases: 0, total_cases: 0 }, { id: 4, name: "Prestige", is_system: false, open_cases: 0, total_cases: 0 }] },
    ...routes,
  });
  render(
    <MemoryRouter initialEntries={["/cases/11"]}>
      <Routes><Route path="/cases/:id" element={<CaseDetail />} /></Routes>
    </MemoryRouter>,
  );
  return spy;
}

async function ready() {
  return screen.findByRole("button", { name: /^Move to / });
}

describe("CaseDetail", () => {
  it("case heading is the customer name", async () => {
    renderCase({});
    const h = await screen.findByRole("heading", { level: 1, name: "Ravi Kumar" });
    expect(h.className).toContain("text-2xl");
    expect(within(h).getByRole("link", { name: "Ravi Kumar" })).toHaveAttribute("href", "/customers/5");
    expect(screen.getByRole("link", { name: "9848012345" })).toHaveAttribute("href", "tel:9848012345");
    expect(screen.getByText(/HDFC · Case #11/)).toBeInTheDocument();
    expect(screen.getByText("₹25,00,000")).toBeInTheDocument();
  });

  it("project shown once, in Details", async () => {
    renderCase({});
    await screen.findByRole("heading", { level: 1, name: "Ravi Kumar" });
    const details = screen.getByRole("region", { name: "Details" });
    expect(within(details).getByText("NA")).toBeInTheDocument();
    expect(screen.getAllByText("NA")).toHaveLength(1);
  });

  it("unit no. can be changed", async () => {
    const spy = renderCase({ "POST /api/cases/11/unit": { body: { ...aCase, unit_number: "B-14" } } }, standard, { ...aCase, unit_number: "A-1" });
    const details = await screen.findByRole("region", { name: "Details" });
    expect(within(details).getByText("A-1")).toBeInTheDocument();
    await userEvent.click(within(details).getByRole("button", { name: "Change unit no." }));
    const dialog = screen.getByRole("dialog", { name: "Change unit no." });
    const field = within(dialog).getByLabelText("Unit no.");
    await userEvent.clear(field);
    await userEvent.type(field, "b-14");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/unit")).toEqual({ unit_number: "B-14" }));
  });

  it("contact log shows for a case from a lead", async () => {
    renderCase({
      "GET /api/leads/9": { body: { lead: {}, contacts: [{ id: 1, response: "interested", remark: "Coming tomorrow", next_call_date: null, created_at: "2026-11-22T13:00:00Z", created_by: "Anil" }] } },
    }, standard, { ...aCase, lead_id: 9 });
    const log = await screen.findByRole("list", { name: "Contact log" });
    expect(log).toHaveTextContent("Coming tomorrow");
  });

  it("New case for Ravi opens the form", async () => {
    renderCase({ "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] } });
    await userEvent.click(await screen.findByRole("button", { name: "+ New case for Ravi" }));
    expect(screen.getByRole("dialog", { name: "New case" })).toBeInTheDocument();
  });

  it("shows the stage history in order, with each date", async () => {
    renderCase({});
    const list = await screen.findByRole("list", { name: "Stage history" });
    const items = within(list).getAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveTextContent("Received");
    expect(items[1]).toHaveTextContent("Logged in");
    expect(items[1]).toHaveTextContent("Asha");
    expect(items[1].textContent).toMatch(/2026/);
  });

  it("shows every stage of the bank as a step, marking the current one", async () => {
    renderCase({}, standard, { ...aCase, current_stage_id: 3 });
    const steps = await screen.findByRole("list", { name: "Progress" });
    const items = within(steps).getAllByRole("listitem");
    expect(items).toHaveLength(6);
    expect(items[2]).toHaveAttribute("aria-current", "step");
    expect(items[2]).toHaveTextContent("Under sanction");
  });

  it("Move to next stage button names the next stage", async () => {
    renderCase({}, standard, { ...aCase, current_stage_id: 3 });
    expect(await ready()).toHaveTextContent("Move to Sanctioned →");
  });

  it("uses the bank's own stages", async () => {
    const own = [
      { id: 21, code: "received", name: "Received", sort_order: 1, requires_document: false },
      { id: 22, code: "bank_review", name: "Bank review", sort_order: 2, requires_document: false },
      { id: 23, code: "disbursed", name: "Disbursed", sort_order: 3, requires_document: false },
    ];
    renderCase({}, own, { ...aCase, current_stage_id: 21 });
    expect(await ready()).toHaveTextContent("Move to Bank review →");
    expect(within(screen.getByRole("list", { name: "Progress" })).getAllByRole("listitem")).toHaveLength(3);
  });

  it("asks for a tracking number when the case moves to Logged in", async () => {
    const spy = renderCase({ "POST /api/cases/11/stage": { body: { ...aCase, current_stage_id: 2 } } });
    await userEvent.click(await ready());
    const dialog = screen.getByRole("dialog", { name: "Move to Logged in" });
    await userEvent.type(within(dialog).getByLabelText("Tracking number"), "hd1");
    expect(within(dialog).getByLabelText("Tracking number")).toHaveValue("HD1");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm move" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/stage")).toEqual({ stage_id: 2, tracking_number: "HD1" }));
  });

  it("refuses a tracking number with symbols", async () => {
    const spy = renderCase({ "POST /api/cases/11/stage": { body: { ...aCase, current_stage_id: 2 } } });
    await userEvent.click(await ready());
    const dialog = screen.getByRole("dialog", { name: "Move to Logged in" });
    await userEvent.type(within(dialog).getByLabelText("Tracking number"), "ab-12");
    expect(within(dialog).getByLabelText("Tracking number")).toHaveValue("AB-12");
    expect(within(dialog).getByText("Tracking number can contain only letters and numbers.")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Confirm move" })).toBeDisabled();
    expect(bodyOf(spy, "POST /api/cases/11/stage")).toBeUndefined();
  });

  it("a disbursed case can still change project", async () => {
    renderCase({}, standard, { ...aCase, status: "completed", current_stage_id: 6 });
    await screen.findByRole("heading", { level: 1, name: "Ravi Kumar" });
    expect(screen.getByRole("button", { name: "Change project" })).toBeInTheDocument();
  });

  it("a dropped case cannot change project", async () => {
    renderCase({}, standard, { ...aCase, status: "dropped" });
    await screen.findByRole("heading", { level: 1, name: "Ravi Kumar" });
    expect(screen.queryByRole("button", { name: "Change project" })).not.toBeInTheDocument();
  });

  it("shows the project and changes it", async () => {
    const spy = renderCase({
      "POST /api/cases/11/project": { body: { ...aCase, project_id: 4 } },
    });
    expect(await screen.findByRole("heading", { level: 1, name: "Ravi Kumar" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Change project" }));
    const dialog = screen.getByRole("dialog", { name: "Change project" });
    await userEvent.type(within(dialog).getByRole("combobox", { name: "Project" }), "Pre");
    await userEvent.click(within(dialog).getByRole("option", { name: "Prestige" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/project")).toEqual({ project_id: 4 }));
  });

  it("asks for sanction details when the case moves to Sanctioned and sends them", async () => {
    const spy = renderCase({ "POST /api/cases/11/stage": { body: { ...aCase, current_stage_id: 4 } } }, standard, { ...aCase, current_stage_id: 3 });
    await userEvent.click(await ready());
    const dialog = screen.getByRole("dialog", { name: "Move to Sanctioned" });
    expect(within(dialog).getByLabelText("Sanctioned amount (₹)")).toHaveValue("2500000.00");
    await userEvent.type(within(dialog).getByLabelText("Interest rate (%)"), "8.5");
    await userEvent.type(within(dialog).getByLabelText("Tenure (months)"), "240");
    const date = within(dialog).getByLabelText("Sanction date");
    await userEvent.clear(date);
    await userEvent.type(date, "2026-11-18");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm move" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/stage")).toEqual({
      stage_id: 4,
      sanction: { amount: "2500000.00", rate_percent: "8.5", interest_type: "floating", tenure_months: 240, date: "2026-11-18" },
    }));
  });

  it("asks for disbursed amount and date when the case moves to Disbursed", async () => {
    const spy = renderCase({ "POST /api/cases/11/stage": { body: { ...aCase, current_stage_id: 6, status: "completed" } } }, standard, { ...aCase, current_stage_id: 5 });
    await userEvent.click(await ready());
    const dialog = screen.getByRole("dialog", { name: "Move to Disbursed" });
    const date = within(dialog).getByLabelText("Disbursement date");
    await userEvent.clear(date);
    await userEvent.type(date, "2026-11-18");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm move" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/stage")).toEqual({
      stage_id: 6, disbursement: { amount: "2500000.00", date: "2026-11-18" },
    }));
  });

  it("Other stage lets you pick any stage, including going back", async () => {
    const spy = renderCase({ "POST /api/cases/11/stage": { body: { ...aCase, current_stage_id: 1 } } }, standard, { ...aCase, current_stage_id: 3, tracking_number: "HD1" });
    await ready();
    await userEvent.click(screen.getByRole("button", { name: "Other stage…" }));
    const dialog = screen.getByRole("dialog", { name: "Move to another stage" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Move to stage"), "Received");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm move" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/stage")).toEqual({ stage_id: 1 }));
  });

  it("shows a document warning as a status, not an error, when a required stage has no file", async () => {
    renderCase({
      "POST /api/cases/11/stage": {
        body: { ...aCase, current_stage_id: 2, warning: "This stage normally needs at least one document. None is uploaded yet." },
      },
    });
    await userEvent.click(await ready());
    const dialog = screen.getByRole("dialog", { name: "Move to Logged in" });
    await userEvent.type(within(dialog).getByLabelText("Tracking number"), "HD1");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm move" }));
    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("needs at least one document");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("change assignee posts to /assign", async () => {
    const spy = renderCase({ "POST /api/cases/11/assign": { body: { ...aCase, assignee_id: 3 } } });
    await ready();
    await userEvent.click(screen.getByRole("button", { name: "Change assignee" }));
    const dialog = screen.getByRole("dialog", { name: "Assign case" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Assigned to"), "3");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodyOf(spy, "POST /api/cases/11/assign")).toEqual({ assignee_id: 3 }));
  });

  it("requires a remark when the drop reason is Other, and does not send the drop", async () => {
    const spy = renderCase({ "POST /api/cases/11/drop": { body: { ...aCase, status: "dropped" } } });
    await ready();
    await userEvent.click(screen.getByRole("button", { name: "Drop this case…" }));
    const dialog = screen.getByRole("dialog", { name: "Drop case #11?" });
    await userEvent.selectOptions(within(dialog).getByLabelText("Reason"), "Other");
    await userEvent.click(within(dialog).getByRole("button", { name: "Drop case" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("A remark is required");
    expect(bodyOf(spy, "POST /api/cases/11/drop")).toBeUndefined();
  });

  it("has no move or drop controls on a dropped case", async () => {
    renderCase({}, standard, { ...aCase, status: "dropped" });
    await screen.findByRole("list", { name: "Stage history" });
    expect(screen.queryByRole("button", { name: /^Move to / })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Other stage…" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Drop this case…" })).not.toBeInTheDocument();
  });
});

describe("CaseDetail documents", () => {
  it("uploads a file under a label to the case's document endpoint", async () => {
    const spy = renderCase({
      "POST /api/cases/11/documents": { status: 201, body: { id: 90, label: "PAN", file_name: "pan.pdf", content_type: "application/pdf", size_bytes: 10, stage_id: 1, uploaded_at: "2026-11-17T10:00:00Z" } },
    });
    await ready();
    await userEvent.click(screen.getByRole("button", { name: "+ Upload" }));
    const dialog = screen.getByRole("dialog", { name: "Upload a document" });
    await userEvent.type(within(dialog).getByLabelText("Document label"), "PAN");
    const file = new File(["%PDF-1.4 test"], "pan.pdf", { type: "application/pdf" });
    await userEvent.upload(within(dialog).getByLabelText("File"), file);
    await userEvent.click(within(dialog).getByRole("button", { name: "Upload" }));
    await waitFor(() => {
      const call = spy.mock.calls.find(([u, init]) => String(u) === "/api/cases/11/documents" && init?.method === "POST");
      expect(call).toBeDefined();
      expect(call?.[1]?.body).toBeInstanceOf(FormData);
    });
  });
});
