import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Stages } from "./Stages";
import { bodyOf, mockApi } from "../../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

describe("Stages", () => {
  it("has a 'requires documents' toggle for each stage that saves the change", async () => {
    const spy = mockApi({
      "GET /api/admin/stage-lists": {
        body: [{ bank_id: null, name: "Standard stages", stages: [{ id: 4, code: "sanctioned", name: "Sanctioned", sort_order: 4, requires_document: false, role: "sanction" }] }],
      },
      "PATCH /api/admin/stages/4": { status: 204 },
    });
    render(<Stages />);
    const toggle = await screen.findByRole("checkbox", { name: "Requires documents: Sanctioned" });
    expect(toggle).not.toBeChecked();
    await userEvent.click(toggle);
    await waitFor(() => expect(bodyOf(spy, "PATCH /api/admin/stages/4")).toEqual({ requires_document: true }));
  });

  it("shows one section per stage list", async () => {
    mockApi({
      "GET /api/admin/stage-lists": {
        body: [
          { bank_id: null, name: "Standard stages", stages: [{ id: 1, code: "received", name: "Received", sort_order: 1, requires_document: false, role: "start" }] },
          { bank_id: 3, name: "HDFC", stages: [{ id: 31, code: "link_sent", name: "Link sent", sort_order: 2, requires_document: false, role: "start" }] },
        ],
      },
    });
    render(<Stages />);
    const hdfc = await screen.findByRole("region", { name: "HDFC" });
    expect(hdfc).toHaveTextContent("Link sent");
    expect(screen.getByRole("region", { name: "Standard stages" })).toHaveTextContent("Received");
  });
});
