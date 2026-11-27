# Running and testing Nexflow locally

This is for local testing only. Not yet set up for a live server: no hosting, no email
sending yet, local file storage, and plain HTTP cookies.

## Option A: Docker (easiest)

Needs Docker Desktop (or Docker Engine with the Compose plugin).

1. Create your settings file and set a secret of 32+ characters:

       cp .env.example .env
       # edit .env: NEXFLOW_LINK_SECRET=some-long-random-text-of-32-chars-or-more

2. Start everything (PostgreSQL, API, web app):

       docker compose up --build

3. Create the first admin account (only works while the database has no users):

       docker compose exec backend /nexflow create-admin owner "Your Name"
       # it asks for the password on the next line; or set NEXFLOW_ADMIN_PASSWORD in the command

   The image has no shell, so if `exec` fails, run it as a one-off with the password in the environment:

       docker compose run --rm -e NEXFLOW_ADMIN_PASSWORD='choose-a-password' backend /nexflow create-admin owner "Your Name"

4. Open the app at http://localhost:8081 and sign in. The API is at http://localhost:8080.

5. Run the Go test suite against the compose database (the `test` service only runs
   when asked; `docker compose up` does not start it):

       docker compose run --rm --build test

6. Stop, or reset everything including data:

       docker compose down          # keeps data
       docker compose down -v       # deletes the database and uploaded files

## Option B: without Docker

Needs: Go 1.27+, Node 24+, PostgreSQL 15+ (16 recommended), and a database called `nexflow`.

1. Settings. Export these in your shell (or put them in `.env` and `source` it):

       export NEXFLOW_DB_URL="postgres://USER:PASS@localhost:5432/nexflow?sslmode=disable"
       export NEXFLOW_LINK_SECRET="some-long-random-text-of-32-chars-or-more"
       export NEXFLOW_UPLOAD_DIR="$PWD/uploads"
       export NEXFLOW_INSECURE_COOKIES=true
       export NEXFLOW_TEST_DB_URL="postgres://USER:PASS@localhost:5432/postgres?sslmode=disable"

   The test user must be allowed to create databases (tests make a fresh one each run).

2. API (from `backend/`):

       go run ./cmd/nexflow create-admin owner "Your Name"    # prompts for the password
       go run ./cmd/nexflow                                   # API on :8080; migrations run at start

3. Web app (from `web/`, in another terminal):

       npm install
       npm run dev                                            # http://localhost:5173, /api is forwarded to :8080

## Running the tests

Backend (from `backend/`; needs NEXFLOW_TEST_DB_URL, otherwise database tests are skipped with a message):

    go test ./...                       # everything, including the 21 acceptance checks
    go test ./acceptance/ -v            # only the spec §13 acceptance checks
    go test ./modules/cases/ -run Tracking -v   # one area

Backup retention (plain bash, no database needed):

    bash infra/backup/test_retention.sh

Web (from `web/`):

    npm test                            # unit and component tests (Vitest)
    npm run build                       # type check and production build
    npx playwright install chromium     # once; needs system libraries on Linux
    npm run test:e2e                    # phone-width browser check at 360 px

If Playwright reports missing libraries on Linux, run `npx playwright install-deps chromium` (needs sudo).

## What to try by hand

1. Sign in as the owner. Admin → add a bank, add a user (employee, with an email).
2. Sign in as the employee. New customer → enter a mobile. Enter the same mobile again: the warning links to the first customer.
3. Open the customer → open a new case (bank, amount, assignee).
4. Open the case → move to "Logged in" (asks for a tracking number), then to "Sanctioned" (shows a warning if the stage needs documents).
5. Upload a PDF on the case, then Open it (a short-lived link).
6. Search for the name, the mobile (with +91), the tracking number, or the bank.
7. Dashboard and Reports; Export CSV opens in Excel.
8. Drop a case: choose "Other" and try without a remark (refused).
9. Projects (admin): Admin → Banks & lists → Projects → add "Prestige". NA is always first and cannot be renamed.
10. Open a new case: the Project box starts empty; type "pre" to find Prestige, or pick NA. Saving without a project is refused.
11. Projects in the top bar: open Prestige, see its cases, Download CSV (opens in Excel). Home and Reports have a Project filter.
12. New customer with a mobile that already exists: the warning offers "Add this case to <name>". For a dropped customer it offers "Bring back <name> and add this case"; their old dropped cases stay dropped.
13. Tracking number: type "ab12" when moving to Logged in, it shows and saves as "AB12"; "ab-12" is refused.

## Known limits in this local setup

- Email is not sent. Alerts stay queued until SMTP is configured.
- Uploaded files are kept on the local disk (`NEXFLOW_UPLOAD_DIR`).
- No backup copy off the machine; `infra/backup/backup.sh` needs `pg_dump` installed.
- Cookies are not marked secure (plain HTTP). Keep `NEXFLOW_INSECURE_COOKIES=true` for local use only.
- There is no login rate limiting.
- The Docker images were written but not built on the machine where this was prepared (Docker was not installed there). If `docker compose up --build` fails, send the error and it can be fixed.
