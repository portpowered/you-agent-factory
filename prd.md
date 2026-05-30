# PRD: CLI Batch Work Submission (`you submit batch`)

## Context

### Customer ask

Implement CLI batch work submission so operators and agents can submit a
`FACTORY_REQUEST_BATCH` to a **running** factory via file path, stdin, or inline
JSON—without hand-written `curl` boilerplate.

### Problem

Today, batch ingress to a live factory is only practical through:

- `curl -X PUT …/factory-sessions/{session}/work-requests/{request_id}` with a
  JSON body, or
- Dropping files under `factory/inputs/BATCH/` for the watcher.

Unary `you submit` handles one work item. `you run --work` submits a batch only
at factory startup. There is no first-class CLI that mirrors the documented HTTP
upsert path for an already-running session.

### Solution

Add `you submit batch` under `you submit`. It reads the same canonical
`FACTORY_REQUEST_BATCH` JSON as watched inputs and `you run --work`, validates it
locally, and upserts via `PUT /factory-sessions/{session}/work-requests/{requestId}`.
Support file path, piped stdin, explicit `-`, optional `--file`, inline JSON, and
`--dry-run` for validate-only runs. Success output (human and `--json`) aligns
with the unary submit response contract, including per-work identifiers when the
API returns them.

## Goals

- Operators discover batch submit next to unary `you submit`.
- Scripts and agents submit multi-work batches to a running factory in one command.
- All ingress modes (file, pipe, inline) produce the same validated HTTP body.
- Invalid batch JSON fails locally before any network call.
- `--dry-run` confirms shape and summarizes work without contacting the server.
- Packaged and reference docs describe CLI batch submit alongside `curl` and
  watched-folder ingress.

## Project-level acceptance criteria

- [ ] `you submit batch` is registered under `you submit` (not a separate top-level verb).
- [ ] Running `you submit batch` with valid `FACTORY_REQUEST_BATCH` JSON results in
  HTTP `201` and accepted work on a reachable factory (default session `~default`
  when `--session` is omitted).
- [ ] Batch JSON is accepted from: filesystem path (positional or `--file`), piped
  stdin or positional `-`, and inline `{…}` positional when the argument is JSON.
- [ ] `--dry-run` validates input, prints a summary, performs no HTTP, and exits `0`
  on valid input even when the factory is unreachable.
- [ ] Human stdout and `--json` on success include `requestId`, `traceId`, work
  count, and per-work `name`, `workTypeName`, and `workId` when the API provides them.
- [ ] Reference and packaged docs (`you docs batch-inputs`) include CLI examples
  for file, pipe, inline, and dry-run alongside existing `curl` guidance.
- [ ] Typecheck, lint, and project tests pass.

## User stories

### cli-submit-batch-001: Shared canonical batch loader (file path)

**Description:** As a maintainer, I want one canonical batch JSON loader used by
`you run --work` and `you submit batch` so parsing rules never diverge.

**Acceptance criteria:**

- [ ] Reading batch JSON from an existing filesystem path returns a validated
  `FACTORY_REQUEST_BATCH` work request (same semantics as today’s `you run --work`
  file load).
- [ ] Retired field aliases and conflicting trace fields are rejected with the same
  error guidance as today’s run loader tests.
- [ ] `you run --work` behavior is unchanged for file-based batches (regression tests pass).
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-002: Upsert API returns per-work identifiers

**Description:** As an agent, I want the batch upsert response to include work
identifiers so I can verify submission without listing all work.

**Acceptance criteria:**

- [ ] Successful `PUT /work-requests/{request_id}` (session-scoped variant included)
  returns `201` with `requestId`, `traceId`, and a `works` array where each item
  includes `name`, `workTypeName`, and `workId`.
- [ ] Multi-work batch upsert populates `works` for every accepted item in API tests.
- [ ] OpenAPI schema and generated types reflect optional `works` on upsert response.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-003: Command discovery and help

**Description:** As an operator, I want to discover batch submit next to unary
submit so I know which command to use for multi-work ingress.

**Acceptance criteria:**

- [ ] `you submit batch --help` documents batch input modes (positional path,
  optional `--file`, `-`/stdin, pipe-with-no-args, inline JSON), `--dry-run`,
  `--session`, and global `--server` / `--json` / `--verbose`.
- [ ] Help states the command expects `FACTORY_REQUEST_BATCH` and points to
  `you docs batch-inputs`.
