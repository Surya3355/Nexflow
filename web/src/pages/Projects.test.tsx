import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { Projects } from "./Projects";
import { ProjectDetail } from "./ProjectDetail";
import { mockApi } from "../test/fetchMock";

afterEach(() => vi.restoreAllMocks());

const projects = [
  { id: 1, name: "NA", is_system: true, open_cases: 5, total_cases: 9 },
  { id: 4, name: "Prestige Lakeside", is_system: false, open_cases: 2, total_cases: 3 },
  { id: 6, name: "Sobha Dream Acres", is_system: false, open_cases: 0, total_cases: 1 },
];

const caseRow = {
  id: 1042, customer_id: 5, bank_id: 1, project_id: 4, project_name: "Prestige Lakeside", loan_amount: "4500000.00",
  current_stage_id: 3, assignee_id: 7, status: "active", created_at: "2026-11-03T04:44:00Z", customer_name: "Ravi Kumar",
  customer_mobile: "9848012345", bank_name: "HDFC", stage_name: "Under sanction", stage_code: "under_sanction",
  stage_order: 3, stage_count: 6, assignee_name: "Priya", stage_since: "2026-11-12T04:00:00Z", needs_document: false,
};

function renderAt(path: string) {
  const spy = mockApi({
    "GET /api/config/projects": { body: projects },
    "GET /api/config/banks": { body: [{ id: 1, name: "HDFC" }] },
    "GET /api/users": { body: [{ id: 7, full_name: "Priya", username: "priya" }] },
    "GET /api/cases": { body: [caseRow] },
  });
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/projects" element={<Projects />} />
        <Route path="/projects/:id" element={<ProjectDetail />} />
      </Routes>
    </MemoryRouter>,
  );
  return spy;
}

describe("Projects", () => {
  it("lists projects with NA first and their case counts", async () => {
    renderAt("/projects");
    const list = await screen.findByRole("list", { name: "Projects" });
    const items = within(list).getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("NA");
    expect(items[1]).toHaveTextContent("Prestige Lakeside");
    expect(items[1]).toHaveTextContent("2 open");
    expect(items[1]).toHaveTextContent("3 total");
    expect(within(items[1]).getByRole("link")).toHaveAttribute("href", "/projects/4");
  });

  it("the search box narrows the list", async () => {
    renderAt("/projects");
    await screen.findByRole("list", { name: "Projects" });
    await userEvent.type(screen.getByRole("searchbox", { name: "Find a project" }), "sob");
    const items = within(screen.getByRole("list", { name: "Projects" })).getAllByRole("listitem");
    expect(items.map((i) => i.textContent)).toEqual([expect.stringContaining("Sobha Dream Acres")]);
  });

  it("a project shows all its cases and a CSV download", async () => {
    const spy = renderAt("/projects/4");
    expect(await screen.findByRole("heading", { name: "Prestige Lakeside" })).toBeInTheDocument();
    const list = await screen.findByRole("list", { name: "Cases" });
    expect(within(list).getByRole("link", { name: /Ravi Kumar/ })).toHaveAttribute("href", "/cases/1042");
    const calls = spy.mock.calls.map(([u]) => String(u)).filter((u) => u.startsWith("/api/cases"));
    expect(calls.pop()).toBe("/api/cases?project_id=4");
    expect(screen.getByRole("link", { name: "⬇ Download CSV" })).toHaveAttribute("href", "/api/reports/export.csv?project_id=4");
  });

  it("the bank filter narrows the project's cases and the CSV", async () => {
    const spy = renderAt("/projects/4");
    await userEvent.selectOptions(await screen.findByLabelText("Bank"), "1");
    await waitFor(() => expect(spy.mock.calls.some(([u]) => String(u) === "/api/cases?project_id=4&bank_id=1")).toBe(true));
    expect(screen.getByRole("link", { name: "⬇ Download CSV" })).toHaveAttribute("href", "/api/reports/export.csv?project_id=4&bank_id=1");
  });
});
