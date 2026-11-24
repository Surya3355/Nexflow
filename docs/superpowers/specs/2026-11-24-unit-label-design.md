# Unit number label by project type (round 15, task 5)

Status: **approved by owner 2026-11-24.** Backlog: `2026-11-22-round-15-backlog.md`, Task 5. Small change.

## Why
Staff enter the flat, villa or plot number when a lead becomes a case. The label should say which, and builder projects
should always have it, so the case can be matched to the unit the builder sells.

## Decisions (owner)
- Label by project type: High-rise apartments and Apartments → **Flat no.**; Villas → **Villa no.**; Plots → **Plot no.**;
  NA and old projects with no type → **Unit no.** Hint: "e.g. A-1012" (block letters are allowed).
- **Required on builder projects** (project has a type) at Add case, from a lead or "+ New case". Optional on NA and on
  old untyped projects.
- **Existing cases:** not forced. "Not set" stays and can be filled in any time. On a typed project an existing number
  can be changed but not cleared. Moving a case to another project does not ask for it.

## Approaches
- **Server rule + web labels (chosen):** the server refuses an empty unit on a typed project; the web labels and checks
  it first. Matches how every Nexflow rule is enforced.
- Web only: rejected, any other path (API, future screens) could skip the rule.

## Not changing
Allowed characters and length (`^[A-Z0-9][A-Z0-9 /-]{0,19}$`, saved in capitals), reports, CSV.

## Errors
`cases: enter the unit number for this project` (server). The web shows "Enter the flat no." / "villa no." / "plot no.".
