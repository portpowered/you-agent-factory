# Cleanup Idea: Retire the Legacy `pkg/interfaces` State Shim

## Why this cleanup exists

`pkg/interfaces` still exports a small legacy compatibility surface in
`pkg/interfaces/constants.go`:

- `type State string`
- `StateCompleted = "completed"`
- `StateFailed = "failed"`

That surface overlaps conceptually with the canonical runtime lifecycle enum in
`pkg/interfaces/factory_runtime.go`, which already owns:

- `FactoryStateCompleted = "COMPLETED"`
- `FactoryStateFailed = "FAILED"`

The lowercase `State` shim is no longer broadly shared. In the current tree,
its only runtime consumer is the CLI token-place suffix check in
`pkg/cli/run/run.go`.

This leaves `pkg/interfaces` carrying an exported compatibility type whose only
remaining job is to support two string comparisons in one CLI package.

## Requested change

Remove the legacy exported `State` compatibility shim from
`pkg/interfaces/constants.go` and keep the terminal-place suffix logic local to
the CLI package.

Keep the cleanup narrow:

- do not change the place ID convention of `{work_type_id}:{state_value}`
- do not change the observable `CountTokenStates` behavior in `pkg/cli/run`
- do not change `FactoryState` values or factory runtime lifecycle behavior
- do not broaden this into unrelated enum renames or public API contract edits

## Relevant files

- `pkg/interfaces/constants.go`
- `pkg/interfaces/factory_runtime.go`
- `pkg/cli/run/run.go`
- `pkg/cli/run/run_test.go`

## Acceptance criteria

- `pkg/interfaces/constants.go` no longer exports `State`,
  `StateCompleted`, or `StateFailed`.
- `pkg/cli/run/run.go` no longer depends on the legacy `interfaces.State` shim
  for terminal and failed place-state detection.
- The CLI keeps using the same lowercase place-state suffix behavior for
  `"completed"` and `"failed"`.
- Existing CLI tests continue to prove the observable behavior, especially the
  token-state counting path and terminal/failed helper behavior.

## Review guidance

Prefer behavioral proof in `pkg/cli/run/run_test.go` over adding new inventory
or contract-guard coverage. The regression risk is changing how terminal token
states are counted from place IDs, not the presence or absence of the legacy
export itself.
