# Idea Payload Template (mandatory)

Every `idea` work item submitted by the **ideafy** meta-planner MUST use this
shape. Vague packets cause plan/process agents to invent the wrong system
(for example building a `pkg/root` test subsystem when the ask was only for
external functional proofs after a refactor).

Fill every section. Prefer short bullets over long prose. Do not submit an idea
whose `ask` could be satisfied by two materially different architectures.

## Required Markdown Shape

Use this outline when drafting the idea, then map it into the JSON payload
fields below.

```text
ask:
  <one plain-language paragraph: who cares, what must be true afterward, and
   why this packet exists now>

shape of requirements:
  1. system shape requirements
     - <what may change in package/layout/contracts>
     - <what must stay unchanged>
     - <where work is allowed to live; where it must NOT live>
  2. test coverage requirements
     - <external/public proof expected (CLI/API/events/functional suite)>
     - <reuse existing tests if sufficient, else add only the missing proofs>
     - <explicitly forbid inventing helper subsystems solely for tests>
  3. acceptance criteria
     - <observable outcomes that prove confidence for an external user>
     - <migration/cleanup complete only when those outcomes hold>
```

## Required JSON Payload Fields

Every submitted `idea.payload` MUST include at least:

| Field | Purpose |
|---|---|
| `title` | Short packet title (often includes packet id) |
| `packetId` | Stable packet id when one exists |
| `ask` | Plain-language ask (same content as the Markdown `ask:`) |
| `systemShapeRequirements` | Array of system/layout/contract constraints |
| `testCoverageRequirements` | Array of external proof / coverage constraints |
| `acceptanceCriteria` | Array of reviewer-verifiable outcomes (≥2 behavioral) |
| `outOfScope` | Explicit non-goals and forbidden inventions |
| `antiPatterns` | Concrete wrong implementations to reject |
| `currentProgress` | Why this packet is next (evidence, not slogans) |
| `requestedOutcome` | Single long-form synthesis of ask + requirements + delivery |
| `changedPathLease` | Paths the worker may change; call out excludes |
| `sourcePath` | Plan/meta source when applicable |

Always append the delivery contract to `requestedOutcome` (or as its own final
acceptance bullet): required CI terminal green, blocking PR conversation
comments addressed, merge conflicts reconciled, PR merged, then Factory
complete.

## Writing Rules

1. **External-user first.** Prefer proofs through public process composition
   (`root.BuildProcess`, CLI, REST/MCP, Factory Events, public Work surfaces)
   under `tests/functional/...` or equivalent public suites.
2. **Name the proof home.** Say exactly which suite/directory owns the proof.
3. **Forbid invented scaffolding.** If the ask is "prove X still works after
   refactor Y", say explicitly: do not create new peer/root/wire helper
   packages or test-only subsystems unless the ask lists them.
4. **Reuse before invent.** Require validating whether existing tests already
   cover the behavior; add tests only for real gaps.
5. **One architecture.** An implementer reading only this payload must not need
   chat context to choose between "add functional tests" and "build a new
   root composition API".
6. **Leases are real.** `changedPathLease` must exclude paths that would pull
   the worker into a different packet.

## Filled Example (FUN-style proof — correct)

```text
ask:
  After the Automations CUT→fold→DEL track, prove from an external user's
  perspective that Automations still works: cron, watchers, hosted sources,
  script pollers, and reconciliation remain inert until runtime lifecycle,
  then activate through public Automations/Work surfaces.

shape of requirements:
  1. system shape requirements
     - Keep packaged Automations root as wire/, internal/, transports/ plus
       thin root contracts; do not fold or delete packages in this packet
     - Do not invent new pkg/root or pkg/wire Automations helper APIs solely
       for tests
     - Functional proofs construct the process only through root.BuildProcess
       and replace effects only via edges.Edges
  2. test coverage requirements
     - Prefer extending tests/functional/automations
     - First validate whether existing functional proofs already cover each
       sidecar family; add only missing external proofs
     - Do not add peer import scanners or root unit scaffolds as a substitute
       for external behavior proof
  3. acceptance criteria
     - We are confident Automations is stable post-migration because each
       sidecar family has an external functional proof of inert construction
       and post-lifecycle activation
     - No production or functional-suite import of deleted automations/service
       or peer imports of automations/internal
     - Required CI green and PR merged before Factory completion
```

Corresponding JSON fragment:

```json
{
  "title": "FUN-automations: public-process proof for Automations",
  "packetId": "FUN-AUTO",
  "ask": "After Automations CUT→fold→DEL, prove from an external-user perspective that Automations sidecars stay inert until runtime lifecycle and then activate through public Automations/Work surfaces.",
  "systemShapeRequirements": [
    "Keep Automations root as wire/, internal/, transports/ plus thin contracts; no fold/delete in this packet",
    "Do not invent new pkg/root or pkg/wire Automations helper APIs solely for tests",
    "Construct only through root.BuildProcess; replace effects only via edges.Edges"
  ],
  "testCoverageRequirements": [
    "Own proofs under tests/functional/automations",
    "Validate existing functional coverage first; add only missing external proofs for cron, filesystem watchers, hosted sources, script pollers, and reconciliation",
    "Do not substitute peer-import scanners or root unit scaffolds for external behavior proof"
  ],
  "acceptanceCriteria": [
    "Each Automations sidecar family has an external functional proof of inert construction and post-lifecycle activation",
    "No functional-suite or production peer import of deleted automations/service or automations/internal",
    "Typecheck, lint, and tests pass; required CI green; PR merged before Factory completion"
  ],
  "outOfScope": [
    "New Automations HTTP/CLI/MCP adapters",
    "New pkg/root or pkg/wire Automations composition helpers created only to make tests compile",
    "Package fold/delete or PSS-I0* shared integrators"
  ],
  "antiPatterns": [
    "Building a root/wire 'AutomationsRootFromEdges' subsystem because tests asked for public composition",
    "Treating package-shape or import-boundary unit tests as sufficient FUN proof without external lifecycle behavior",
    "Rewriting unrelated functional suite topology outside the Automations lease"
  ],
  "currentProgress": [
    "Automations IMP/LWR/adapters and CUT→fold→DEL are Factory-terminal",
    "Deletion alone does not prove external Automations behavior still works"
  ],
  "changedPathLease": [
    "tests/functional/automations/**",
    "exclude pkg/root and pkg/wire except unavoidable compile fixes unrelated to new Automations helper APIs"
  ],
  "requestedOutcome": "Extend or add focused functional proofs under tests/functional/automations so cron, filesystem watchers, hosted sources, script pollers, and reconciliation remain inert until runtime lifecycle, then activate through public Automations/Work surfaces without inventing root/wire test subsystems or importing automations/internal or deleted automations/service. Delivery contract: loop until required CI is terminal green, blocking PR conversation comments are addressed, conflicts are reconciled, the PR is merged, and only then complete Factory work."
}
```

## Anti-Example (ambiguous — do not submit)

```text
ask:
  Prove Automations as a packaged service through public process composition.

requestedOutcome:
  Close FUN-automations with focused functional proofs and owner-local root
  contract tests when required.
```

Why this fails: "public process composition" + "owner-local root contract tests
when required" invites inventing `pkg/root`/`pkg/wire` helpers. The correct
packet names the external suite, forbids new root/wire scaffolding, and makes
acceptance about observable Automations behavior—not about creating a test
subsystem.
