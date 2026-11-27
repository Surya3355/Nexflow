import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PaymentFields } from "./PaymentFields";
import { todayIST } from "./BankQueries";

describe("PaymentFields", () => {
  it("does not offer dates after today (India time)", () => {
    render(<PaymentFields amount="" date="" kind="part" partPayments onAmount={() => {}} onDate={() => {}} onKind={() => {}} />);
    expect(screen.getByLabelText("Payment date")).toHaveAttribute("max", todayIST());
  });
});
