import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BankQueries } from "./BankQueries";
import { bodyOf, mockApi } from "../../test/fetchMock";
import type { BankQuery, CaseQueries } from "../../api/types";

afterEach(() => vi.restoreAllMocks());

const open: BankQuery = {
  id: 5, number: 1, body: "Latest 3 months salary slips", raised_on: "2026-11-20", due_on: "2026-11-21",
  assignee: { id: 2, name: "Ravi" }, status: "open", proof: null, events: [],
};
const sent: BankQuery = {
  id: 6, number: 2, body: "Builder NOC copy", raised_on: "2026-11-22", due_on: null,
  assignee: { id: 3, name: "Anita" }, status: "sent",
  proof: { note: "Emailed NOC to DOVR", document: { id: 40, label: "Query 2 proof", file_name: "noc.pdf" }, at: "2026-11-25T06:00:00Z", by: "Anita" },
  events: [],
};
const resolved: BankQuery = { ...open, id: 7, number: 3, body: "PAN card copy", status: "resolved", due_on: null };
const data: CaseQueries = { queries: [open, sent, resolved], open_count: 2, events: [] };

function show(props: Partial<Parameters<typeof BankQueries>[0]> = {}) {
  const onChanged = vi.fn();
  const onOpenDocument = vi.fn();
  render(<BankQueries caseId={11} data={data} readOnly={false} onChanged={onChanged} onOpenDocument={onOpenDocument} {...props} />);
  return { onChanged, onOpenDocument, box: screen.getByRole("region", { name: "Bank queries" }) };
}

