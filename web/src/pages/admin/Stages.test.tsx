import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Stages } from "./Stages";
import { bodyOf, mockApi } from "../../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

describe("Stages", () => {
  it("has a 'requires documents' toggle for each stage that saves the change", async () => {
    const spy = mockApi({
      "GET /api/config/stages": {
        body: [{ id: 4, code: "sanctioned", name: "Sanctioned", sort_order: 4, requires_document: false }],
      },
      "PATCH /api/admin/stages/4": { status: 204 },
    });
    render(<Stages />);
    const toggle = await screen.findByRole("checkbox", { name: "Requires documents: Sanctioned" });
    expect(toggle).not.toBeChecked();
    await userEvent.click(toggle);
    await waitFor(() => expect(bodyOf(spy, "PATCH /api/admin/stages/4")).toEqual({ requires_document: true }));
  });
});
