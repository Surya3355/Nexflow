import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { OfficerSelect } from "./OfficerSelect";
import { bodyOf, mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const officers = [
  { id: 1, bank_id: 7, bank_name: "HDFC", team: "dovr", team_name: "DOVR", name: "Ravi", phone: "9876543210" },
  { id: 2, bank_id: 7, bank_name: "HDFC", team: "appraisal", team_name: "Appraisal", name: "Appu", phone: "9876543211" },
  { id: 3, bank_id: 8, bank_name: "SBI", team: "dovr", team_name: "DOVR", name: "Other", phone: "9876543212" },
];

function Harness({ onValue }: { onValue: (v: number | null) => void }) {
  const [v, setV] = useState<number | null>(null);
  return <OfficerSelect bankId={7} team="dovr" teamName="DOVR" value={v} onChange={(x) => { setV(x); onValue(x); }} />;
}

describe("OfficerSelect", () => {
  it("lists only the bank's officers in that team", async () => {
    mockApi({ "GET /api/officers": { body: officers } });
    render(<Harness onValue={() => {}} />);
    const select = screen.getByLabelText("DOVR officer (optional)");
    await screen.findByRole("option", { name: "Ravi · 9876543210" });
    expect(screen.queryByRole("option", { name: /Appu/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Other/ })).not.toBeInTheDocument();
    expect(select).toHaveValue("");
    expect(screen.getByRole("option", { name: "Not known yet" })).toBeInTheDocument();
  });

  it("adds a new officer once on double click and selects it", async () => {
    const onValue = vi.fn();
    let release: () => void = () => {};
    const spy = mockApi({ "GET /api/officers": { body: officers } });
    render(<Harness onValue={onValue} />);
    await screen.findByRole("option", { name: "Ravi · 9876543210" });
    await userEvent.click(screen.getByRole("button", { name: "+ New officer" }));
    await userEvent.type(screen.getByLabelText("New officer name"), "Kiran");
    await userEvent.type(screen.getByLabelText("New officer mobile"), "9000000009");
    spy.mockImplementationOnce(() => new Promise<Response>((resolve) => {
      release = () => resolve(new Response(JSON.stringify({ id: 9, bank_id: 7, bank_name: "HDFC", team: "dovr", team_name: "DOVR", name: "Kiran", phone: "9000000009" }),
        { status: 201, headers: { "Content-Type": "application/json" } }));
    }));
    const save = screen.getByRole("button", { name: "Save officer" });
    await userEvent.click(save);
    await userEvent.click(save);
    expect(save).toBeDisabled();
    release();
    await waitFor(() => expect(onValue).toHaveBeenCalledWith(9));
    const posts = spy.mock.calls.filter(([u, init]) => String(u) === "/api/officers" && init?.method === "POST");
    expect(posts).toHaveLength(1);
    expect(JSON.parse(String(posts[0][1]?.body))).toEqual({ bank_id: 7, team: "dovr", name: "Kiran", phone: "9000000009", email: "" });
    expect(await screen.findByRole("option", { name: "Kiran · 9000000009" })).toBeInTheDocument();
    expect(screen.getByLabelText("DOVR officer (optional)")).toHaveValue("9");
    void bodyOf;
  });

  it("shows the server's message when adding fails", async () => {
    mockApi({ "GET /api/officers": { body: officers }, "POST /api/officers": { status: 409, body: { error: "officers: this bank already has this mobile number in that team" } } });
    render(<Harness onValue={() => {}} />);
    await userEvent.click(screen.getByRole("button", { name: "+ New officer" }));
    await userEvent.type(screen.getByLabelText("New officer name"), "Ravi");
    await userEvent.type(screen.getByLabelText("New officer mobile"), "9876543210");
    await userEvent.click(screen.getByRole("button", { name: "Save officer" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("already has this mobile");
  });
});

describe("OfficerSelect mobile or email", () => {
  it("shows an email-only officer by email and adds one with only an email", async () => {
    const spy = mockApi({
      "GET /api/officers": { body: [{ id: 4, bank_id: 7, bank_name: "HDFC", team: "dovr", team_name: "DOVR", name: "Mail", phone: "", email: "mail@hdfc.example" }] },
      "POST /api/officers": { status: 201, body: { id: 10, bank_id: 7, bank_name: "HDFC", team: "dovr", team_name: "DOVR", name: "Kiran", phone: "", email: "kiran@hdfc.example" } },
    });
    render(<Harness onValue={() => {}} />);
    expect(await screen.findByRole("option", { name: "Mail · mail@hdfc.example" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "+ New officer" }));
    await userEvent.type(screen.getByLabelText("New officer name"), "Kiran");
    await userEvent.click(screen.getByRole("button", { name: "Save officer" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a mobile number or an email.");
    await userEvent.type(screen.getByLabelText("New officer email"), "kiran@hdfc.example");
    await userEvent.click(screen.getByRole("button", { name: "Save officer" }));
    expect(await screen.findByRole("option", { name: "Kiran · kiran@hdfc.example" })).toBeInTheDocument();
    expect(bodyOf(spy, "POST /api/officers")).toEqual({ bank_id: 7, team: "dovr", name: "Kiran", phone: "", email: "kiran@hdfc.example" });
  });
});
