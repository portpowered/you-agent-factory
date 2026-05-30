# Backend Service Package Extraction Closeout

This closeout records behavior-preservation guardrails and the canonical
regression bundle for extracting factory sessions, local models, and hosted
workers out of `pkg/service`.

## Non-goals Confirmed

- No intentional OpenAPI, generated client, UI route, or CLI semantic changes.
- Script poller and cron watcher behavior remain service-owned for this effort.
- No new exported test-only symbols were added solely to satisfy package-size lint.

## Package Ownership

### `pkg/factorysessions`

- Preserved behavior:
  live session registry, `~default` alias, folder-target discovery,
  validate-only open (discovered targets without starting a runtime),
  `ErrFactorySessionNotFound` on close of unknown sessions, and canceled
  open-session requests not leaving newly registered live sessions.
- Dependency direction:
  `pkg/service` composes `*factorysessions.Registry` and owns runtime
  lifecycle; `pkg/factorysessions` does not import `pkg/service`.

### `pkg/localmodels`

- Preserved behavior:
  model list/detail projection, managed asset pull/cache, managed runtime
  reuse on repeat `InvokeModel`, `ErrModelNotFound`, model-not-available
  outcomes, and the same HTTP model invocation path as factory-level
  `MODEL_INVOKE`.
- Dependency direction:
  `pkg/service` delegates catalog reads and wires invocation through existing
  worker executors with `localmodels.AssetPuller` and `localmodels.Manager`.

### `pkg/hostedworkers`

- Preserved behavior:
  service-mode HOSTED Linear poller startup on poller workstations,
  structured disable logging for unsupported providers and misconfiguration,
  restart/backoff supervision, secret resolution, and normalized work
  submission through the existing submitter seam.
- Dependency direction:
  `pkg/service` stores `hostedworkers.Config` at build time and delegates
  poll cycles; script/cron pollers stay in `pkg/service/poller_watcher.go`.

### `pkg/service/factorysave`

- Preserved behavior:
  session-scoped factory save by mode (`REPLACE_CURRENT`,
  `UPSERT_NAMED_AND_ACTIVATE`), stale-version and not-idle error families,
  structured validation targets, and post-save activation scoped to the
  targeted session root.
- Dependency direction:
  `pkg/service` composes `factorysave.Service` through `wireFactorySaveCollaborator`
  and implements `factorysave.Host` for registry, idle checks, activation, and
  readback; `factorysave` does not import `pkg/api` or `pkg/service`.

### `pkg/service/runtimebuild`

- Preserved behavior:
  one runtime bundle build path for default startup, session open, and
  post-save replacement so work submission and model catalog reads behave the
  same regardless of which flow activated the runtime.
- Dependency direction:
  `pkg/service` injects `buildRuntimeBundle` as the `BundleBuilder` from
  `newRuntimeBuildService`; `runtimebuild` does not import `pkg/api` or
  `pkg/service`.

### `pkg/service` (slim orchestration)

- Preserved behavior:
  factory build/run, runtime replacement, named-factory and editable-definition
  flows, dashboard render seams, and script/cron poller regressions unchanged
  aside from import-path moves.
- Package-size outcome:
  root `pkg/service` remains within the 15-file `pkg-file-count` cap without
  new broad waivers for extracted concerns.

## Canonical Regression Bundle

Run from the repository root after changes touching these packages:

```text
make vet backend-size pkg-maint pkg-file-count
go test ./pkg/factorysessions/... ./pkg/localmodels/... ./pkg/hostedworkers/...
go test ./pkg/service/... ./pkg/api/...
```

For a full maintainer gate aligned with pull-request verification:

```text
make lint
```

## Verification Record (UTC)

Validated on `ralph/backend-service-refactor` after extractions 001–004 (2026-05-30):

- `make vet backend-size pkg-maint pkg-file-count` — passed
- `go test ./pkg/factorysessions/... ./pkg/localmodels/... ./pkg/hostedworkers/...` — passed
- `go test ./pkg/service/... ./pkg/api/...` — passed

Validated on `ralph/service-composition-seams-recovery` after composition seams 001–006 (2026-05-31):

- `make backend-size pkg-maint pkg-file-count` — passed
- `go test ./pkg/service/runtimebuild/... ./pkg/service/... ./pkg/api/...` — passed
- No new `pkg/service` waivers for logic re-inlined after `factorysave` / `runtimebuild` extraction
