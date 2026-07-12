# Progo codebase critical review — current state

**Original audit date:** 2026-07-12

**Current review date:** 2026-07-12

**Original audited commit:** `695e5ebd1ea8b620b21b4620ed2b281fc60174d5`

**Current reviewed commit:** `7b4ab01436316b3c50307589fd3d10481ebcd13f`

**Remediation series:** 56 conventional commits, 239 files changed
**Scope:** backend, frontend, migrations, authentication, federation, Docker/Compose, deployment, observability, CI/CD, tests, and expected project-management capabilities.

## Executive verdict

The repository is no longer in the state described by the original review. The major security, dependency, session, CI, operational, accessibility, and baseline product gaps have been remediated.

The current checkout is a credible **controlled-deployment production candidate**. It is not independently certified for an unrestricted public deployment: there has been no external penetration test, long-duration load test, real third-party federation interoperability campaign, or production incident exercise. Production also depends on operators configuring SMTP, malware scanning, off-host encrypted backups, OAuth credentials, TLS, and monitoring correctly.

Of the 52 original findings:

| Disposition | Count | Meaning |
|---|---:|---|
| Resolved | **45** | The reported defect or missing capability has an implemented control and relevant verification. |
| Partially resolved | **7** | Material remediation exists, but maintainability or depth can still be improved. |
| Open critical/high release blockers from the original audit | **0** | No original critical security or baseline-feature blocker remains unimplemented. |

The seven partial residuals are `OE-2`, `OE-3`, `OE-6`, `STD-10`, `TEST-3`, `TEST-5`, and `TEST-7`. They are described honestly below.

## Current validation evidence

### Backend

- `go test -count=1 -covermode=atomic -coverprofile ... ./...` — passed.
- PostgreSQL integration suite with the real 40-migration schema — passed.
- Merged unit and integration statement coverage — **56.3%**.
- Core merged coverage: project **51.2%**, ticket **46.7%**, comment **52.4%**, notification **51.8%**.
- `go vet ./...` — passed.
- Four fuzz targets ran locally without a crash or invariant failure: HTTP Message Signature fields, inbound ActivityPub activities, federation URLs, and GitHub repository identifiers.
- `govulncheck@v1.6.0 ./...` — **0 reachable vulnerabilities**. It reported vulnerable symbols only in unused dependency paths: 3 imported-package and 24 required-module advisories were not called by this code.

### Frontend

- TypeScript project build — passed.
- ESLint — passed.
- Vitest — **10 files / 17 tests passed**.
- Production Vite build — passed.
- Main client chunk — 493.68 kB minified / 148.80 kB gzip; graph functionality remains a separate 194.59 kB chunk.
- Complete dependency audit — **0 advisories** across 555 production, development, and optional dependency entries.
- Production dependency audit — **0 advisories** across 157 entries.

### Live Docker

The complete local stack was rebuilt from the reviewed commit.

| Service | Result |
|---|---|
| PostgreSQL | Running and healthy. |
| Redis | Running and healthy. |
| Migrations | Exited 0; schema is `40`, `dirty=false`. |
| API | Running and healthy. |
| Worker | Running; Asynq processing started, delivery worker started, metrics listening on `:9091`, and due-notification candidates were processed. |
| Frontend | Running; HTTP 200. |
| Prometheus | Running and successfully scraping protected metrics. |

Live behavior:

- `/health` returned 200 with dependency-free liveness.
- `/ready` returned 200 with PostgreSQL, schema, and Redis all `ok`.
- an unauthenticated authenticated-API request returned 401.
- metrics rejected an unauthenticated request and returned 200 with the configured bearer token.
- `go_sql_max_open_connections{db_name="primary"}` reported 25.
- the frontend emitted CSP, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, and a strict referrer policy.

### Restore exercise

A second Compose project was created on separate ports and isolated PostgreSQL, Redis, and attachment volumes. The drill:

1. created database and attachment probes with `before-backup`;
2. created and validated a PostgreSQL custom-format dump and attachment archive;
3. mutated both probes to `after-backup`;
4. stopped the isolated API, dropped and recreated its database, restored the dump, replayed migrations, and restored attachments;
5. confirmed both probes returned to `before-backup`;
6. confirmed schema `40:false` and healthy readiness after restore.

