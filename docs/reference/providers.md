---
author: Agent Factory Team
last-modified: 2026-08-09
doc-id: agent-factory/guides/providers
---

# Providers, worker models, and ACP agents

Choose the worker's provider and model from the input the worker must perceive
and the kind of work it must do. The `modelProvider` selects the adapter and
`model` selects the model passed to that adapter. The tier descriptions below
are operator guidance for the configured models, not guarantees about latency,
quality, pricing, or availability in every provider account.

## Choose by input first

| Need | Prefer | Decision consequence |
|------|--------|----------------------|
| Text, code, repository changes, or image-aware review | A configured Codex GPT-5.6 tier | The bundled Codex route accepts prompt and image input, but GPT-5.6 does not provide video or audio understanding. Do not assign audiovisual judging to it. |
| Text and code through the bundled Claude CLI | `CLAUDE` with `claude-sonnet-5` when that model is available to the installed CLI/account | The bundled Claude provider does not declare image input. Treat it as a text/code route unless your separately configured ACP integration documents different capabilities. |
| A video or audio file that the worker must inspect | `ANTIGRAVITY` with an AGY model | Recorded AGY CLI runs inspected referenced media through the workspace file-parsing path, including both visual content and an audio track. This is version-sensitive evidence, not a universal guarantee for every AGY release. |
| More than five reference images for one ImageGen call | Split the work or redesign the prompt | `referenced_image_paths` accepts zero through five paths per call. Retrying the same call with six or more paths does not make it valid. |

GPT-5.6's lack of video/audio understanding is a hard selection constraint for
the configured Codex tiers. AGY's audiovisual behavior is a recorded CLI
capability: the adapter's formal `imageInput` capability is still false, while
the AGY agent can inspect files made available in its workspace. Those are
different input paths; do not treat AGY as accepting provider-native image
tokens.

## Current configured model guidance

Use these as a practical starting point when the models are available. The
intended roles are routing guidance observed in this repository's Factory
configuration and validation runs; they are not a claim that one tier always
outperforms another.

| Provider and model | Good starting point | Effort guidance |
|--------------------|---------------------|-----------------|
| `CODEX` / `gpt-5.6-luna` | Difficult implementation, deep review, or work where correctness is more important than throughput. The checked-in Factory uses this tier for its processor at `max`. | Codex accepts the provider-neutral effort vocabulary and forwards it as its native reasoning setting. Use the value your selected model/account supports. |
| `CODEX` / `gpt-5.6-sol` | Planning, ideation, and ordinary analysis. The checked-in Factory uses this tier for planner and ideafier workers at `medium`. | `medium` is the observed Factory choice; it is not a hard requirement for the model. |
| `CODEX` / `gpt-5.6-terra` | Balanced implementation and verification when a general GPT-5.6 tier is preferable. | Choose an effort supported by the selected model/account; do not infer a media capability from the tier name. |
| `CLAUDE` / `claude-sonnet-5` | General text/code work when the Claude CLI exposes this model. | The current Claude adapter rejects `minimal`; its other canonical effort values are forwarded to Claude's `--effort` option, subject to the installed CLI/model. |

The provider-neutral worker contract recognizes `minimal`, `low`, `medium`,
`high`, `xhigh`, and `max`. That vocabulary is not portable across providers:
Claude rejects `minimal`, and AGY has a smaller allowlist. Keep the effort
choice with the provider/model decision rather than assuming that a value
accepted by Codex is accepted everywhere.

## ANTIGRAVITY / AGY model and effort allowlist

The following AGY values are the current adapter allowlist and were also
observed in recorded AGY CLI 1.1.11 runs on 2026-08-08. This is intentionally
versioned guidance: check the installed AGY release after an upgrade.

Supported model IDs:

