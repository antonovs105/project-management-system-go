# Progo codebase critical review

**Audit date:** 2026-07-12  
**Repository commit:** `695e5ebd1ea8b620b21b4620ed2b281fc60174d5` (`fix: preserve deploy state on public check failure`)  
**Scope:** backend, frontend, migrations, runtime configuration, Docker/Compose, deployment scripts, CI/CD, tests, and expected project-management capabilities.

## Remediation addendum — 2026-07-12

The findings below describe commit `695e5eb...` before remediation. The working tree now contains a stabilization release that resolves the deploy-blocking findings and a selected baseline product slice.

### Resolved

| Area | Resolution | Verification |
|---|---|---|
| Go security baseline | Go and Docker are aligned on 1.26.5; CI runs pinned `govulncheck`. | Final scan: 0 reachable vulnerabilities. |
| Frontend advisories | Direct dependencies and transitive overrides were upgraded; pnpm 11.1.2 is pinned. | Full and production audits: 0 known vulnerabilities. |
| Browser sessions | Removed JWT persistence/decoding from `localStorage`; browser auth uses a 12-hour `HttpOnly`, `Secure`, `SameSite=Strict` cookie with logout/token-version revocation. Bearer auth remains for non-browser clients. | Middleware, handler lifecycle, frontend behavior, and live 401 checks pass. |
| CSRF/session boundary | Credentialed CORS plus trusted-origin enforcement protects unsafe cookie-authenticated requests. | Origin middleware tests pass. |
| OAuth | Google and GitHub authorization-code flows use derived RFC 7636 S256 PKCE verifiers. | OAuth URL/service tests pass. |
| Browser headers | CSP, frame denial, MIME sniffing prevention, referrer and permissions policies are emitted by nginx and generated Caddy config. | Live frontend response contains CSP. |
| Supply chain | Docker bases and Compose infrastructure images are digest-pinned; GitHub Actions are SHA-pinned; SSH uses a pre-provisioned host key; installer archives require SHA-256 verification. | Docker builds, shell syntax, and all Compose configs pass. |
| Container isolation | Frontend runs as nginx without capabilities, read-only, with owned tmpfs paths; frontend was removed from federation networks. API and worker share one local image. | Complete live stack is healthy under Docker Desktop. |
| CI maintenance | Added frontend test/type/lint/build/audit job, Linux race tests, a 35% backend coverage floor, Dependabot, least-privilege permissions, and CODEOWNERS. | Equivalent local gates pass except Windows race instrumentation (no host C compiler); CI provides Linux execution. |
| Database reliability | Pool max-open/max-idle/lifetime/idle-time are configurable and bounded; `database/sql` stats are exported to Prometheus. | Live metrics report `go_sql_max_open_connections{db_name="primary"} 25`. |
| Health semantics | `/health` is dependency-free liveness; `/ready` checks PostgreSQL schema and the shared Redis client. | Live responses: both 200, readiness checks all `ok`. |
| Error classification | Removed production HTTP status classification based on `strings.Contains(err.Error())`; core domains use stable typed error categories. | Repository scan is clean and relevant package tests pass. |
| Authentication throttling | Public auth has independent IP and normalized-account throttles; account identifiers are SHA-256 hashed. | Identifier normalization/body-preservation test passes. |
| Frontend tests | Added Vitest/jsdom/Testing Library behavior tests alongside the existing contract checks. | 2/2 behavior and 8/8 contract tests pass. |
| Accessibility/performance | Board drag-and-drop supports keyboard sensors; routes are lazy-loaded into separate production chunks. | Typecheck/lint/build pass; chunked build output confirmed. |
| Ticket search/pagination | Added generated PostgreSQL `tsvector`, GIN index, bounded `q` API filter, 50-item server pages, and usable previous/next UI. | Migration 30 and focused service tests pass. |
| Due dates | Added indexed due-date storage, API create/update/clear semantics, forms, and board display. | Migration 31 and date parser tests pass. |
| Labels | Added project-scoped labels, permissioned CRUD, ticket assignment, settings UI, forms, and board badges. | Migration 32, label tests, full Go/frontend builds, and integration tests pass. |

### Still open (not safe to pretend resolved)

- Password recovery, local email verification, privileged-account MFA, session inventory, and durable authentication-event alerting require an email/MFA/product design and remain release blockers for an internet-facing public service.
- Activity history, attachments/object storage, richer notification types/preferences/email delivery, and project archive/restore remain product-completeness work.
- Optimistic concurrency, generated API clients, broad decomposition of oversized files, local/remote workspace consolidation, and realtime-hub consolidation remain maintainability work.
- End-to-end browser tests, ephemeral install/upgrade/rollback/restore tests, backup retention/off-host encryption, and restore drills remain operational confidence gaps.
- Actor private-key rotation and a uniform structured error response envelope remain security/API follow-ups.
- Coverage is now enforced at 35%, but final measured backend statement coverage is only 39.0%; core project/ticket/comment/notification packages still need substantially deeper tests.

### Review correction

The original OE-8 statement said the final schema retained dormant `labels` tables. That was incorrect: migration 5 dropped the legacy bigint tables. The integration safety test creates an unrelated `public.labels` table only to prove migrations do not destroy external tables. The implemented label feature therefore uses a new UUID-based schema in migration 32.

## Executive verdict

Progo is a substantial working full-stack system, not a CRUD prototype. It has a Go/Echo API, React client, PostgreSQL persistence, Redis/Asynq jobs, ActivityPub federation, HTTP Message Signatures, a maintenance CLI, metrics, migrations, and blue-green deployment. Several security controls are unusually good for a diploma-scale application: outbound federation SSRF protection, signature and digest verification, production configuration guards, role/permission checks, token invalidation by version, request limits, and meaningful database integration tests.

