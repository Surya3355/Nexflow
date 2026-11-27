# Hardening before production (round 16) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix every code-review and QA finding of 2026-11-27 except email sending and hosting.

**Architecture:** Changes stay inside the existing modules (access, documents, cases, workflows,
alerts, archive, apihttp) plus one new migration per data change. New cross-cutting pieces are
two HTTP middlewares (origin check, login limiter) and one shared validation helper package.

**Tech Stack:** Go (chi, pgx v5, sqlc, scs), PostgreSQL 16, React 18 + Vite + Vitest + Playwright, nginx.

**Spec:** `docs/superpowers/specs/2026-11-27-hardening-design.md` (approved 2026-11-27).

## Global Constraints

- Every task is test-first: write the failing test, see it fail, implement, see it pass, commit.
- Go has no local toolchain. Run Go tests in Docker against the compose database:
  `GT='docker run --rm --network nexflow_default -v /home/surya/workspace/nexflow/backend:/src -v nexflow-gomod:/go/pkg/mod -v nexflow-gocache:/root/.cache/go-build -w /src -e NEXFLOW_TEST_DB_URL=postgres://nexflow:nexflow-local@db:5432/postgres?sslmode=disable -e NEXFLOW_LINK_SECRET=local-test-secret-0123456789abcdef0123 golang:<GO_IMAGE>'`
  then `$GT go test ./modules/access/ -run TestName -count=1`. `<GO_IMAGE>` is `1.25` until Task 1 and the new version after.
- sqlc regenerates with `docker run --rm -v /home/surya/workspace/nexflow/backend:/src -w /src sqlc/sqlc generate`; generated code is committed.
- Web: `cd web && npx vitest run <file>`; `npm run build`.
- Messages to users are plain and short, like existing ones. India time (IST) for "today".
- New migrations start at `000011`. Each has a down migration that reverses it.
- Commit after every task with the configured identity, ending with
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Do not push.
- Limits (spec §5): bank name 100, names 150, other free text 2,000 characters; amounts > 0,
  ≤ 1,000,000,000 (₹100 crore), ≤ 2 decimals; payment date ≤ today IST.
- Login (spec §3/§4.1): 5 wrong in a row → 15 minutes lock; 20 attempts per minute per address;
  4 password checks at once.
- Email queue (spec §6): max 5 attempts; alerts older than 3 days skipped.

## Review Focus

1. A locked username during a real staff morning: the lock must expire on its own and the right
   password after 15 minutes must work (Task 3 test `TestLockExpires`).
2. Switching off a user who has nothing open must not demand a takeover person (Task 4 test
   `TestDeactivateWithNothingOpen`).
3. The app must still start on the existing local database (data already present, owner role
   `nexflow`) after the audit-role change (Task 6 test `TestAppRoleOnExistingDB` plus a
   `docker compose up` check).
4. The origin check must not break the real app: requests from `http://localhost:8081` through
   nginx and from Vite dev `http://localhost:5173` pass (Task 7 tests).
5. A payment entered late at night IST: "today" is the IST date, so 23:30 IST today is allowed
   (Task 9 test `TestPaymentDateTodayIST`).

---

### Task 1: Toolchain, dependencies, images, test setup

**Files:**
- Modify: `backend/Dockerfile`, `backend/go.mod`, `web/package.json`, `web/package-lock.json`,
  `web/Dockerfile`, `docker-compose.yml`, `docs/local-testing.md`

- [ ] **Step 1:** Look up the newest stable Go on Docker Hub (`golang:1.2x`); set `go` in
  `go.mod` and both Go stages in `backend/Dockerfile` to it. Final stage
  `gcr.io/distroless/static-debian12:nonroot`; uploads volume must stay writable (create
  `/data/uploads` owned by 65532 via a `COPY --chown` of an empty dir from the build stage).
