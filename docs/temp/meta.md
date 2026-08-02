# Current world state

## System architecture

- Customer ask: implement `docs/internal/projects/acp-client/final-proposal.md`
  completely, with P0 ACP Chat/Factory behavior and the Worker Sessions control
  plane delivered through singular, statically injected service roots.
- Current code has none of the proposed `chat_sessions`, `worker_sessions`,
  `events`, or top-level ACP transport packages. `RuntimeOpeningFactory` and
  `runtimeOpener` remain live migration targets.
- Live default PETRI session: `5146e0a4-a4d3-4d93-a51c-560a5243a474`.

## Operational notes

- The prior lint/Bun queue was replaced. Current queue began this pass with two
  failed legacy tasks whose only last output was successful workspace setup.
- Both failures were retried once to `task:init` after capacity recovered and
  failed again immediately with unchanged setup-only output. Do not retry again
  without new process-dispatch/provider evidence or a narrow correction.
- Root has unrelated Claude provider/test changes. ACP workers must preserve
  them and resolve normal shared-file conflicts through the delivery loop.
- `docs/temp/**` is gitignored planner state.

# Progressive change notes

## High-level track state

- ACP Phase 0 is active under request `planner-acp-v0-contracts-20260802`: nine
  ideas are processing and the loopback is guarded on all nine completions.
- Subsequent work follows the semantic DAG in final-proposal section 9.3;
  numbered phases are review checkpoints, not repository freezes.
- Every loopback must reconcile queue health and verify actual merged evidence
  before admitting the next dependency-ready vertical slice.
