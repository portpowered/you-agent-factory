# PRD: Workspace Tabs for Concurrent Factory Sessions

## Introduction

Evolve infinite-you from a single active factory runtime into a workspace-style host that can run multiple factory sessions at the same time. The product model should feel similar to VS Code tabs: each tab points to one concrete factory instance, runs independently, and can be opened, switched, and closed without affecting the other tabs. One infinite-you server remains the shared entrypoint, but each tab represents its own live factory session with isolated runtime state.

The main abstraction change is multiplicity. Customers should be able to run multiple factories however they wish: from separate folders, from the same folder more than once, or from named factories and default factories exposed from the same underlying layout. Tabs are the user-facing way to manage that multiplicity, but the backend should be designed around many independent running factory instances rather than one mutable current factory.

## Goals

- Allow one infinite-you server to host multiple concurrent factory sessions.
- Allow sessions to be created from separate folders, repeated runs of the same folder, or named/default factory variants selected by the customer.
- Present active sessions as tabs in the dashboard header, with a `+` control to open another factory folder.
- Keep runtime state, work, events, logs, inputs, cron activity, and watchers isolated per tab/session.
- Preserve a simple default session so current single-factory workflows still feel natural.

## User Stories

### US-001: Open a factory folder as a new tab
**Description:** As a customer, I want to click `+`, select a folder and factory target, and open it as a running factory tab so I can work with multiple factories side by side.

**Acceptance Criteria:**
- [ ] The dashboard header includes a visible `+` control for opening a new tab.
- [ ] The open-tab flow accepts a folder path and validates that it contains a runnable factory layout.
- [ ] The open-tab flow can target either the default factory in that folder or a named factory available from that folder's layout.
- [ ] If the folder exposes exactly one runnable target, the system opens it immediately without showing a target picker.
- [ ] If the folder exposes multiple runnable targets, the system shows a compact second-step target picker.
- [ ] Opening a valid target starts a new factory session and selects its tab.
- [ ] Opening an invalid folder shows a clear validation error without affecting existing tabs.
- [ ] Opening the same folder and factory target again is allowed and creates another independent session when the customer wants parallel runs.
- [ ] Typecheck/lint passes.
- [ ] Verify in browser using dev-browser skill.

### US-002: Run multiple folder-backed sessions concurrently
**Description:** As a customer, I want each tab to represent an independent running session so work in one factory instance does not interfere with another.

**Acceptance Criteria:**
- [ ] Introduce a manager abstraction that owns multiple live factory session handles at once.
- [ ] Starting a new session does not pause, replace, or mutate other running sessions.
- [ ] Each session has its own engine, runtime config, event history, file watcher, cron watchers, and log context.
- [ ] Stopping one session cancels only that session's engine and sidecars.
- [ ] Integration or package-level tests cover at least two simultaneous sessions from different folders and at least two simultaneous sessions from the same folder.
- [ ] Typecheck/lint passes.

### US-003: Scope API calls to the selected tab
**Description:** As a dashboard or API user, I want all reads and mutations to target the active tab's session so each tab behaves like its own running instance.

**Acceptance Criteria:**
- [ ] Add session-scoped API routes for work submission, work request upsert, work list, work get, status, events, and current factory definition.
- [ ] The active tab in the dashboard determines which session-scoped routes the UI uses.
- [ ] Unknown session IDs return `404 NOT_FOUND` with a clear error message.
- [ ] Existing unscoped routes continue to work for the default session during the transition period.
- [ ] Contract tests confirm generated backend and frontend types include the session-scoped paths.
- [ ] Typecheck/lint passes.

### US-004: Switch between tabs cleanly
**Description:** As a dashboard user, I want to switch tabs and immediately see the correct work, events, and status for that session.

**Acceptance Criteria:**
- [ ] Tabs show a human-friendly label using `(folder name + factory name/default)` or an equivalent compact form.
- [ ] Switching tabs reconnects the event stream and reloads session-scoped data.
- [ ] Timeline, current selection, work charts, and submit-work state are reset or partitioned so data from one tab does not leak into another.
- [ ] Loading, empty, offline, and error states are shown explicitly while switching or reconnecting.
- [ ] Typecheck/lint passes.
- [ ] Verify in browser using dev-browser skill.