- `gemini-3.6-flash-high`
- `gemini-3.6-flash-medium`
- `gemini-3.6-flash-low`
- `gemini-3.5-flash-high`
- `gemini-3.5-flash-medium`
- `gemini-3.5-flash-low`
- `gemini-3.1-pro-high`
- `gemini-3.1-pro-low`
- `claude-sonnet-4-6`
- `claude-opus-4-6-thinking`
- `gpt-oss-120b-medium`

The separate AGY effort value may be omitted or set to `low`, `medium`, or
`high`. Do not invent `minimal`, `xhigh`, or `max` for AGY. Some AGY model IDs
already carry a `-low`, `-medium`, or `-high` selection; that suffix is part of
the model ID, while `reasoningEffort` is a separate worker field. Select AGY
with the public worker value `modelProvider: ANTIGRAVITY` and choose one of the
model IDs above. Factory dispatch supplies the AGY workspace and native
process settings; do not copy provider-native flags onto a Factory or `you run`
surface unless that surface explicitly documents them.

## Configure a Factory worker or one ad-hoc run

Put durable worker identity and policy in the worker frontmatter. Use a
one-shot `you run` override only when the same Factory should run once with a
different provider, model, effort, permission request, or prompt source. The
generic run flags are the public boundary; provider-native options such as
AGY's `--add-dir`, `--effort`, and `--print-timeout` belong to Factory dispatch
and are not generic `you run` flags.

| Durable Factory setting | One-shot `you run` counterpart | Boundary |
|-------------------------|--------------------------------|----------|
| `modelProvider` | `--provider` | Selects the provider adapter for this invocation. |
| `model` | `--model` | Overrides the selected provider's model for this invocation. |
| `reasoningEffort` | `--worker-reasoning-effort` | Uses the canonical `minimal`, `low`, `medium`, `high`, `xhigh`, or `max` vocabulary; the selected provider may reject a value it cannot map. |
| `skipPermissions` | `--skip-permissions` | Requests an invocation-only permission shortcut; the selected provider must support it. |
| worker `timeout` and workstation `limits.maxExecutionTime` | No generic `--timeout` override | These remain Factory execution limits. For AGY print-mode dispatch, the applicable worker timeout becomes the adapter's print timeout; the adapter uses a five-minute default when no positive request is supplied. |
| workstation `promptFile` or prompt body | `--to-file` | Supplies one exact, multiline primary prompt for a one-shot invocation; it is not a worker/provider setting. |

For example, this is a durable `AGENT_WORKER` definition. The worker owns the
provider, model, effort, permission policy, and per-attempt timeout; the
workstation owns the prompt and step behavior.

```yaml
---
type: AGENT_WORKER
modelProvider: CODEX
model: gpt-5.6-luna
executorProvider: SCRIPT_WRAP
reasoningEffort: high
skipPermissions: true
timeout: 45m
---

You are the implementation worker. Make the requested change and report the
verification you ran.
```

For one run, keep the prompt in a UTF-8 file and pass only its path. This
PowerShell shape is safe for multiline text and paths containing spaces:

```powershell
$promptPath = Join-Path (Get-Location) "prompt files\release brief.txt"
$promptText = @"
Review the release notes and identify the highest-risk rollback step.
Keep the answer concise, but preserve the exact wording of the risk.
"@
[IO.File]::WriteAllText($promptPath, $promptText, [Text.UTF8Encoding]::new($false))

you run --provider codex --model gpt-5.6-luna --worker-reasoning-effort high --to-file "prompt files\release brief.txt"
```

`--to-file` is mutually exclusive with positional prompt text, non-empty
stdin, and a signature-defined `--to`; it preserves the file's line endings,
blank lines, Unicode, and trailing newline. See `you docs run` for the complete
input-source contract. `--worker-reasoning-effort` is an invocation override,
not a provider-native `--effort` spelling.

### ANTIGRAVITY dispatch details

Factory dispatch, rather than the operator's `you run` command, owns AGY's
native process arguments. In the current print-mode command adapter, every
dispatch receives `--add-dir <working-directory>` so AGY can read files from
the Factory workspace. The adapter also derives the effective `--print-timeout`
from the execution request and passes it to AGY; when no positive request is
available, the adapter's default is `5m`. Set the durable Factory timeout for a
long media review instead of inventing `--print-timeout` on `you run`.

