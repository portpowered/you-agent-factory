# Harnesses

`you` dispatches agent work through harnesses: bundled model-provider CLIs and
Agent Client Protocol (ACP) stdio integrations. Pick a harness with
`--provider` / role-specific provider flags on `you run`, or set
`modelProvider` on a worker. List every selectable identity with
`you workers list`.

This page is the customer-facing inventory of shipped harness identities. For
ACP install, custom integrations, and Factory selection details, see
`you docs providers`. For worker field ownership, see `you docs workers`.

## Bundled provider CLIs

These identities ship in the model-provider catalog
(`packages/model-providers/generated/catalog.json`) and run through native
provider adapters:

| Identity | Display name | Launch executable | Support level | Notes |
|----------|--------------|-------------------|---------------|-------|
| `codex` | Codex | `codex` | production | Default packaged-Factory backend in many examples |
| `claude` | Claude Code | `claude` | experimental | Anthropic Claude Code CLI |
| `antigravity` | Antigravity | `agy` | experimental | Terminal-agent integration |

Install and authenticate the matching CLI before a live run. When you do not
have a harness installed yet, prove Factory routing with
`you run --with-mock-workers` (see `you docs mock-workers`).

Representative one-shot:

```bash
you run --named @you/goal --provider codex --model gpt-5 \
  --to "Add one focused unit test and run it"
```

## Packaged ACP stdio integrations

ACP harnesses use `executorProvider: ACP` on an agent worker and name the
integration in `modelProvider` (for example `cursor-acp`). The packaged stdio
catalog is data-backed at
`pkg/services/providers/internal/services/builtins/wire/catalog.json`.

| Identity | Launch command | Aliases |
|----------|----------------|---------|
| `pi-acp` | `npx pi-acp` | |
| `openclaw-acp` | `openclaw acp` | |
| `gemini-acp` | `gemini --acp` | |
| `cursor-acp` | `cursor-agent acp` | |
| `copilot-acp` | `copilot --acp --stdio` | |
| `droid-acp` | `droid exec --output-format acp` | `factory-droid`, `factorydroid` |
| `fast-agent-acp` | `uvx fast-agent-mcp acp` | |
| `grok-build-acp` | `grok agent stdio` | |
| `iflow-acp` | `iflow --experimental-acp` | |
| `kilocode-acp` | `npx -y @kilocode/cli acp` | |
| `kimi-acp` | `kimi acp` | |
| `kiro-acp` | `kiro-cli-chat acp` | |
| `mux-acp` | `mux acp` | |
| `opencode-acp` | `npx -y opencode-ai acp` | |
| `pool-acp` | `pool acp` | |
| `qoder-acp` | `qodercli --acp` | |
| `qwen-acp` | `qwen --acp` | |
| `reasonix-acp` | `reasonix acp` | |
| `trae-acp` | `traecli acp serve` | |
| `zeroclaw-acp` | `zeroclaw acp` | |

That is twenty packaged ACP identities, plus the three bundled provider CLIs
above. Operators can add more with `you workers acp add`.

Representative ACP run after the agent is installed and authenticated:

```bash
you run --named @you/goal --provider cursor-acp --model auto --skip-permissions \
  --to "Add a simple unit test, run it, and finish the goal"
```

## Choosing and verifying a harness

1. Install the agent CLI or ACP launch command and complete its login flow.
2. Confirm it appears as selectable: `you workers list`.
3. Point a worker or run flag at that identity (`--provider`,
   `--executor-provider`, `--planner-provider`, and similar role flags on
   packaged Factories).
4. Prove end-to-end readiness with a small `you run`. Factory validation checks
   config shape only; it does not launch the harness.

Custom ACP integrations persist under operator settings
(`~/.you-agent-factory/config.json` on macOS/Linux,
`%USERPROFILE%\.you-agent-factory\config.json` on Windows). Prefer
`you workers acp add` / `you workers acp delete` over hand-editing that file.

## Related

- `you docs providers` — ACP setup, custom integrations, JavaScript `agent.run`
- `you docs workers` — `AGENT_WORKER` / `executorProvider` / `modelProvider`
- `you docs run` — run shapes, `--with-mock-workers`, server and site flags
- Packaged Factory catalog:
  [`packaged-factories.md`](./packaged-factories.md)
- Model-provider catalog source:
  `packages/model-providers/generated/catalog.json`
- ACP catalog source:
  `pkg/services/providers/internal/services/builtins/wire/catalog.json`