- [ ] **Step 2:** Test image includes `infra/backup`: compose `test` builds with
  `context: .` and `dockerfile: backend/Dockerfile` (target `test`), or the test target copies
  `infra/backup` to `/infra/backup`; pick the one that keeps `../../infra/backup/test_retention.sh`
  resolving from `/src/acceptance`. Put `test` under `profiles: ["test"]` so `docker compose up`
  no longer starts it; update `docs/local-testing.md` (`docker compose run --rm test` still works).
- [ ] **Step 3:** Web: upgrade `react-router-dom` (latest 6.x with fixed `@remix-run/router`),
  `vite`, `@vitejs/plugin-react`, `vitest`; nginx image to current stable alpine; node build image
  to current LTS. Run `npm audit --omit=dev` → 0; `npm audit` → no high/critical.
- [ ] **Step 4:** Verify: `$GT go build ./... && $GT go vet ./...`;
  `docker run --rm -v .../backend:/src -w /src golang:<new> sh -c 'go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...'`
  → "No vulnerabilities found"; `npm test` and `npm run build` pass; full Go suite via
  `docker compose run --rm --build test` → all pass including `TestAcceptance19`.
- [ ] **Step 5:** Commit "Upgrade Go and web libraries; non-root image; backup test runs".

### Task 2: Link secret check

**Files:** Modify `backend/internal/platform/config/config.go`; Test `config_test.go`.

- [ ] **Step 1: Failing tests** `TestLoadRejectsExampleSecret` (value
  `change-me-to-32-or-more-random-characters` → error mentioning `NEXFLOW_LINK_SECRET`),
  `TestLoadRejectsLowVarietySecret` (`aaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbbb` → error),
  `TestLoadAcceptsRandomSecret` (`openssl rand -hex 32`-style value → ok).
- [ ] **Step 2:** Run, see FAIL.
- [ ] **Step 3:** In `Load`, refuse the exact example value and any secret with fewer than 16
  distinct bytes; error text: `config: NEXFLOW_LINK_SECRET is the example value or too simple; make one with: openssl rand -hex 32`.
  Update `.env.example` comment to show the command; local `.env` gets a fresh random value.
- [ ] **Step 4:** Run, PASS. Check `docker-compose.yml` test secret still passes (it has ≥16 distinct).
- [ ] **Step 5:** Commit.

### Task 3: Login lock, address limit, hash cap

**Files:**
- Create: `migrations/000011_login_lock.up.sql`/`.down.sql` (users: `failed_logins int NOT NULL DEFAULT 0`, `locked_until timestamptz`)
- Create: `backend/modules/access/limiter.go` (address limiter + hash semaphore)
- Modify: `modules/access/login.go`, `modules/access/session.go` (login handler),
  `modules/access/users.go` (`ResetPassword` clears lock), `modules/access/queries/users.sql`,
  `modules/http/errors.go`, `internal/platform/config/config.go` (`TrustProxy bool` from `NEXFLOW_TRUST_PROXY=true`),
  `web/nginx.conf` (`proxy_set_header X-Real-IP $remote_addr;`), `docker-compose.yml` (`NEXFLOW_TRUST_PROXY: "true"`)
- Test: `modules/access/login_lock_test.go`, `modules/access/limiter_test.go`

**Interfaces — Produces:**
- `var ErrLocked = errors.New("access: too many attempts")`; `type LockedError struct{ Until time.Time }` with `Is(ErrLocked)`; `Service.SetClock(func() time.Time)`.
- `type AddressLimiter struct`; `NewAddressLimiter(perMinute int, clock func() time.Time) *AddressLimiter`; `Allow(addr string) bool`.
- `type HashGate`; `NewHashGate(n int, wait time.Duration) *HashGate`; `Do(ctx, fn func()) error` returns `ErrBusy` after `wait`.
- `func ClientAddr(r *http.Request, trustProxy bool) string`.

