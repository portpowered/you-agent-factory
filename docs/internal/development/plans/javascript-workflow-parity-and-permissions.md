# JavaScript Workflow Parity and Permission Model Simplification

---
author: operator
last modified: 2026, august, 22
doc-id: PLAN-JS-001
status: proposed
---

# problem statement

Our JavaScript orchestrator carries a two-axis permission model whose safety axis cannot be enforced at the provider boundary, and it diverges from the Claude Code `Workflow` surface in three ways that cost real reliability.

## customer ask

Make the JavaScript workflow surface behave consistently with how it works in Claude, and replace the `READ_ONLY` / `skipPermissions` split with a single flag — `DEFAULT` or `SKIP_PERMISSIONS` — that propagates parameters down to the agent instead of pretending to sandbox it.

## solution

Collapse the permission model to one per-call enum that maps directly onto the flag we actually pass to the provider, keep the declarative policy layer strictly as an allowlist over that enum, and close three specific parity gaps: structured child output, variadic pipeline stages, and contract truth.

## Verified delivery status (2026-08-24)

JS-P1, JS-P2, JS-P3, and JS-P4 are merged in PRs #2206, #2209, #2220,
and #2232 respectively. The structured-child-output and generated-contract
acceptance work in JS-P5 and JS-P7 is merged in PRs #2230 and #2229.
JS-P6, the variadic-pipeline work, remains open in PR #2226. The plan remains
open for that unfinished acceptance work; this note records PR state only.

# original document

`C:\Users\andre\.claude\skills\dispatching-you-subagents\references\claude-workflow-tool.md`

## Part 1 — The permission model

### Measured current state

There are two independent axes, and they do not compose.

**Axis 1, declarative:** `defaultPolicy` resolves into `EffectivePolicy`
(`pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract/policy_types.go:36`)
carrying `Mode`, `AllowNetwork`, `AllowConnectors`, `AllowDangerFullAccess`,
`WritableRoots`, plus allowlists for runners, models, efforts, route profiles and
commands. `ModeReadOnly = "READ_ONLY"` is the **only** mode constant defined — the enum
is already effectively binary, expressed as `READ_ONLY` versus unset, and then softened
by four independent escape hatches.

**Axis 2, per-call:** `skipPermissions` is one of nine fields on `agent.run`
(`orchestratorcontract/javascript_child.go:12-20`), normalized by
`NormalizeJavaScriptChild` with `additionalProperties: false`.

### Why the safety axis is not real

`ValidateCapability` and `DeniedCapabilitiesForReadOnly`
(`orchestratorcontract/policy_capability.go`) are referenced by exactly five files, all
inside the same orchestrator-contract package. Nothing in `pkg/services/workers` or
`pkg/services/providers` consumes them.

Meanwhile the provider adapters do this:

- `providers/internal/services/execution/internal/adapters/codex/command.go:61` appends
  `--dangerously-bypass-approvals-and-sandbox`
- `.../adapters/claude/command.go:62` appends `--dangerously-skip-permissions`
- `.../adapters/agy/command.go:126` appends `--dangerously-skip-permissions`

each gated on `request.SkipPermissions` — **axis 2**, never axis 1.

So `policy.mode: READ_ONLY` denies a set of capability names to the in-process bounded
tool executor (`workers/.../agentrun/tools.go`, which implements `read_file`,
`list_directory`, `write_file`), and denies nothing to the provider's own shell, file
and network tools. A child agent under `READ_ONLY` can still write files, execute
processes, and reach the network, because those actions happen inside the provider, and
the provider's sandbox is being explicitly disabled on the adjacent line.

This was independently observed under adversarial test: a live `@you/subagent` run under
`agentTools.policy: READ_ONLY` created a file via shell redirect with no denial and no
diagnostic, executed `python -c`, and fetched a URL over the open network — while the
documentation promised "Write tools are denied."

**The defect is not that enforcement is missing. It is that the field implies a
guarantee the architecture cannot deliver from where it sits.** Two axes, one of which
is unenforceable at the only boundary that matters, is strictly worse than one honest
axis — it converts an absent guarantee into a false one.