describe("BankQueries", () => {
  it("lists queries with their status, count and the right buttons", () => {
    const { box } = show();
    expect(within(box).getByText("2 open")).toBeInTheDocument();
    expect(within(box).getByText("Latest 3 months salary slips")).toBeInTheDocument();
    expect(within(box).getByText("Open")).toBeInTheDocument();
    expect(within(box).getByText("Sent to bank")).toBeInTheDocument();
    expect(within(box).getByText(/Due 21 Nov, overdue/)).toBeInTheDocument();
    expect(within(box).getByRole("button", { name: "Send proof" })).toBeInTheDocument();
    expect(within(box).getByRole("button", { name: "Bank accepted" })).toBeInTheDocument();
    expect(within(box).getByRole("button", { name: "Bank rejected" })).toBeInTheDocument();
    expect(within(box).getByText("Resolved (1)")).toBeInTheDocument();
  });

  it("hides the proof until View proof is clicked, and opens the file", async () => {
    const { box, onOpenDocument } = show();
    expect(within(box).queryByText(/Emailed NOC to DOVR/)).not.toBeVisible();
    await userEvent.click(within(box).getByText("View proof"));
    expect(within(box).getByText(/Emailed NOC to DOVR/)).toBeVisible();
    await userEvent.click(within(box).getByRole("button", { name: "noc.pdf" }));
    expect(onOpenDocument).toHaveBeenCalledWith(40);
  });

  it("adds a query", async () => {
    const spy = mockApi({
      "GET /api/users": { body: [{ id: 2, username: "ravi", full_name: "Ravi", role: "employee", is_active: true }] },
      "POST /api/cases/11/queries": { status: 201, body: open },
    });
    const { onChanged } = show();
    await userEvent.click(screen.getByRole("button", { name: "+ Add query" }));
    const dialog = screen.getByRole("dialog", { name: "Add bank query" });
    await within(dialog).findByRole("option", { name: "Ravi" });
    await userEvent.type(within(dialog).getByLabelText("What did the bank ask for?"), "PAN copy");
    await userEvent.selectOptions(within(dialog).getByLabelText("Assign to"), "2");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add query" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const body = bodyOf(spy, "POST /api/cases/11/queries") as Record<string, unknown>;
    expect(body).toMatchObject({ body: "PAN copy", assignee_id: 2, due_on: null });
    expect(body.raised_on).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  it("send proof needs a note or a file, then posts a form", async () => {
    const spy = mockApi({ "POST /api/queries/5/send": { body: { ...open, status: "sent" } } });
    const { box, onChanged } = show();
    await userEvent.click(within(box).getByRole("button", { name: "Send proof" }));
    const dialog = screen.getByRole("dialog", { name: "Send proof to bank" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Mark sent to bank" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Add a note, a file, or both.");
    expect(spy).not.toHaveBeenCalled();
    await userEvent.type(within(dialog).getByLabelText("Note"), "Uploaded on portal");
    await userEvent.click(within(dialog).getByRole("button", { name: "Mark sent to bank" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const form = spy.mock.calls[0][1]?.body as FormData;
    expect(form.get("note")).toBe("Uploaded on portal");
  });

  it("bank rejected asks for the reason", async () => {
    const spy = mockApi({ "POST /api/queries/6/reject": { body: { ...sent, status: "open", proof: null } } });
    const { box, onChanged } = show();
    await userEvent.click(within(box).getByRole("button", { name: "Bank rejected" }));
    const dialog = screen.getByRole("dialog", { name: "Bank rejected the proof" });
    await userEvent.type(within(dialog).getByLabelText("What did the bank say?"), "Needs letterhead");
    await userEvent.click(within(dialog).getByRole("button", { name: "Reopen query" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/queries/6/reject")).toEqual({ reason: "Needs letterhead" });
  });

  it("accept, remove and remove proof ask first", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const spy = mockApi({
      "POST /api/queries/6/accept": { body: { ...sent, status: "resolved" } },
      "DELETE /api/queries/5": { status: 204 },
      "POST /api/queries/6/remove-proof": { body: { ...sent, status: "open", proof: null } },
    });
    const { box, onChanged } = show();
    await userEvent.click(within(box).getByRole("button", { name: "Bank accepted" }));
    await userEvent.click(within(box).getByRole("button", { name: "Remove Latest 3 months salary slips" }));
    await userEvent.click(within(box).getByText("View proof"));
    await userEvent.click(within(box).getByRole("button", { name: "Remove proof" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(3));
    expect(confirm).toHaveBeenCalledTimes(3);
    const keys = spy.mock.calls.map(([u, i]) => `${i?.method} ${new URL(String(u), "http://x").pathname}`);
    expect(keys).toEqual(["POST /api/queries/6/accept", "DELETE /api/queries/5", "POST /api/queries/6/remove-proof"]);
  });

  it("a query sent to the bank can still be edited or removed", async () => {
    const spy = mockApi({ "DELETE /api/queries/6": { status: 204 } });
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const { box, onChanged } = show();
    expect(within(box).getAllByRole("button", { name: "Edit" })).toHaveLength(2);
    await userEvent.click(within(box).getByRole("button", { name: "Remove Builder NOC copy" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    expect(spy.mock.calls.some(([url, init]) => String(url).endsWith("/api/queries/6") && init?.method === "DELETE")).toBe(true);
  });

  it("says File deleted when the proof file is gone", async () => {
    const gone: BankQuery = { ...sent, proof: { ...sent.proof!, document: null, file_deleted: true, file_name: "noc.pdf" } };
    const { box } = show({ data: { queries: [gone], open_count: 1, events: [] } });
    await userEvent.click(within(box).getByText("View proof"));
    expect(within(box).getByText("noc.pdf · File deleted")).toBeVisible();
    expect(within(box).queryByRole("button", { name: "noc.pdf" })).not.toBeInTheDocument();
  });

  it("asks to choose an assignee before adding", async () => {
    const spy = mockApi({ "GET /api/users": { body: [] } });
    show();
    await userEvent.click(screen.getByRole("button", { name: "+ Add query" }));
    const dialog = screen.getByRole("dialog", { name: "Add bank query" });
    await userEvent.type(within(dialog).getByLabelText("What did the bank ask for?"), "PAN");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add query" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Choose who the query is assigned to.");
    expect(spy.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
  });

  it("read-only shows no buttons", () => {
    const { box } = show({ readOnly: true });
    expect(within(box).queryByRole("button", { name: /Send proof|Bank accepted|Add query|Remove/ })).not.toBeInTheDocument();
  });
});