- [ ] **Step 1: Failing tests:**
  - `TestFiveWrongLocks`: 5 wrong → 6th call with the **right** password returns `ErrLocked`, `Until` = 5th failure + 15 min.
  - `TestRightPasswordResetsCount`: 4 wrong, right, 4 wrong → still logs in.
  - `TestLockExpires`: after lock, clock +15m1s, right password → ok and count 0.
  - `TestResetPasswordUnlocks`: locked user, admin `ResetPassword` → new password logs in at once.
  - `TestUnknownUserLocksToo`: 5 attempts on `nosuchuser` → 6th gives `ErrLocked` (in-memory map, keyed by lowercase name, same rule).
  - `TestAddressLimiter`: 20 `Allow` true, 21st false, clock +61s → true.
  - `TestHashGateBusy`: gate(1, 50ms) held by one fn → second `Do` returns `ErrBusy`.
  - Handler test `TestLoginLockedResponse`: status 429, JSON `error` text `Too many attempts. Try again in 15 minutes.` (minutes rounded up).
- [ ] **Step 2:** Run, FAIL.
- [ ] **Step 3:** Implement. Count update in one `UPDATE ... RETURNING` (no read-then-write race). Address limiter: fixed one-minute window per address, map pruned on each call. Hash gate: buffered channel of 4, wait 5 s. Login handler order: address limit (429 `Too many attempts. Wait a minute.`) → `Authenticate` inside gate (503 `Busy, try again.`). `ClientAddr` uses `X-Real-IP` only when `trustProxy`, else `RemoteAddr` host.
- [ ] **Step 4:** Run, PASS; run `./modules/access/ ./modules/http/ ./acceptance/` all pass.
- [ ] **Step 5:** Web `Login.tsx` shows the server message as-is (check existing error display; add a Vitest if it maps 401 only). Commit.

### Task 4: Switch off, switch on, change role (backend)

**Files:**
- Create: `modules/access/deactivate.go`; Test `modules/access/deactivate_test.go`
- Modify: `modules/access/queries/users.sql`, `modules/access/permissions.go` (reuse `ActionManageUsers`),
  `modules/http/handlers.go`, `modules/http/router.go`, `modules/http/errors.go`

**Interfaces — Produces:**
- `func (s *Service) OpenWork(ctx, actor User, userID int64) (OpenWork, error)`; `type OpenWork struct{ Leads, Cases, Queries int }`.
- `func (s *Service) Deactivate(ctx, actor User, userID int64, takeoverID *int64) error`.
- `func (s *Service) Reactivate(ctx, actor User, userID int64) error`.
- `func (s *Service) ChangeRole(ctx, actor User, userID int64, role Role) error`.
- `type Reassigner interface { ReassignAllTx(ctx context.Context, from, to int64) (leads, cases, queries int, err error) }`; `Service.SetReassigner(Reassigner)`. Implemented in Task 5.
- Errors: `ErrTakeoverRequired`, `ErrTakeoverInvalid`, `ErrLastAdmin`, `ErrSelfDeactivate`.
- Routes: `GET /api/admin/users/{id}/open-work`, `POST /api/admin/users/{id}/deactivate` `{takeover_id}`, `POST /api/admin/users/{id}/reactivate`, `PUT /api/admin/users/{id}/role` `{role}`.

- [ ] **Step 1: Failing tests:** `TestDeactivateMovesWork` (fake reassigner called with from/to; user inactive; sessions end — `loadActive` fails), `TestDeactivateWithNothingOpen` (no takeover → ok), `TestDeactivateNeedsTakeover` (open work, nil → `ErrTakeoverRequired`), `TestTakeoverMustBeActiveOther` (inactive or same id → `ErrTakeoverInvalid`), `TestCannotDeactivateSelf`, `TestLastAdminProtected` (demote or switch off the only active admin → `ErrLastAdmin`; with two admins ok), `TestNonAdminForbidden` (manager → `ErrForbidden` on all four), `TestAuditEntries` (`user.deactivated` with takeover and counts, `user.reactivated`, `user.role_changed` with before/after), `TestReactivate`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement in one transaction (`history` tx context like other services); lock the user rows `FOR UPDATE`; count active admins inside the tx. Map new errors: takeover/last admin/self → 409 with plain text. **Step 4:** PASS. **Step 5:** Commit.

### Task 5: Reassign all open work