### US-005: Close a tab without affecting other sessions
**Description:** As a customer, I want to close one factory tab so I can stop or detach that session without disrupting other work.

**Acceptance Criteria:**
- [ ] Each non-required tab can be closed from the header.
- [ ] Closing a tab stops that session's runtime and removes it from the active tab list.
- [ ] Closing one tab leaves all remaining sessions intact.
- [ ] Closed sessions do not retain in-memory event history or runtime state in the first version.
- [ ] Typecheck/lint passes.

### US-006: Preserve a default session
**Description:** As an existing user, I want the app to still open into one obvious default factory session so local single-factory usage remains simple.

**Acceptance Criteria:**
- [ ] Server startup creates a default session from the current startup folder/config behavior.
- [ ] If no extra tabs are opened, the experience still works like today's single-factory flow.
- [ ] On restart, only the default session is restored automatically in the first version.
- [ ] Existing unscoped CLI and API flows continue to target the default session.
- [ ] Typecheck/lint passes.

### US-007: Add session-aware CLI support
**Description:** As a CLI user, I want to target a session explicitly so I can submit and inspect work for a specific open tab.

**Acceptance Criteria:**
- [ ] Add `--session` or equivalent explicit session selector to submit and work-list commands.
- [ ] Omitting the selector targets the default session.
- [ ] CLI requests use session-scoped URLs when a non-default session is selected.
- [ ] CLI tests cover default and explicit session routing.
- [ ] Typecheck/lint passes.

### US-008: Isolate session artifacts and replay output
**Description:** As an operator, I want logs and replay artifacts to stay separated by session so concurrent debugging remains understandable.

**Acceptance Criteria:**
- [ ] Logs include session ID, folder path, and runtime instance ID.
- [ ] SSE history and live events for one session include only that session's events.
- [ ] Replay recording writes one artifact per session.
- [ ] Tests confirm events from one session are not visible through another session's stream.
- [ ] Typecheck/lint passes.

## Functional Requirements

- FR-1: The system must treat a live factory session as the primary runtime unit rather than a single mutable current factory.
- FR-2: Each session must resolve to one concrete factory target chosen by the customer.
- FR-3: A factory target may come from a separate folder, a repeated run of the same folder, or a named/default factory selection within a folder.
- FR-4: The dashboard must render active sessions as tabs in the header.
- FR-5: The dashboard must provide a `+` control for opening another factory folder as a new tab.
- FR-6: The system must assign a stable session identifier used in URLs, UI state, runtime maps, logs, and event routing.
- FR-7: The system must allow multiple sessions to run concurrently within one server process.
- FR-8: Each session must isolate engine state, runtime config, event history, work, watchers, cron behavior, and runtime sidecars.
- FR-9: The system must expose session-scoped routes for work, status, events, and factory-definition readback.
- FR-10: The system must preserve a default session and keep existing unscoped routes as aliases to that default during the transition period.
- FR-11: The system must validate requested folder paths before creating a session and return actionable errors for invalid layouts.
- FR-12: The system must allow users to close a tab and terminate only that session.
- FR-13: The first version must not restore non-default tabs after server restart.
- FR-14: The first version must not retain closed-session runtime state or event history in memory.
- FR-15: Replay output must be emitted separately per session.
- FR-16: Documentation and CLI help must explain the difference between default-session behavior and explicit session targeting.
- FR-17: The system must allow multiple simultaneous sessions that originate from the same folder and factory target.

## Non-Goals

- No billing, quotas, tenant plans, or external SaaS multi-tenancy.
- No cross-server distributed orchestration of sessions.
- No persistent restoration of all previously open tabs in the first version.
- No retained stopped-session snapshots or archived in-memory histories in the first version.
- No dependency-injection framework migration as part of the first implementation milestone.

## Design Considerations

