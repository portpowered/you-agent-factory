# Cleanup Analyzer Report: Exhaustion Rules Guidance Sweep

Date: 2026-04-20

## Scope

Focused inventory over active Agent Factory guidance and examples to verify the
retired public `exhaustion_rules` vocabulary no longer appears as current
authoring guidance.

## Evidence

Command:

```powershell
rg -n -i "exhaustion[_ ]rules?|exhaustion-rule|exhaustion rule" libraries/agent-factory/docs libraries/agent-factory/examples docs/intents/agent-factory.md --glob '!libraries/agent-factory/docs/development/**'
```

Final inventory result after the cleanup:

- The command above returns 4 matching lines across 2 files.
- All remaining matches are explicitly historical migration or compatibility
  references.

## Classification

| Evidence | Classification | Rationale |
| --- | --- | --- |
| `libraries/agent-factory/docs/guides/workstation-guards-and-guarded-loop-breakers.md:118` | retain | Historical migration snippet intentionally preserves the retired field name so customers can translate existing configs to the guarded `LOGICAL_MOVE` replacement. |
| `libraries/agent-factory/docs/guides/workstation-guards-and-guarded-loop-breakers.md:237` | retain | Historical compatibility note explicitly limits `exhaustion_rules[]` to migration framing and rejects it as active authoring guidance. |
| `libraries/agent-factory/docs/guides/workstation-guards-and-exhaustion-limits.md:10` | retain | The legacy guide path is preserved only as a historical redirect note for old links and explicitly tells readers to use the guarded loop-breaker guide for current authoring. |
| `libraries/agent-factory/docs/guides/workstation-guards-and-exhaustion-limits.md:18` | retain | The retained historical note limits `exhaustion_rules[]` to compatibility vocabulary and gives only migration direction, not active recommendation text. |

## Outcome

Implemented in this sweep:

- Replaced the old `workstation-guards-and-exhaustion-limits.md` content with a
  short historical redirect note so bookmarked links still resolve without
  leaving active recommendation text in place.
- Repointed the active inbound related-doc link in
  `parent-aware-fan-in.md` to the guarded loop-breaker guide so current docs no
  longer route readers to the retired guidance.
- Reduced the active work-config guide wording to avoid repeating the retired
  field name outside migration-specific surfaces.

## Verification

Re-run:

```powershell
rg -n -i "exhaustion[_ ]rules?|exhaustion-rule|exhaustion rule" libraries/agent-factory/docs libraries/agent-factory/examples docs/intents/agent-factory.md --glob '!libraries/agent-factory/docs/development/**'
```

The scan should return only the four retained historical references classified
above.