- [ ] Help does not advertise unary-only flags (`--name`, `--work-type-name`,
  `--payload`, `--work-type-id`).
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-004: Submit batch to running factory (HTTP + dry-run)

**Description:** As an operator, I want to upsert a canonical batch to a running
factory session, or validate locally without sending traffic.

**Acceptance criteria:**

- [ ] With valid batch JSON from a file, the CLI issues `PUT` to
  `/factory-sessions/{session}/work-requests/{requestId}` where `requestId` in
  the path matches the body; `Content-Type` is `application/json`.
- [ ] Body `type` must be `FACTORY_REQUEST_BATCH` with at least one `works` entry;
  violations fail locally with a clear message before HTTP.
- [ ] HTTP `201` is treated as success; other statuses surface API error message when
  present; unreachable factory errors match unary submit transport style.
- [ ] `--session` scopes the request like unary submit.
- [ ] `--dry-run` parses and validates only, prints summary including `requestId`,
  work count, work names, `relationCount`, `batchSource`, and
  `dry-run: no request sent`; performs zero HTTP calls; exits `0` on valid input
  even when the server is down.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-005: Piped stdin and explicit `-`

**Description:** As an agent, I want to pipe batch JSON so I can submit without a
temp file.

**Acceptance criteria:**

- [ ] `cat batch.json | you submit batch` submits when stdin is not a TTY.
- [ ] `you submit batch -` reads batch JSON from stdin.
- [ ] `you submit batch` with no args and interactive TTY stdin fails immediately
  with usage guidance (does not hang waiting for input).
- [ ] Empty piped stdin fails with a clear empty-input error.
- [ ] When a file path or `--file` is provided, stdin is ignored.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-006: Inline JSON positional

**Description:** As a script author, I want to pass a small batch document as one
positional argument.

**Acceptance criteria:**

- [ ] Positional whose first non-whitespace byte is `{` is parsed as inline JSON,
  not as a filesystem path.
- [ ] A non-existent path that does not look like JSON errors as missing file/JSON,
  not as JSON parse of the path string.
- [ ] Inline JSON uses the same canonical validation as file and stdin input.
- [ ] Help notes shell length limits; large batches should use file or pipe.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-007: Optional `--file` flag

**Description:** As a script author, I want an explicit file flag when positional
arguments are awkward.

**Acceptance criteria:**

- [ ] `--file <path>` reads batch JSON; `--file -` reads stdin.
- [ ] When both `--file` and a positional path are set, `--file` wins (documented in help).
- [ ] Positional path remains the primary documented form.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-008: Human success output

**Description:** As an operator, I want confirmation that lists what was submitted
and what to run next.

**Acceptance criteria:**

- [ ] On `201`, stdout includes `requestId`, `traceId`, work count, and each accepted
  work’s `name` and `workTypeName`.
- [ ] When the API returns `workId`, each work line includes it and a hint
  `you work show <work-id>`; otherwise hints use `you work list --name <name>`.
- [ ] Long name lists truncate (at most ten lines); `relationCount` shown when
  relations are non-empty.
- [ ] Full batch JSON and per-work payloads are not printed on stdout.
- [ ] `--verbose` logs endpoint, `batchSource` (`file`, `stdin`, `inline`), byte size,
  `requestId`, and work count on stderr—never payload content.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-009: JSON success output

**Description:** As a script, I want machine-readable batch submit confirmation.

**Acceptance criteria:**

- [ ] Global `--json` emits one object with at minimum: `requestId`, `traceId`,
  `workCount`, `relationCount`, `sessionId`, `endpointPath`, `batchSource`, and
  `works` (each with `name`, `workTypeName`, `workId` when returned).
- [ ] `--json` with `--dry-run` emits `dryRun: true` and summary fields without
  `traceId` unless present in input.
- [ ] Exit code `0` on success; non-zero on validation or HTTP errors.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-010: Error surfaces

**Description:** As an agent, I want validation failures before HTTP and API
failures after HTTP to be distinguishable.

**Acceptance criteria:**

- [x] Canonical validation errors include retired-field guidance where applicable.
- [x] Missing or empty `requestId`, empty `works`, and invalid JSON fail locally.
- [x] HTTP `400`/`404` (and `409` if applicable) print status and bounded API message;
  no success JSON on failure.
- [x] Tests cover invalid JSON, empty works, mocked `400`, and mocked `404`.
- [x] Typecheck passes.
- [x] Tests pass.

