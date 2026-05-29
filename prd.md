# PRD: OpenCode Agent Support

## Introduction

infinite-you already routes OpenCode-backed model work through the built-in `opencode` runner (`opencode run` with `--model`, `--session`, `--dir`, and optional `--dangerously-skip-permissions`). The [OpenCode CLI](https://opencode.ai/docs/cli/) also supports selecting a named OpenCode agent profile via `opencode run --agent <name>` (and the same flag on the TUI entrypoint). Factories cannot express that choice today, so every OpenCode dispatch uses the CLI default agent even when authors maintain custom agents under `.opencode/agent` or global agent directories.

This project adds first-class OpenCode agent selection to factory configuration and backend dispatch, wires it through the existing provider execution path, records it in inference diagnostics, and proves the behavior with targeted backend tests.

## Context

### Customer ask

Add agent support for OpenCode using the OpenCode CLI documentation, including corresponding backend tests.

### Problem

- `pkg/workers/provider` builds `opencode run` without `--agent`, so custom OpenCode agent profiles never participate in factory execution.
- Worker and workstation `AGENTS.md` frontmatter, OpenAPI factory schemas, and `ProviderInferenceRequest` have no field for an OpenCode agent name.
- Operators cannot tell from inference diagnostics which OpenCode agent profile was requested for a dispatch.
- Existing OpenCode tests cover model, session, directory, and permission flags only.

### Solution

Introduce an `openCodeAgent` configuration field (worker-owned default with optional workstation override), map it into `ProviderInferenceRequest` when the resolved runner is `opencode`, append `--agent <name>` to `opencode run` invocations per the CLI contract, surface the resolved agent in provider request metadata, extend factory/OpenAPI shapes where worker and workstation settings are serialized, document the field for factory authors, and add backend tests that lock command construction and dispatch wiring.

## Goals

- Let factory authors pin a named OpenCode agent profile on OpenCode-backed workers and workstations.
- Pass the resolved agent to `opencode run --agent` during model dispatch.
- Ignore or reject misconfiguration when `openCodeAgent` is set but the resolved runner is not OpenCode, with a clear operator-visible outcome.
- Record the requested OpenCode agent in inference request metadata for debugging.
- Prove behavior with backend unit and dispatch-level tests (no UI-only verification stories).

## Project-Level Acceptance Criteria

- [ ] A worker or workstation with `openCodeAgent: "<name>"` and resolved runner `opencode` dispatches successfully and invokes `opencode run` with `--agent <name>` before the user prompt argument.
- [ ] When `openCodeAgent` is omitted, OpenCode dispatch behavior is unchanged (no `--agent` flag).
- [ ] When `openCodeAgent` is set but the resolved runner is not `opencode`, dispatch fails before subprocess start with a field-scoped error that names the configured agent and the resolved runner.
- [ ] Inference request diagnostics include the resolved OpenCode agent name when one was configured.
- [ ] Factory/OpenAPI worker and workstation shapes accept `openCodeAgent` and round-trip through config load and public projection where those entities are already serialized.
- [ ] Reference documentation describes `openCodeAgent`, runner precedence, and the link to OpenCode's `opencode agent list` workflow.
- [ ] Repository quality gate passes: backend and UI typecheck, lint, and affected test suites are green.

## User Stories

### opencode-agent-support-001: Author OpenCode agent names in factory config

**Description:** As a factory author, I want to declare which OpenCode agent profile a worker or workstation should use so I can reuse agents I created with `opencode agent create` without duplicating prompts in infinite-you.

**Acceptance Criteria:**
- [ ] Worker `AGENTS.md` frontmatter accepts optional `openCodeAgent` (non-empty string).
- [ ] Workstation `AGENTS.md` frontmatter accepts optional `openCodeAgent` that overrides the worker default for that step when both are set.
- [ ] Loaded runtime worker and workstation config retain the resolved `openCodeAgent` value through factory build.
- [ ] Typecheck passes
- [ ] Tests pass

### opencode-agent-support-002: Reject OpenCode agent config on non-OpenCode runners

**Description:** As an operator, I want a clear configuration error when `openCodeAgent` is set but the dispatch will not use the OpenCode runner, so misconfigured factories fail fast instead of silently ignoring the field.

**Acceptance Criteria:**
- [ ] When `openCodeAgent` is non-empty and the resolved runner for a model dispatch is not `opencode`, the factory reports a validation or dispatch-time error that references `openCodeAgent` and the resolved runner ID.
- [ ] When `openCodeAgent` is empty or unset, non-OpenCode runners behave exactly as before.
- [ ] Typecheck passes
- [ ] Tests pass

### opencode-agent-support-003: Forward OpenCode agent into provider inference requests

**Description:** As a backend maintainer, I want the resolved OpenCode agent name on `ProviderInferenceRequest` so provider behavior can build CLI arguments without re-reading frontmatter.

**Acceptance Criteria:**
- [x] `ProviderInferenceRequest` carries an `OpenCodeAgent` field populated from workstation override, else worker default, when the resolved runner is `opencode`.
- [x] `AgentExecutor` (or the shared inference-request builder) sets `OpenCodeAgent` only for OpenCode dispatches; the field stays empty for other runners.
- [x] Cloning and diagnostic helpers preserve `OpenCodeAgent` without mutating shared maps.
- [x] Typecheck passes
- [x] Tests pass

### opencode-agent-support-004: Pass `--agent` to `opencode run`

**Description:** As a factory operator, I want OpenCode dispatches to invoke my configured agent profile so tool permissions and system prompts defined in OpenCode take effect.

**Acceptance Criteria:**
- [ ] For `ModelProvider` `opencode` with non-empty `OpenCodeAgent`, built CLI args are `opencode run --agent <name> ...` with `--model`, `--session`, `--dir`, and `--dangerously-skip-permissions` unchanged relative to today's ordering when those fields are set.
- [ ] For empty `OpenCodeAgent`, built args match today's `opencode run` shape (no `--agent`).
- [ ] Existing OpenCode capability guards (unsupported structured output, image input, worktree) still apply unchanged.
- [ ] Typecheck passes
- [ ] Tests pass

### opencode-agent-support-005: Surface OpenCode agent in inference diagnostics

**Description:** As an operator reviewing inference events, I want to see which OpenCode agent was requested so I can correlate factory behavior with OpenCode session logs.

**Acceptance Criteria:**
- [ ] Provider request metadata for OpenCode dispatches includes `opencode_agent` with the resolved agent name when configured.
- [ ] OpenCode dispatches without a configured agent omit `opencode_agent` from request metadata.
- [ ] Typecheck passes
- [ ] Tests pass

### opencode-agent-support-006: Extend factory OpenAPI shapes for `openCodeAgent`

**Description:** As an API consumer editing factory definitions through the API or UI, I want `openCodeAgent` on worker and workstation schemas so saved factories preserve the setting.

**Acceptance Criteria:**
- [ ] OpenAPI `Worker` and `Workstation` (or equivalent factory-definition components) include optional `openCodeAgent` string fields with descriptions referencing OpenCode CLI agent selection.
- [ ] Regenerated Go and TypeScript clients expose the new fields; factory-definition validation accepts non-empty strings and rejects invalid types.
- [ ] A contract or config round-trip test proves `openCodeAgent` survives OpenAPI decode and public projection for at least one worker and one workstation fixture.
- [ ] Typecheck passes
- [ ] Tests pass

### opencode-agent-support-007: Document OpenCode agent configuration for factory authors

**Description:** As a factory author reading reference docs, I want clear guidance on `openCodeAgent` and how it maps to the OpenCode CLI so I can configure agents without reading backend source.

**Acceptance Criteria:**
- [ ] `docs/reference/workers.md` documents `openCodeAgent` on model workers used with runner or `modelProvider` OpenCode.
- [ ] `docs/reference/workstations.md` documents workstation-level override and precedence over the worker default.
- [ ] Docs mention that agent names correspond to OpenCode agents discoverable via `opencode agent list` per https://opencode.ai/docs/cli/.
- [ ] Typecheck passes

## Functional Requirements

- FR-1: Factory config **MUST** accept optional `openCodeAgent` on worker and workstation definitions.
- FR-2: Workstation `openCodeAgent` **MUST** override worker `openCodeAgent` when both are set for the same dispatch.
- FR-3: When the resolved runner is `opencode` and `openCodeAgent` is non-empty, subprocess invocation **MUST** include `opencode run --agent <openCodeAgent>`.
- FR-4: When `openCodeAgent` is set and the resolved runner is not `opencode`, dispatch **MUST** fail with an explicit configuration error before starting the provider subprocess.
- FR-5: Inference diagnostics **MUST** record the resolved OpenCode agent under request metadata key `opencode_agent` when configured.
- FR-6: OpenAPI factory worker and workstation schemas **MUST** include `openCodeAgent` when those entities are already part of the public contract.

## Non-Goals

- Managing OpenCode agents from infinite-you (`opencode agent create`, `list`, or interactive setup).
- UI pickers or live discovery of OpenCode agents in the dashboard (documentation points authors to `opencode agent list`).
- Supporting OpenCode `--agent` on non-`run` entrypoints such as `opencode serve`, `web`, or `acp`.
- Adding new OpenCode optional capabilities (structured output, image input, worktree) in this lane.
- Changing runner prerequisite checks beyond ensuring the existing `opencode` binary lookup still applies.

## High-Level Technical Design

1. **Configuration surface:** Add `OpenCodeAgent string` to `interfaces.WorkerConfig` and workstation runtime config (frontmatter key `openCodeAgent`). Parse via existing `agents_config.go` paths with camelCase canonical naming.
2. **Precedence:** Resolve agent name as `workstation.openCodeAgent` if non-empty, else `worker.openCodeAgent`.
3. **Validation:** During factory build or dispatch preparation, if resolved agent is non-empty and `ResolveRunnerSelection` does not yield `opencode`, return a configuration error.
4. **Inference contract:** Extend `interfaces.ProviderInferenceRequest` with `OpenCodeAgent string`; populate in `inferenceRequestForExecutionRequest` when the effective model provider is `opencode`.
5. **CLI mapping:** In `openCodeProviderBehavior.BuildArgs`, after `run` and before model/session flags, append `--agent`, `<name>` when `req.OpenCodeAgent != ""`, matching OpenCode CLI flag order from https://opencode.ai/docs/cli/ (`--model`, `--agent`, `--session`, etc. are all documented on `opencode run`; place `--agent` after `--model` and before `--session` for stable tests).
6. **Diagnostics:** Extend `workDiagnosticsForInferenceRequest` request metadata with `opencode_agent` when non-empty.
7. **Contracts:** Add `openCodeAgent` to `api/components/schemas/data-models/Worker.yaml` and the workstation schema, regenerate clients, align `ui/src/api/factory-definition` validation if present.
8. **Tests:** Extend `provider_behavior_test.go` and `inference_provider_test.go` table cases; add config/dispatch test showing rejection when `openCodeAgent` is set with `runner: codex`.

## Supporting Technical and UX Considerations

- OpenCode agent names are opaque strings managed outside infinite-you; do not validate against a local registry in v1.
- Keep `openCodeAgent` separate from infinite-you worker `name` and from `model`/`modelProvider` fields to avoid conflating factory topology with OpenCode's agent subsystem.
- `skipPermissions` continues to map to `--dangerously-skip-permissions` independently of agent selection.
- Session resume (`--session`) and working directory (`--dir`) behavior remains unchanged when an agent is configured.

## Success Metrics

- A factory can run two workstations with different `openCodeAgent` values against the same OpenCode runner without prompt duplication.
- Inference event streams show `opencode_agent` metadata for configured dispatches.
- No regression in existing OpenCode provider tests when `openCodeAgent` is unset.

## Open Questions

None for v1; agent name validation against `opencode agent list` output is intentionally deferred.
