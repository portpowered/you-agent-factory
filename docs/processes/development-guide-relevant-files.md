# Development Guide Relevant Files

This inventory records the checked-in files and directories that the maintainer development guide should describe when it gives repository-root workflow instructions.

| Path | Role | Notes |
| --- | --- | --- |
| `go.mod` | Repository-root marker | Maintainer commands and worktree-aware tests should treat the directory containing `go.mod` as the canonical repository root. |
| `Makefile` | Root command surface | The development guide should describe quality and generation commands as root-level invocations instead of teaching a nested package workflow. |
| `api/` | Authored API contract workspace | OpenAPI validation and bundling start from the repository root, then shell into `api/` only where the documented workflow requires it. |
| `api/components/schemas/shared/` | Shared OpenAPI fragment family | Cross-surface helper schemas such as maps and diagnostics live here and referenced fragments must use file-relative `$ref`s. |
| `api/components/schemas/runtime/` | Runtime OpenAPI fragment family | Request, response, status, and work-submission contract fragments are authored here one schema per file. |
| `api/components/schemas/factory-config/` | Factory-config OpenAPI fragment family | Public named-factory and factory-topology schemas are authored here and should stay split from `api/openapi-main.yaml`. |
| `cmd/factory/` | CLI entrypoint | Root-level build and smoke commands compile or execute the `factory` binary from this source tree. |
| `docs/development/*-closeout.md` | Cleanup verification artifacts | Narrow cleanup lanes record the exact root-level validation bundle here when maintainers need durable proof beyond `progress.txt`. |
| `docs/development/openapi-schema-standardization-inventory.md` | OpenAPI cleanup inventory | Records the authored fragment layout, remaining inline schemas, and the canonical bundle and generation verification surfaces for schema-standardization work. |
| `docs/development/development.md` | Active maintainer guide | Must describe the real repository-root layout used in this checkout and avoid stale `libraries/agent-factory` instructions. |
| `factory/` | Maintainer workflow surface | Contains checked-in operator guidance and active inbox directories that the development guide may reference for workflow-related tasks. |
| `pkg/api/openapi_contract_test.go` | OpenAPI contract guard surface | Focused authored-versus-bundled contract assertions live here, including fragment-layout and `/events` schema wiring checks. |
| `pkg/api/testdata/canonical-event-vocabulary-stream.json` | OpenAPI vocabulary fixture | Canonical bundled-contract fixtures for event payload validation live here and must be updated alongside public schema field renames. |
| `pkg/interfaces/factory_runtime.go` | Handwritten public work-request boundary structs | Watched-file batch ingestion, generated worker output parsing, and fixture helpers still marshal these structs directly, so their JSON tags must stay aligned with the camelCase OpenAPI contract. |
| `pkg/` | Go implementation surface | Package-specific test commands in the guide should reference the real package paths under this root. |
| `tests/` | Smoke and fixture surface | Functional and release-facing checks run from the repository root against these checked-in fixtures. |
| `ui/` | Embedded dashboard workspace | UI build, test, and Storybook commands remain part of the same repository-root workflow. |

## Reusable Rules

- When maintainer docs describe command execution, anchor the instructions to the repository root that contains `go.mod` and `Makefile`.
- If a workflow temporarily changes directories, state that it starts from the repository root and why the subdirectory hop is required.
- When a cleanup lane closes with path or contract-alignment work, record the exact root-level verification commands in a `docs/development/*-closeout.md` artifact so the proof survives beyond `progress.txt`.
- Use `pkg/api/openapi_contract_test.go` for narrow OpenAPI contract guards when the work is about authored schema structure or bundled route/schema alignment rather than handler runtime behavior.
- When public OpenAPI field names change, update `pkg/api/testdata/canonical-event-vocabulary-stream.json` together with the contract guards so fixture validation keeps exercising the current bundled vocabulary.
- When public request-batch field names change, update `pkg/interfaces/factory_runtime.go`, `pkg/factory/work_request_json.go`, and watched-file/worker batch fixtures together; those handwritten JSON boundaries are not generated and will drift silently if only `api/openapi.yaml` and generated clients are regenerated.
- For Redocly-bundled vendor extensions that must keep a schema pointer in `api/openapi.yaml`, store the extension value as a string JSON Pointer; nested `$ref` objects under `x-*` fields are inlined during bundling.
