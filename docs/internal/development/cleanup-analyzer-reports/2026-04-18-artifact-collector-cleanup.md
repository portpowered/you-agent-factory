# Cleanup Analyzer Report: Artifact Collector Cleanup

Date: 2026-04-18

## Scope

Agent Factory cleanup pass for the unwired artifact collector subsystem, its artifact-bundle model family, and the stale `ArtifactCollector` tick group. Runtime artifact behavior remains on workflow context artifact directories, replay artifacts, generated factory events, and event projections.

## Analyzer Commands

Original deadcode finding:

```bash
git grep -n -e "ArtifactCollectorSubsystem.TickGroup" origin/main -- libraries/agent-factory/docs/development/deadcode-baseline.txt
```

Branch-base caller inventory:

```bash
git grep -n -e "ArtifactCollectorSubsystem" -e "NewArtifactCollector" origin/main -- libraries/agent-factory
git grep -n -e "ArtifactBundle" -e "PostAction" -e "ArtifactStatus" -e "TokenSummary" -e "WorkflowMetrics" origin/main -- libraries/agent-factory
```

Final active Go inventory:

```bash
rg -n "ArtifactCollectorSubsystem|NewArtifactCollector|ArtifactBundle|PostAction|ArtifactStatus|TokenSummary|WorkflowMetrics" libraries/agent-factory -g "*.go"
rg -n "ArtifactCollectorSubsystem.TickGroup" libraries/agent-factory/docs/development/deadcode-baseline.txt
```

Retained artifact-surface inventory:

```bash
rg -n "ArtifactDir" libraries/agent-factory -g "*.go"
rg -n "ReplayArtifact" libraries/agent-factory -g "*.go"
rg -n "FactoryEvent" libraries/agent-factory/pkg libraries/agent-factory/tests -g "*.go"
rg -n -g "*.go" -- "--record|--replay|RecordPath|ReplayPath" libraries/agent-factory/pkg/cli libraries/agent-factory/pkg/service libraries/agent-factory/tests
```

## Findings

- The branch base accepted `pkg/factory/subsystems/artifacts.go:54:39: unreachable func: ArtifactCollectorSubsystem.TickGroup` in `docs/development/deadcode-baseline.txt`.
- `pkg/factory/runtime/factory.go` did not construct `NewArtifactCollector`, so the collector could not run in normal runtime execution.
- The branch-base collector inventory had 21 matches, all in the deadcode baseline, `pkg/factory/subsystems/artifacts.go`, and `pkg/factory/subsystems/artifacts_test.go`.
- The branch-base artifact-bundle inventory had 53 matches, all in the collector implementation/tests or `pkg/interfaces/artifact_collection.go`.
- Active artifact features use existing workflow context, replay, generated event, and projection surfaces rather than `ArtifactBundle`.

## Removed Symbols

- `subsystems.ArtifactCollectorSubsystem`
- `subsystems.NewArtifactCollector`
- `subsystems.ArtifactCollector`
- `interfaces.ArtifactBundle`
- `interfaces.ArtifactStatus`
- `interfaces.PostAction`
- `interfaces.TokenSummary`
- `interfaces.WorkflowMetrics`

## Retained Surfaces

- `factory/context.FactoryContext.ArtifactDir` remains the workflow context artifact directory.
- Worker prompt context keeps `.Context.ArtifactDir` through `workers.PromptContext`.
- `interfaces.ReplayArtifact`, replay load/save logic, side-effect matching, and record/replay test helpers remain active.
- CLI and service record/replay paths remain active through `--record`, `--replay`, `RecordPath`, and `ReplayPath`.
- Generated `pkg/api/generated.FactoryEvent` payloads and event projections remain the canonical runtime history surface.

## Retained Surface Verification

- `pkg/factory/context` tests verify `FactoryContext.ArtifactDir` is derived and created with the workflow run directories.
- `pkg/workers` prompt-renderer tests verify `.Context.ArtifactDir` renders through `workers.PromptContext`.
- `pkg/replay`, `pkg/service`, `pkg/cli`, and `pkg/cli/run` tests verify replay artifact storage plus record/replay command and service configuration surfaces remain active.
- `pkg/api`, `pkg/factory/projections`, and `ui` tests verify generated `FactoryEvent` payloads and dashboard event projections remain the canonical history/read-model path.

## Outcome

- Deleted the unwired collector subsystem implementation and its tests.
- Deleted the artifact-bundle DTO family after caller analysis found no active Go consumers outside the removed collector path.
- Removed the stale `ArtifactCollector` tick group from the active subsystem enum.
- Removed the accepted deadcode baseline finding for `ArtifactCollectorSubsystem.TickGroup`.
- Preserved active runtime history and artifact behavior on workflow context artifact directories, replay artifacts, and generated factory events.

## Final Match Expectations

- `rg -n "ArtifactCollectorSubsystem|NewArtifactCollector|ArtifactBundle|PostAction|ArtifactStatus|TokenSummary|WorkflowMetrics" libraries/agent-factory -g "*.go"` returns 0 active Go matches.
- `rg -n "ArtifactCollectorSubsystem.TickGroup" libraries/agent-factory/docs/development/deadcode-baseline.txt` returns 0 matches.
- `rg -n "ArtifactDir" libraries/agent-factory -g "*.go"` returns active workflow context and prompt-context matches; the cleanup run observed 12 matches.
- `rg -n "ReplayArtifact" libraries/agent-factory -g "*.go"` returns active replay model, recorder, reducer, harness, and functional-test matches; the cleanup run observed 80 matches.
- `rg -n "FactoryEvent" libraries/agent-factory/pkg libraries/agent-factory/tests -g "*.go"` returns active generated event, event-history, projection, replay, and test matches; the cleanup run observed 531 matches.

## Retained Exceptions

- Historical cleanup report files may mention removed names as audit evidence.
- No active Go exceptions are retained for `ArtifactCollectorSubsystem`, `NewArtifactCollector`, `ArtifactBundle`, `PostAction`, `ArtifactStatus`, `TokenSummary`, or `WorkflowMetrics`.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/factory/subsystems ./pkg/interfaces -count=1
go test ./pkg/factory/context ./pkg/workers ./pkg/replay ./pkg/service ./pkg/cli ./pkg/cli/run ./pkg/api ./pkg/factory/projections -count=1
cd ui && bun run test -- App.test.tsx && cd ..
make lint
```
