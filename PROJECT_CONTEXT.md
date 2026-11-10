# DSA Loan Management System — Project Context

> **Purpose of this document:** Give an AI coding assistant (Codex CLI, Claude Code, or another model) the business and technical context needed to help discover, design, plan, and build this project consistently.
>
> **Suggested repository name:** `dsa-loan-management`
>
> **Document status:** Initial project context / discovery baseline. This is not yet a complete implementation specification. Items explicitly marked as undecided must be clarified during discovery rather than guessed.

---

## 1. Project overview

Build an internal loan-origination and operations management system for a small Direct Selling Agent (DSA) business that helps customers obtain loans from banks.

The business primarily handles **home loans** through HDFC, SBI, and other banks. It occasionally handles site/plot loans and LAP (Loan Against Property). Confirm the exact product terminology and scope with the business before implementing those less-common products.

The system should replace the current informal, phone-call-driven way of tracking leads and applications. It should give managers and employees a shared view of customers, leads, loan applications, workflow progress, pending actions, documents, and business reports.

### Product boundary

- This is initially a tool for **one existing business**.
- It is for internal staff only.
- Customers will not have accounts and will not log into the system.
- The system may produce customer-shareable document checklists or other exports, but this does not mean customers have portal access.
- Build a useful first version for the uncle's business. Do not design or implement a multi-tenant SaaS product unless a later explicit decision changes the scope.
- Commercialisation may be considered in the future, but it is not a current requirement.

### Primary outcome

At any point, a manager should be able to answer:

- What leads have come in?
- Who first received each lead?
- Who is currently responsible for progressing it?
- Which customer and loan applications are associated with it?
- Which bank is handling each application?
- What stage has each application reached?
- What is waiting on the customer, employee, or bank?
- Which documents are still missing or need verification?
- Which applications were sanctioned or disbursed during a selected period?
- Where is manager intervention or escalation needed?

---

## 2. Business background

### Business type

A DSA/intermediary business that sources customers and assists them through bank loan application processes.

### Loan products

| Product | Expected frequency | Notes |
|---|---:|---|
| Home loan | Primary | Main workflow to design first |
| Site/plot loan | Rare | Confirm exact product terminology and bank-specific process |
| LAP (Loan Against Property) | Rare | Include only when the workflow is understood |

The system must not assume that all loan products or banks follow the same process.

### Banks

- HDFC
- SBI
- Other banks may be added over time.

Bank-specific workflows and document requirements should be configurable or represented in a maintainable way. Avoid hardcoding one bank's process as the universal workflow.

### Lead sources

Leads can originate from:

1. Project leads.
2. Referrals from existing customers.
3. Existing customers seeking another loan.
4. Balance-transfer opportunities where a customer moves an existing loan to another bank.
5. Other sources that may be identified later.

The system should record the source of a lead in a way that supports reporting. The exact source taxonomy and any additional fields can be finalised during detailed requirements discovery.

---

## 3. Team and users

### Initial team

