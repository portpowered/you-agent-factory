# Cleanup Analyzer Report: Engine Firing Shim Cleanup

Date: 2026-04-18

## Scope

Agent Factory cleanup pass for the obsolete engine-local transition enablement
shim. Transition enablement remains owned by `pkg/factory/scheduler`; engine
runtime and subsystem dispatch paths use scheduler-owned APIs directly.

## Starting Deadcode Finding

The accepted deadcode baseline on the branch base included this finding:

```text
pkg/factory/engine/firing.go:16:6: unreachable func: applyCardinality
```

The branch base caller inventory also showed that `pkg/factory/engine/firing.go`
only delegated to scheduler APIs:

```text
libraries/agent-factory/pkg/factory/engine/firing.go:10:// findEnabledTransitions delegates to the scheduler package's exported function.
libraries/agent-factory/pkg/factory/engine/firing.go:11:func findEnabledTransitions(n *state.Net, marking *petri.MarkingSnapshot) []interfaces.EnabledTransition {
libraries/agent-factory/pkg/factory/engine/firing.go:12:	return scheduler.FindEnabledTransitions(n, marking)
libraries/agent-factory/pkg/factory/engine/firing.go:15:// applyCardinality delegates to the scheduler package's exported function.
libraries/agent-factory/pkg/factory/engine/firing.go:16:func applyCardinality(tokens []interfaces.Token, card petri.ArcCardinality) []interfaces.Token {
```

## Final Active Go Inventory

Commands were run from the repository root.

```bash
rg -n "applyCardinality" libraries/agent-factory/pkg/factory -g "*.go"
```

Result: no active Go matches.

```bash
rg -n "findEnabledTransitions" libraries/agent-factory/pkg/factory -g "*.go"
```

Result: no active Go matches.

```bash
rg -n "FindEnabledTransitions" libraries/agent-factory/pkg/factory -g "*.go"
```

Result:

```text
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:110:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:149:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:194:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:249:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:302:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:355:	if enabled := eval.FindEnabledTransitions(context.Background(), n, &belowThreshold); len(enabled) != 0 {
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:368:	enabled := eval.FindEnabledTransitions(context.Background(), n, &atThreshold)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:389:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:422:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:460:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:487:	enabled := eval.FindEnabledTransitions(ctx, n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:532:	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:537:	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 1 {
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:542:	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 1 {
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:547:	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:599:		enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:635:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:672:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement_test.go:714:	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
libraries/agent-factory/pkg/factory\scheduler\enablement.go:49:// FindEnabledTransitions identifies all transitions whose input arcs are satisfied
libraries/agent-factory/pkg/factory\scheduler\enablement.go:51:func (e *EnablementEvaluator) FindEnabledTransitions(ctx context.Context, n *state.Net, marking *petri.MarkingSnapshot) []interfaces.EnabledTransition {
libraries/agent-factory/pkg/factory\scheduler\enablement.go:332:// FindEnabledTransitions identifies all transitions whose input arcs are satisfied
libraries/agent-factory/pkg/factory\scheduler\enablement.go:340:func FindEnabledTransitions(n *state.Net, marking *petri.MarkingSnapshot) []interfaces.EnabledTransition {
libraries/agent-factory/pkg/factory\scheduler\enablement.go:342:	return eval.FindEnabledTransitions(context.Background(), n, marking)
libraries/agent-factory/pkg/factory\subsystems\noop_dispatcher.go:52:	enabled := scheduler.FindEnabledTransitions(d.state, &snapshot.Marking)
libraries/agent-factory/pkg/factory\subsystems\subsystem_dispatcher.go:108:	enabled := d.evaluator.FindEnabledTransitions(ctx, d.state, &snapshot.Marking)
```

## Deleted Files And Symbols

Deleted files:

- `pkg/factory/engine/firing.go`
- `pkg/factory/engine/firing_test.go`

Deleted symbols:

- `engine.findEnabledTransitions`
- `engine.applyCardinality`

The deleted test file contained wrapper-only enablement tests for the engine
shim. The active production dispatch paths retained scheduler API calls through
`scheduler.FindEnabledTransitions` and `scheduler.EnablementEvaluator`.

## Migrated Scheduler Cases

The scheduler test suite now owns the observable enablement cases that were
unique or underrepresented in the engine wrapper tests:

- Positive multi-input named binding with `MatchColorGuard`.
- `AllWithParentGuard` plus `CardinalityAll`, including parent-scoped token
  selection and exclusion of unrelated child tokens.
- `VisitCountGuard` threshold enablement.

These cases live in `pkg/factory/scheduler/enablement_test.go` and assert the
returned bindings and selected token IDs through the scheduler evaluator
surface.

## Baseline Update

`docs/development/deadcode-baseline.txt` no longer contains:

```text
pkg/factory/engine/firing.go:16:6: unreachable func: applyCardinality
```

The baseline remains managed by `make lint`, which runs the pinned deadcode
comparison for the Agent Factory module.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/factory/scheduler -count=1
go test ./pkg/factory/engine -count=1
go test ./pkg/factory/scheduler ./pkg/factory/engine -count=1
make lint
```

Results on 2026-04-18:

- `go test ./pkg/factory/scheduler -count=1` passed.
- `go test ./pkg/factory/engine -count=1` passed.
- `go test ./pkg/factory/scheduler ./pkg/factory/engine -count=1` passed.
- `make lint` passed; the deadcode baseline matched.

US-004 closeout results on 2026-04-18 20:14 -07:00:

- `go test ./pkg/factory/scheduler ./pkg/factory/engine -count=1` passed,
  including scheduler-owned observable enablement coverage such as
  `TestEnablementEvaluator_BindsAllTokensForMatchingParentGuard`.
- `make lint` passed; `go vet ./...` completed and the deadcode baseline
  matched.
