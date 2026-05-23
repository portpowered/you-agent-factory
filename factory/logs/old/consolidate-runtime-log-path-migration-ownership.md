# Cleanup Idea: Consolidate Runtime Log Path Migration Ownership

## Why

At `HEAD` `ca28e875`, the prior local cleanup request for editable
current-factory-definition transport is stale because that work landed on
`main` in PR `#286`.

The next best non-overlapping cleanup seam is backend-local in
`pkg/logging/runtime_logger.go`. After the recent name-standardization change,
the runtime logger still spreads canonical and legacy runtime-log path
construction and migration setup across repeated helpers:

- canonical `~/.you-agent-factory/logs` path assembly
- legacy `~/.agent-factory/logs` path assembly
- migration preconditions around legacy-exists and canonical-missing
- repeated test setup for canonical-versus-legacy directory behavior

This seam is narrow, behavior-preserving, and materially lower collision risk
than the currently busy dashboard, replay, import/export, and session-routing
areas.

## What To Change

Keep the cleanup backend-local and implementation-ready.

1. Simplify `pkg/logging/runtime_logger.go` by extracting small unexported
   helpers that own:
   - canonical runtime-log directory construction
   - legacy runtime-log directory construction
   - legacy-to-canonical migration precondition checks
2. Preserve the existing public behavior:
   - default runtime log directory remains `~/.you-agent-factory/logs`
   - legacy `.agent-factory/logs` migrates only when the canonical directory
     does not already exist
   - canonical directory wins unchanged when both canonical and legacy
     directories already exist
3. Trim repeated setup and assertions in
   `pkg/logging/runtime_logger_test.go` so the tests prove behavior through
   observable filesystem outcomes rather than copy-heavy setup blocks.
4. Only touch `pkg/cli/root.go` and its tests if one shared constant cleanly
   removes the remaining duplicated default runtime-log path literal without
   broadening the change.

## Constraints

- Do not change runtime logger public behavior or rolling-file policy.
- Do not change the canonical default path string exposed to users.
- Do not broaden this into unrelated logger refactors or CLI flag redesign.
- Prefer deletion of repeated path-building and migration logic over adding a
  new exported abstraction.

## Acceptance Criteria

- `pkg/logging/runtime_logger.go` no longer repeats canonical and legacy
  runtime-log path construction or migration setup logic.
- Focused runtime logger tests still prove:
  - the default path resolves to `~/.you-agent-factory/logs`
  - legacy `.agent-factory/logs` migrates when canonical is absent
  - canonical directory remains authoritative when both directories exist
- Any touched CLI help text remains behaviorally identical.

## Verification

Run focused backend checks:

- `go test ./pkg/logging`
- `go test ./pkg/cli/...`