AGY's completion is stream-based. The adapter parses the final `result` event
and its response; it does not treat a zero process exit as task acceptance. A
recorded missing-file run exited zero and reported `status: SUCCESS` while the
response declined the task, so use a response contract or structured verdict
when the workflow must distinguish task success from process completion.

AGY effort has two constraints to keep separate. The print-mode command
adapter accepts a separate `reasoningEffort` only when it is empty, `low`,
`medium`, or `high`. The current native AGY PTY route rejects a separate effort,
so omit `reasoningEffort` when the selected AGY model ID already encodes its
`-low`, `-medium`, or `-high` tier. Do not pass AGY's native `--effort` or
`--add-dir` directly to a generic `you run` invocation. If a task requires
video or audio inspection, route it to AGY; if one ImageGen call needs more
than five references, split or redesign it instead.

## ImageGen reference limit

ImageGen's `referenced_image_paths` parameter accepts **0–5 paths per call**.
The limit applies to one ImageGen request, not to the number of images in an
entire Factory Session. If a task needs six or more references, split it into
multiple calls with an intermediate synthesis step, or reduce the reference
set before calling ImageGen. Do not keep retrying an over-limit request.

## ACP agents use a separate execution layer

The `executorProvider` on an agent worker selects the execution mechanism;
`ACP` selects Agent Client Protocol execution. The separate `modelProvider`
names the configured integration. ACP integrations let `you` start a compatible
agent over local stdio while Factory Sessions, work routing, event recording,
and final-result selection remain owned by the Factory runtime.

Use `you workers list` to discover all workers, `you workers acp` to manage
custom ACP integrations, and `you run` to prove that an
agent can complete real work. Factory validation checks configuration shape; it
does not launch an agent or prove that a custom provider is still installed.

## Install an ACP agent

Install and authenticate the agent before configuring a worker. `you` includes
a broad, data-backed stdio catalog. Representative entries are:

| Provider identity | Launch command |
|-------------------|----------------|
| `cursor-acp` | `cursor-agent acp` |
| `kiro-acp` | `kiro-cli-chat acp` |
| `opencode-acp` | `npx -y opencode-ai acp` |
| `gemini-acp` | `gemini --acp` |

For Cursor, confirm that the command is installed and that the account is
authenticated:

```bash
cursor-agent --version
cursor-agent acp --help
```

List the built-in presets and any operator-added ACP integrations:

```bash
you workers list
```

`selectable` means the identity is present in the Providers catalog. The
definitive readiness check is a small `you run`, because the agent executable,
authentication, and negotiated ACP session are checked when execution starts.

## Add a custom ACP integration

Use a stable lowercase provider identity and pass the complete launch command
as one `--argument` value:

```bash
you workers acp add --name company-cursor --transport stdio --argument "cursor-agent acp"
you workers list
```

P0 supports stdio only. The command may include arguments; `you` parses it when
the Providers service is constructed. Adding `cursor-acp`, `kiro-acp`, or
`opencode-acp` overrides that preset's launch command. Deleting such an override
restores the built-in preset.

ACP integrations are operator settings in:

- macOS/Linux: `~/.you-agent-factory/config.json`
- Windows: `%USERPROFILE%\.you-agent-factory\config.json`

The persisted section has this shape:

```json
{
  "workers": {
    "acp": {
      "integrations": [
        {
          "id": "generated-settings-entry-id",
          "name": "company-cursor",
          "transport": "stdio",
          "command": "cursor-agent acp"
        }
      ]
    }
  }
}
```

Prefer the CLI over editing this file. The generated `id` identifies the
settings entry; workers select the integration by `name`. Timeout and permission
policy do not belong in this operator section. Use normal Factory Session and
worker limits for timeouts, and `skipPermissions` for the existing invocation
permission behavior.