**Files:** Create `modules/cases/reassign_all.go` (or a small `modules/handover` package if imports cycle — access must not import cases); Test alongside. Modify `cmd/nexflow/main.go` wiring.

**Interfaces — Consumes:** `access.Reassigner`. **Produces:** `type Handover struct`; `NewHandover(pool, rec history.Recorder, caseNotifier, leadNotifier)`; `ReassignAllTx` reads the tx from ctx (`history.TxFromContext`).

- [ ] **Step 1: Failing test** `TestReassignAll`: user A has 2 open leads, 1 converted lead, 2 active cases, 1 dropped case, 2 open queries, 1 resolved query → counts (2,2,2); only open/active items move; each moved lead/case gets an `assigned` alert to B and a history entry (`lead.assigned`/`case.assigned` with `reason: "handover"`); each moved query gets `query.reassigned`; no alert for queries (spec §4.2).
- [ ] **Step 2–4:** FAIL → implement with set-based `UPDATE ... RETURNING id` per table, then history/alert per id → PASS.
- [ ] **Step 5:** Assignee pickers already use `ListActiveUsers`; add test `TestInactiveNotInPickers` in the bank-query and lead assign paths (assigning to an inactive user → existing `ErrAssigneeInactive` or equivalent; add it where missing). Commit.

### Task 6: Permanent audit history (app role)

**Files:** Create `migrations/000012_app_role.up.sql`/`.down.sql`; Modify `cmd/nexflow/main.go`,
`internal/platform/testdb` (if tests should run as the app role), `docs/database-schema.md` §6.
Test: `migrations/app_role_test.go`.

- [ ] **Step 1: Failing tests:** `TestAppRoleCannotChangeAudit` (pool with `AfterConnect: SET ROLE nexflow_app`: `INSERT` into audit ok; `UPDATE audit_log`/`DELETE FROM audit_log` → permission denied), `TestAppRoleCanUseEverythingElse` (insert/update/delete on cases, sessions, etc. ok; sequences usable), `TestAppRoleOnExistingDB` (migrate a DB seeded at 000010 with rows, then up → works).
- [ ] **Step 2:** FAIL.
- [ ] **Step 3:** Migration: grant `nexflow_app` SELECT/INSERT/UPDATE/DELETE on all tables except `audit_log`, USAGE/SELECT on all sequences, `ALTER DEFAULT PRIVILEGES` for future tables, `GRANT nexflow_app TO CURRENT_USER`. `main.go`: migrate with a plain pool, then open the app pool with `AfterConnect` running `SET ROLE nexflow_app`. Also covers the `sessions` table used by scs.
- [ ] **Step 4:** PASS; full suite; `docker compose up -d --build backend`, log in at :8081 works.
- [ ] **Step 5:** Commit.

### Task 7: Origin check and browser security headers

**Files:** Create `modules/http/origin.go`, Test `origin_test.go`; Modify `router.go`,
`internal/platform/config/config.go` (`AllowedOrigins []string` from comma list `NEXFLOW_ALLOWED_ORIGINS`), `web/nginx.conf`, `docker-compose.yml`.

**Produces:** `func RequireSameOrigin(allowed []string) func(http.Handler) http.Handler`.

- [ ] **Step 1: Failing tests:** POST with `Origin: http://evil.example` → 403 `Request refused.`; POST with Origin equal to `http://` + request Host → passes; Origin missing, `Referer` same host → passes; neither → 403; GET with foreign Origin → passes; Origin in allowed list (`http://localhost:5173`) → passes; `X-Forwarded-Host` used for the host when present (nginx sends Host already; test both).
- [ ] **Step 2–4:** FAIL → implement, mount before every route group (login too) → PASS. Fix any existing handler tests/acceptance helpers that POST without Origin by setting it in the shared test client.
- [ ] **Step 5:** nginx adds for all responses (`always`): `Content-Security-Policy "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"`, `X-Frame-Options DENY`, `X-Content-Type-Options nosniff`, `Referrer-Policy same-origin`, and a commented HSTS line for when HTTPS exists. Compose sets `NEXFLOW_ALLOWED_ORIGINS: http://localhost:8081`. Verify with `curl -sI localhost:8081` and a Playwright e2e run (CSP must not break the app). Commit.

