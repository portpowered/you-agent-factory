# Factory session process restart

This note describes which live factory sessions survive a **service process
restart** (stop and start of `you run`, the API server, or an equivalent
`FactoryService.Run` cycle). Use it when reviewing multi-tab dashboard behavior,
writing operator runbooks, or extending session registry code.

## Operator summary

- After the service process restarts, **only the `~default` factory session is
  live again**.
- Any **additional** factory sessions that were open before restart (for example
  a second dashboard tab backed by a named folder target) are **not** restored.
  Operators must **re-open** those sessions through the dashboard or
  `POST /factory-sessions` (or `you session create`) using the same folder and
  target they used before.
- Persisted factory definitions on disk under the service root are unchanged;
  only the **in-memory live session registry** is reset. Re-opening a session
  starts a new live runtime for that target; it does not resurrect the pre-restart
  in-process engine, event buffers, or pause flags.

## Maintainer contract

`FactoryService.Run` registers exactly one live session at startup:

1. Build the default runtime bundle for the configured service root.
2. Register `~default` through `registerLiveSession` with
   `defaultSessionTargetFromRuntimeBundle`.
3. Do **not** replay prior `OpenFactorySession` registrations from disk or API
   history.

Named or folder-target sessions created via `OpenFactorySession` exist only in
`factorysessions.Registry` for the lifetime of that process. A new process
rebuilds the registry from scratch with the default entry only.

Session-scoped runtime state (live handles, sidecars, in-memory pause windows,
and per-session artifact paths tied to the prior handle) is likewise
process-local. Callers that need continuity across restart must rely on
persisted factory files, work artifacts, and explicit re-open—not on implicit
session restoration.

## Regression test

`TestFactoryService_Run_RestartsOnlyDefaultSession` in
`pkg/service/runtime_session_runtime_test.go` is the authoritative behavioral
proof:

1. Start a running service with default factory `alpha` and open a second live
   session for factory `beta`.
2. Assert two live sessions before stop.
3. Stop the service and start a new `FactoryService` instance on the same root
   directory.
4. Assert exactly one live session id (`~default`) and that the default runtime
   directory still points at the `alpha` factory directory from before restart.

Do not change expected session count or default runtime directory in that test
without an explicit product decision and operator-doc update.

## Related surfaces

- Live session inventory: `GET /factory-sessions` (only `~default` immediately
  after restart until operators open more).
- Dashboard session tabs: tab selection and pause flags in
  `dashboardSessionStore` are browser-local; they do not repopulate server-side
  sessions after restart.
- CLI: `you session list` reflects the same registry as the API.