The exact POSIX `backup.sh` and `restore-backup.sh` wrappers could not run natively on this Windows host because neither `sh` nor a WSL distribution is installed. CI runs those exact wrappers on Ubuntu; the local exercise verified their underlying destructive database, attachment, migration, and readiness operations against disposable infrastructure.

# 1. Overengineering and maintainability

## OE-1 — Product-priority inversion

**Status: Resolved**

Federation remains a defining feature, but the baseline product is no longer missing around it. The remediation added account recovery, verified email, MFA, session inventory, labels, indexed search, pagination, due dates, activity history, attachments, richer notifications, archive/restore, portability, API tokens, and outbound webhooks.

The correct current strategy is federation-first differentiation on top of a viable PM baseline, not federation instead of a baseline.

## OE-2 — Oversized files

**Status: Partially resolved — medium residual maintainability risk**

Material decomposition is complete in the two worst original hotspots:

- the 2,105-line project repository is split by project lifecycle, membership, roles, and invites;
- the 1,312-line project settings component is split, with GitHub settings lazy-loaded;
- common ticket workspace primitives and the generic realtime hub were extracted.

Remaining handwritten hotspots still deserve later decomposition:

| File | Current approximate lines | Residual concern |
|---|---:|---|
| `internal/activitypub/remoteinbox/repository.go` | 1,162 | Multiple inbound activity applicators remain together. |
| `internal/user/service.go` | 1,126 | Local accounts, sessions, OAuth, and provider HTTP code share one service file. |
| `cmd/api/main.go` | 1,106 | Composition, middleware, probes, and process lifecycle remain together. |
| `internal/activitypub/federation/service.go` | 1,044 | Discovery, remote projects, tickets, and document parsing remain together. |
| `internal/ticket/repository.go` | 1,014 | CRUD, ranking, ActivityPub projection, labels, and recipients remain together. |

These are no longer release blockers, but they remain the clearest source-level maintainability debt.

## OE-3 — Duplicated local and remote workspaces

**Status: Partially resolved — low/medium residual risk**

Local and remote pages now share `TicketBoard`, `ProjectTicketSummary`, classification controls, common formatting, and pagination controls. Federation-specific transport and mutation orchestration still live in the remote page, while the local page manages richer local capabilities. A further mutation-adapter abstraction is possible, but forcing complete unification would hide meaningful protocol differences.

## OE-4 — Duplicated realtime hubs

**Status: Resolved**

Ticket and notification event fanout use one typed `internal/realtime` implementation with bounded subscriber buffers, Redis reconnection, echo suppression, and close behavior. Concurrent fanout has a bounded load test and the Linux CI race detector covers this code.

## OE-5 — Ceremony-heavy quality gates

**Status: Resolved**

Documentation enforcement is limited to exported API/package documentation. The architecture scanner excludes generated/cache/temp trees and has regression coverage. CI time is now concentrated on behavior, race, vulnerabilities, integration, browser accessibility, fuzzing, coverage, and restore safety.

## OE-6 — Manual API contract duplication

**Status: Partially resolved — medium residual contract risk**

OpenAPI now generates the frontend schema, `openapi-fetch` provides a typed transport, CI rejects stale generated output, and runtime Echo routes are compared with the OpenAPI paths. DTO-shape contract tests cover important schemas.

Residual: the Go handlers and the OpenAPI document are still authored separately, and not every live request/response is automatically validated against its schema. A generated Go server boundary or response-validation middleware would close the final gap.

## OE-7 — Blue-green complexity without verification

**Status: Resolved for the documented single-VM requirement**

Zero/low-downtime single-VM deployment remains an explicit operational choice. The deployment now preserves previous state on failed public checks, pins/verifies artifacts, validates Compose, creates verified backups, supports guarded restoration, and has disposable CI restore coverage. The implementation is complex, but now has matching safety controls.

## OE-8 — Dormant label schema

**Status: Resolved; original evidence corrected**

The original review incorrectly treated an integration safety table as dormant application schema. The old bigint tables were removed by migration 5. A real UUID label model is now implemented end to end with case-insensitive uniqueness and ticket assignments.

