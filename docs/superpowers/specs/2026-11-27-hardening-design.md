# Hardening before production (round 16)

Status: **draft for owner review** (2026-11-27).
Source: the code review and QA test of 2026-11-27 (branch `docs/project-foundation` at `ab1da38`).

## 1. Why

Both reviews said Nexflow is not production ready. The business flows work, but the app is
missing protection and server plumbing a live system needs, plus a few things the decisions
already promised (switching off a user, a permanent audit history). The owner asked to fix
**everything found, including the medium and smaller items**, except the parts that need a
domain and a host. Those come in a later round.

## 2. Scope

**In this round:** every review and QA finding below, in sections 4 to 7.

**Later, once the owner gives a domain and hosting (not in this round):**
- sending emails (connecting an email service)
- a self-service "Forgot password" link by email (owner asked 2026-11-27)
- HTTPS, the production hosting setup
- a daily backup schedule, with copies kept off the server

**Non-goals:** new business features; changing who may do what (the role rules in
`00-decisions.md` O5 stay as they are, apart from the role change and switch-off in §4.2).

**Anti-goals:** a fix that locks real staff out for long or makes daily work slower; losing or
silently changing existing data while tightening the checks (old rows that break a new rule
stay readable, and the rule applies when they are next saved).

## 3. Owner's answers (2026-11-27)

- **Login lock:** after **5 wrong passwords in a row** for a username, that username can't log
  in for **15 minutes**. An admin resetting the password unlocks it at once. (Self-service
  reset comes with email.)
- **Switching off a leaving employee:** the admin **picks who takes over**. All the person's
  open leads, cases and bank queries move to that person, recorded in history, and the new
  assignee gets the normal "assigned" alert.
- **Payment dates:** **today or earlier only**. Future dates are refused.
- **Phone top bar:** a **☰ menu button** that opens a list of all pages (Home, Projects,
  Reports, Officers, admin pages, Log out). Desktop stays as it is.

## 4. Security and accounts

### 4.1 Login protection
- Count wrong passwords per username in the database (survives restarts). 5 in a row →
  locked until 15 minutes after the 5th. A right password before that resets the count. A
  locked username gets the same "wrong username or password" style message, plus "too many
  attempts, try again in N minutes" (Claude's call: say it plainly, since staff need to know
  to wait; it only reveals that the username exists after 5 tries).
- Unknown usernames count too (in memory, so an attacker can't tell real from fake by the
  lock).