### Task 8: Shared validation helpers

**Files:** Create `backend/internal/platform/validate/validate.go`, Test `validate_test.go`.

**Produces:** `Name(s string, max int) (string, bool)` (trim, non-empty, rune count ≤ max); `Text(s string, max int) (string, bool)` (trim, may be empty); `Email(s string) (string, bool)` (empty ok; `net/mail.ParseAddress` with bare address equal to input, contains a dot after `@`); `Amount(s string) (pgtype.Numeric, bool)` (> 0, ≤ 1000000000, ≤ 2 decimals, no exponent); constants `MaxName=150`, `MaxBankName=100`, `MaxText=2000`.

- [ ] **Step 1: Failing table tests** with: `"  "`→false; 150 vs 151 runes (Devanagari counts as runes); `"nope"`,`"a@b"`,`"a@b.co"`(ok),`"A@B.CO "`(ok trimmed); amounts `"0"`,`"-1"`,`"100.555"`,`"1e5"`,`"999999999999999999"`→false, `"1000000000"`,`"100.5"`,`"2500000.00"`→ok.
- [ ] **Step 2–4:** FAIL → implement → PASS. **Step 5:** Commit.

### Task 9: Apply data rules everywhere

**Files:** Modify `modules/workflows/workflows.go` (+ new migration `000013_bank_name_unique_ci` with `CREATE UNIQUE INDEX banks_name_ci_uidx ON banks (lower(name))`, which first raises a clear error listing duplicates), `modules/cases/create.go` (`parseAmount` → `validate.Amount`), `modules/cases/payments.go`, `modules/customers/*.go`, `modules/leads/*.go`, `modules/access/users.go`, `modules/officers/officers.go`, `modules/workflows/projects.go`, `modules/bankqueries/bankqueries.go`, `modules/documents/upload.go`, `modules/http/errors.go`, `modules/http/queries_handlers.go`.
Tests: next to each module (`*_rules_test.go`).

- [ ] **Step 1: Failing tests** (one per row of spec §5):
  - `TestCreateBankDuplicate` (exact and `qa-hdfc` vs `QA-HDFC` → `ErrBankNameTaken`, 409 not 500), `TestCreateBankBlank` (`"   "` → `ErrBankNameRequired`), `TestBankNameTooLong` (101).
  - `TestAmountRules` on create case, edit case, sanctioned, payment (`100.555`, `999999999999999999`, `0` → `ErrInvalidAmount`, 400).
  - `TestPaymentFutureDate` (tomorrow IST → `ErrPaymentDateFuture`, 400 `Payment date can't be in the future.`), `TestPaymentDateTodayIST` (clock 2026-11-27 18:00 UTC = 23:30 IST, date 2026-11-27 → ok).
  - `TestEmailFormat` for user, customer, lead, contact, officer (`nope` → 400 `Enter a valid email.`).
  - `TestNameRules` for user full name, customer, lead, project, contact, officer (blank, 151).
  - `TestTextLimit` (remark/note/address 2,001 → 400).
  - `TestUploadMissingCase` (case 999999 → 404).
  - `TestSendProofNotMultipart` (form-urlencoded → 400 `Send the proof as a file upload.`).
  - `TestLargeAmountNot500` at handler level.
- [ ] **Step 2–4:** FAIL → implement (validate in services, add error mappings) → PASS; full suite.
- [ ] **Step 5:** Web: `addPayment` date input gets `max` = today; forms show server messages (existing pattern). Commit.

### Task 10: Same-stage move, upload time, stage-move response

**Files:** Modify `modules/cases/stage.go`, `modules/documents/upload.go`, `modules/http/handlers.go` (changeStage returns the same body as getCase).
Tests: `cases/stage_test.go`, `documents/upload_test.go`, `http` handler test.

