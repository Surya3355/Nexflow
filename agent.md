# agent.md — Working Guide for Nexflow

This file tells AI coding agents how to work on this repository. Read it first.

## 1. What this project is

Nexflow is an internal tracking portal for a small Direct Selling Agent (DSA) business. Staff track leads and home-loan cases from first contact through bank processing, sanction, documentation and disbursement.

- Internal staff only. No customer accounts or portals.
- One business. Not a multi-tenant SaaS.
- A web app for the first version, designed to work well on phone browsers. A native mobile app is a later phase.
- One backend and one database. Any later client calls the same API.

Confirmed business decisions are in `docs/00-decisions.md`. That file is the source of truth for scope. Background context in older notes is not binding.

## 2. Rules for agents

- **Do not invent business rules.** Fields, stages, permissions and document lists come from `docs/00-decisions.md` or from the owner. If something is open, ask one focused question.
- **Do not pick the technology stack** until the owner asks for it. Architecture rules in this file apply to any stack.
- **Follow the Superpowers method:** brainstorm, write a spec, write a plan of small steps, then implement with test-driven development for business rules and critical workflows.
- **Specs go in** `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`. **Plans go in** `docs/superpowers/plans/`.
- **Do not build** anything beyond the approved spec. Do not add commission, accounting, customer portal, call logging or employee monitoring.
- **Commit and push only when the owner asks.** Use the configured git identity. Do not override it.
- **Never commit secrets.** Use `.env.example` for variable names only. Real `.env` files stay out of git.

## 3. Architecture principles

- **Modular monolith.** One backend service with clear internal modules. No microservices.
- **One source of truth.** One database. Web and mobile both call the same API.
- **Server-side authorization.** Every API request checks the user's role and permission on the server. The client is never trusted.
- **Keep stage history.** Moving a case to a later stage adds a new event with a date. Never overwrite earlier milestones.
- **Separate concepts.** Lead, customer and loan case are different records. A lead can become a case. A customer can have many cases.
- **Configuration, not code, for lists.** Banks, stages and dropdown values are data maintained by the admin. Bank-specific logic must not be hard-coded.
- **Private files.** Uploaded documents are never public. Access uses short-lived, authorized links.
- **Audit history.** Record who did what and when for lead creation, assignment, reassignment, stage changes and deletions.
- **Simple first.** No workflow engine or other infrastructure until a clear need is shown.

## 4. Proposed folder structure

This layout is a proposal. It names folders by role, not by technology.

```
nexflow/
├── agent.md                  # this file
├── README.md                 # how to run the project (added later)
├── docs/
│   ├── 00-decisions.md       # confirmed decisions and open questions
│   └── superpowers/
│       ├── specs/            # design specs (dated)
│       └── plans/            # implementation plans (dated)
├── backend/                  # API, business rules, database migrations, tests
│   └── modules/              # one folder per domain area (see below)
├── web/                      # web app for managers and employees (phone-friendly)
├── mobile/                   # native mobile app (later phase, not started)
├── infra/                    # deployment, containers, CI configuration (no secrets)
└── .env.example              # variable names only
```

Backend domain modules (names may change after the spec):

| Module | Covers |
|---|---|
| `access` | Users, roles, permissions, login |
| `leads` | Lead capture, source, assignment, reassignment history |
| `customers` | Reusable customer records (fields to be confirmed) |
| `cases` | Loan cases, bank, stage, tracking number, milestone dates |
| `workflows` | Stage lists and bank-specific configuration (admin-maintained) |
| `documents` | Checklists, uploads, document status (later phase) |
| `reports` | Daily operational view and date-filtered reports (later phase) |
| `history` | Notes and audit events |

Each module keeps its own business rules, data access and tests together. Modules talk through explicit interfaces, not by reaching into each other's tables.

## 5. Testing expectations

- Business rules (permissions, stage changes, reassignment, deletion limits) get tests first.
- Every permission rule has a test that proves the server refuses an unauthorized action.
- Each step in a plan is small enough to test on its own.

## 6. Current phase

Discovery and requirements. Next steps are agreed in `docs/00-decisions.md`, section 11. Implementation has not started and must wait for an approved spec and plan.
