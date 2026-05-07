# Functional Test Meta Audit

This audit records the `tests/functional_test` cases that currently validate repository structure, packaged asset internals, docs topology, or command-registration inventory instead of observable runtime behavior.

## Classification

| Test surface | Classification | Why |
| --- | --- | --- |
| `tests/functional_test/functional_harness_guardrail_test.go` | `delete` | Parses test source with `go/ast` to enforce a local harness-construction policy. That is a suite-maintenance guardrail, not user-visible behavior. |
| `tests/functional_test/reference_docs_surface_test.go` | `delete` | Walks `docs/reference` and asserts markdown link topology and exact topic-file presence. This validates docs layout, not runtime behavior. |
| `tests/functional_test/cli_docs_smoke_test.go` | `rewrite as behavioral` | The `agent-factory docs` command is public, but the current test compares output byte-for-byte with files under `pkg/cli/docs/reference`. That couples the functional suite to packaged reference-file structure instead of checking user-visible command behavior. |
| `tests/functional_test/cleanup_smoke_test.go` | `rewrite as behavioral` | The leading request/status/event assertions are behavioral, but the removed-route inventory checks, dashboard bundle string scans, and CLI registration-list assertions inspect implementation surfaces rather than the supported behavior. |
| `tests/functional_test/generated_api_smoke_test.go` | `rewrite as behavioral` | The generated API flow is real runtime coverage, but `assertGeneratedDashboardRoutesRemoved` is a removed-route inventory check and should be replaced with a narrower supported-surface assertion. |
| `tests/functional_test/legacy_unary_retirement_smoke_test.go` | `retain` | Exercises live submission paths through HTTP, watched files, startup work-file loading, replay, and cron submission. It validates runtime ingestion behavior rather than source or asset structure. |
| `tests/functional_test/project_agnostic_cleanup_smoke_test.go` | `retain` | Verifies emitted runtime requests, events, and public-facing serialized values do not leak retired product naming. The assertions target observable outputs rather than repository layout. |
| `tests/functional_test/worker_public_contract_smoke_test.go` | `retain` | Confirms the public worker contract exposed through flattened config, generated OpenAPI types, replay artifacts, and provider execution requests. This is a public contract test, not a source-structure test. |

## Notes For Follow-Up

- Remove or move `functional_harness_guardrail_test.go` out of the functional suite entirely; it is the clearest pure meta test in scope.
- Delete `reference_docs_surface_test.go`; docs topology belongs in docs-specific checks, not the functional runtime suite.
- Keep the behavioral setup portions of `cleanup_smoke_test.go` and `generated_api_smoke_test.go`, but replace route-inventory, bundle-string, and command-registration assertions with checks against supported requests and responses.
- If `cli_docs_smoke_test.go` stays in the functional suite, it should assert stable user-facing docs command behavior such as topic availability and core content markers, not exact parity with the packaged markdown files on disk.
