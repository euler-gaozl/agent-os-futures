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

Failed, cancelled and unknown-effect tasks can also open their retained reading
records even when no report was generated. The evidence panel does not request
a report check without an answer. Output/context and per-task budget errors have
plain-language messages; displaying saved readings does not mark a failed task
complete or retry its model/tool calls.

## Structured research records

New Web tasks pin an `as_of` classification date and the `futures-evidence-v1`
output contract in Runtime-owned metadata. This date sorts statistical windows;
it does not establish what was publicly known on that historical day. Older
clients can omit it, and older Markdown answers remain readable.

`GET /api/tasks/{id}/report-check` is a read-only, app-scoped projection. It
checks strict bounded JSON, dates, stock/flow/cumulative-flow/capacity labels,
reported/estimate/plan labels and exact quotations from completed document-read
receipts. It never reads the current Knowledge library as a substitute. A quote
match includes the immutable operation/version and Unicode position. The Web
opens that exact receipt and highlights the quote, including across page bounds;
failed field checks do not hide an otherwise matching original quotation.

The v1 expression matcher recognizes only direct clauses with adjacent explicit
date, scope, metric, amount and unit. It does not infer omitted years, carry
subjects between clauses, or treat distant estimate words as evidence. Other
wording is preserved with a review flag; it is not evidence that the underlying
fact is false. The optional arithmetic comparison needs an adjacent pair in the
same quotation, introduced by an explicit same-basis statement, matching
measure/unit/scope/nature and disjoint ordered periods. An explicit different-
basis statement rejects the comparison. This is deliberately limited consistency
checking, not a general semantic verifier, source authenticity/completeness check,
stable-sample certification, or independent cross-check. Question/gap text remains
Agent-generated investigation material even when structural checks pass.

The v2 validator also separates ratios (including percentage-point differences)
and outstanding contract commitments from capacity and physical inventory.
It flags period-dependent metric labels, incompatible comparisons even before
source checking, quantities in question/gap prose, and explicit plan labels
without linked plan facts. These are finite rules, not general prose entailment;
neutral questions and gap descriptions can still require human source review.
Historical raw results are never rewritten by a validator upgrade.

Each response includes the raw-result SHA-256 and validator version. Limits are
96 KiB JSON, 40 facts, 30 comparisons, 8 questions; retained-source lookup uses at
most 48 requests / 256 KiB text / 15 seconds and the first 8 receipts per document.
Budget exhaustion or unsupported format is reported as pending review. The BFF
does not persist a second report authority. Raw model output is retained by
Runtime. JSON export bundles the unchanged raw output and the check, including
source URL/publication/capture labels copied from that retained read; labels
are not independently verified metadata. Legacy export stays Markdown. No schema/config
migration or new credential is needed for reading reports. Enabling in-task MCP
feedback requires the separate connector configuration described below.

## In-task research feedback

`POST /mcp/research` offers the versioned, read-only
`check_research_draft_v2` tool. It accepts only a bounded JSON draft string and
`as_of`, never a run ID or URL, and makes no outbound requests or DB writes.
It returns the draft hash, validator, repairable issues and source-review issues
within 5,900 UTF-8 bytes for the complete MCP tool result, including escaped
JSON text, with total/truncated counts. Each issue keeps its path and code;
codes with identical explanations reuse their first explanation (`message` is
then omitted). When one code has multiple distinct explanations, each is kept. The
notice explains this convention. This stays below Runtime's 6,000-byte archive
threshold, avoiding repeated artifact paging just to read duplicate guidance.
It does not return the
draft, create a report authority, match retained quotations, or claim that the
caller-supplied date equals a task's pinned date. Runtime retains tool receipts.
The final Web check still uses that task's actual immutable readings.

This route has a separate service-auth chain: only this service's `SERVICE_TOKEN`
is accepted, never BUSINESS_TOKEN, DEV_LOCAL or static-path exemptions. Register
that credential in Registry under a dedicated name, allow the exact Futures
host:port in Runtime/ToolHub MCP policy, discover/check/activate the connector,
and bind its exact revision to a new Agent version. Never put the credential in
prompts, Web or Git. Existing tasks retain their old bindings. MCP registration
and these deployment/configuration steps are not performed by service startup.

New submission instructions request at most two draft checks and correction of
repairable issues without changing quotations or dropping key evidence. This is
an Agent instruction, not a new hard per-tool quota; existing Runtime step/model
budgets bound execution. No new decision loop or Runtime business rule is added.

The small CSS/React foundation and bounded HTTP proxy were copied from the recorded integration commit. Each repository owns its copy, dependency lock and tests; no build reads the sibling Web. Existing UI appearance is preserved for extraction. Complete redesign and real desktop/narrow-screen workflows remain the platform P3 gate.

Independent tests use synthetic HTTP peers and do not prove paid provider or deployed-service operation. Current source/build/CI/live validation is recorded separately in the integration P2 ledger. Upgrade and rollback require the same HTTP/data/tool contract and bundle the matching BFF plus Web; there is no DB migration to undo.

Run scripts/Package-Web.ps1 -Destination <new-path> for a complete immutable native BFF/Web package, then scripts/Test-RelocatedPackage.ps1 -Package <path> to move it to a new sibling and exercise the actual executable with synthetic HTTP peers. The original location must be absent. No source, Node compiler or database is required by the package.
