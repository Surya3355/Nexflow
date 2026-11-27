import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ProjectForm } from "./ProjectForm";
import { bodyOf, mockApi } from "../../test/fetchMock";
import type { Project } from "../../api/types";

afterEach(() => vi.restoreAllMocks());

const prestige: Project = {
  id: 2, name: "Prestige", is_system: false, open_cases: 0, total_cases: 0,
  type: "villas", address: "Road 1", active_contacts: 2,
};
const full = {
  ...prestige,
  contacts: [
    { id: 11, name: "Ravi", phone: "9876543210", email: "", label: "Sales office" },
    { id: 12, name: "Suresh", phone: "9000000002", email: "s@x.in", label: "" },
  ],
};

describe("ProjectForm", () => {
  it("adds a project with type, address and one contact in one request", async () => {
    const spy = mockApi({ "POST /api/admin/projects": { status: 201, body: full } });
    const onSaved = vi.fn();
    render(<ProjectForm onClose={() => {}} onSaved={onSaved} />);
    await userEvent.type(screen.getByLabelText("Project name"), "Green Meadows");
    await userEvent.click(screen.getByLabelText("High-rise apartments"));
    await userEvent.type(screen.getByLabelText("Address"), "Survey 12");
    await userEvent.click(screen.getByRole("button", { name: "+ Add contact" }));
    await userEvent.type(screen.getByLabelText("Contact name"), "Ravi");
    await userEvent.type(screen.getByLabelText("Mobile"), "9876543210");
    await userEvent.type(screen.getByLabelText("Label (optional)"), "Sales office");
    await userEvent.click(screen.getByRole("button", { name: "Add contact" }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/admin/projects")).toEqual({
      name: "Green Meadows", type: "high_rise", address: "Survey 12",
      contacts: [{ name: "Ravi", phone: "9876543210", email: "", label: "Sales office" }],
    });
  });

  it("offers the four types in the owner's order", () => {
    mockApi({});
    render(<ProjectForm onClose={() => {}} onSaved={() => {}} />);
    const group = screen.getByRole("radiogroup", { name: "Type" });
    expect(within(group).getAllByRole("radio").map((r) => r.closest("label")?.textContent)).toEqual([
      "High-rise apartments", "Apartments", "Plots", "Villas",
    ]);
  });

  it("refuses to save without a contact", async () => {
    const spy = mockApi({});
    render(<ProjectForm onClose={() => {}} onSaved={() => {}} />);
    await userEvent.type(screen.getByLabelText("Project name"), "P");
    await userEvent.click(screen.getByLabelText("Villas"));
    await userEvent.type(screen.getByLabelText("Address"), "Road");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Add at least one contact.");
    expect(spy).not.toHaveBeenCalled();
  });

  it("sends nothing when a removal is cancelled", async () => {
    const spy = mockApi({ "GET /api/config/projects/2": { body: full } });
    const onClose = vi.fn();
    render(<ProjectForm project={prestige} onClose={onClose} onSaved={() => {}} />);
    await userEvent.click(await screen.findByRole("button", { name: "Remove Ravi" }));
    expect(screen.queryByText(/Ravi/)).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
    expect(spy.mock.calls.filter(([, init]) => (init?.method ?? "GET") !== "GET")).toHaveLength(0);
  });

  it("saves the kept, edited and new contacts in one PUT", async () => {
    const spy = mockApi({ "GET /api/config/projects/2": { body: full }, "PUT /api/admin/projects/2": { body: full } });
    const onSaved = vi.fn();
    render(<ProjectForm project={prestige} onClose={() => {}} onSaved={onSaved} />);
    await userEvent.click(await screen.findByRole("button", { name: "Remove Ravi" }));
    await userEvent.click(screen.getByRole("button", { name: "Edit Suresh" }));
    const label = screen.getByLabelText("Label (optional)");
    await userEvent.type(label, "Site office");
    await userEvent.click(screen.getByRole("button", { name: "Save contact" }));
    await userEvent.click(screen.getByRole("button", { name: "+ Add contact" }));
    await userEvent.type(screen.getByLabelText("Contact name"), "Accounts");
    await userEvent.type(screen.getByLabelText("Mobile"), "9000000003");
    await userEvent.click(screen.getByRole("button", { name: "Add contact" }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(bodyOf(spy, "PUT /api/admin/projects/2")).toEqual({
      name: "Prestige", type: "villas", address: "Road 1",
      contacts: [
        { id: 12, name: "Suresh", phone: "9000000002", email: "s@x.in", label: "Site office" },
        { name: "Accounts", phone: "9000000003", email: "", label: "" },
      ],
    });
  });

  it("shows the server's message", async () => {
    mockApi({
      "GET /api/config/projects/2": { body: full },
      "PUT /api/admin/projects/2": { status: 409, body: { error: "workflows: this number is already a contact on this project" } },
    });
    render(<ProjectForm project={prestige} onClose={() => {}} onSaved={() => {}} />);
    await screen.findByRole("button", { name: "Remove Ravi" });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This number is already a contact on this project.");
  });

  it("cannot save or change contacts until they have loaded, and offers a retry", async () => {
    let fail = true;
    const spy = vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      if (String(input).endsWith("/api/config/projects/2") && !fail) {
        return new Response(JSON.stringify(full), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response(JSON.stringify({ error: "down" }), { status: 500, headers: { "Content-Type": "application/json" } });
    });
    render(<ProjectForm project={prestige} onClose={() => {}} onSaved={() => {}} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load the contacts.");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "+ Add contact" })).toBeDisabled();
    fail = false;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("button", { name: "Remove Ravi" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    expect(spy.mock.calls.filter(([, init]) => (init?.method ?? "GET") !== "GET")).toHaveLength(0);
  });

  it("refuses to save while a contact is still being typed", async () => {
    const spy = mockApi({ "GET /api/config/projects/2": { body: full }, "PUT /api/admin/projects/2": { body: full } });
    const onSaved = vi.fn();
    render(<ProjectForm project={prestige} onClose={() => {}} onSaved={onSaved} />);
    await screen.findByRole("button", { name: "Remove Ravi" });
    await userEvent.click(screen.getByRole("button", { name: "+ Add contact" }));
    await userEvent.type(screen.getByLabelText("Contact name"), "Accounts");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Add or discard the contact you are editing first.");
    expect(bodyOf(spy, "PUT /api/admin/projects/2")).toBeUndefined();
    expect(onSaved).not.toHaveBeenCalled();
  });
});
