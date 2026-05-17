# split-functionallong-provider-template-helpers-from-default-support

## Why

The provider functional-test helper surface currently mixes two different
ownership lanes in one file.

Current live evidence on `main`:

- `tests/functional/providers/helpers_test.go` defines both default-build test
  helpers and helpers that are only consumed by
  `//go:build functionallong`
  `tests/functional/providers/cli_template_resolution_long_test.go`
- the live deadcode baseline still reports those long-only helper symbols as
  unreachable even though the long suite calls them
- the concrete long-lane helper cluster is:
  - `buildModelWorkerConfig`
  - `writeNamedWorkerAgents`
  - `writeExecutionTemplateWorkstationAgents`
  - `configureResourceGatedTemplateWorkstation`
  - `configureExecutionTemplateWorkstation`
  - `configureTwoInputResourceGatedTemplateWorkstation`
  - `writeTwoInputResourceSeeds`
  - `writeExecutionTemplateSeed`
  - `twoInputTemplateArgs`
  - `executionTemplatePrompt`
  - `executionTemplateWantPrompt`
  - `assertProviderArgsPrompt`
  - `assertProviderStdin`
  - `assertProviderExecutionFields`

This is a narrow simplification seam: helper ownership should match the build
lane that actually uses the helpers instead of leaving live functionallong
support stranded in the default-build deadcode baseline.

## Do

- inspect `tests/functional/providers/helpers_test.go` and split the helper
  surface so functionallong-only template helpers live under a
  `//go:build functionallong` owner file, or add the minimum local helper
  contract coverage needed if one or two helpers truly need default-build
  ownership
- keep the surviving default-build helper file focused on helpers that are
  actually consumed by default-build provider tests
- remove only the deadcode-baseline findings that are proven stale because the
  helpers remain live through the correct build lane
- preserve the observable behavior of the provider template-resolution tests in
  both default and `functionallong` lanes

## Constraints

- do not broaden this lane into provider runtime changes, template semantics,
  CLI contract changes, or wider test rewrites outside
  `tests/functional/providers/*`
- do not keep stale deadcode-baseline entries for helpers that remain live
  after ownership is aligned
- prefer aligning helper ownership with the build tags over adding generic
  dummy callers just to silence deadcode
- keep verification behavioral at the provider functional-test and deadcode
  command boundaries rather than source-layout assertions

## Verification

- run `go test ./tests/functional/providers`
- run `go test -tags functionallong ./tests/functional/providers`
- run `go run ./cmd/deadcodecheck`