The current checkout is nevertheless **not production-ready**. The primary blockers are:

1. The declared Go 1.25.0 toolchain is far behind its security patch level. `govulncheck` found **25 reachable vulnerabilities** when the code was analyzed with Go 1.25.0. The current fixed release in that line is Go 1.25.12.
2. The locked frontend production graph has **43 audit advisories: 22 high, 20 moderate, and 1 low**. Directly locked affected packages include Axios 1.13.2 and React Router 7.11.0.
3. A 72-hour bearer JWT is persisted in browser `localStorage`. One successful XSS or compromised same-origin script can steal a reusable session.
4. Frontend quality gates are effectively absent from CI. The eight frontend tests only search source text; they do not render the application or test user behavior.
5. Backend statement coverage is only **38.5% overall**, with the core `project`, `ticket`, `comment`, and `notification` packages at 13.5%, 22.4%, 14.6%, and 15.1% respectively.
6. The product prioritizes a very complex federation/deployment layer while baseline project-management and account-lifecycle features remain absent: labels, due dates, search, usable pagination, activity history, attachments, password recovery, local email verification, and comprehensive notifications.

### Overall assessment

| Area | Rating | Summary |
|---|---:|---|
| Functional breadth | 7/10 | Strong project, ticket, role, federation, GitHub, notification, and admin surface. |
| Backend security design | 7/10 | Good authorization and federation defenses, undermined by patch and session-management gaps. |
| Frontend security | 3/10 | Vulnerable lockfile, persistent bearer token, no CSP, and no behavioral security tests. |
| Maintainability | 4/10 | Clear vertical slices, but several 1,000-2,100-line files and substantial duplication. |
| Test confidence | 4/10 | Many backend tests and good DB integration scenarios, but low measured coverage and almost no real frontend testing. |
| Production operations | 5/10 | Good health/metrics/deploy foundations, but mutable artifacts, unbounded DB pool, weak supply-chain verification, and no restore drill. |
| Product completeness | 5/10 | Advanced differentiating features exist before several baseline PM and identity features. |

## Scope and method

The review inspected 80 production Go files (26,569 lines), 50 Go test files (15,555 lines), 42 frontend TypeScript/TSX files (9,030 lines), 58 SQL migration files, Dockerfiles, Compose definitions, Caddy generation, installer/deploy scripts, OpenAPI, and both GitHub Actions workflows.

Validation performed:

- `go test -count=1 -covermode=atomic -coverprofile=... ./...` — passed with 38.5% total statement coverage when the compiler temp directory was outside the backend tree.
- `go vet ./...` — passed.
- `node --test tests/*.test.mjs` — 8/8 passed.
- `tsc -b --pretty false` — passed.
- `eslint .` — passed.
- `vite build` — passed; main bundle was 607.49 kB minified / 176.91 kB gzip, plus a 194.19 kB graph chunk.
- `govulncheck` with local Go 1.26.3 — 3 reachable standard-library advisories, fixed in Go 1.26.4/1.26.5.
- `govulncheck` with the repository-declared Go 1.25.0 — 25 reachable standard-library vulnerabilities; the newest observed fix requirement is Go 1.25.12.
- `pnpm audit --prod --json` — 43 advisories in the locked production dependency graph.

Limitations:

- Database integration tests could not be rerun locally because the Docker daemon was not running. CI is configured to run them against PostgreSQL.
- The race detector could not run in this Windows environment because CGO requires a C compiler that is not installed. CI also does not currently run `-race`.
- No live production instance, real secrets, external federation peer, browser accessibility tooling, penetration test, or restore exercise was available.
- Dependency audit results are inventory findings. Several React Router advisories concern SSR/RSC paths that this client-only Vite application does not use, and several Axios advisories concern its Node adapter. They still show that the lockfile is not being patched or gated.

## What is already done well

The following should be preserved during refactoring:

- `backend/internal/activitypub/netguard/netguard.go` validates schemes, redirects, DNS results, and private/reserved addresses, including validation at dial time to reduce DNS-rebinding risk.
- `backend/internal/activitypub/httpsig/service.go` verifies signed method/authority/path/date, body digest, signature age, actor identity, and refreshed keys.
- Inbox requests are limited to 1 MiB and globally bounded by the API body limit.
- ActivityPub writes are transactionally deduplicated by activity AP ID and inbox target.
- JWT parsing rejects algorithms other than HS256, and `token_version` allows password changes to invalidate old tokens.
- Production startup rejects weak/missing secrets, HTTP public URLs, localhost identities, wildcard CORS, insecure federation HTTP, and private-network federation.
- Project authorization is permission-based rather than relying on frontend visibility.
- PostgreSQL constraints cover many role/status/value invariants and relationship uniqueness rules.
- The backend image runs as a non-root user; backend services use read-only filesystems and `no-new-privileges` in production Compose.
- API/worker roles, Prometheus metrics, readiness checks, delivery retries, dead deliveries, and database backups are present.
- The integration suite creates isolated schemas and applies the actual migrations, which is much more valuable than repository mocks alone.

# 1. Overengineering review

## OE-1 — Product-priority inversion: federation before baseline PM features

**Severity:** High maintainability/product risk

ActivityPub is not inherently overengineering if federation is the product's defining requirement. The problem is proportion: a large fraction of backend code, migrations, admin UI, delivery recovery, remote caching, HTTP signatures, domain moderation, and deployment work supports federation while basic PM workflows are incomplete.

Evidence:

- Federation includes `activitypub/c2s`, delivery queues, moderation, remote actors, remote inboxes, remote projects, HTTP signatures, WebFinger, domain blocks, recovery loops, and separate local/remote workspaces.
- Migrations already retain `labels` and `ticket_labels`, but there is no label service, repository, handler, API client, or UI.
- There are no ticket due dates, activity history, attachments, ticket search, or practical pagination controls.
- Local account recovery and email verification do not exist.