## OE-9 — Eager frontend routes

**Status: Resolved**

Routes and heavy secondary panels are lazy-loaded. The graph visualization, GitHub settings, administration, account, invitation, and federation surfaces build as separate chunks.

## OE-10 — Docker cache context

**Status: Resolved**

Backend cache, coverage, binary, and temporary artifacts are excluded from the Docker context. Live builds transferred a small backend context rather than the local Go cache.

# 2. Vulnerabilities and security

## SEC-1 — Unpatched Go baseline

**Status: Resolved**

Go is aligned on 1.26.5 in the module/toolchain, Docker build, and CI. Current `govulncheck` found zero reachable vulnerabilities.

## SEC-2 — Vulnerable frontend graph

**Status: Resolved**

Direct dependencies, transitive overrides, and the lockfile were upgraded; pnpm 11.1.2 is pinned. Current complete and production-only audits both report zero advisories.

## SEC-3 — Long-lived JWT in `localStorage`

**Status: Resolved**

Browser authentication uses a 12-hour `HttpOnly`, `Secure`, `SameSite=Strict` cookie. Tokens are not persisted or decoded from browser storage. Logout, password changes, session revocation, token versioning, and per-session validation invalidate credentials. Bearer credentials remain available for explicit API clients.

## SEC-4 — Mutable/unverified supply chain

**Status: Resolved**

Docker bases and infrastructure images use digests, GitHub Actions use commit SHAs, SSH relies on provisioned known-host keys, and downloaded installation archives require SHA-256 verification. Production defaults no longer silently consume mutable `main` artifacts.

## SEC-5 — OAuth without PKCE

**Status: Resolved**

Google and GitHub authorization-code flows derive and verify RFC 7636 S256 PKCE values in addition to authenticated, expiring state.

## SEC-6 — Missing browser headers

**Status: Resolved**

Nginx and generated Caddy configuration emit CSP, frame denial, nosniff, referrer, and permissions policies. Headers were confirmed against the live frontend.

## SEC-7 — IP-only authentication throttling

**Status: Resolved**

Public auth uses both IP and normalized-account identifiers. Account identifiers are SHA-256 hashed before limiter storage, request bodies are preserved, and Redis provides replica-consistent buckets with bounded failure time.

## SEC-8 — CI/CD trust hardening

**Status: Resolved**

Workflows use least-privilege permissions and SHA-pinned actions. CI includes tests, race detection, vet, reachable-vulnerability scanning, dependency audit, generated-contract checks, browser E2E/accessibility, fuzz smoke, coverage artifacts, and a restore drill. CODEOWNERS and Dependabot are present.

## SEC-9 — Frontend on federation network

**Status: Resolved**

The frontend is not attached to federation-only networks. Container permissions are also hardened.

## SEC-10 — Private-key encryption rotation

**Status: Resolved**

Actor private keys use versioned authenticated encryption, the active key plus previous decryption keys are configurable, and rotation/decryption behavior is tested. Configuration parity covers API, worker, local Compose, and blue/green Compose.

# 3. Industry standards and patterns

## STD-1 — Unlimited/unobservable DB pool

**Status: Resolved**

Maximum open/idle connections and lifetime/idle time are bounded and configurable. `database/sql` pool statistics are exported. The live API reported a 25-connection maximum.

## STD-2 — Error classification by message text

**Status: Resolved**

Core domains use stable error categories with `errors.Is/As`. A centralized serializer emits a consistent machine-readable envelope and request correlation without disclosing internal errors.

## STD-3 — Mixed liveness/readiness

**Status: Resolved**

`/health` is dependency-free. `/ready` uses bounded checks against the shared PostgreSQL and Redis clients and validates the required schema.

## STD-4 — Incomplete pagination

**Status: Resolved**

List responses preserve array compatibility while exposing `Link` and `X-Pagination-*` metadata through CORS. Visible pagination exists for projects, invitations, users, audit events, activity, federation inbox/follows, local tickets, remote tickets, members, invites, comments, delivery administration, and remote actors. Member reference data drains all server pages rather than truncating assignee choices.

## STD-5 — No optimistic concurrency

**Status: Resolved**

