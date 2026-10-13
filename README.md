# Agent OS futures

Owns the futures HTTP BFF and its separately built React/TypeScript Web. Service role is **futures**, default port **8098**. The integration repository selects this owner by exact commit. This repository never reads a database and has no migration command; release migration owner is none. Registry, ToolHub, Runtime, Knowledge, Storage and Sources retain their own data and authorization.

## Build

Go 1.27.2, Node 24.19.0 and pnpm 11.25.0. Fixed SDK rc.2 is vendored with canonical Go checksums; GOWORK=off and GOPROXY=off need no sibling clone or private module token. Run scripts/Build.ps1 for native BFF/healthcheck binaries. Run pnpm --dir web install --frozen-lockfile --ignore-scripts, pnpm --dir web test and pnpm --dir web run build. Docker builds only this service and Web; runtime image has no Node compiler or DB.

Use deploy/service.example.json as the configuration contract, never commit real configuration. Start the BFF with --config pointing to a private file and FUTURES_WEB_DIR pointing to web/dist in a source build, or web in a Package-Web release (relative to its root). The healthcheck reads its private config from AGENTOS_CONFIG. Readiness verifies this process, not every downstream service; UI reports downstream availability separately.

## Boundaries

User authentication uses BUSINESS_TOKEN; outbound requests use a separate SERVICE_TOKEN. DEV_LOCAL=1 is for this exact loopback port only and still rejects foreign Origin/Host and unmarked writes. Static GET/HEAD assets are public; API/metrics/readyz remain protected. Startup rejects a missing/short effective user token or reuse of SERVICE_TOKEN, including DEV_LOCAL. Futures retains the prior ADMIN_TOKEN fallback when BUSINESS_TOKEN is absent, provided it is separate from SERVICE_TOKEN. No secrets are embedded in Web assets. Only published futures Agents and app-scoped Runtime tasks are exposed; no tool/credential/admin forwarding.

Research records include a saved-reading panel and document citation buttons.
`source://document/<id>` links open only an app-scoped retained document read;
unknown IDs show an explicit missing citation instead of being resolved from
another task or from the current Knowledge library. `/api/tasks/{id}/sources`
and `/sources/record` proxy only Runtime's corresponding app evidence paths,
using the service identity and no-store responses. Install a Runtime providing
these additive endpoints first; an older Runtime leaves the report readable
but shows source lookup unavailable. No schema/config/credential change is needed.

The reader retains plain text, Unicode pagination, read ranges and metadata
labels from the saved source. Source labels are not an independent truth check.
Only successful document_read receipts are listed; other capture tools are not
yet represented. A completed task still needs its facts/citations checked.

The small CSS/React foundation and bounded HTTP proxy were copied from the recorded integration commit. Each repository owns its copy, dependency lock and tests; no build reads the sibling Web. Existing UI appearance is preserved for extraction. Complete redesign and real desktop/narrow-screen workflows remain the platform P3 gate.

Independent tests use synthetic HTTP peers and do not prove paid provider or deployed-service operation. Current source/build/CI/live validation is recorded separately in the integration P2 ledger. Upgrade and rollback require the same HTTP/data/tool contract and bundle the matching BFF plus Web; there is no DB migration to undo.

Run scripts/Package-Web.ps1 -Destination <new-path> for a complete immutable native BFF/Web package, then scripts/Test-RelocatedPackage.ps1 -Package <path> to move it to a new sibling and exercise the actual executable with synthetic HTTP peers. The original location must be absent. No source, Node compiler or database is required by the package.
