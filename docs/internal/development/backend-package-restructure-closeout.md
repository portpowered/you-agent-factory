# Backend Package Restructure Closeout

This closeout records the behavior-preservation guardrails and final validation
bundle for the backend package restructure PRD.

## Non-goals Confirmed

- No intentional runtime behavior changes were introduced as part of the
  package moves.
- No generated API contracts were changed as part of this restructure.
- No package-tree cleanup outside the approved destinations in
  `backend-package-restructure-design.md` was bundled into the migration.

## Phase-by-Phase Preservation

### Phase 1: Canonical target tree

- Preserved behavior:
  the phase only defined the approved destination tree, dependency rules, and
  stable-versus-implementation package boundaries.
- Regression evidence:
  the design doc plus `progress.txt` recorded the target homes for
  `pkg/workers`, `pkg/factory`, `pkg/listeners`, `pkg/buffers`,
  `pkg/workcontent`, and `pkg/apisurface`.

### Phase 2: `pkg/workers/process`

- Preserved behavior:
  subprocess request shaping, env merging, timeout handling, exit-code routing,
  and platform-specific process cancellation.
- Regression evidence:
  `go test ./pkg/workers/... ./pkg/service/...`
  `go vet ./...`
  `go run ./cmd/pkgmaintcheck ./pkg`
  `make build`

### Phase 3: `pkg/workers/prompting`

- Preserved behavior:
  prompt rendering, prompt-template validation, and workstation template-field
  resolution.
- Regression evidence:
  `go test ./pkg/workers/... ./pkg/service/... ./pkg/api/... ./pkg/config/... ./pkg/replay/...`
  `go test -tags functionallong ./tests/functional/workflow -run 'TestLogicalMove_(Success|NoMatchRejected)'`
  `go vet ./...`
  `make build`

### Phase 4: `pkg/workers/provider`

- Preserved behavior:
  provider CLI execution, failure normalization, retryability typing,
  diagnostics capture, and provider session extraction.
- Regression evidence:
  provider-focused worker tests plus the package-level build and vet bundle
  recorded in `progress.txt`.

### Phase 5: `pkg/workers/executor`

- Preserved behavior:
  agent, script, noop, and workstation orchestration, including logical-move,
  model-backed, and script-backed execution flows.
- Regression evidence:
  `go test ./pkg/workers/... ./pkg/service/... ./pkg/factory/runtime/... ./pkg/config/... ./pkg/replay/... ./tests/functional/workflow/...`
  `go vet ./...`
  `go run ./cmd/pkgmaintcheck ./pkg`
  `make build`

### Phase 6: `pkg/factory/requests`

- Preserved behavior:
  work-request normalization, request JSON validation, relation indexing,
  request ID shaping, and trace propagation.
- Regression evidence:
  `go test ./pkg/factory/requests/... ./pkg/factory/engine/... ./pkg/factory/subsystems/... ./pkg/service/... ./pkg/listeners/... ./pkg/testutil/... ./pkg/api/...`
  `go test ./tests/functional/guards_batch/...`
  `go run ./cmd/pkgmaintcheck ./pkg`
  `make build`

### Phase 7: `pkg/factory/events`

- Preserved behavior:
  canonical event-history recording, replay snapshot generation,
  subscription behavior, and generated event shaping.
- Regression evidence:
  `go test ./pkg/factory/events/...`
  `go test ./pkg/factory/runtime/...`
  `go test ./pkg/service/...`
  `go run ./cmd/pkgmaintcheck ./pkg`
  `make build`

### Phase 8: helper-package re-homing

- Preserved behavior:
  file-watcher ingestion behavior moved under `pkg/service/ingest`, and the
  typed runtime buffer moved under `pkg/factory/runtime/buffers` without
  changing runtime dispatch semantics.
- Regression evidence:
  `go test ./pkg/service/ingest/... ./pkg/service/... ./pkg/factory/runtime/... ./pkg/factory/engine/... ./pkg/factory/subsystems/...`
  `go vet ./...`
  `go run ./cmd/pkgmaintcheck ./pkg`
  `make build`

## Approved Import-Path Moves

- `pkg/workers` implementation moved into:
  `pkg/workers/process`, `pkg/workers/prompting`,
  `pkg/workers/provider`, and `pkg/workers/executor`
- `pkg/factory` implementation moved into:
  `pkg/factory/requests` and `pkg/factory/events`
- `pkg/listeners` moved to `pkg/service/ingest`
- `pkg/buffers` moved to `pkg/factory/runtime/buffers`

`pkg/workcontent` and `pkg/apisurface` remain intentional top-level stable
boundaries as documented in the approved design.

## Compatibility Seams

Temporary root-package compatibility shims remain under `pkg/workers`:

- `pkg/workers/prompting_compat.go`
- `pkg/workers/provider_compat.go`
- `pkg/workers/executor_compat.go`

Removal plan:

- keep these seams only while same-package worker tests and any remaining
  broad imports still rely on the `pkg/workers` root boundary
- remove them in a follow-up cleanup once consumers have moved fully to the
  direct subpackage imports and the root package can stay contract-only

No compatibility shim was kept for the factory package moves; imports were
cut over directly to `pkg/factory/requests` and `pkg/factory/events`.

## Mergeability Follow-up

This final iteration also fixed the inherited
`tests/functional/runtime_api` smoke fixture so it no longer declares the same
`agent-slot` requirement on both the worker and the workstation. The runtime
adds workstation resources to execution gating directly, so the duplicate
fixture requirements prevented the scripted execution step from enabling.

## Final Validation

Run from the repository root:

```text
go test ./...
go vet ./...
make build
go run ./cmd/pkgmaintcheck ./pkg
```
