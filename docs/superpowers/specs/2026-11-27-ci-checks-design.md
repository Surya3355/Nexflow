# Automatic checks on pull requests (CI)

Status: **approved by owner** (2026-11-27, "yes, set up the workflows" after seeing the four checks).

## Why

Tests only run when Claude runs them on the owner's computer. The owner wants GitHub to run
them on every pull request, so nothing is merged with a failing test or a known security issue.

## Decided

- Runs on every pull request into `main`, and on every push to `main`.
- GitHub-hosted runners (`ubuntu-latest`): free and unlimited, since the repository is public.
- Four checks, shown separately on the pull request:
  1. **Backend tests**: `go vet` and the whole Go suite against a fresh PostgreSQL 16 service
     (includes the non-superuser owner migration test and the backup retention check).
  2. **Web tests and build**: `npm ci`, `npm test`, `npm run build`.
  3. **Phone checks**: the Playwright phone-width tests (`npm run test:e2e`).
  4. **Security scan**: `govulncheck` for Go and `npm audit --audit-level=high` for the web.

## Claude's calls

- The workflow may only read the code (`permissions: contents: read`). A newer push to the same
  pull request cancels the run still going.
- Versions come from the repo: Go from `backend/go.mod`, Node 24 as in `web/Dockerfile`,
  Playwright from `package.json`.
- Not now: Dependabot (automatic pull requests for library updates). The owner can turn it on later.
- Not checked: Go formatting (two older files are not formatted; fixing them is unrelated).

## Approaches

1. **One workflow file with four jobs (chosen):** each check shows on its own, and they run side by side.
2. Docker Compose inside the runner, as on the owner's computer: slower (builds images) and
   harder to read when it fails.
3. A runner on the owner's own machine: needs the PC switched on and adds risk for a public repository.
