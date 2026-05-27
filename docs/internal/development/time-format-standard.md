# Time Format Standard

This document records the observable time contract for Agent Factory runtime,
API, CLI, and website surfaces. It is maintainer guidance for new features that
publish timestamps, elapsed durations, or unavailable time values.

## Contract

Machine-readable API responses, event-stream payloads, replay fixtures, and
generated contract models expose timestamps as RFC3339/RFC3339Nano-compatible
strings with explicit timezone information. Prefer UTC at runtime emission
boundaries. Offset timestamps are acceptable only when preserving an incoming
diagnostic value is the contract being modeled.

Machine-readable elapsed durations use numeric milliseconds on public API and
event contracts. Keep internal Go state as `time.Duration` where that is the
natural domain representation, then convert to `durationMillis` at the API or
event boundary. Do not add public `durationNanos`, duration strings, or
feature-specific elapsed-time units for covered runtime surfaces.

Human-readable CLI timestamps render as explicit UTC wall-clock values through
`pkg/cli/timedisplay`. CLI elapsed durations use compact labels from the same
package, such as `500ms`, `5s`, `1m30s`, and `2h15m`. Missing CLI time values
render as `n/a`; they must not expose Go zero-time output.

Human-readable website timestamps and durations render through
`ui/src/components/ui/formatters.ts` and the lower-level locale helpers in
`ui/src/i18n/formatters.ts`. Operational wall-clock displays should include
concise local timezone context through `LocalizedTimezoneNote` when users need
to compare displayed local times with API values. Keep raw ISO disclosure only
where it helps diagnostics, and only for valid timestamps.

Missing, zero, malformed, or not-yet-known values must render as explicit
fallbacks or be omitted according to the owning contract. Customer-visible
surfaces must not display `0001-01-01...`, `Invalid Date`, parser errors,
`NaNms`, or raw invalid timestamp text.

## Examples

Accepted machine-readable timestamps:

```json
"2026-04-21T12:00:04Z"
"2026-04-21T12:00:04.750Z"
"2026-04-21T19:00:04+07:00"
```

Accepted machine-readable duration:

```json
{
  "durationMillis": 875
}
```

Accepted CLI display values:

```text
2026-04-03 11:59:15 UTC
500ms
1m30s
n/a
```

Accepted website display values depend on locale and browser timezone. For an
English browser in UTC, representative values include:

```text
Apr 10, 2026, 6:16 PM
3m 12s
3 minutes, 12 seconds
Unavailable
Timezone: UTC
```

## Shared Entry Points

Use these owners instead of reformatting timestamps or durations inline:

| Surface | Owner |
| --- | --- |
| API schemas and generated contracts | `api/components/schemas/**`, `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, `ui/src/api/generated/openapi.ts` |
| API workstation-request projections | `pkg/api/workstation_request_projection.go` |
| Runtime event emission | `interfaces.CanonicalEventTime(...)`, `pkg/factory/events`, provider/script/model event emitters |
| CLI display | `pkg/cli/timedisplay` |
| Website display | `ui/src/components/ui/formatters.ts`, `ui/src/i18n/formatters.ts`, `LocalizedTimezoneNote` |
| Cross-surface correlation proof | `pkg/api/boundarytests` and focused current-selection UI tests |

## Verification

Use behavior-level tests for the boundary being changed:

- API and event contract changes: run `make api-smoke` and focused
  `go test ./pkg/api/contracttests ./pkg/api -count=1`.
- Runtime event changes: add package tests at the event emitter boundary and
  assert UTC-normalized `eventTime` JSON plus numeric `durationMillis`.
- CLI display changes: test rendered command output or `pkg/cli/timedisplay`
  helpers, depending on the changed boundary.
- Website display changes: test the feature component or shared formatter with
  locale, invalid input, missing input, duration, and raw ISO disclosure cases.

Regression tests should assert observable behavior. Do not add broad source
inventory checks just to prove time-format usage; use them only when the
scanned source shape itself is the supported contract.