### Target model

One per-call field. One value passed down. No inference.

```
agent.run({ ..., permissions: "DEFAULT" | "SKIP_PERMISSIONS" })
```

- `DEFAULT` — the provider is launched with **no** bypass flag. Its own approval and
  sandbox behavior applies, whatever the user has configured for that provider.
- `SKIP_PERMISSIONS` — the provider is launched with its bypass flag, exactly as today.

The value we hold is the value we pass. There is nothing to enforce, because there is
nothing being claimed beyond the flag itself.

### What to keep, and why (recommendation, with the tradeoff stated)

The instinct to "remove all that complexity" is right about the capability booleans and
wrong about one thing, so I want the tradeoff explicit rather than buried.

`defaultPolicy` is genuinely our advantage over Claude Code's single `isolation` knob: it
is an **auditable manifest checked before a Dispatch exists**. A factory author can
declare what a workflow is permitted to do, and an operator can read it without running
anything. Claude Code has no equivalent. Deleting the whole policy layer would trade a
real capability for tidiness.

So the recommendation is:

- **Delete** `AllowNetwork`, `AllowConnectors`, `AllowDangerFullAccess`,
  `WritableRoots`, `Mode`, the six `Capability` constants, `ValidateCapability`, and
  `DeniedCapabilitiesForReadOnly`. Every one of these describes a boundary we do not
  hold. They are the false guarantee.
- **Keep** the policy layer as an **allowlist over the new enum**: a factory may declare
  `allowedPermissions: ["DEFAULT"]`, and a child requesting `SKIP_PERMISSIONS` is
  rejected *before dispatch* with a named diagnostic. This is enforceable — it gates a
  value we control — and it preserves the audit property.
- **Keep** `AllowedModels`, `AllowedReasoningEfforts`, `AllowedRunners`, `MaxAgents`,
  `Concurrency`, `MaxDepth`, `MaxRetries` and the duration/size bounds. All of these
  gate values or counts the runtime genuinely owns.

If we later gain a real sandbox boundary — a container, a restricted provider mode, an
OS-level jail — capability names can come back, attached to something that enforces them.
Reintroducing an honest guarantee is cheap. Living with a dishonest one is not.

### Delivery — strangler, four independently mergeable steps

**JS-P1. Characterization tests first.** Pin today's behavior before changing it: what
`agent.run` accepts and rejects, which provider flags result from each combination of
`policy.mode` and `skipPermissions`, and what `DeniedCapabilitiesForReadOnly` returns.
Coverage of this surface is currently concentrated in
`javascript_policy_test.go` and `policy_capability.go`'s own tests, neither of which
asserts the provider command line. That gap is why the divergence survived. Add the
provider-command assertion as the characterization.

**JS-P2. Introduce `permissions` alongside `skipPermissions`.** Both accepted;
`permissions` wins when both are present; `skipPermissions: true` maps to
`SKIP_PERMISSIONS`. Emit a deprecation diagnostic naming the replacement. No behavior
change for existing workflows — every current script keeps working.

**JS-P3. Introduce `allowedPermissions` and reject at validation.** A factory declaring
`allowedPermissions: ["DEFAULT"]` causes a `SKIP_PERMISSIONS` child request to fail
validation with a diagnostic naming the factory, the child label, and the requested
value. Acceptance is a rejection observable before any dispatch record exists.

**JS-P4. Delete the old axis.** Remove `skipPermissions` from the field list, remove
`Mode` and the four capability escape hatches, delete the capability constants and their
validators, and update the packaged docs. This is the step that changes behavior for
authored factories, so it lands alone and last.

Each step leaves `main` releasable. No step exists to repair fallout from an earlier one.

## Part 2 — Parity with the Claude Code `Workflow` surface

Both are policy-bounded JavaScript orchestrators over agent children in a locked-down VM
with no imports, filesystem, shell or network, returning JSON-only final values. The
naming overlap is near-total. Three divergences cost us something real.