## Use an ACP agent in a Factory

In a split Factory, select the provider in `workers/<name>/AGENTS.md`:

```yaml
---
type: AGENT_WORKER
executorProvider: ACP
modelProvider: cursor-acp
skipPermissions: true
---

You are a focused software engineer. Make the requested change and run focused
verification before reporting the result.
```

The workstation remains a normal `AGENT_RUN`; ACP is not a separate
workstation or worker type:

```yaml
---
type: AGENT_RUN
---

{{ (index .Inputs 0).Payload }}
```

Validate the Factory, then run a small end-to-end task and keep the response
stream visible until it reaches a terminal result:

```bash
you factory config validate ./factory
you run --factory ./factory/factory.json --provider cursor-acp --model auto --skip-permissions "Add one table-driven test and run the focused test command."
```

`--skip-permissions` is invocation-only. Omit it when the ACP agent should ask
through its supported permission flow.

For a portable JSON or YAML Factory, set the same identity on its worker:

```json
{
  "name": "executor",
  "type": "AGENT_WORKER",
  "executorProvider": "ACP",
  "modelProvider": "cursor-acp",
  "skipPermissions": true
}
```

## Use ACP with a packaged Factory

Packaged Factories materialize lazily under
`~/.you-agent-factory/factories` and remain editable. Inspect the catalog, ask
for the named Factory's generated help to materialize it without executing
work, set `executorProvider` on the agent worker, validate it, and run it by
name:

```bash
you factory list
you run --named @you/goal --help
you factory config validate ~/.you-agent-factory/factories/@you/goal
you run --named @you/goal --provider cursor-acp --model auto --skip-permissions "Add a simple unit test, run it, and finish the goal."
```

For `@you/goal`, set `"executorProvider": "ACP"` and
`"modelProvider": "cursor-acp"` on the `goal-executor` worker in its
materialized `factory.json`. On Windows, the same
Factory is under `%USERPROFILE%\.you-agent-factory\factories\@you\goal`.

## Use ACP from JavaScript

JavaScript workflows select the same Providers catalog identity on
`agent.run`:

```javascript
return (async function () {
  phase("test");
  return await agent.run({
    label: "cursor-test-author",
    executorProvider: "ACP",
    modelProvider: "cursor-acp",
    prompt: "Add one table-driven unit test and run the focused test command."
  });
})();
```

Run it from the project directory whose files the agent should inspect:

```bash
you run --factory ./test.workflow.js --skip-permissions
```

JavaScript receives the structured child result. Use `you session show`,
`you session dispatches`, and the printed Factory Session links when deeper
inspection is needed. See `you docs javascript-workflows` for the complete host
API and lifecycle contract.

## Remove a custom integration

Delete by provider name and confirm it no longer appears:

```bash
you workers acp delete --name company-cursor
you workers list
```

Factories that still name the deleted custom identity can pass offline Factory
validation because the identity is syntactically valid. Their next real run
fails provider selection. Update `modelProvider`, reinstall the integration,
or use a built-in preset.

## Troubleshooting

- `ACP command is unavailable` means the configured launch command could not be
  created or found. Run its executable directly and check `PATH`.
- `ACP authentication required` means the agent advertised authentication but
  could not open a session. Complete the agent's own login flow and retry.
- An unknown-provider or failed-state result after deleting a custom integration
  means a worker still references its old `modelProvider`.
- A permission cancellation means the agent requested an operation that was not
  approved. Retry with the intended permission flow or deliberately add
  `--skip-permissions` for that invocation.
- `you factory config validate` proves Factory structure only. A terminal
  `you run` proves command startup, ACP negotiation, agent execution, routing,
  and result projection together.

## Related

- `you docs workers` for worker and workstation field ownership
- `you docs run` for terminal output modes and run selection
- `you docs javascript-workflows` for JavaScript child execution
- `you docs sessions` for Factory Session and dispatch inspection