Projects and tickets carry versions, mutation APIs require `If-Match`, stale mutations fail with precondition responses, and the UI sends current versions. Archive/restore uses the same concurrency model.

## STD-6 — Accessibility not gated

**Status: Resolved**

The board supports keyboard movement and explicit equivalent controls. Dialogs manage focus and Escape/restore behavior. Vitest includes axe checks; Playwright covers browser journeys and accessibility; localization guards prevent silent hardcoded regressions.

## STD-7 — Weak frontend container hardening

**Status: Resolved**

The frontend runs unprivileged with a read-only root filesystem, only required tmpfs mounts, all capabilities dropped, and `no-new-privileges`.

## STD-8 — No dependency/security maintenance

**Status: Resolved**

Automated vulnerability and dependency gates, pinned toolchains, Dependabot, race tests, and browser/build checks are part of CI.

## STD-9 — Configuration drift

**Status: Resolved to a practical enforceable contract**

A typed configuration inventory and parity tests ensure every runtime environment field is represented in examples and all Compose roles. Blue/green duplication still exists by design, but drift is now test-detectable.

## STD-10 — API ergonomics

**Status: Partially resolved — low/medium residual risk**

Errors, request IDs, pagination metadata, optimistic concurrency, generated frontend schemas, and typed contract calls are standardized. Residual improvements are a discoverable hosted OpenAPI documentation route and comprehensive live response-schema validation for every endpoint.

# 4. Tests and coverage

## TEST-1 — Source-text-only frontend tests

**Status: Resolved**

The frontend now has rendered React behavior tests, axe accessibility checks, localization source analysis, and real Playwright journeys against the API/PostgreSQL stack. Source guards remain only for deliberate structural invariants.

## TEST-2 — Frontend absent from CI

**Status: Resolved**

CI installs from the frozen lockfile, verifies generated API types, runs tests, lint, build, dependency audit, and browser E2E/accessibility.

## TEST-3 — Weak core data-path coverage

**Status: Partially resolved — medium residual test risk**

The original unit-only percentages understated repository confidence because real PostgreSQL integration tests were excluded. Merged coverage is now 56.3%, with core packages between 46.7% and 52.4%, and regression floors enforce those results.

Residual: unit-only coverage remains low in project, comment, label, account, activity-history, attachment, and portability repositories. More focused transaction/authorization tests would improve diagnosis speed and edge-case confidence even though the integration suite exercises the real schema.

## TEST-4 — Environment-sensitive architecture test

**Status: Resolved**

Generated and temporary coverage trees are excluded and the scanner has regression tests.

## TEST-5 — No race, fuzz, load, or resilience tests

**Status: Partially resolved — medium residual operational risk**

Linux CI runs `-race`. Four untrusted-input fuzz targets run in CI. Concurrent realtime fanout, Redis outage bounds, database-readiness failure, delivery retry/recovery, webhook lease recovery, restore behavior, and federation failure states have deterministic tests.

Residual: there is no long-duration k6/Vegeta-style load profile, soak test, multi-replica chaos environment, or measured capacity target for SSE, inbox traffic, and database saturation.

## TEST-6 — No coverage policy/trend

**Status: Resolved**

CI publishes unit and integration coverage profiles, enforces an aggregate floor, merges execution evidence, and enforces risk-based floors for project, ticket, comment, and notification packages.

## TEST-7 — Partial/self-referential OpenAPI checks

**Status: Partially resolved — low/medium residual contract risk**

Runtime routes are compared with OpenAPI, generated TypeScript freshness is enforced, and important DTOs are validated. Full request/response schema validation is still not automatic for every handler and status code.

## TEST-8 — Deployment safety untested

**Status: Resolved**

CI validates scripts and Compose, starts disposable infrastructure, creates a verified backup, mutates data, runs the guarded restore, and checks the restored value. The local isolated drill additionally confirmed database, attachment, migration, and readiness recovery.

# 5. Expected project features

## FEAT-1 — Account recovery and verified identity

**Status: Implemented**

Forgot/reset password, local email verification/resend, enumeration-resistant responses, session inventory/revocation, MFA and recovery codes, security-event history, email outbox delivery, and offline owner recovery are present. Privileged accounts are restricted until MFA enrollment.

