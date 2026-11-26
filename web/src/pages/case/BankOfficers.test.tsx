import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BankOfficers } from "./BankOfficers";
import { bodyOf, mockApi } from "../../test/fetchMock";
import type { CaseOfficer } from "../../api/types";

afterEach(() => vi.restoreAllMocks());

const teams: CaseOfficer[] = [
  { team: "tdep", team_name: "TDEP", officer: { id: 1, name: "Asha", phone: "9876543210", removed: false } },
  { team: "dovr", team_name: "DOVR", officer: null },
  { team: "appraisal", team_name: "Appraisal", officer: { id: 3, name: "Old", phone: "9876543211", removed: true } },
];

describe("BankOfficers", () => {
  it("shows each reached team, a call link, Officer not set and removed", () => {
    render(<BankOfficers caseId={11} bankId={7} officers={teams} readOnly={false} onChanged={() => {}} />);
    const box = screen.getByRole("region", { name: "Bank officers" });
    expect(within(box).getByRole("link", { name: "9876543210" })).toHaveAttribute("href", "tel:9876543210");
    expect(within(box).getByText("Asha")).toBeInTheDocument();
    const notSet = within(box).getByText("Officer not set");
    expect(notSet.className).toContain("amber");
    expect(within(box).getByText(/\(removed\)/)).toBeInTheDocument();
    expect(within(box).getAllByRole("button", { name: /^Change .* officer$/ })).toHaveLength(3);
  });

  it("changes an officer", async () => {
    const onChanged = vi.fn();
    const spy = mockApi({
      "GET /api/officers": { body: [{ id: 5, bank_id: 7, bank_name: "HDFC", team: "dovr", team_name: "DOVR", name: "Ravi", phone: "9000000001" }] },
      "POST /api/cases/11/officers": { body: { id: 11 } },
    });
    render(<BankOfficers caseId={11} bankId={7} officers={teams} readOnly={false} onChanged={onChanged} />);
    await userEvent.click(screen.getByRole("button", { name: "Change DOVR officer" }));
    const dialog = screen.getByRole("dialog", { name: "DOVR officer" });
    await within(dialog).findByRole("option", { name: "Ravi · 9000000001" });
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.selectOptions(within(dialog).getByLabelText("DOVR officer (optional)"), "5");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    expect(bodyOf(spy, "POST /api/cases/11/officers")).toEqual({ team: "dovr", officer_id: 5 });
  });

  it("read-only hides Change, and nothing shows when no team is reached", () => {
    const { unmount } = render(<BankOfficers caseId={11} bankId={7} officers={teams} readOnly onChanged={() => {}} />);
    expect(screen.queryByRole("button", { name: /^Change/ })).not.toBeInTheDocument();
    unmount();
    render(<BankOfficers caseId={11} bankId={7} officers={[]} readOnly={false} onChanged={() => {}} />);
    expect(screen.queryByRole("region", { name: "Bank officers" })).not.toBeInTheDocument();
  });
});

describe("BankOfficers wording for the standard list", () => {
  it("calls the Officer team's dialog Bank officer, not Officer officer", async () => {
    mockApi({ "GET /api/officers": { body: [] } });
    render(<BankOfficers caseId={11} bankId={2} officers={[{ team: "officer", team_name: "Officer", officer: null }]} readOnly={false} onChanged={() => {}} />);
    await userEvent.click(screen.getByRole("button", { name: "Change Bank officer" }));
    expect(screen.getByRole("dialog", { name: "Bank officer" })).toBeInTheDocument();
    expect(screen.queryByText(/Officer officer/)).not.toBeInTheDocument();
  });
});

describe("BankOfficers email", () => {
  it("shows an email link, and both links when the officer has both", () => {
    render(<BankOfficers caseId={11} bankId={7} readOnly onChanged={() => {}} officers={[
      { team: "tdep", team_name: "TDEP", officer: { id: 1, name: "Mail", phone: "", email: "mail@hdfc.example", removed: false } },
      { team: "dovr", team_name: "DOVR", officer: { id: 2, name: "Both", phone: "9876543210", email: "both@hdfc.example", removed: false } },
    ]} />);
    expect(screen.getByRole("link", { name: "mail@hdfc.example" })).toHaveAttribute("href", "mailto:mail@hdfc.example");
    expect(screen.getByRole("link", { name: "9876543210" })).toHaveAttribute("href", "tel:9876543210");
    expect(screen.getByRole("link", { name: "both@hdfc.example" })).toHaveAttribute("href", "mailto:both@hdfc.example");
    expect(screen.getAllByRole("link")).toHaveLength(3);
  });
});