- 2 managers: the business owner (the user's uncle) and his business partner.
- 2 existing employees.
- 2 additional employees are planned.
- The user will participate occasionally.

Expected initial user count: approximately 6–7. Expected near-term maximum: around 10 users.

### How work is assigned today

- Managers usually receive leads first.
- Employees may also receive leads directly, particularly leads related to projects they handle.
- Either a manager or an employee may have the initial discussion with a customer.
- A manager may later assign the lead/application to an employee for further processing.
- A manager may step in and communicate with the customer whenever required.

### Important ownership distinction

Do not model all responsibility as one simple `owner` field.

The system should be able to distinguish at least:

- **Original lead receiver:** the person who first received the lead.
- **Current assignee / responsible employee:** the person expected to progress the work.
- **Manager oversight:** the manager responsible for oversight, where applicable.
- **Other participants:** staff members who may communicate with or assist the customer.

A lead may be reassigned. The system should preserve assignment history, including who assigned it, to whom, and when. The exact role and permission rules still need discovery.

### Activity logging philosophy

The business does **not** want every phone call logged.

Record meaningful business events, such as:

- Lead created.
- Lead assigned or reassigned.
- Application created.
- Application submitted to a bank.
- Bank processing started or updated.
- Important bank query received.
- Documents requested, received, or verified.
- Sanction received.
- Bank visit or signing completed.
- Disbursement completed.
- Application put on hold, rejected, or cancelled.
- Important notes or manager intervention.

Provide a concise notes/update facility for relevant context. Do not build call recording, call tracking, employee surveillance, or a requirement to log every customer interaction.

---

## 4. Core domain concepts

The initial domain model should distinguish these concepts:

### Lead

A potential loan opportunity received by the business.

A lead captures its source, initial receiver, current assignment, status, and relevant basic information. A lead may or may not become a customer/application.

### Customer

The person or people seeking a loan. Customer information should be reusable rather than duplicated every time the same person approaches the business.

The business has not yet finalised the exact customer fields. For now, use the generic section/concept name **Customer Data**.

Potential categories of customer information (not a final field specification):

- Personal and contact information.
- Employment and income information.
- Existing financial commitments.
- Property and proposed loan information.
- Supporting documents.

Do not invent a final exhaustive field list or validation rules. Discover these with the business before implementing the detailed data model.

### Loan application

A specific loan request being processed with a particular bank.

A customer may have multiple loan applications over time, including applications with different banks. Each application must have its own:

- Bank.
- Loan product/type.
- Submission method.
- Workflow state and milestone dates.
- Document checklist and document status.
- Important notes and event history.
- Outcome, such as sanctioned, rejected, cancelled, or disbursed.

Do not treat a customer, lead, and loan application as the same record.

### Important modelling principle

One customer can have multiple applications. A lead may lead to a customer and one or more loan applications. The exact relationship and conversion rules should be documented during data-model design.

---

## 5. Loan application workflows

The system must support different workflows depending on bank, loan product, and submission method.

### Example: HDFC home-loan flow

A representative flow discussed by the business:

1. Collect initial customer details.
2. Generate and send the application link.
3. Record that the link was sent.
4. Customer/employee completes the application.
5. Application is submitted.
6. Bank receives and processes the application.
7. Bank may request clarifications or additional documents.
8. Sanction is received.
9. Customer visits the bank for physical signing/documentation and carries the required documents.
10. Disbursement is completed.

This is an illustrative workflow, not a final exhaustive HDFC specification. Confirm actual stages, transitions, and terminology with the business.

### Example: SBI home-loan flow

A representative flow discussed:

1. Collect the required customer documents.
2. Visit the branch and submit/log the application physically.
3. Record that the bank has received the application.
4. Continue through the bank's processing stages.
5. Handle queries or additional document requests.
6. Record sanction.
7. Complete signing/documentation and disbursement steps.

### Workflow requirements

- Support both online and physical submission.
- Do not assume all banks use application links.
- Allow bank-specific and, where necessary, loan-product-specific stages.
- Record milestone dates, not just the current stage.
- Preserve application status history.
- Allow meaningful updates and notes.
- Support customer-side, employee-side, and bank-side pending actions.
- Make it possible to identify the next action and responsible person.
- Avoid an unnecessarily complex workflow engine in the first release. Start with a well-structured, maintainable configurable workflow model appropriate for a small business.

### Statuses, flags, and exceptions

Keep the main application stage separate from temporary flags or exceptions.

For example, an application could be:

- Main stage: `Bank Processing`
- Flag: `Escalation`
- Escalation reason: a bank delay or unresolved issue
- Responsible manager: a named manager
- Resolution state: pending/resolved

Other possible exception or waiting conditions include:

- Customer response pending.
- Documents pending.
- Bank query received.
- On hold.
- Rejected.
- Cancelled.

These are examples for discovery, not a final enum list. Avoid combining every temporary condition into one large status field. Decide the exact lifecycle and whether each item is a status, flag, task, or outcome during design.

---

## 6. Document management and checklists

Document handling is an important part of the business process, especially where applications or signing are handled physically.

### Requirements

- Maintain document checklists appropriate to the bank, loan product, and customer profile (for example, salaried or self-employed, if relevant).
- Consider separate checklists for initial application submission and post-sanction/signing/disbursement.
- Track each required document's state. Candidate states include:
  - Requested.
  - Received.
  - Verified.
  - Pending / missing.
  - Rejected or needs re-submission, if the business confirms this is needed.
- Allow staff to upload and manage documents securely.
- Show what is missing and who needs to act.
- Preserve useful document history, such as replacement or re-submission, where appropriate.
- Generate a customer-friendly checklist that staff can share through channels such as WhatsApp or as a PDF.
- Customers do not log in to the system or upload documents through a customer portal in the initial scope.
- Checklist templates should be maintainable as bank requirements change.

### Security note

Loan files may contain highly sensitive identity, income, banking, and property information. Use private storage and controlled access. Do not expose public file URLs.

The exact document types, file-size limits, retention rules, and access permissions must be established during detailed discovery.

---

## 7. Reporting and dashboards

The business needs both day-to-day operational visibility and business-performance reporting.

### Daily operational view

The dashboard/reporting capability should help staff identify:

- New leads.
- Unassigned leads.
- Leads/applications needing follow-up.
- Customer action pending.
- Documents pending.
- Applications in bank processing.
- Bank queries.
- Escalations.
- Sanctions received.
- Bank visits or signing/documentation pending.
- Disbursements completed.
- Follow-ups due or overdue.

The final definitions and filters for each metric must be agreed with the business.

### Business-performance view

Potential measures:

- Number of leads.
- Leads by source.
- Applications by bank.
- Applications by employee/assignee.
- Sanctioned applications.
- Disbursed applications.
- Sanctioned/disbursed loan amounts.
- Rejected and cancelled applications.
- Conversion rates.

Avoid ambiguous metrics. Define the numerator, denominator, date basis, and inclusion/exclusion rules before implementation.

### Date filtering and milestone reporting

Support useful date filters such as:

- Today.
- Yesterday.
- This week.
- This month.
- Custom date range.

**Important:** Reports must use the date of the actual milestone/event being reported, not only the application's current status.

Example: If an application is sanctioned today and disbursed next month, it must appear in today's sanction report even though its current status may later become `Disbursed`.

This implies storing milestone/event dates and designing report queries around them.

---

## 8. Commission and accounting boundary

The bank calculates and pays commission directly into the business's account. The business does not want the initial system to calculate, reconcile, or manage bank commission.

Therefore:

- Commission calculation is **out of scope for the initial release**.
- Commission reconciliation and accounting are also out of scope.
- Do not build financial accounting features without a new explicit requirement.
- This area may be revisited as future scope if the business requests it.

---

## 9. Platform and user experience

### Required platforms

- Web application.
- Mobile application.

The user has requested feature availability on both platforms. Both should use the same backend, database, and business rules.

Likely usage pattern:

- Employees may use mobile devices to update application progress and upload documents while working.
- Managers may primarily use the web dashboard for oversight and reporting.
- This usage pattern must not become a reason to omit important functionality from either platform without an explicit product decision.

### Internal-only access

- No customer accounts.
- No customer-facing portal in the initial release.
- Staff authentication is required.
- Role-based permissions should be designed after discovering the actual responsibilities of managers and employees.

### UX principles

- Keep routine updates fast and straightforward.
- Make current stage, pending action, assignee, and next step easy to understand.
- Avoid unnecessary data entry.
- Do not force staff to log every call.
- Provide useful search and filtering for customers, leads, and applications.
- Make mobile document capture/upload practical.
- Use clear labels understandable to business users; avoid exposing technical jargon.

---

## 10. Initial technical direction

The following is a proposed starting point, not an irrevocable technology decision. Validate it against current tooling, team capability, cost, and maintainability before implementation.

### Proposed architecture

A **modular monolith** is preferred for the initial scale. Do not start with microservices or Kubernetes.

| Area | Proposed technology | Rationale |
|---|---|---|
| Web | React + TypeScript | Mature ecosystem and maintainable UI |
| Mobile | React Native + Expo | Shared TypeScript ecosystem with web |
| Backend | Python + FastAPI | Familiar, typed API development |
| Database | PostgreSQL | Relational data and reporting |
| File storage | Private S3-compatible object storage | Secure document storage |
| Authentication | Managed auth or a well-supported backend auth approach | Avoid implementing unsafe auth primitives |
| Deployment | Docker-based deployment | Repeatable environments |
| Reverse proxy / TLS | Caddy or Nginx | HTTPS and routing |
| CI | GitHub Actions | Automated checks and deployment pipeline |
| Error monitoring | Sentry or equivalent | Operational visibility |
| Scheduled work | Simple scheduled jobs initially | Reminders and recurring reports if needed |

The architecture should be deliberately simple and support approximately 6–10 internal users initially.

### Design principles

- One backend and one source of truth.
- Clear modular boundaries (for example: identity/access, leads, customers, applications, workflows, documents, reporting, notifications).
- Typed API contracts.
- Database migrations committed to version control.
- Automated tests for business-critical rules.
- Separate development, staging (if justified), and production configuration.
- Environment variables/secrets must not be committed.
- Avoid premature abstractions and infrastructure complexity.
- Keep the option to evolve the system without building a SaaS platform now.

### Cost awareness

The business is small and the system is initially for a limited number of users. Keep infrastructure economical and explain recurring costs before adopting paid services. Any cost estimates must be rechecked against current provider pricing, taxes, storage, backups, and usage.

---

## 11. Security, privacy, and reliability

This application will process sensitive personal and financial information. Security is a core requirement, not a later enhancement.

At minimum, the design must address:

- HTTPS for all traffic.
- Secure authentication and session handling.
- Role-based access control and server-side authorization checks.
- Private document storage.
- Short-lived, authorised access links for files when needed.
- Encryption in transit and appropriate encryption at rest.
- Safe handling of secrets and API credentials.
- Audit history for important business changes.
- Database backups.
- Independent backup of uploaded documents/object storage.
- Tested restore procedures.
- File type and size validation.
- Protection against unauthorised document enumeration/access.
- Appropriate logging without leaking personal or financial data.
- Data retention and deletion policies.
- Applicable Indian data-protection and privacy requirements, to be reviewed before production use.

Do not assume a database backup automatically includes uploaded files. Database and object-storage backup/restore plans must both exist.

Do not collect data just because it might be useful someday. Collect only what the business needs to process and manage loans.

---

## 12. Scope boundaries

### Initial scope direction

The initial product should focus on:

1. Staff authentication and basic roles/permissions.
2. Lead capture and source tracking.
3. Assignment, reassignment, and assignment history.
4. Customer Data records.
5. Multiple loan applications per customer.
6. Bank-specific application workflow and milestone tracking.
7. Important notes and event history.
8. Document uploads and checklist templates/statuses.
9. Operational dashboard and date-filtered reports.
10. Secure web and mobile access.

The precise release order should be decided in the implementation plan after discovery and technical design.

### Explicitly not in initial scope

- Customer login or customer portal.
- Multi-tenant SaaS architecture.
- Commission calculation or reconciliation.
- Full accounting/ledger functionality.
- Logging every phone call.
- Call recording or employee surveillance.
- Microservices/Kubernetes.
- Unnecessary AI features or automation before core workflows are reliable.

Potential future features must not distract from delivering a dependable first version.

---

## 13. Open questions — do not guess

The following areas still require discovery. Ask the user/business one focused question at a time, prioritising questions that affect the domain model or core workflows.

1. **Exact Customer Data fields:** To be decided later with the business. Do not block initial project setup on this.
2. **Lead lifecycle:** Exact stages from lead received through qualified, converted, inactive, lost, or other outcomes.
3. **Application lifecycle:** Exact bank-specific stages and allowed transitions for the most common HDFC and SBI home-loan flows.
4. **Role permissions:** What each manager, employee, and occasional administrator can view, create, edit, assign, export, or delete.
5. **Assignment rules:** Whether employees can reassign, whether manager approval is needed, and how manager oversight is represented.
6. **Multiple applicants:** How co-applicants, joint borrowers, guarantors, and property owners should be represented.
7. **Bank and product configuration:** Who can maintain bank workflows, products, and checklist templates.
8. **Documents:** Exact checklist contents, verification rules, file limits, retention, and whether document versions are required.
9. **Tasks and reminders:** Whether the business needs due dates, follow-up scheduling, overdue alerts, or notifications, and through which channels.
10. **Reports:** Exact metric definitions, filters, export needs, and who can access each report.
11. **Search:** Required search fields and duplicate-customer/lead handling.
12. **Audit and deletion:** Which actions require immutable history, and what correction/deletion policy is appropriate.
13. **Deployment and operations:** Hosting preference, budget confirmation, backups, support, and who will administer production.
14. **Other loan products:** Confirm site/plot loan terminology and the actual LAP workflow before adding them.

When a question is not needed for the current phase, defer it rather than forcing an early decision.

---

## 14. How the AI coding assistant should work

This project should be developed using the **Superpowers** methodology (repository: https://github.com/obra/superpowers), where compatible with the selected coding CLI and its current installation instructions.

### Required working approach

1. Start by understanding the repository and this context document.
2. Do not immediately generate the entire application.
3. Use a discovery/brainstorming process to turn this context into explicit business requirements.
4. Ask focused questions one at a time when a decision is genuinely needed.
5. Keep a record of confirmed decisions and unresolved questions.
6. Produce and review specifications before implementation.
7. Design the domain model and workflows before creating database tables.
8. Create an implementation plan broken into small, testable increments.
9. Use test-driven development for business logic and critical workflows.
10. Implement incrementally; run relevant tests and checks after each change.
11. Review security, permissions, data integrity, and error handling as part of each relevant feature.
12. Update project documentation when decisions or behaviour change.

### Do not

- Invent business rules, customer fields, bank stages, permissions, or document requirements.
- Treat assumptions in this document as confirmed specifications where they are labelled illustrative or undecided.
- Build every feature in one giant code-generation step.
- Introduce microservices, Kubernetes, or complex infrastructure without a demonstrated need.
- hardcode one bank's workflow for all banks.
- conflate leads, customers, and loan applications.
- overwrite milestone history when an application moves to a later stage.
- make customer documents publicly accessible.
- add commission/accounting features without approval.
- add a customer portal without approval.
- log every call or introduce employee surveillance.

### Expected first tasks

Before writing production application code:

1. Inspect the repository and available development environment.
2. Read this document fully.
3. Confirm the intended use and availability of the Superpowers skills for the chosen CLI.
4. Run a structured business-requirements discovery session, using the open questions above as a guide.
5. Produce an initial set of reviewed documents, such as:
   - `docs/01-business-overview.md`
   - `docs/02-business-requirements.md`
   - `docs/03-functional-requirements.md`
   - `docs/04-user-roles-and-permissions.md`
   - `docs/05-loan-workflows.md`
   - `docs/06-document-management.md`
   - `docs/07-reporting-requirements.md`
   - `docs/08-non-functional-requirements.md`
   - `docs/09-technical-architecture.md`
   - `docs/10-implementation-plan.md`
6. Ask for review/approval of the requirements and design before starting implementation.

These filenames are a suggested documentation structure. Combine or split documents if that makes the specification clearer, but preserve traceability between business needs, requirements, design, and tests.

---

## 15. Initial project success criteria

The first useful release should allow the business to:

- Add and find leads.
- Know who first received each lead and who currently handles it.
- Reassign work while preserving assignment history.
- Maintain reusable customer records.
- Track more than one loan application for a customer.
- See each application's bank, current stage, important milestone dates, and pending action.
- Handle different bank submission processes.
- Know which documents are missing, received, or verified.
- Securely upload and retrieve documents.
- Identify escalations and applications requiring attention.
- See daily operational status and meaningful business-performance reports.
- Use the system on both web and mobile.
- Restrict access to authorised staff.

The product is successful when it gives the managers a reliable, current picture of the business and reduces missed follow-ups, missing documents, and uncertainty about application progress—without creating excessive data-entry work for employees.

---

## 16. Current decision summary

| Topic | Current direction |
|---|---|
| Repository | `dsa-loan-management` |
| Initial customer | One existing DSA business |
| Product focus | Home loans first |
| Other products | Site/plot loans and LAP are occasional; confirm details |
| Banks | HDFC, SBI, and extensible to others |
| Users | Around 6–7 initially; around 10 expected near-term |
| Access | Internal staff only |
| Platforms | Web and mobile |
| Customer portal | Not required |
| Lead sources | Project leads, referrals, repeat customers, balance transfers, others |
| Assignment | Track original receiver, current assignee, manager oversight, and history |
| Activity logging | Important milestones and notes; not every call |
| Workflows | Bank/product-specific; online and physical submission |
| Documents | Checklists, status tracking, secure uploads, shareable customer checklist |
| Reports | Operational and performance reporting with milestone-date filters |
| Commission | Excluded from initial scope |
| Architecture | Modular monolith preferred |
| Development method | Superpowers-inspired discovery, specification, planning, and TDD |
| Immediate next step | Continue business discovery and produce reviewed requirements before implementation |

---

**End of initial context.** Treat this as the baseline for further discovery, not permission to fill in every unspecified detail.
