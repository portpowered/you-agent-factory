# Provider Error Corpus Audit

This document records the audited local mapping between upstream Codex error cases and Agent Factory's normalized provider-failure classes. It is for contributors changing `pkg/workers` error normalization, retry behavior, or provider-error tests.

## Audit Source

- Upstream source: [`openai/codex` `codex-rs/protocol/src/error.rs`](https://github.com/openai/codex/blob/ca3246f77a5eca14f3424d786ce8855fe8811dbc/codex-rs/protocol/src/error.rs)
- Audited upstream commit: `ca3246f77a5eca14f3424d786ce8855fe8811dbc`
- Audit date: `2026-04-20`

The upstream Rust enum is not emitted directly inside Agent Factory. The local implementation classifies provider failures from raw CLI stdout and stderr. The mapping below therefore records the supported upstream subset as representative raw failure shapes and the local class they must normalize to.

## Current Local Ownership

| File | Current responsibility |
| --- | --- |
| `pkg/workers/provider_behavior.go` | Owns provider-specific substring classification for Claude and Codex-family raw output. Only `claude` has a dedicated classifier; all other providers currently fall through to the Codex classifier. |
| `pkg/workers/provider_error_corpus.go` | Owns loading and validation of the shared provider-error corpus used by worker and functional tests. |
| `pkg/workers/testdata/provider_error_corpus.json` | Owns the audited shared raw failure corpus for supported Claude, Codex, and Cursor-family provider failures plus expected normalized metadata. |
| `pkg/workers/inference_provider.go` | Owns normalization from subprocess failures into `ProviderError`, including the split between execution failures, exit failures, timeouts, and misconfiguration. |
| `pkg/workers/provider_errors.go` | Owns the local stable runtime classes, their families, and the retry vs terminal vs throttle-pause decision contract. |
| `pkg/workers/provider_errors_test.go` | Covers type-to-family and family-to-runtime-decision behavior from shared corpus entries. |
| `pkg/workers/inference_provider_test.go` | Holds the main unit coverage for shared Codex, Cursor-family, and Claude raw failure samples plus run-error normalization. |
| `tests/functional_test/provider_error_smoke_test.go` | Holds runtime smoke coverage for provider-failure behavior, including retry, throttle pause, failure routing, and shared-corpus dispatch metadata assertions. |

## Current Fixture Inventory

The current implementation loads supported provider failures from the shared corpus in `pkg/workers/testdata/provider_error_corpus.json`:

- `pkg/workers/provider_errors_test.go` iterates shared corpus entries to prove stable type, family, retry, and throttle-pause decisions.
- `pkg/workers/inference_provider_test.go` uses shared corpus entries for supported Codex, Cursor-family, and Claude normalization cases, while keeping intentionally unique run-error and unknown edge cases inline.
- `tests/functional_test/provider_error_smoke_test.go` uses shared corpus entries for supported smoke scenarios and derives bounded `ERROR:` lines from the corpus before layering extra transcript noise for unique edge cases.

Inline raw payloads should remain limited to intentionally unique edge cases that the shared supported corpus does not represent directly.

## Supported Audited Mapping Subset

Agent Factory currently exposes these stable local classes from `pkg/interfaces/provider_failure.go`:

- `internal_server_error`
- `throttled`
- `auth_failure`
- `permanent_bad_request`
- `timeout`
- `unknown`
- `misconfigured`

The supported upstream Codex and Cursor-family subset maps to local classes as follows.

| Upstream case or raw family | Representative upstream message or raw shape | Local type | Local family | Notes |
| --- | --- | --- | --- | --- |
| `CodexErr::InternalServerError` | `We're currently experiencing high demand, which may cause temporary errors.` | `internal_server_error` | `retryable` | Supported by the current Codex-family matcher and shared corpus entries for both Codex and Cursor-family raw failures. |
| Windows subprocess exit without an audited provider signal | `codex exited with code 4294967295` plus the Codex banner or subprocess stderr, but no `authentication_error`, `api key`, `401`, or `403` marker | `internal_server_error` | `retryable` | Treat this as an intermittent Codex process failure. Operator-facing failure metadata and diagnostics must describe the subprocess failure and must not point users toward authentication remediation. |
| `CodexErr::ServerOverloaded` | `Selected model is at capacity. Please try a different model.` | `throttled` | `throttle` | This remains distinct from temporary server errors because runtime behavior must pause only the affected provider/model lane. |
| `CodexErr::UsageLimitReached` | `You've hit your usage limit...` | `throttled` | `throttle` | Local coverage already treats usage-limit text as a throttle case. |
| Upstream auth and unauthorized shapes | `authentication_error`, `api key`, `unauthorized`, `forbidden`, `401`, `403` | `auth_failure` | `terminal` | The local code normalizes these as terminal provider configuration or authentication failures. |
| `CodexErr::InvalidRequest` and equivalent 400 request failures | `invalid_request_error`, `bad request`, `400 item`, `400 previous response`, `400 ` | `permanent_bad_request` | `terminal` | The local classifier excludes timeout-shaped text from this bucket. |
| Upstream timeout and timeout-shaped transport failures | `deadline exceeded`, `timed out`, `timeout` | `timeout` | `retryable` | Local runtime retries these without throttle pause. |
| Unaudited or unsupported raw failures | Anything outside the supported subset above | `unknown` | `terminal` | Unsupported strings must remain `unknown` until explicitly audited and added. |

## Cursor Coverage Note

Agent Factory does not currently define a separate `cursor` provider enum or a dedicated Cursor classifier. The current code path routes every non-Claude provider through `codexProviderBehavior`, so Cursor-family raw failures are presently treated as Codex-family failures by inference from the current implementation. Any future dedicated Cursor provider should keep this audited mapping aligned with the same upstream source unless the provider output format materially differs.

## Runtime Contract

`pkg/workers/provider_errors.go` is the canonical runtime decision layer:

- `internal_server_error` and `timeout` are `retryable`
- `throttled` is `retryable` and triggers throttle pause
- `auth_failure`, `permanent_bad_request`, `unknown`, and `misconfigured` are `terminal`

That runtime contract is the reason the high-demand message and the Windows `4294967295` subprocess exit belong under `internal_server_error` rather than `throttled`, `auth_failure`, or `unknown`: Agent Factory should retry both without entering the throttle-pause lane, and the Windows path should surface process-failure diagnostics instead of authentication guidance.

## Remaining Audit Notes On 2026-04-20

- The current code does not separate a dedicated Cursor provider classifier from the Codex-family fallback path.
- Unsupported or unaudited provider strings still intentionally normalize to `unknown` until they are reviewed against the upstream source and added to the shared corpus plus matcher.

The shipped implementation now covers the audited high-demand mapping, the Windows `4294967295` subprocess mapping, and the shared-corpus regression path. Future work should only add a separate Cursor-specific classifier if provider output diverges enough that the shared Codex-family matcher stops being the correct owner.
