# PRD: Factory Session Load Failure Debuggability

## Context

`POST /factory-sessions` currently collapses some readable-but-broken factory folders into `not_runnable` or validate-only `initsNewFactory: true` when target discovery hits a runtime config load failure. That hides the real problem from operators, API consumers, and dashboard users, and it can mislead the UI into suggesting factory initialization for an already existing factory.

This work preserves the real failure during discovery, emits structured operator logs, returns a safe validation-style API error for broken factory config loads, and updates the dashboard to render actionable inline diagnostics instead of init-new-factory guidance.

## Goals

- Preserve the real reason factory target discovery failed when runtime config loading fails.
- Distinguish config-load failure from missing, unreadable, non-directory, and genuinely non-runnable targets on the public API surface.
- Show actionable diagnostics in the dashboard for validation and open-session failures without losing user context.
- Prevent `initNewFactory` affordances for folders that contain a broken factory rather than an empty or uninitialized folder.
- Keep diagnostics safe for customer-facing use while still useful for operators and maintainers.

## Project-Level Acceptance Criteria

- `POST /factory-sessions` preserves and classifies readable-folder runtime config load failures distinctly from `not_runnable`, missing, unreadable, and non-directory outcomes.
- The backend emits structured error logs for discovery-time config load failures with enough request and target identity context for operators to diagnose the rejected folder.
- Validate-only and session-open flows return the same config-load-failed classification for the same broken folder and continue to preserve existing runnable-factory and empty-folder behavior.
- The public API returns a structured `BAD_REQUEST` response with a stable failure code, safe top-level message, and issue targets that identify the broken factory subject.
- The dashboard renders inline, accessible failure diagnostics from the new API response and only offers init-new-factory behavior for genuine empty-folder validation results.
- Typecheck, lint, generated-artifact checks, and targeted backend/API/UI tests pass.

## User Stories

### factory-session-load-failure-debuggability-prd-001: Log config load failures during target discovery
**Description:** As an operator, I want discovery-time config load failures recorded in structured logs so that I can tell why a readable folder was rejected without reproducing under a debugger.

**Acceptance Criteria:**
- [x] When `/factory-sessions` discovery probes a readable candidate and runtime config loading fails, the backend emits a structured error log instead of silently suppressing the failure.
- [x] The log includes the submitted folder path, the probed factory target path, target identity when known, and a safe summary of the underlying load failure.
- [x] The log is emitted for both validate-only and session-open discovery paths and is not emitted for successful discovery.
- [x] Typecheck passes
- [x] Tests pass

### factory-session-load-failure-debuggability-prd-002: Return structured API diagnostics for broken factory config loads
**Description:** As an API consumer, I want a structured validation-style error for broken factory config loads so that I can distinguish them from missing folders, unreadable folders, and genuine init-new-factory cases.

**Acceptance Criteria:**
- [x] When a readable folder exists but runtime config loading fails during `/factory-sessions` discovery, the API returns `BAD_REQUEST` rather than `initsNewFactory: true` or a generic `not_runnable` outcome.
- [x] The response includes a stable public failure code that is distinct from missing, unreadable, non-directory, and not-runnable classifications.
- [x] The response includes a safe top-level message and structured issue targets that identify the affected factory subject and summarize the load failure without exposing stack traces or unsafe internals.
- [x] The same broken folder yields the same failure classification and issue payload shape for validate-only and session-open requests.
- [x] Validate-only requests for genuinely empty readable folders still return init-new-factory metadata rather than the new config-load-failed error.
- [x] If the public API shape changes, the authored OpenAPI fragments, bundled contract, and generated Go and TypeScript clients are updated together.
- [x] Typecheck passes
- [x] Tests pass

### factory-session-load-failure-debuggability-prd-003: Show actionable inline diagnostics in the dashboard
**Description:** As a dashboard user, I want broken-factory launch failures shown inline with actionable details so that I understand what needs fixing without being sent down the new-factory path.

**Acceptance Criteria:**
- [x] When the dashboard receives the new config-load-failed response from `/factory-sessions`, it renders an inline error panel on the current launch flow instead of treating the folder as an init-new-factory candidate.
- [x] The panel shows the top-level message and the structured issue list returned by the API, with readable text and semantics that do not rely on color alone.
- [x] The UI does not create, switch to, or partially initialize a new session tab for the failed request.
- [x] Existing init-new-factory affordances remain available only for genuine empty-folder validation responses.
- [x] The failure presentation works on desktop and mobile widths and preserves keyboard-readable access to the returned issues.
- [x] Typecheck passes
- [x] Tests pass
- [x] Verify in browser using dev-browser skill

### factory-session-load-failure-debuggability-prd-004: Keep validate and open-session flows aligned across runnable, empty, and broken folders
**Description:** As a maintainer, I want validation and open-session discovery to classify the same folder the same way so that backend, API, and UI behavior do not diverge across preflight and launch paths.

**Acceptance Criteria:**
- [x] The same broken factory folder yields the same config-load-failed classification whether the request is validate-only or a direct session-open attempt.
- [x] A runnable factory folder remains discoverable and openable in both validate-only and session-open flows without the new error classification.
- [x] A truly empty readable folder still returns init-new-factory discovery metadata only in validate-only flows and does not regress into a broken-config classification.
- [x] Regression coverage proves the distinction between runnable-factory, empty-folder, and broken-config cases at the service/API layer and for the dashboard launch behavior that depends on those outcomes.
- [x] Typecheck passes
- [x] Tests pass
