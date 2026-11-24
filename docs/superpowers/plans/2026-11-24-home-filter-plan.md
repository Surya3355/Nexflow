# Home Tabs and Filter: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Home shows four list tabs and one Filter button whose panel holds the old filter row.
**Architecture:** Web only. `Home.tsx` keeps applied filter state; a new `home/FilterPanel.tsx` edits a draft and
returns it on Apply. Leads are filtered and sorted in the page (`home/leadFilter.ts`).
**Tech Stack:** React 18 + TypeScript, Tailwind, Vitest, Playwright.
**Spec:** `docs/superpowers/specs/2026-11-24-home-filter-design.md`

## Global Constraints
- Tabs: `role="tablist"` aria-label "Lists"; each `role="tab"` with `aria-selected`; names "Leads · N", "My cases · N",
  "All · N", "Needs attention · N".
- Filter button accessible name: "Filter" or "Filter, N on"; visible text "Filter" plus "· N" when N > 0; disabled on
  Needs attention.
- Panel: Dialog titled "Filter"; buttons "Clear all", "Apply". Field labels unchanged: Bank, Project, Stage, Assigned
  to, From, To.
- Commits only when the owner asks.

## Review Focus
1. Bank cleared while a stage is set → stage clears too (stage belongs to a bank). Test in Task 1.
2. Cancel/close the panel without Apply → nothing changes. Test in Task 1.
3. Filter set on All, then switch to Leads → only Project/Assigned apply; bank/date ignored, not lost. Test in Task 1.

---

### Task 1: Tabs, Filter panel, lead filtering
**Files:** Create `web/src/pages/home/FilterPanel.tsx`, `web/src/pages/home/leadFilter.ts`; modify
`web/src/pages/Home.tsx`, `web/src/pages/Home.test.tsx`, `web/e2e/phone.spec.ts`.
**Produces:**
```ts
export type Filters = { bankId: string; projectId: number | null; stageId: string; assigneeId: string; from: string; to: string };
export const noFilters: Filters;
export function countFilters(f: Filters, scope: "leads" | "mine" | "all"): number;
export function FilterPanel(p: { open: boolean; scope: "leads" | "mine" | "all"; value: Filters; banks; projects; users; onClose(): void; onApply(f: Filters): void }): JSX.Element
export function filterLeads(leads: LeadRow[], f: Filters): LeadRow[]; // project + assignee; due first (server order), then not contacted, then rest (server order)
```
- [ ] Tests (update existing chip tests to tabs): employee/manager defaults via `aria-selected`; Filter → panel → pick
  Bank + Assigned → Apply → one `/api/cases?bank_id=1&assignee_id=8` call, button "Filter, 2 on"; close without Apply
  sends nothing new; clearing bank clears stage; Leads tab with Assigned = Priya shows only her leads and count; Leads
  panel shows only Project and Assigned to; bank set on All is ignored on Leads and back on All; Filter disabled on
  Needs attention; not-contacted lead sorts before a contacted not-due lead; Clear all resets.
- [ ] Run → FAIL; implement; web suite → PASS; build; update phone e2e (project box now inside the Filter panel) → PASS.
