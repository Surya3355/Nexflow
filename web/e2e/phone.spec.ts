import { expect, test } from "@playwright/test";

// Spec §12 and plan Task 35: no horizontal scroll at phone width.
test("login page has no horizontal scroll at 360 px", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.goto("/");
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
});

// UI redesign plan Task 13: signed-in screens also fit a 360 px phone.
const caseRow = {
  id: 1042, customer_id: 5, bank_id: 1, loan_amount: "4500000.00", current_stage_id: 3, assignee_id: 1, status: "active",
  created_at: "2026-11-03T04:44:00Z", customer_name: "Ravi Kumar Venkata Subramanian", customer_mobile: "9848012345",
  bank_name: "HDFC", project_id: 4, project_name: "Prestige Lakeside Habitat Phase 2", stage_name: "Under sanction", stage_code: "under_sanction", stage_order: 3, stage_count: 6,
  assignee_name: "Priya Sharma", stage_since: "2026-11-12T04:00:00Z", needs_document: true, tracking_number: "HD88213",
};
const stages = ["Received", "Logged in", "Under sanction", "Sanctioned", "Documentation", "Disbursed"].map((name, i) => ({
  id: i + 1, code: name.toLowerCase().replace(/ /g, "_"), name, sort_order: i + 1, requires_document: false,
}));
const api: Record<string, unknown> = {
  "/api/me": { id: 1, username: "priya", full_name: "Priya Sharma", role: "employee" },
  "/api/cases": Array.from({ length: 8 }, (_, i) => ({ ...caseRow, id: 1042 + i })),
  "/api/cases/attention": [{ ...caseRow, reason: "No documents at this stage" }],
  "/api/config/banks": [{ id: 1, name: "HDFC" }],
  "/api/config/projects": [
    { id: 1, name: "NA", is_system: true, open_cases: 5, total_cases: 9 },
    { id: 4, name: "Prestige Lakeside Habitat Phase 2", is_system: false, open_cases: 8, total_cases: 12 },
  ],
  "/api/users": [{ id: 1, full_name: "Priya Sharma", username: "priya" }],
  "/api/reports/dashboard": {
    new_customers_today: 4, new_cases_today: 5, new_customers: 34, new_cases: 41, stage_moves_today: 7,
    cases_by_stage: { received: 9, under_sanction: 11 }, sanctions: { count: 12, amount: "46000000.00", dropped: 1 },
    disbursements: { count: 7, amount: "29000000.00", dropped: 0 },
  },
  "/api/reports/rows": { columns: ["Case ID", "Customer", "Mobile", "Bank", "Stage"], rows: [["1042", "Ravi Kumar", "9848012345", "HDFC", "Under sanction"]] },
  "/api/config/stages": stages,
  "/api/cases/1042": caseRow,
  "/api/cases/1042/history": [{ id: 1, reached_at: "2026-11-03T04:44:00Z", stage_code: "received", stage_name: "Received", changed_by: "Priya Sharma" }],
  "/api/cases/1042/documents": [],
  "/api/config/banks/1/stages": stages,
  "/api/customers/5": { id: 5, name: caseRow.customer_name, mobile: "9848012345", email: "ravi.kumar@example.com", status: "active", cases: [caseRow] },
};

for (const path of ["/", "/cases/1042", "/customers/5", "/reports", "/projects", "/projects/4"]) {
  test(`signed-in ${path} has no horizontal scroll at 360 px`, async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 740 });
    await page.route("**/api/**", (route) => {
      const url = new URL(route.request().url());
      const body = api[url.pathname];
      return body === undefined ? route.fulfill({ status: 404, json: { error: "not mocked" } }) : route.fulfill({ json: body });
    });
    await page.goto(path);
    await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible();
    await page.waitForLoadState("networkidle");
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });
}

// Spec §5.2: between 760 and 1099 px the side cards move above the list and stay visible.
test("Home shows Today and Needs attention at 900 px", async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 900 });
  await page.route("**/api/**", (route) => {
    const body = api[new URL(route.request().url()).pathname];
    return body === undefined ? route.fulfill({ status: 404, json: { error: "x" } }) : route.fulfill({ json: body });
  });
  await page.goto("/");
  await expect(page.getByRole("region", { name: "Today" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Needs attention" })).toBeVisible();
});

// Spec 2026-11-21 §6: the project box opened on Home still fits a 360 px phone.
test("project box on Home has no horizontal scroll at 360 px", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.route("**/api/**", (route) => {
    const url = new URL(route.request().url());
    const body = api[url.pathname];
    return body === undefined ? route.fulfill({ status: 404, json: { error: "not mocked" } }) : route.fulfill({ json: body });
  });
  await page.goto("/");
  await page.getByRole("combobox", { name: "Project" }).click();
  await expect(page.getByRole("option", { name: "Prestige Lakeside Habitat Phase 2" })).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
});