## FEAT-2 — Labels/tags

**Status: Implemented**

Project-scoped label CRUD, ticket assignment, validation, indexes, UI management, and board/detail display are complete.

## FEAT-3 — Search and usable pagination

**Status: Implemented**

Tickets use indexed PostgreSQL full-text search. Operational list screens expose real pagination and boundary metadata.

## FEAT-4 — Due dates/scheduling baseline

**Status: Implemented**

Tickets support due dates, clearing/updating them, indexes, forms, board display, and due/overdue notification dispatch. Sprints and milestones remain optional scope enhancements, not baseline blockers.

## FEAT-5 — Activity history

**Status: Implemented**

Project/ticket lifecycle and changes produce user-visible, paginated activity records with actor context.

## FEAT-6 — Attachments

**Status: Implemented**

Ticket attachments have bounded upload, filename/content validation, hashes, authorization, local object storage, orphan cleanup, optional ClamAV scanning, download controls, UI, and integration tests.

## FEAT-7 — Notifications

**Status: Implemented baseline**

Assignment, due-soon, overdue, mentions/comments, invitations, federation failure, and security events are supported, with per-type in-app/email preferences and durable email dispatch. Additional product-specific events can be added without changing the architecture.

## FEAT-8 — Archive/restore

**Status: Implemented**

Projects and tickets support versioned archive/restore workflows and dedicated archived views. Hard deletion remains an explicit privileged destructive action.

## FEAT-9 — Owner onboarding/recovery

**Status: Implemented**

CLI creation remains the secure bootstrap boundary. The frontend guides privileged enrollment, the installer/runbook documents first login, the last owner cannot be demoted, and offline owner recovery revokes sessions and can reset MFA.

## FEAT-10 — Backup lifecycle

**Status: Implemented**

Backups include PostgreSQL and attachments, checksums, retention, optional off-host copy, optional age encryption, mandatory pre-restore safety backup, guarded restore confirmation, migration replay, and automated restore verification.

## FEAT-11 — Import/export and portability

**Status: Implemented**

Versioned project and user export, project import, and ticket import are exposed through API and UI.

## FEAT-12 — API credentials and outbound webhooks

**Status: Implemented**

Users can create/revoke expiring scoped API tokens. Projects can manage signed outbound webhooks with event selection, encrypted secrets, durable deliveries, retries, stale-lease recovery, and delivery inspection.

## FEAT-13 — Localization

**Status: Implemented and gated**

English/Ukrainian coverage includes account, administration, federation, project, ticket, delivery, and integration surfaces. Dates/numbers use locale-aware helpers, catalogs have parity tests, and an AST source guard rejects untranslated user-facing strings.

## FEAT-14 — Accessibility

**Status: Implemented baseline and gated**

Keyboard board interaction, explicit movement controls, focus-managed dialogs, accessible labels, route loading states, axe checks, and real-browser accessibility coverage are present. Manual screen-reader testing remains a release practice rather than something source code can prove.

# Current remaining work

No original critical/high release blocker remains. Recommended follow-up, in order:

1. Split the five remaining 1,000+ line handwritten Go files by cohesive responsibility.
2. Add automatic live request/response schema validation or generate the Go server boundary from OpenAPI.
3. Increase focused repository/transaction tests beyond the current merged floors.
4. Establish capacity targets and long-duration load/soak/chaos tests for SSE, federation inboxes, Redis loss, and PostgreSQL saturation.
5. Publish a discoverable versioned API documentation route.
6. Before a public launch, run an external penetration test, real federation interoperability campaign, and operator exercise with production SMTP, ClamAV, TLS, monitoring alerts, and encrypted off-host backup credentials.

## Final conclusion

The remediation changed the project from an ambitious but uneven diploma-scale system into a broad, security-conscious full-stack platform with credible operational safeguards. Its strongest areas are now federation security, identity/session controls, baseline PM completeness, typed API contracts, deployment recovery, and live observability.

The remaining weaknesses are no longer missing fundamentals. They are engineering-depth issues: several large modules, incomplete automatic schema enforcement, moderate rather than deep repository coverage, and the absence of sustained capacity/chaos evidence. Those should be tracked as the next hardening cycle, not represented as already solved.
