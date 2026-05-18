# simplify-cron-watcher-runtime-lookup-width

## Why

The cron watcher startup path in `pkg/service/cron_watcher.go` still accepts a
broader runtime interface than it uses.

Current live evidence on `main`:

- `startCronWatchersForRuntime` accepts `interfaces.RuntimeConfigLookup`
- that function already receives `factoryDir` separately and passes the runtime
  lookup only into cron registration and trigger helpers
- the downstream helpers in the same file only need workstation lookup
  behavior plus the already-derived workflow identity
- `pkg/service/factory.go` already proves the wider path-aware dependency is
  not needed at watcher-start time because it passes `FactoryDir()` and
  `FactoryConfig()` explicitly before handing off the runtime lookup

This keeps an unnecessary interface-width dependency alive in a service path
that should stay simple and local.

## Do

- narrow `startCronWatchersForRuntime` to accept
  `interfaces.RuntimeWorkstationLookup` instead of
  `interfaces.RuntimeConfigLookup`
- keep `factoryDir` as the entrypoint-owned source for workflow identity
- preserve the existing `registerCronJobs`, `triggerCronAtStart`, and
  `submitCronTickForRuntime` behavior
- update service tests or helper typing only where needed to reflect the
  narrower dependency

## Constraints

- do not change cron trigger behavior, retry behavior, tags, or scheduler
  lifetime semantics
- do not widen this into cron schema, API, or runtime-config normalization work
- do not reintroduce path-aware runtime methods below the watcher-start
  boundary once workflow identity is already derived
- keep verification behavioral at the service watcher boundary

## Verification

- run `go test ./pkg/service -run TestFactoryService_StartCronWatchersForRuntime_DisablesInvalidSchedulesWithoutAffectingValidCronJobs -count=1`
- run `go test ./pkg/service -count=1`
