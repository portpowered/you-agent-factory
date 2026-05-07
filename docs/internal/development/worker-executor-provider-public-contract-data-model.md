# Worker Executor Provider Public Contract Data Model

This artifact records the public worker-contract cleanup that separates worker
executor selection from model-provider selection and retires public-only
runtime fields from the shipped Agent Factory worker surface.

## Change

- PRD, design, or issue: `prd.json` for Agent Factory worker executor-provider
  contract cleanup
- Owner: Agent Factory maintainers
- Packages or subsystems: `api/openapi.yaml`, `pkg/interfaces`, `pkg/config`,
  `pkg/replay`, `pkg/cli/init`, `pkg/factory`, docs/examples/tests
- Canonical package responsibility artifact: `api/openapi.yaml` owns the
  public worker schema; `pkg/config` owns compatibility aliases and canonical
  serialization; `pkg/interfaces` may retain runtime-only worker fields that
  are not part of the public config contract.

## Trigger Check

- [x] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API or generated schema
- [x] Package-local model another package must interpret

## Public Boundary Rules

- Public worker config uses `executorProvider` for executor adapter selection.
- Public worker config keeps `modelProvider` for model-routing and provider
  diagnostics.
- Public worker config must not emit `provider`, `sessionId`, or `concurrency`
  in OpenAPI, generated `Factory` payloads, canonical `factory.json`, replay
  artifacts, scaffolded examples, or smoke fixtures.
- Runtime-only fields such as `SessionID` and `Concurrency` may remain on
  `pkg/interfaces.WorkerConfig`, but they must stay boundary-only private via
  explicit mapper code instead of JSON tags that leak back into public output.
- If the boundary temporarily accepts legacy `provider`, canonical
  `executorProvider` wins when both are present and only `executorProvider` may
  be emitted publicly.

## Affected Surfaces

- `libraries/agent-factory/api/openapi.yaml`
- `libraries/agent-factory/pkg/api/generated/server.gen.go`
- `libraries/agent-factory/pkg/interfaces/worker_config.go`
- `libraries/agent-factory/pkg/config/*.go`
- `libraries/agent-factory/pkg/replay/*.go`
- `libraries/agent-factory/pkg/cli/init/init.go`
- `libraries/agent-factory/pkg/api/*contract*test.go`
- `libraries/agent-factory/ui/src/api/generated/openapi.ts`
- `libraries/agent-factory/docs/{authoring-agents-md,authoring-workflows,work,workstations}.md`
- `libraries/agent-factory/factory/workers/*/AGENTS.md`

## Verification

```bash
cd libraries/agent-factory
make generate-go-api
go test ./...
```
