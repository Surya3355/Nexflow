import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { Archive } from "./Archive";
import { mockApi } from "../../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const items = [
  { id: 4, entity_type: "case", entity_id: 11, archived_at: "2026-11-01T00:00:00Z", purge_after: "2027-11-01T00:00:00Z", restored: false },
];

describe("Archive", () => {
  it("shows a Restore button only to an admin", async () => {
    mockApi({ "GET /api/admin/archive": { body: items } });
    render(<Archive role="admin" />);
    expect(await screen.findByRole("button", { name: "Restore case 11" })).toBeInTheDocument();
  });

  it("does not show a Restore button to a manager", async () => {
    mockApi({ "GET /api/admin/archive": { body: items } });
    render(<Archive role="manager" />);
    await waitFor(() => expect(screen.getByText("case 11")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: /Restore/ })).not.toBeInTheDocument();
  });
});