- [ ] **Step 1: Failing tests:** `TestMoveToCurrentStage` → `ErrAlreadyAtStage` (409 `Already at this stage.`), no history row; `TestReplaceReturnsUploadTime` (`UploadedAt` non-zero, equals the new version's time); `TestChangeStageResponseMatchesGet` (`builder_contact`, `project_type` equal to GET).
- [ ] **Step 2–4:** FAIL → implement → PASS. **Step 5:** Commit.

### Task 11: Query proof documents can't be touched by normal uploads

**Files:** Create `migrations/000014_document_kind.up.sql`/`.down.sql` (`documents.kind text NOT NULL DEFAULT 'normal' CHECK (kind IN ('normal','query_proof'))`, backfill from `bank_query` proof references); Modify `modules/documents/upload.go`, `tx.go` (`InsertTx` sets `query_proof`), queries; Test `documents/proof_label_test.go`.

- [ ] **Step 1: Failing tests:** normal upload with label `Query 1 proof` (any case) → `ErrReservedLabel` 400 `This name is used for bank query proof. Choose another.`; matching ignores case and spaces (`query 1  PROOF`); proof via the query step still works and replacing proof only happens through query steps; backfill marks existing proof docs.
- [ ] **Step 2–4:** FAIL → implement (reserved pattern `^query \d+ proof$` after normalising) → PASS. **Step 5:** Commit.

### Task 12: Case delete locks the case; purge deletes files after commit

**Files:** Modify `modules/archive/delete.go`, `modules/archive/purge.go`; Tests in `archive/`.

- [ ] **Step 1: Failing tests:** `TestDeleteWaitsForConcurrentUpload` (hold a tx that locks the case `FOR UPDATE` and inserts a document; start delete in a goroutine; commit the first; the archive copy includes the document); `TestPurgeKeepsFilesWhenCommitFails` (file remover fake + forced failure after file step → files still present).
- [ ] **Step 2–4:** FAIL → lock case row `FOR UPDATE` before snapshot; collect file keys, remove after `Commit` → PASS. **Step 5:** Commit.

### Task 13: Graceful shutdown, health check, missed-job catch-up

**Files:** Modify `cmd/nexflow/main.go`, `cmd/nexflow/schedule.go`, `modules/http/router.go`; Create `migrations/000015_job_runs.up.sql`/`.down.sql` (`job_runs(name text PRIMARY KEY, last_day date NOT NULL)`); Tests `cmd/nexflow/schedule_test.go`, router health test.

**Produces:** `func schedule(ctx, jobs Jobs, clock func() time.Time, tick <-chan time.Time, runs JobRuns)`; `type JobRuns interface{ LastDay(ctx, name) (time.Time, bool, error); SetDay(ctx, name, day) error }`.

- [ ] **Step 1: Failing tests:** `TestDailyCatchUpOnStart` (start at 09:00 IST, last run yesterday → purge and followups run once immediately); `TestDailyNotBefore2am` (start 01:30 IST, last run yesterday → nothing until 02:00); `TestDailyOncePerDay` (two ticks after 02:00 → one run); `TestHealthzChecksDB` (closed pool → 503).
- [ ] **Step 2–4:** FAIL → implement: daily jobs run when IST time ≥ 02:00 and `last_day` < today; `SetDay` after success. Shutdown: `srv.Shutdown` with 30 s, `main` waits on a done channel, cancels the scheduler and waits for a running job, then closes the pool → PASS.
- [ ] **Step 5:** Compose backend healthcheck can't use a shell in distroless: add `nexflow healthcheck` subcommand that GETs `http://127.0.0.1:8080/healthz`; compose `healthcheck: test: ["CMD", "/nexflow", "healthcheck"]`. nginx proxies `/healthz`. Commit.

### Task 14: Email queue ready for go-live; "assigned" on create

**Files:** Create `migrations/000016_alert_skip.up.sql`/`.down.sql` (`alert_log.skipped_at timestamptz`, `skip_reason text`); Modify `modules/alerts/queries/alerts.sql`, `modules/alerts/queue.go`, `modules/cases/create.go`, `modules/cases/fromlead.go`, `modules/leads/create.go`; Tests `alerts/queue_test.go`, `cases/create_test.go`, `leads/create_test.go`.

- [ ] **Step 1: Failing tests:** `TestDispatchSkipsAfterFiveFailures` (5th failure → `skipped_at` set, reason `failed 5 times`; not picked again); `TestDispatchSkipsOld` (queued 3 days + 1 minute ago → skipped `older than 3 days`, sender not called); `TestDispatchSkipsNoEmail` (recipient without email → skipped `no email address`); `TestFailingDoNotBlockNewer` (100 failing + 1 new → new one is tried in the same run); `TestCreateCaseAssignedToOtherQueuesAlert` (creator ≠ assignee → one `assigned`; creator = assignee → none); same for lead create and lead→case.
- [ ] **Step 2–4:** FAIL → implement (`UnsentAlerts` filters `skipped_at IS NULL AND attempts < 5`, orders by `attempts, queued_at`; a skip pass runs first) → PASS. **Step 5:** Commit.

### Task 15: Team page (web) — switch off, switch on, change role

**Files:** Modify `web/src/pages/admin/Team.tsx`, `web/src/api/client.ts`, `web/src/api/types.ts`; Create `web/src/components/DeactivateDialog.tsx`; Tests `Team.test.tsx`, `DeactivateDialog.test.tsx`.

**Consumes:** Task 4 routes.

- [ ] **Step 1: Failing tests:** each active row has **Change role** (select admin/manager/employee, save) and **Switch off**; inactive rows show "Switched off" and **Switch on**; the dialog shows `Open leads 2 · Open cases 3 · Open bank queries 1`, a required "Who takes over?" picker of other active users, and when everything is 0 just "Switch off <name>?"; server 409 text is shown.
- [ ] **Step 2–4:** FAIL → implement with existing `Dialog`, `ui.ts` styles → PASS. **Step 5:** Commit.

### Task 16: Phone ☰ menu and long-name wrap

**Files:** Modify `web/src/components/TopBar.tsx`, case/customer page headers (find the customer name heading in `web/src/pages/`); Tests `TopBar.test.tsx`; e2e `web/e2e/` phone check.

- [ ] **Step 1: Failing tests:** under 640 px (`sm`), links hidden and a `☰` button (`aria-label="Menu"`, `aria-expanded`) opens a list with every link the role sees plus Log out; clicking a link closes it; Escape closes it; desktop unchanged. e2e at 360 px: case page with a 120-character unbroken name has `scrollWidth <= clientWidth`.
- [ ] **Step 2–4:** FAIL → implement (`break-words`/`[overflow-wrap:anywhere]` on name headings) → PASS. **Step 5:** Commit.

### Task 17: Smaller fixes

**Files:** `modules/reports/csv.go`, `modules/cases/edit.go`; tests beside.

- [ ] **Step 1: Failing tests:** `TestNeutraliseTabCR` (`"\t=1"`, `"\r=1"` prefixed with `'`); `TestCaseUpdatedAuditHasAmounts` (`case.updated` details have `loan_amount: {before, after}` when it changes).
- [ ] **Step 2–4:** FAIL → implement → PASS. **Step 5:** Commit.

### Task 18: Docs and final verification

**Files:** `docs/00-decisions.md` (add "10n. Round 16 decisions (2026-11-27)" with the owner's four answers and the later items), `docs/database-schema.md`, `docs/local-testing.md`, `agent.md` if it lists env vars.

- [ ] **Step 1:** Update docs.
- [ ] **Step 2:** Full verification: `docker compose run --rm --build test` all pass; `npm test`, `npm run build`, Playwright e2e (in the Playwright image) all pass; `govulncheck`, `npm audit` clean; `docker compose up -d --build`, log in at :8081, curl headers.
- [ ] **Step 3:** Commit "Record round 16 decisions".
- [ ] **Step 4:** Fresh whole-branch review (superpowers:requesting-code-review), fix findings, re-run Step 2.
