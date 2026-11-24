import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ProjectDetail } from "./ProjectDetail";
import { mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

describe("ProjectDetail", () => {
  it("shows the type, address and contacts above the cases", async () => {
    mockApi({
      "GET /api/config/projects/4": {
        body: {
          id: 4, name: "Green Meadows", is_system: false, open_cases: 0, total_cases: 0, type: "villas", address: "Survey 12, Kollur", active_contacts: 1,
          contacts: [{ id: 21, name: "Ravi", phone: "9876543210", email: "", label: "Sales office" }],
        },
      },
      "GET /api/config/banks": { body: [] },
      "GET /api/users": { body: [] },
      "GET /api/cases": { body: [] },
    });
    render(
      <MemoryRouter initialEntries={["/projects/4"]}>
        <Routes><Route path="/projects/:id" element={<ProjectDetail />} /></Routes>
      </MemoryRouter>,
    );
    expect(await screen.findByRole("heading", { level: 1, name: "Green Meadows" })).toBeInTheDocument();
    const about = screen.getByRole("region", { name: "About this project" });
    expect(within(about).getByText("Villas")).toBeInTheDocument();
    expect(within(about).getByText("Survey 12, Kollur")).toBeInTheDocument();
    expect(within(about).getByText(/Sales office · Ravi/)).toBeInTheDocument();
    expect(within(about).getByRole("link", { name: "9876543210" })).toHaveAttribute("href", "tel:+919876543210");
  });
});