- Also limit attempts from one network address: 20 per minute (Claude's call), answered with
  "too many attempts, wait a minute".
- Cap how many password checks run at once (Claude's call: 4) so a flood can't use up the
  server's memory; extra requests wait briefly and then get "busy, try again".
- Admin password reset clears the lock.

### 4.2 Users: switch off, switch on, change role
- **Team page** (admin only) gets **Switch off**, **Switch on** and **Change role**.
- Switch off: a dialog lists how many open leads, cases and open bank queries the person has
  and asks who takes over (required when there is at least one; only active users can be
  picked). One step, in one transaction: reassign everything, record each change in history,
  queue "assigned" alerts, switch the user off, end their sessions.
- Alerts on takeover (Claude's call): moved leads and cases queue the normal "assigned" alert.
  Bank queries have no "assigned" alert today, and adding one is a new feature, so moved
  queries send none; the move is recorded in history, and the new assignee gets the query's
  due and overdue alerts from then on, as for any query.
- Switched-off users: can't log in, disappear from every "assigned to" and assignee picker,
  stay visible by name on old records and in history (marked "switched off" on Team).
- Change role: admin, manager, employee. **The last active admin can't be demoted or switched
  off**, and an admin can't switch themselves off (Claude's calls).
- Both actions are in the audit history.

### 4.3 Link password (`NEXFLOW_LINK_SECRET`)
- Refuse to start when it is the example value from `.env.example`, or has too little
  variety (Claude's call: fewer than 16 different characters). The error says how to make one.

### 4.4 Dependencies and images
- Go toolchain to the current supported release (1.26.x or later); `govulncheck` shows no
  reachable issues.
- Web libraries: upgrade `react-router-dom`, vite, vitest so `npm audit` shows no production
  issues and no high or critical development ones.
- Backend container runs as a non-admin user; image versions pinned; nginx on the current
  stable line.

### 4.5 Browser security settings and forged requests
- nginx adds: a content security policy (own site only), no framing by other sites, HSTS
  (only sent once HTTPS exists, so prepared but off for local), `Referrer-Policy`,
  `X-Content-Type-Options`.
- The API refuses any changing request (POST, PUT, PATCH, DELETE) whose `Origin` header (or
  `Referer` when Origin is missing) isn't the app's own address. Requests with neither are
  refused too (browsers always send one; scripts must set it).

### 4.6 Permanent audit history
- The app connects to the database with a role that can only add to and read the audit
  history, never change or delete it. Migrations run with the owner role, the app with the
  limited role (the `nexflow_app` role from migration 000002 is used, or replaced if it
  doesn't fit). Local Docker setup and docs are updated to match.

## 5. Data rules (clear messages instead of server errors)

| Field | Rule |
|---|---|
| Bank name | required after trimming spaces; unique ignoring capitals; 100 characters max; duplicate gives a clear message (create and rename alike) |
| Loan, sanctioned, payment amounts | more than 0; at most ₹100 crore; at most 2 decimals (refused, not rounded) |
| Payment date | today or earlier (India time) |
| Email (users, customers, leads, contacts, officers) | valid email format when given |
| Names (users, customers, leads, projects, contacts, officers) | required after trimming; 150 characters max |
| Other free text (remarks, notes, addresses) | a sensible max (Claude's call: 2,000 characters) |
| Upload to a missing case or record | 404 "not found" |
| Moving a case to the stage it is already on | refused with "already at this stage"; no history row |
| `/send` on a query sent as a normal form | clear 400 "send the proof as a file upload", not the 10 MB message |

Existing rows that break a rule (for example QA's unnamed bank) are not changed by the
migration; the rule applies when they are next saved. Duplicate bank names already in the
database: the migration reports them and stops (Claude's call), so they are fixed by hand
first. There are none today outside QA data.

## 6. Reliability

- **Bank-query proof labels:** documents carry a kind (normal or query proof). A normal upload
  can't replace or be given a query-proof label; query proof is only added and removed through
  the query steps.
- **Deleting a case** locks the case first, like every other change, so nothing is missed in
  the archived copy.
- **Restart:** the server stops taking new requests and waits up to 30 seconds (Claude's call)
  for requests and jobs in progress before closing the database.
- **Missed 2 am jobs:** on start, and on every tick, a job that hasn't run for today runs
  (the last run date is stored).
- **Purge:** files are deleted only after the database change is saved.
- **Email queue (ready for when email is connected):** an alert is tried at most 5 times;
  alerts older than 3 days are marked "skipped" and not sent; failing alerts no longer block
  newer ones. A user with no email address gets their alerts marked "skipped" too.
- **"Assigned" alert on create:** creating a lead or case already assigned to someone else
  queues the alert, as reassigning does.
- **Health check:** `/healthz` also checks the database; Docker uses it.

## 7. Smaller fixes

- CSV: also escape cells starting with a tab or carriage return.
- Stage-move answer returns the same case fields as opening the case.
- Upload that replaces a file returns the real upload time.
- The `case.updated` history entry records the old and new loan amount.
- Phone: long unbroken names wrap instead of making the page scroll sideways; ☰ menu (§3).
- Test setup: the Docker test image includes `infra/backup`, so the backup test runs.

## 8. Approaches

1. **One round, grouped by area (chosen, Claude's call).** One plan with small tasks in the
   order security → data rules → reliability → smaller fixes, each test-first, committed one
   by one. Fixes share files (login, users, documents), so doing them together avoids touching
   the same code twice.
2. **Several separate rounds** (security now, the rest later). Slower to "production ready"
   and touches the same files repeatedly.
3. **Put a security layer in front** (a ready-made proxy with login limits and headers) instead
   of changing the app. Covers only part of §4, still needs the rest, and adds a piece the
   owner would have to run.

## 9. Left to the builder

Exact table and column names, the migration split, the Origin check details for the local
setup (`localhost:8081`), the exact wording of messages (plain, short, like existing ones),
and how to test the lock and catch-up without waiting real time (use an injectable clock as
existing tests do).