Impact: engineering cost is concentrated in the hardest distributed-system edge cases while mainstream users still cannot perform routine PM and account-recovery tasks. Every future feature must also answer local/remote federation semantics, raising its cost.

Recommendation: explicitly choose one of two product strategies:

1. **Federation-first:** document federation as the non-negotiable differentiator and freeze protocol expansion until baseline PM/account capabilities are complete.
2. **Project-management-first:** place federation behind a feature flag/module boundary and ship a smaller local product first.

## OE-2 — Oversized composition, repository, service, and UI files

**Severity:** High maintainability risk

Largest production hotspots:

| File | Lines | Concern |
|---|---:|---|
| `backend/internal/project/repository.go` | 2,105 | Membership, roles, invites, project CRUD, ActivityPub documents, delivery side effects, and helper SQL in one repository. |
| `frontend/src/features/projects/ProjectSettingsPanel.tsx` | 1,312 | Overview metrics, settings, roles, members, invitations, GitHub integration, and multiple forms in one component. |
| `backend/internal/activitypub/remoteinbox/repository.go` | 1,274 | Persists and applies many unrelated ActivityPub activity types. |
| `backend/internal/activitypub/federation/service.go` | 1,123 | Discovery, follows, remote projects/invites/tickets, parsing, and remote writes. |
| `backend/cmd/api/main.go` | 1,010 | Config validation, dependency composition, workers, route setup, metrics, health checks, logging, and rate limiting. |
| `frontend/src/lib/api.ts` | 755 | Every DTO, filter, normalization helper, URL helper, and API call. |

Impact: large change surfaces, merge conflicts, difficult focused testing, and high cognitive load. Package-level coverage hides which responsibilities are untested.

Recommendation:

- Split project persistence into `project`, `membership`, `role`, and `invite` repositories or cohesive files behind one package interface.
- Dispatch inbound ActivityPub types to per-activity applicators.
- Move API composition to an `internal/app` package and keep `cmd/api/main.go` as a small entry point.
- Split `ProjectSettingsPanel` by tab/feature and lazy-load secondary panels.
- Split the API client by domain while sharing one configured transport.

## OE-3 — Duplicated local and remote project workflows

**Severity:** Medium

`ProjectWorkspace.tsx` (618 lines) and `RemoteProjectWorkspace.tsx` (583 lines) both define summary widgets, ticket statistics, optimistic ticket movement, ticket create/edit/delete flows, and board rendering. The APIs differ, but most presentation and client-state logic is duplicated.

Impact: fixes for validation, accessibility, optimistic rollback, and UI behavior must be implemented twice and can drift.

Recommendation: create shared `ProjectSummary`, `TicketWorkspace`, editor, and mutation-adapter abstractions. Keep federation-specific capabilities in a small adapter rather than a separate page implementation.

## OE-4 — Duplicated realtime hubs

**Severity:** Low/Medium

`backend/internal/ticket/events.go` and `backend/internal/notification/events.go` implement nearly the same local subscriber map, Redis subscription/reconnect loop, duplicate-suppression map, close behavior, and publish timeout.

Recommendation: use a small generic internal pub/sub helper parameterized by channel name, key extraction, and payload type. Do not create a full event-bus framework; one tested helper is enough.

## OE-5 — Quality gates that enforce ceremony rather than risk

**Severity:** Medium

`backend/internal/commentguard/comment_guard_test.go` requires GoDoc-style comments on every top-level declaration, including unexported helpers. Go's useful convention is documentation for exported API; enforcing comments on private helpers produces repetitive comments such as “currentUserID returns...” that restate code.

`backend/internal/architecture/architecture_test.go` recursively scans every `.go` file under the module instead of analyzing package source through `go/packages`. It skipped `.gocache` but not `.gotmp`. During coverage, it parsed generated `*.cover.go` files and falsely reported architecture violations. Moving `GOTMPDIR` outside the module made the same code pass.

Recommendation:

- Restrict documentation enforcement to exported declarations and package comments, or use standard linters.
- Replace filesystem-wide AST walks with `go/packages`, or at minimum exclude every generated/cache/temp directory and test the scanner against them.
- Spend CI time on vulnerability, race, integration, accessibility, and behavior checks instead.

## OE-6 — Manual contract duplication

**Severity:** Medium

The API contract is represented independently in:

- Echo route registration,
- a 3,606-line handwritten OpenAPI file,
- a manually enumerated route list in `openapi_contract_test.go`,
- handwritten TypeScript models in `types.ts`, and
- handwritten client methods in `api.ts`.

The tests verify selected routes and selected response schemas, but they do not automatically prove that every runtime route, request, response, and frontend type matches OpenAPI.

Recommendation: make OpenAPI the source of truth and generate the TypeScript client/types, or generate OpenAPI from typed Go route definitions. Add a runtime-route-versus-contract comparison and validate the document with a standards-compliant OpenAPI validator.

## OE-7 — Blue-green deployment complexity without matching verification

**Severity:** Medium

Blue-green deployment is justified if near-zero downtime on one VM is a requirement. Otherwise it doubles API, worker, frontend, port, health-check, and configuration definitions. `deploy/docker-compose.bluegreen.yml` duplicates the full backend environment four times.

The scripts perform useful local readiness checks and a backup, but there are no automated deploy tests, rollback tests, restore tests, or retention rules. After Caddy switches, a public readiness failure exits without automatically restoring the previous Caddy configuration.

Recommendation: either document zero-downtime as a requirement and test failure/rollback paths, or simplify to one Compose stack with a short controlled restart.

## OE-8 — Dormant schema

**Severity:** Low