### JS-P5. Structured child output (highest value)

`agent.run` **explicitly rejects** `schema` and `outputSchema` — they are pinned in the
rejected-field test alongside `writableRoots`, `allowNetwork`, `concurrency`, `maxAgents`
and the timeout family. Children therefore return prose that the parent must parse or
re-prompt.

The residue is visible in our own shipped output: `@you/spawn` and `@you/deep-research`
both emit `"schemaValidated": false` and `"subject": ""` — vestigial fields for a feature
that was never wired.

Claude Code's model is worth copying exactly: `schema` forces the child to call a
structured-output tool, validation happens at the tool-call layer, and the model retries
on mismatch rather than the parent receiving something unparseable. For fan-out-then-merge
work, validated child output is most of the reliability.

Acceptance: a child declared with a schema returns a validated object; a child that
produces a non-conforming value retries and then fails with a diagnostic naming the
violated path; the vestigial `schemaValidated` field reports truthfully.

### JS-P6. Variadic `pipeline` stages

Ours is `pipeline(items, worker, next?)` — capped at two stages. Claude Code's is
variadic with no barrier between stages, so item A can be in stage 3 while item B is
still in stage 1, and wall-clock equals the slowest single-item chain rather than the sum
of per-stage slowest.

With two stages, a `review → verify → synthesize` shape forces either a nested `parallel`
inside `next` — losing ordering — or an explicit barrier, which idles every fast item
until the slowest one catches up. This is the one place the reference is plainly better
and the fix is contained.

Acceptance: a three-stage pipeline over N items completes in slowest-chain time rather
than sum-of-stage-maxima, demonstrated with recorded dispatch timestamps; a stage that
throws drops that item and skips its remaining stages without failing the run.

### JS-P7. Contract truth

Three sources disagree on the `agent.run` field list. Verified 2026-08-22:
`contracts/javascript/runtime-api.json` contains `skipPermissions` and `preset` but is
**missing `executorProvider` and `resourceId`** — two of the nine fields that
`javascript_child.go` actually accepts. The shipped JSON artifact is stale, and
`you docs javascript-workflows` disagrees with both.

A hand-maintained contract artifact will drift again. The fix is to generate it from the
Go contract, which is the same mechanism proposed in
`docs-generation-from-testable-sources.md` — this story is that plan's first consumer and
its proof case.

Acceptance: the JSON artifact is generated from `javascript_child.go`; a CI check fails
when they disagree; a field added to the Go contract appears in the artifact and in the
packaged docs without anyone editing either.

### Deliberately not copied

- **`Date.now()` / `Math.random()` throwing.** That constraint exists because Claude Code
  replays the longest unchanged prefix of agent calls from cache, and nondeterminism
  would poison it. Our resume model is the inverse — explicit
  `workflow.checkpoint({label, state})` and `workflow.resumeState()`, which does not
  snapshot the VM and therefore has no determinism requirement. Ours is manual but
  survives process death; theirs is free but fragile across edits. Different bets, not a
  ranking, and adopting their constraint without their cache would be pure cost.
- **`isolation: 'worktree'` as the sole sandbox knob.** We are moving toward *fewer*
  false guarantees, not toward a knob that implies isolation we would then have to
  deliver per-child.
- **Dropping the invocation surface.** `invocationSignature` turning a workflow into a
  distributable CLI with generated help and positional/stdin/named bindings is ours
  alone and is worth keeping, even though it is the reason the `--model` flag-collision
  bug class exists at all.

## Verification

`make test` plus targeted functional tests near
`pkg/services/factory_runtime/internal/services/orchestration/javascript/`. JS-P1's
provider-command assertion is the regression guard for the entire permissions sequence
and must stay green through JS-P4. JS-P7 additionally requires
`make interfaces-all`, since it changes a generated contract artifact.

## Delivery loop

Implementation finishes when its final head is pushed, the PR is open, CI has started,
and blocking review feedback is addressed. Review owns terminal-and-passing CI, conflict
resolution, and merge. CI-run evidence goes in a PR comment, never a commit.
