# Cleanup Analyzer Report: Dead Factory Public Options

Date: 2026-04-19

## Scope

Agent Factory cleanup pass for the final dead top-level factory public options and their backing `FactoryConfig` fields. The cleanup removes only the factory-level surfaces for channel buffer sizing, tracing, and duplicate work-request recording; the active engine-level work-request recorder remains the canonical bridge into generated factory event history.

Historical cleanup reports under `libraries/agent-factory/docs/development/cleanup-analyzer-reports/` are excluded from active-symbol conclusions because they intentionally preserve prior analyzer output.

## Before Deadcode Evidence

The branch-base deadcode baseline contained the three accepted production findings targeted by this cleanup:

```text
pkg/factory/options.go:154:6: unreachable func: WithChannelBuffer
pkg/factory/options.go:173:6: unreachable func: WithTracer
pkg/factory/options.go:217:6: unreachable func: WithWorkRequestRecorder
```

Evidence command:

```bash
git show e80c97b53:libraries/agent-factory/docs/development/deadcode-baseline.txt
```

## After Deadcode Evidence

The current accepted baseline is empty:

```bash
cd libraries/agent-factory
wc -c docs/development/deadcode-baseline.txt
```

Result:

```text
0 docs/development/deadcode-baseline.txt
```

`make lint` also regenerates the current deadcode report and proves the empty baseline matches the analyzer output:

```bash
cd libraries/agent-factory
make lint
```

Result:

```text
go vet ./...
go run ./cmd/deadcodecheck
[agent-factory:deadcode] baseline matches
```

After the run, `bin/deadcode-current.txt` is also empty.

## Active Symbol Inventory

Active Go inventory for removed non-engine surfaces:

```bash
rg -n "WithChannelBuffer|WithTracer|type Tracer|type WorkRequestRecorder|ChannelBuffer" libraries/agent-factory/pkg/factory -g "*.go"
```

Result: no active Go matches.

Active Go inventory for the work-request recorder name:

```bash
rg -n "WithWorkRequestRecorder|WorkRequestRecorder" libraries/agent-factory/pkg/factory -g "*.go"
```

Result:

```text
libraries/agent-factory/pkg/factory/engine/engine_test.go:706:        WithWorkRequestRecorder(func(_ int, record interfaces.WorkRequestRecord) {
libraries/agent-factory/pkg/factory/engine/engine_test.go:795:        WithWorkRequestRecorder(func(_ int, record interfaces.WorkRequestRecord) {
libraries/agent-factory/pkg/factory/engine/engine_test.go:862:        WithWorkRequestRecorder(func(_ int, record interfaces.WorkRequestRecord) {
libraries/agent-factory/pkg/factory/engine/options.go:83:// WithWorkRequestRecorder registers a callback invoked once for each request
libraries/agent-factory/pkg/factory/engine/options.go:85:func WithWorkRequestRecorder(fn func(int, interfaces.WorkRequestRecord)) Option {
libraries/agent-factory/pkg/factory/runtime/factory.go:147:        engine.WithWorkRequestRecorder(func(tick int, record interfaces.WorkRequestRecord) {
```

Classification:

- `pkg/factory/engine/options.go` is the retained engine-level recorder option.
- `pkg/factory/engine/engine_test.go` verifies the retained engine-level recorder behavior.
- `pkg/factory/runtime/factory.go` is the canonical bridge from engine work-request observations to `FactoryEventHistory.RecordWorkRequest`.
- No remaining match is the removed top-level `factory.WithWorkRequestRecorder` option or a `FactoryConfig.WorkRequestRecorder` field.

Full library inventory excluding historical cleanup reports:

```bash
rg -n "WithChannelBuffer|WithTracer|WithWorkRequestRecorder|type Tracer|type WorkRequestRecorder|ChannelBuffer|WorkRequestRecorder" libraries/agent-factory -g "!**/cleanup-analyzer-reports/**"
```

Result summary:

- 6 active Go matches, all listed in the work-request recorder inventory above.
- 9 non-Go matches in `libraries/agent-factory/tests/adhoc/factory/inputs/idea/default/factory-public-option-deadcode-cleanup.md`, which is checked-in historical planning input rather than active production code.

Active-code conclusion: the only Go matches are the retained engine-level recorder, its tests, and the runtime bridge.

## Outcome

- Removed dead factory-level public options and their backing `FactoryConfig` fields.
- Replaced public channel buffer sizing with the private runtime default used by runtime construction.
- Preserved canonical generated `WORK_REQUEST` event history through `engine.WithWorkRequestRecorder` and `FactoryEventHistory.RecordWorkRequest`.
- Cleared `docs/development/deadcode-baseline.txt` after the pinned deadcode analyzer output dropped to zero accepted findings.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/factory ./pkg/factory/runtime ./pkg/factory/engine -count=1
make lint
```