`labels` and `ticket_labels` exist from the early schema, and an integration safety test explicitly preserves `public.labels`, but the application has no label feature. Dormant tables create ambiguity about supported behavior and migration ownership.

Recommendation: implement labels in the next baseline feature slice or remove the unused tables in a deliberate migration.

## OE-9 — Most frontend routes are eagerly bundled

**Severity:** Medium performance/maintainability risk

`frontend/src/App.tsx` statically imports every route page. Only the graph implementation is lazy-loaded inside `ProjectWorkspace`. The resulting production build places 607.49 kB minified / 176.91 kB gzip in the main JavaScript chunk before the graph chunk.

Impact: login and simple list pages pay the parsing/execution cost of admin, federation, project settings, GitHub, and remote-project code they may never use.

Recommendation: lazy-load route-level pages and large secondary panels, then enforce a bundle budget in CI. Measure actual browser performance rather than splitting every small component mechanically.

## OE-10 — Backend Docker context does not exclude the repository-standard cache

**Severity:** Medium build risk

The repository convention and validation commands use `backend/.cache`, but `backend/.dockerignore` excludes `.gocache` and not `.cache`. A normal local Docker build can therefore send a very large module/build cache into the build context before `COPY . .`; during this audit that ignored cache exceeded 1 GiB.

Impact: slow builds, unnecessary disk/network use, cache invalidation, and accidental inclusion of local tooling artifacts in builder layers.

Recommendation: exclude `.cache`, `.gotmp`, coverage/temp outputs, and other generated directories; keep the Docker context allowlist-oriented where practical.

# 2. Vulnerabilities and security issues

## SEC-1 — Declared Go 1.25.0 is an unpatched security baseline

**Severity:** Critical release blocker

Evidence:

- `backend/go.mod` declares `go 1.25.0`.
- CI uses `actions/setup-go` with `go-version-file: backend/go.mod`, so builds can resolve the declared version rather than a currently patched toolchain.
- An official `govulncheck` run under Go 1.25.0 found **25 reachable standard-library vulnerabilities**.
- Findings included network/TLS, X.509, URL parsing, cookie parsing, PEM/ASN.1 parsing, email parsing, and HTTP/2 paths actually reached from this code.
- The newest observed fix requirement is Go **1.25.12**. The current 1.26 line requires **1.26.5** for the latest TLS fix.

Impact: production behavior can include known denial-of-service, parsing, panic, and TLS weaknesses even if application code is correct.

Fix now:

- Pin CI to `go1.26.5` (preferred) or at least `go1.25.12`.
- Pin the Docker builder to the same patch and immutable image digest.
- Add `govulncheck ./...` to CI and fail on reachable vulnerabilities.
- Define an automated patch-update policy.

