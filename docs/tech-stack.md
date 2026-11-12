# Nexflow — Technology Stack

Status: **Chosen by the owner (round 10, 2026-11-11).** Every agent and every plan follows this file. Do not change a listed choice without the owner's approval. Items marked **[to decide]** are not chosen yet.

## 1. Chosen stack

| Layer | Choice | Notes |
|---|---|---|
| Backend language | **Go** | Single backend service, modular monolith (`agent.md` §3). |
| HTTP router | **chi** (on the standard `net/http`) | Handlers use the standard `http.Handler` signature. Route groups for admin and staff sections. |
| Database | **PostgreSQL** | Confirmed by owner. Schema to be drafted and reviewed before the plan. |
| Database access | **`sqlc`** (owner approved, round 12) | Typed Go code generated from SQL. Keeps queries explicit and reviewable. |
| Migrations | **`golang-migrate`** (owner approved, round 12) | Versioned SQL migrations, run in order. |
| Web frontend | **React** (single-page app) | Calls the Go JSON API. Responsive layout for phone browsers. |
| Login | **Username and password.** Admin creates users. Users can change their own password. | No self sign-up. Passwords hashed with `argon2id` (recommended). |
| Sessions | **Server-side sessions** stored in PostgreSQL (recommended) | The browser holds only a random session cookie. Admin can disable a user at once. |
| Email alerts | **SMTP** | Provider and sender domain **[to decide]**. |
| File storage | Private storage, served only through short-lived links | Location **[to decide]** with hosting. |
| Hosting | India cloud region for data | Provider, server type and cost **[to decide]**. Budget ₹1,000 to ₹3,000 a month. |
| Tests (backend) | `go test` against a real PostgreSQL | Business rules, permissions and stage rules get tests first (`agent.md` §5). |
| Tests (frontend) | **[to decide in plan]** | |

## 2. Repository layout

Follows `agent.md` §4, using the chosen stack:

```
nexflow/
├── backend/        # Go API, business modules, migrations, tests
│   └── modules/    # access, customers, cases, workflows, documents, reports, history
├── web/            # React single-page app
├── docs/           # decisions, specs, plans, this file
└── infra/          # deployment and configuration (no secrets)
```

## 3. Rules for agents

- Business rules come from `docs/00-decisions.md` and the approved spec. Do not invent rules.
- Permissions are enforced on the server for every API request. The React app is never trusted for access control.
- Use PostgreSQL features and migrations. Do not keep schema changes outside migrations.
- Do not add a technology (queue, cache, message broker, second database, extra framework) without the owner's approval.
- Secrets never go in git. `.env.example` lists variable names only.
- Commit and push only when the owner asks.

## 4. Open decisions

1. Hosting provider, server type and cost (within the ₹1,000 to ₹3,000 budget).
2. Sender domain and SMTP provider for alerts.
3. Location of uploaded files (on the server or in India-region object storage).
4. Frontend styling approach and test tools.
5. Resolved in round 12: `sqlc` and `golang-migrate` approved.