- Tabs should feel like an operational workspace, not a browser bookmark strip or a marketing navigation element.
- The active tab should be obvious, and the `+` action should stay visible without crowding the header.
- Folder identity should be inspectable somewhere in the tab or adjacent controls, but the tab label should stay compact.
- Switching tabs should feel quick and deterministic, even if the underlying event stream reconnects.
- The empty state for “no extra tabs yet” should still make the default session feel complete and usable.
- The open-tab flow should be folder-first, with smart default behavior: open immediately when there is only one runnable target, and show target selection only when multiplicity is present.

## Technical Considerations

### Product Model Shift

- Replace the primary concept of “named factory activation” with “workspace sessions bound to folders.”
- Treat session IDs, not factory names, as the main routing identifier.
- Model folder path and factory target as session inputs rather than treating one folder as one singleton runtime.
- Keep named-factory behavior available as one way to select a target factory, rather than making it the core runtime abstraction.

### Codebase Findings

- `FactoryService` currently owns one active runtime through single-instance fields such as `factory`, `listener`, `net`, `runtimeCfg`, and `eventHistory` in `pkg/service/factory.go`. This is the main reason the current design cannot host multiple tabs honestly.
- `ActivateNamedFactory` is a runtime replacement path that requires idle state and swaps one runtime for another. That behavior conflicts with the tabbed-session model.
- `CreateNamedFactory` currently persists and activates, which matches import-and-replace behavior rather than open-another-tab behavior.
- API handlers in `pkg/api/handlers.go` resolve one `s.runtime` and do not have session context for work, status, or events.
- The UI currently assumes global endpoints like `/work`, `/events`, and `/factory/~current`, so tab switching will require scoped endpoint construction and client-state partitioning.
- CLI submit and work-list commands currently hardcode unscoped `/work` requests and will need explicit session-aware routing.

### Suggested Implementation Shape

- Add a `FactorySessionManager` or equivalent service that owns `map[SessionID]*FactorySessionHandle`.
- Extract the current single-runtime construction path into a reusable builder that creates one session handle from one factory target selection.
- Represent session creation input explicitly, for example as `(folder path, factory selector, run identity)` rather than just `folder path`.
- Give each session handle ownership of its engine, listener, event stream, log context, replay recorder, and lifecycle controls.
- Make the API surface resolve a session by session ID before delegating to existing work/status/event logic.
- Keep a default session registered at startup and map legacy unscoped routes to it during migration.

### Dependency Management Guidance

- Do not make adopting `wire` or another dependency-injection framework part of the first workspace-tab implementation milestone.
- Prefer explicit constructor parameters and focused builder functions while the session-manager shape is still evolving.
- Re-evaluate top-level compile-time DI only after the session manager, session handle, and startup graph have stabilized.
- Treat lifecycle ownership, folder binding, and session isolation as the primary design problems; DI tooling is secondary.

## Success Metrics

- Users can run at least two factory sessions concurrently in one server process, including sessions from different folders and sessions from the same folder.
- Opening a new tab from a folder does not interrupt existing sessions.
- Switching tabs never shows mixed work, status, or event data from another session.
- Closing one tab stops only that session.
- Restarting the server restores only the default session in the first version.
- Replay artifacts and logs are clearly separated per session.

## Decisions Captured

- The default public session alias should be `~default`.
- On restart, only the default session should start automatically in the first version.
- Closed or stopped sessions should retain no in-memory state or event history in the first version.
- Leave `POST /factory` in place for now rather than removing it immediately.
- Replay recording should write one artifact per session.
- Leave `wire` and broader DI adoption out of scope for the first implementation phase.
- Customers should be allowed to run multiple sessions from the same folder when they choose to do so.
- Session-scoped routes should use `/factories/{factory-id}`.
- Tab labels should default to `(folder name + factory name/default)` or an equivalent compact form.
- Closing the default tab should be allowed.
- The first version should not expose a dedicated session-list endpoint beyond what the active UI flow needs.
- The open-tab UX should be a two-step folder-first flow with smart default behavior: auto-open when one runnable target exists, and show a compact target picker only when multiple targets exist.

## Open Questions

- A remaining design question is whether runnable targets should always be discovered fresh from disk at open time, or whether the picker should be limited to targets the server already knows how to launch from that folder.