Official references: [Go release history](https://go.dev/doc/devel/release), [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856).

## SEC-2 — Locked frontend production dependencies contain known advisories

**Severity:** Critical release blocker

`pnpm audit --prod --json` reported:

- **22 high**
- **20 moderate**
- **1 low**
- **43 total** across 219 production/optional dependency entries

Important locked packages and minimum observed fixes:

| Package/path | Locked | Minimum fix indicated by audit | Notes |
|---|---:|---:|---|
| `axios` | 1.13.2 | 1.16.0 | Multiple high/moderate prototype-pollution, request manipulation, credential/proxy leakage, and DoS advisories. Some are Node-adapter-only; others affect shared/browser paths. |
| `react-router` via `react-router-dom` | 7.11.0 | 7.15.0 | Several SSR/RSC issues are not reachable in this SPA, but client redirect/XSS advisories and stale patch hygiene still apply. |
| `preact` via graph tooltip | 10.28.1 | 10.28.2 | JSON VNode injection advisory. Graph labels are user-controlled strings, so reachability should be explicitly tested after upgrading. |
| `lodash-es` via force graph | 4.17.22 | 4.17.24 | Code-injection/prototype-pollution advisories; exact vulnerable helpers may not be called by this app. |
| `follow-redirects` | 1.15.11 | 1.15.12 | Authentication-header leakage in affected redirect scenarios; primarily Axios's Node path. |
| `form-data` | 4.0.5 | 4.0.6 | CRLF injection in multipart names/filenames; Node path. |
| `postcss` | 8.5.6 | 8.5.10 | Build-time CSS stringification issue. |
| `picomatch` | 2.3.1 / 4.0.3 | 2.3.2 / 4.0.4 | Build-time glob method-injection/ReDoS advisories. |

Impact: the lockfile makes builds repeatable but repeats known vulnerable versions. The absence of audit/Dependabot gates allowed advisories to accumulate.

Fix now:

- Upgrade direct dependencies and regenerate `pnpm-lock.yaml`.
- Use `pnpm.overrides` for vulnerable transitives when upstream packages have not updated.
- Rerun typecheck, lint, behavioral tests, production build, and audit.
- Add Dependabot/Renovate and a production-dependency audit job.

References: [Axios GHSA-hfxv-24rg-xrqf](https://github.com/advisories/GHSA-hfxv-24rg-xrqf), [React Router GHSA-8x6r-g9mw-2r78](https://github.com/advisories/GHSA-8x6r-g9mw-2r78), [Preact GHSA-36hm-qxxp-pg3m](https://github.com/advisories/GHSA-36hm-qxxp-pg3m).

## SEC-3 — Long-lived bearer JWT is persisted in `localStorage`

**Severity:** High

Evidence:

- `frontend/src/store/auth.ts` persists `token`, `user`, and authentication state through Zustand persistence under `pms.session`.
- `backend/internal/user/service.go:638-645` issues a bearer JWT valid for 72 hours.
- Logout only clears browser state; there is no server-side logout/session revocation endpoint.
- No CSP is emitted by Caddy or frontend nginx.

Impact: any XSS, compromised dependency, malicious browser extension with origin access, or same-origin application compromise can read and exfiltrate a token usable for up to 72 hours. The vulnerable frontend dependency inventory increases the importance of this design weakness.

OWASP explicitly recommends not storing session identifiers/JWTs in web storage because JavaScript can always read them. Prefer an `HttpOnly; Secure; SameSite` cookie or a BFF pattern: [OWASP HTML5 Security](https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html), [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).

Recommendation:

- Use short-lived access sessions and rotating, hashed server-side refresh/session records.
- Deliver the session in `HttpOnly; Secure; SameSite=Lax/Strict` cookies.
- Add CSRF protection for state-changing cookie-authenticated requests.
- Add logout-current-session, logout-all-sessions, device/session listing, and revocation.
- Add `iss`, `aud`, `iat`, and `jti` validation and explicit parser options.

## SEC-4 — Mutable and unverified install/deploy supply chain

**Severity:** High

Evidence:

- README recommends `curl .../deploy/install.sh | sh` from the mutable `main` branch.
- `deploy/install.sh` defaults `PROGO_REF=main`, downloads a GitHub tarball, extracts it, and executes its scripts without a checksum or signature.
- The installer defaults `IMAGE_TAG=main`, also mutable.
- Docker base images use mutable tags rather than immutable digests.
- The frontend builder runs unpinned `npm install -g pnpm`.

Impact: a compromised repository/tag, registry account, DNS/TLS trust path, package publication, or unexpected tag update changes production code without an auditable release decision.

Recommendation:

- Install only versioned releases or immutable commit SHAs.
- Publish and verify SHA-256 checksums/signatures for deployment assets.
- Deploy images by digest and record the digest in deployment state.
- Pin pnpm via Corepack/packageManager and pin base images by digest, with automated digest-update PRs.
- Avoid piping an unverified network response directly to a shell.

Docker documents that tags are mutable and digests are required for fully reproducible integrity: [Docker build best practices](https://docs.docker.com/build/building/best-practices/).

## SEC-5 — OAuth authorization-code flows omit PKCE

**Severity:** Medium

The OAuth flow correctly uses a signed, expiring state value, an HttpOnly browser-binding cookie, fixed provider endpoints, and a one-time frontend exchange code. However, Google and GitHub authorization requests do not send a PKCE challenge, and token exchange does not send a verifier.

RFC 9700 recommends PKCE even for confidential web clients because it protects against authorization-code misuse and injection: [OAuth 2.0 Security Best Current Practice](https://datatracker.ietf.org/doc/rfc9700/).

Recommendation: add transaction-bound S256 PKCE for both providers. Store the verifier server-side or in a protected browser-bound transaction cookie and enforce one-time consumption.

## SEC-6 — Missing browser security headers

**Severity:** Medium

Neither `frontend/nginx.conf` nor generated Caddy configuration sets a Content Security Policy, `frame-ancestors`/X-Frame-Options, `X-Content-Type-Options`, `Referrer-Policy`, or HSTS.

Impact: weaker defense in depth against XSS, clickjacking, MIME confusion, referrer leakage, and HTTPS downgrade on first contact.

Recommendation: set and test at least:

- `Content-Security-Policy` tailored to Vite assets, API/SSE connections, and required images;
- `frame-ancestors 'none'` (and optionally `X-Frame-Options: DENY`);
- `X-Content-Type-Options: nosniff`;
- `Referrer-Policy: strict-origin-when-cross-origin`;
- HSTS after confirming permanent HTTPS/domain readiness;
- a restrictive `Permissions-Policy`.

Reference: [OWASP HTTP Security Response Headers](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Headers_Cheat_Sheet.html).

## SEC-7 — Authentication throttling is IP-only

**Severity:** Medium

Public auth endpoints share a per-IP token-bucket limiter. This is useful, Redis-backed in production, and proxy-aware. It does not track failed attempts by account/email, however, so distributed credential stuffing can rotate source IPs.

Recommendation: add account-aware progressive delays/lockout with careful anti-DoS limits, authentication event logging/alerts, and optional MFA. OWASP recommends associating failed-login counters with the account rather than only source IP: [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html).

## SEC-8 — CI/CD trust hardening is incomplete

**Severity:** Medium

Evidence:

- GitHub Actions use mutable major tags such as `actions/checkout@v4`, `actions/setup-go@v5`, and Docker actions by major version.
- Deployment populates `known_hosts` at run time with `ssh-keyscan`; this is trust on first use and does not authenticate the expected host key.
- There is no CODEOWNERS protection shown for workflow/deploy files, no artifact attestation/SBOM, and no vulnerability scan before pushing/deploying images.

Recommendation:

- Pin every third-party action to a reviewed full commit SHA.
- Store the expected SSH host key/fingerprint as protected configuration and compare it.
- Add least-privilege job-level permissions, workflow CODEOWNERS, image SBOM/provenance, and container scanning.

GitHub states that a full-length commit SHA is the only immutable way to reference an action: [GitHub Actions secure use](https://docs.github.com/en/actions/reference/security/secure-use).

## SEC-9 — Frontend is unnecessarily attached to the federation network

**Severity:** Medium

Both blue and green frontend containers join the external `federation` network. They only serve static assets and health responses; Caddy reaches them through localhost-published ports. Federation connectivity is unnecessary.

Impact: compromise of the nginx/static container gives direct network reachability to federation-connected backends, potentially across multiple local instances.

Recommendation: remove frontend services from the federation network and place them on a narrowly scoped private/default network only if needed.

## SEC-10 — ActivityPub key encryption lacks rotation support

**Severity:** Low/Medium

Private keys are correctly encrypted with AES-GCM using random nonces. Ciphertext has only an `enc:v1:` marker; it has no key identifier or multi-key decrypt support. Replacing `ACTOR_PRIVATE_KEY_ENCRYPTION_KEY` therefore makes existing actor keys unreadable unless every row is re-encrypted atomically.

Recommendation: introduce key IDs, active/legacy decrypt keys, a rotation command, metrics for legacy-key usage, and a tested backup/restore procedure. Use a KMS or secret manager in higher-assurance deployments.

# 3. Deviations from current industry standards and patterns

## STD-1 — Database pool is unlimited and not observable

**Severity:** High reliability risk

`sqlx.Connect` is used without `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`, or `SetConnMaxIdleTime`. Go's default maximum open connection count is unlimited: [database/sql documentation](https://pkg.go.dev/database/sql#DB.SetMaxOpenConns).

Impact: concurrent requests/SSE-related work/background delivery can create enough database connections to exhaust PostgreSQL, the host, or network resources. There are no exported `DB.Stats()` pool metrics.

Recommendation: make pool limits/configuration explicit, size them against PostgreSQL capacity and replica count, configure connection lifetimes, expose pool metrics, and alert on waits/exhaustion.

## STD-2 — Error classification relies on message substrings

**Severity:** Medium

At least 12 handler branches use patterns such as `strings.Contains(err.Error(), "insufficient permissions")`, `"not found"`, and `"already"` to select HTTP status codes.

Impact: rewording or wrapping an error can silently change a 403/404/409 into another response. It also spreads domain-to-HTTP mapping across handlers.

Recommendation: define typed/sentinel domain errors with stable codes, use `errors.Is/As`, and centralize translation to a consistent error envelope such as Problem Details (`application/problem+json`).

## STD-3 — Health and readiness semantics are mixed

**Severity:** Medium

`/health` performs an unbounded `db.Ping()` and returns 500 when PostgreSQL is unavailable. A liveness probe should normally answer whether the process/event loop is alive; dependency failure belongs in readiness. Otherwise an orchestrator can restart healthy application processes during a database outage and amplify the incident.

`/ready` is better: it uses a two-second context and checks schema plus Redis. It creates a new Redis client on every readiness request instead of reusing the existing client.

Recommendation:

- Make `/health` a cheap in-process liveness response.
- Keep dependency checks in `/ready` with explicit bounded timeouts.
- Reuse initialized clients and expose degraded dependency details only as appropriate.

## STD-4 — Pagination contract is incomplete and the frontend effectively disables it

**Severity:** High scalability/product risk

The backend accepts `limit`/`offset`, but most list responses are raw arrays with no `total`, `next`, cursor, or pagination metadata. The frontend nearly always requests offset 0 with hardcoded limits of 100, 200, or 500 and exposes no next/previous/load-more control.

Impact: records past the cap become invisible, while increasing caps shifts the problem into slow responses and large browser state. Offset pagination also becomes inconsistent on frequently changing feeds.

Recommendation: use a standard paginated envelope and cursor pagination for ordered feeds. Add visible pagination/infinite loading and tests covering the boundary past the first page.

## STD-5 — No optimistic concurrency for collaborative edits

**Severity:** Medium

Projects and tickets have timestamps but update APIs do not require an expected version/ETag. Concurrent editors use last-write-wins and can silently overwrite one another; realtime invalidation does not prevent the race.

Recommendation: add a row version or `updated_at` precondition, return ETags, accept `If-Match`, and respond 409/412 on stale updates. The UI should offer refresh/merge behavior.

## STD-6 — Accessibility is not a quality gate

**Severity:** Medium

The Kanban board configures only `PointerSensor`; dnd-kit keyboard sensors and sortable keyboard coordinates are absent. A button has `aria-label="Drag ticket"`, but keyboard users cannot perform the equivalent movement. There are no axe/Playwright/accessibility tests and very few explicit ARIA live/relationship attributes.

Recommendation: implement keyboard drag/reorder or equivalent move controls, focus management, live announcements, and automated axe checks plus manual screen-reader/keyboard verification.

## STD-7 — Frontend container hardening trails backend hardening

**Severity:** Low/Medium

The backend final image uses a non-root user, read-only filesystem, tmpfs, and `no-new-privileges`. The nginx frontend image has no `USER`, is not read-only in Compose, and does not drop capabilities. Database/Redis/migration images are also tag-based and unpinned.

Recommendation: use an unprivileged nginx image/config, read-only root filesystem, writable tmpfs only where needed, `cap_drop: [ALL]`, and immutable digests.

## STD-8 — No automated dependency/security maintenance

**Severity:** High

CI runs backend tests/vet/integration only. It does not run frontend tests/lint/build, `govulncheck`, `pnpm audit`, CodeQL/static analysis, secret scanning, container scanning, SBOM generation, or dependency update automation.

Recommendation: treat these as merge/deploy gates, with documented exceptions for advisories proven unreachable.

## STD-9 — Configuration is powerful but drift-prone

**Severity:** Medium

Configuration exists in YAML, environment variables, `.env`, pmsctl generation/export code, local Compose, instance Compose, and duplicated blue/green service blocks. This is useful for install compatibility but creates many representations of the same fields.

Recommendation: define one typed configuration schema, generate example YAML/env documentation and Compose fragments where possible, and add parity tests proving every production field reaches API and worker roles.

## STD-10 — API ergonomics are inconsistent

**Severity:** Medium

- Errors are mostly `{ "error": "..." }` but mapping and disclosure vary by handler.
- Lists are raw arrays with ad hoc pagination.
- OpenAPI is static and not exposed through a discoverable documentation route.
- Frontend types/client are handwritten rather than contract-generated.
- Some update endpoints return no updated representation while others do.

Recommendation: standardize errors, pagination, idempotency expectations, update responses, and request correlation. Generate client types and publish versioned API documentation.

# 4. Tests and coverage review

## Coverage result

Measured backend statement coverage:

| Package | Coverage |
|---|---:|
| Total | **38.5%** |
| `internal/project` | **13.5%** |
| `internal/comment` | **14.6%** |
| `internal/notification` | **15.1%** |
| `internal/ticket` | **22.4%** |
| `internal/activitypub` | **23.8%** |
| `cmd/api` | **28.9%** |
| `internal/githubintegration` | **28.6%** |
| `internal/activitypub/federation` | **38.6%** |
| `internal/user` | **40.5%** |
| `internal/activitypub/remoteinbox` | **43.7%** |
| `internal/activitypub/httpsig` | 70.7% |
| `internal/config` | 71.4% |
| `internal/middleware` | 78.7% |
| `internal/lexorank` | 84.7% |
| `internal/activitypub/domainblock` | 87.1% |

Coverage is not a quality score by itself, but 13-22% in the central business domains means many failure, authorization, transaction, and data-shape paths are not exercised by the default suite.

## TEST-1 — Frontend tests are source-text assertions, not application tests

**Severity:** Critical test gap

All eight tests in `frontend/tests/frontend-contract.test.mjs` read source files and assert that strings are present or absent. Examples include checking that `api.ts` contains route fragments, a page contains a polling constant, or CSS contains `html.dark`.

These tests can pass when:

- the UI crashes at render time;
- the route is inaccessible;
- a button is not clickable;
- the API payload is wrong;
- authentication persistence is broken;
- mutations fail to invalidate data;
- drag/drop is inaccessible;
- the copied string exists only in dead code or a comment.

Recommendation:

- Add Vitest + React Testing Library for components/hooks.
- Add MSW-backed integration tests for auth, project, ticket, permission, invitation, and error flows.
- Add Playwright end-to-end tests against real API/PostgreSQL for the critical journey.
- Keep a small number of source/contract guards only where they enforce a deliberate invariant.

## TEST-2 — CI never runs frontend quality checks

**Severity:** Critical process gap

`.github/workflows/ci.yml` contains only backend test, vet, and integration steps. A pull request can merge with broken TypeScript, lint failures, failing frontend tests, a failed Vite build, or known production dependency vulnerabilities. The deploy workflow discovers some build failures only after CI has already succeeded.

Recommendation: add a frontend CI job using frozen lockfile install, test, typecheck, lint, build, and audit. Make deploy depend on the complete CI result.

## TEST-3 — Core data paths have low coverage despite many tests

**Severity:** High

There are 371 backend test functions, but most repositories have no focused repository test file. Repository behavior is covered indirectly by a large integration package, while unit tests often use mocks. The huge production repositories account for much of the low coverage.

Recommendation: prioritize transaction/constraint tests for project roles, invites, concurrent ticket movement, GitHub webhook idempotency, notifications, pagination boundaries, and authorization—not superficial line coverage.

## TEST-4 — The architecture test is environment-sensitive

**Severity:** Medium

Coverage initially failed because `.gotmp/*.cover.go` files were scanned as production source. This is a confirmed false positive, not a hypothetical concern.

Recommendation: fix the scanner and add a regression test with generated/cache directories.

## TEST-5 — No race, fuzz, load, or resilience suite

**Severity:** High for a concurrent/federated service

Evidence:

- CI does not use `go test -race`.
- There are zero fuzz tests and zero benchmarks.
- No load test exercises auth limiting, inbox limits, graph bounds, SSE subscriber behavior, or database pool pressure.
- No chaos/resilience test covers Redis loss, PostgreSQL loss, delivery retry/recovery, Caddy switch failure, or partial federation failure.

Recommendation:

- Run unit packages with `-race` in Linux CI.
- Fuzz ActivityPub/JSON-LD parsing, HTTP signature structured-field parsing, URLs/domains, OAuth state, and webhook payloads.
- Add bounded load tests for public endpoints and SSE.
- Add failure-injection tests for queue/database/network transitions.

## TEST-6 — No coverage policy or trend

**Severity:** Medium

CI does not collect or publish coverage and has no floor. A successful `go test` therefore hides a 38.5% result and very low core-package coverage.

Recommendation: publish coverage artifacts and set risk-based package floors. Do not chase 100%; require meaningful coverage for authorization, transactions, parsers, and core workflows.

## TEST-7 — OpenAPI checks are partial and self-referential

**Severity:** Medium

The route contract test checks a manually maintained list against the manually maintained OpenAPI document. It does not derive the complete route set from the running Echo router, nor validate frontend request/response behavior.

Recommendation: compare normalized runtime routes to OpenAPI automatically and run schema-based request/response contract tests.

## TEST-8 — Deployment safety is untested

**Severity:** High operational risk

There are source-string assertions for a few deploy properties, but no disposable-VM/container test of install, upgrade, failed migration, failed health check, rollback, backup restore, secret preservation, or two consecutive upgrades.

Recommendation: test deployment scripts in an ephemeral Linux VM or nested test environment and perform scheduled restore drills.

# 5. Missing features for this kind of project

Not every Jira/GitHub/Linear feature is required. The list below separates baseline gaps from scope-dependent enhancements.

## Baseline release blockers

### FEAT-1 — Account recovery and verified identity

Missing:

- forgot/reset password flow;
- local email verification;
- resend verification;
- recovery after lost OAuth access;
- session/device management;
- MFA for owner/admin accounts.

Why required: users can permanently lose access, public registration accepts unverified email ownership, and privileged accounts have only one factor. A secure recovery flow should use single-use expiring tokens and enumeration-resistant responses: [OWASP Forgot Password](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html).

### FEAT-2 — Labels/tags

The database schema already contains labels and ticket-label joins, but the feature is absent end to end.

Why required: labels are a basic cross-cutting classification mechanism and are especially important because status, priority, and type are fixed enums.

### FEAT-3 — Ticket search and usable pagination

Backend ticket filters include assignee/status/priority/type but no text search. Frontend lists request only the first capped page.

Why required: projects become unusable as soon as they exceed the hardcoded limits. Add indexed search and cursor pagination before claiming scalable project management.

### FEAT-4 — Due dates and scheduling

Tickets have no due date, start date, estimate, milestone, or sprint association.

Why required: without at least a due date, the system tracks state but cannot manage delivery time or overdue work.

### FEAT-5 — Change/activity history

There is admin audit and ActivityPub history, but no user-facing ticket/project history showing who changed title, status, priority, assignee, role, or links.

Why required: collaborative work needs accountability, debugging, and recovery from accidental changes. Federation makes provenance even more important.

### FEAT-6 — Attachments

There is no attachment/file model, upload API, storage backend, malware/content validation, or UI.

Why required: real tickets commonly need screenshots, specifications, and evidence. If intentionally excluded, document external-link-only scope.

### FEAT-7 — Complete notifications

The only database notification type is `ticket.assigned`. Missing events include comments, mentions, status changes, invitations, due/overdue items, role changes, federation failures, and admin/security events. There are no email notifications or preference controls.

Why required: users cannot depend on the product without continuously polling boards.

### FEAT-8 — Archive/restore and safe deletion

Projects and tickets are hard-deleted, with cascading database effects and federation tombstones. There is no archive, recycle bin, retention window, restore, or user-facing export before deletion.

Why required: accidental deletion is inevitable in collaborative systems; database backups are not a usable per-item recovery mechanism.

## Strongly expected before a public production release

### FEAT-9 — Owner onboarding and operational recovery

The first owner can only be created with `pmsctl`. That is secure and avoids a public bootstrap endpoint, but the product needs clearer installer output, first-login guidance, owner recovery/runbook, and validation that at least one owner remains.

### FEAT-10 — Backup lifecycle

Deploy creates SQL backups but has no retention, off-host copy, encryption policy, restore command, or automated restore verification.

### FEAT-11 — Import/export and data portability

There is no project export/import, user data export, or bulk ticket import. This is important for adoption and disaster recovery, especially for a federated product.

### FEAT-12 — API/service credentials and outbound webhooks

The system has user JWTs and inbound GitHub webhooks but no scoped API tokens/service accounts, general outbound webhooks, or integration management. Automation currently depends on browser/user credentials or direct database/CLI access.

### FEAT-13 — Consistent localization

English/Ukrainian infrastructure exists, but many feature/admin strings remain hardcoded English. Localization is not complete or tested for missing keys, overflow, pluralization, and dates.

### FEAT-14 — Accessibility completion

Keyboard-equivalent board movement, focus behavior, screen-reader announcements, contrast checks, reduced motion, and automated accessibility tests are missing.

## Scope-dependent enhancements, not immediate blockers

- milestones, sprints, backlog planning, and roadmap views;
- estimates, time tracking, workload/capacity, and reports;
- custom fields and configurable workflows/statuses;
- saved filters/views and bulk editing;
- organizations/teams across projects;
- SAML/OIDC enterprise SSO and directory provisioning;
- calendar, email, Slack/Teams, and more source-control integrations;
- advanced graph analytics and pipeline automation.

These should not be prioritized ahead of the baseline gaps unless they are explicit thesis/product requirements.

# Ordered remediation plan

## P0 — Before the next production deployment

1. Pin Go to 1.26.5 or 1.25.12 consistently in CI and Docker; add `govulncheck`.
2. Upgrade the frontend dependency graph until production audit has no unresolved high advisories; document any unreachable exceptions.
3. Add a full frontend CI job: frozen install, behavioral tests, typecheck, lint, build, and audit.
4. Replace persistent 72-hour localStorage bearer sessions with a secure server-managed cookie/session design, or at minimum sharply reduce lifetime while implementing the migration.
5. Add CSP and baseline security headers.
6. Make release/install artifacts immutable and verifiable; stop defaulting production install to mutable `main` assets/images.

## P1 — Next stabilization iteration

1. Configure/observe DB pool limits and correct health/readiness semantics.
2. Replace string-matched errors with typed errors and a standard error envelope.
3. Add account-aware auth throttling, auth event audit/alerts, PKCE, and privileged-account MFA.
4. Remove frontend containers from the federation network; harden the frontend container.
5. Add real frontend component/integration/E2E tests and Linux race tests.
6. Fix the brittle architecture test and add coverage reporting/floors.
7. Implement labels, ticket search, and actual pagination.
8. Add password reset/local email verification/session management.

## P2 — Product completeness

1. Add due dates, activity history, attachments, richer notifications, and archive/restore.
2. Refactor oversized backend/frontend files and shared local/remote workspace logic.
3. Generate clients/types from the API contract.
4. Add optimistic concurrency for edits.
5. Test installer/upgrade/rollback/restore in ephemeral infrastructure.
6. Add backup retention, off-host encrypted copies, and restore drills.

## P3 — Only after the baseline is stable

Add sprints/milestones, reporting, custom workflows, integrations, advanced graph analytics, and automation according to actual user requirements.

# Final conclusion

The project demonstrates strong technical ambition and some genuinely mature security work, especially around federation. Its biggest weakness is prioritization and verification: complex distributed features are better engineered than the ordinary account, frontend, dependency, and project-management paths most users depend on.

The correct next move is a stabilization release, not another advanced feature. Patch the toolchains and dependencies, redesign browser sessions, put the frontend into CI with real behavior tests, bound database resources, and complete the baseline PM/account lifecycle. After that, federation becomes a credible differentiator instead of a complexity multiplier sitting on an insecure and partially tested foundation.