### cli-submit-batch-011: Reference and packaged documentation

**Description:** As a new contributor, I want docs to show CLI batch submit
alongside curl and watched-folder ingress.

**Acceptance criteria:**

- [ ] `docs/reference/batch-inputs.md` adds a CLI subsection with examples for
  file, `--file`, pipe, inline JSON, and `--dry-run`; keeps existing `curl` example.
- [ ] Ingress comparison covers: `you submit` (single), `you submit batch` (running
  factory), `you run --work` (startup), watched `factory/inputs/BATCH/`.
- [ ] Packaged `you docs batch-inputs` content matches reference updates.
- [ ] Doc tests guard `you submit batch` and `FACTORY_REQUEST_BATCH` markers where
  other CLI examples are guarded.
- [ ] Typecheck passes.
- [ ] Tests pass.

### cli-submit-batch-012: End-to-end smoke (optional)

**Description:** As a maintainer, I want one functional smoke proving batch CLI
reaches a running factory when the harness supports it.

**Acceptance criteria:**

- [x] If the existing smoke harness can start a factory and accept work-request
  upserts, one smoke runs `you submit batch` with a minimal checked-in batch file
  and asserts success markers in output.
- [x] If harness cost is prohibitive, implementation notes document deferral and
  httptest coverage from earlier stories is cited as the verification substitute.
- [x] Typecheck passes.
- [x] Tests pass (or smoke story cancelled with documented justification).

## Functional requirements

- FR-1: Register `you submit batch` under `you submit`.
- FR-2: Input precedence: `--file` (including `-`) → positional `-` → existing file
  path → inline `{…}` → piped stdin when no positional/`--file` → usage error on TTY
  with no input.
- FR-3: Validate with canonical batch parser before HTTP; `--dry-run` skips HTTP.
- FR-4: Upsert via `PUT` to session-scoped `/work-requests/{requestId}` only (not
  `POST /work`).
- FR-5: Preserve unary `you submit` behavior and flag surface on the parent command.
- FR-6: Success output field names align with unary submit response contract
  (`workId`, `workTypeName`, `name`, `traceId`, `sessionId`, `endpointPath`).
- FR-7: Reuse existing CLI HTTP, session path, and diagnostic patterns from unary submit.

## Non-goals

- Extending unary `you submit` with batch flags or multiple payloads.
- Replacing `you run --work` or watched-folder ingestion.
- Staging multimodal files from the CLI in v1.
- Top-level `you batch submit` verb.
- Pipe or inline input for unary `you submit` in this feature.

## High-level technical design

1. **Shared loader** — Extract file-path batch loading from the run command into a
   shared CLI package; run delegates without behavior change. Extend with stdin,
   inline JSON, and `--file` resolution for batch submit only.
2. **Command** — New batch subcommand on submit with config mirroring unary HTTP
   fields (`Server`, `SessionID`, `JSON`, diagnostics). Wire test injection hook
   like unary submit.
3. **API** — Extend `UpsertWorkRequestResponse` with `works[]` populated from
   accepted batch items; regenerate OpenAPI types before CLI success output stories.
4. **Output** — Human and JSON formatters share identifier vocabulary with unary
   submit; dry-run uses a distinct JSON shape with `dryRun: true`.
5. **Docs** — Update reference and embedded packaged topic together; extend doc
   tests and optional smoke.

**Dependencies:** Coordinate field naming with CLI submit response contract PRD;
post-submit inspection (`you work show` / `you work list`) is the documented verify loop.

## Supporting considerations

- **Idempotency:** `requestId` is the stable upsert key; re-submit behavior follows
  server rules—no client-side dedupe beyond the document’s id.
- **Diagnostics:** No payload bodies, tokens, or prompts in verbose stderr lines.
- **Security:** Same trust model as unary submit (local factory URL, no new auth).

## Success metrics

- An agent submits a multi-work batch to a running factory in one command without `curl`.
- Pipe and file paths produce identical HTTP bodies for the same JSON document.
- Invalid batch JSON fails locally with zero network calls in automated tests.
- `you docs batch-inputs` examples match implemented CLI behavior.

## Decisions (resolved)

| ID | Decision |
|----|----------|
| D-1 | `--file` is optional; positional path is primary; `--file` wins when both set. |
| D-2 | Ship API `works[]` on upsert response together with CLI success output when possible. |
| D-3 | `--dry-run` is in v1: validate locally, summarize, no HTTP, exit `0` on valid input. |
