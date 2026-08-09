# Relationships

Use these constructs when one submission should order sibling Work, attach
children to a parent for fan-in, or make a workstation join inputs by authored
Work name.

`you docs relationships` is the canonical guide for `DEPENDS_ON`,
`PARENT_CHILD`, and `SAME_NAME` semantics. `SPAWNED_BY` is the runtime lineage
record for work created by a workstation. See `you docs guards` for the full
per-input guard contract, and
`you docs batch-inputs` for full batch file field tables and watched
folder placement.

## Quick Choice

| Need | Relation type | Where it is declared |
|------|---------------|---------------------|
| One sibling work item must wait for another sibling to reach a state | `DEPENDS_ON` | `relations[]` on a `FACTORY_REQUEST_BATCH` |
| Child membership under one parent for parent-aware guards | `PARENT_CHILD` | `relations[]` on a `FACTORY_REQUEST_BATCH` |
| Join workstation inputs whose authored Work names match | `SAME_NAME` | `workstations[].inputs[].guards[]` |
| Read which workstation spawned a child token in templates or traces | `SPAWNED_BY` | Runtime on work tokens; appears in `.Relations` |
| Gate dispatch until a condition is true | Guards, not relations | `you docs guards` |

`DEPENDS_ON` and `PARENT_CHILD` can appear in the same submitted batch when
the workflow needs both prerequisite ordering and parent membership. `SAME_NAME`
is different: it is a per-input workstation guard, not an entry in batch
`relations[]`. A workstation can use `SAME_NAME` together with either batch
relation.

## Source And Target Semantics

| Relation type | Source means | Target means | `requiredState` |
|---------------|--------------|--------------|-----------------|
| `DEPENDS_ON` | The blocked work item | The prerequisite work item | Optional. Defaults to `complete`. Names a state on the target work type. |
| `PARENT_CHILD` | The child work item | The parent work item | Must be omitted. Supplying it rejects the whole batch. |
| `SAME_NAME` | No relation source; the input carrying the guard | The peer input named by `matchInput` | Not applicable. The selected Work names must be equal. |
| `SPAWNED_BY` | The spawned child work item | The spawning context (for example a parent work item or fanout source) | Not used on batch files. Runtime records the spawn lineage. |

Read `DEPENDS_ON` as: the source waits for the target. Read `PARENT_CHILD` as:
the source is a child of the target. `SAME_NAME` has no source-to-target
direction: put it on the guarded input, and set `matchInput` to the peer input
whose selected Work name must equal the guarded input's Work name.

## `DEPENDS_ON`

Use `DEPENDS_ON` for ordinary prerequisite ordering between siblings in one
batch. It blocks dispatch ordering; it does not create parent lineage for
parent-aware input guards.

```json
{
  "type": "DEPENDS_ON",
  "sourceWorkName": "publish",
  "targetWorkName": "review",
  "requiredState": "complete"
}
```

Read that as: `publish` waits until `review` reaches `complete`. When
`requiredState` is omitted, the factory defaults to `complete`.

`DEPENDS_ON` example inside a batch:

```json
{
  "requestId": "release-gate",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "review",
      "workTypeName": "story",
      "payload": { "title": "Review release notes" }
    },
    {
      "name": "publish",
      "workTypeName": "story",
      "payload": { "title": "Publish release" }
    }
  ],
  "relations": [
    {
      "type": "DEPENDS_ON",
      "sourceWorkName": "publish",
      "targetWorkName": "review",
      "requiredState": "complete"
    }
  ]
}
```

After validation, the factory attaches each `DEPENDS_ON` relation to the
blocked source work token. The scheduler keeps the source from running until
the target reaches the required state. If the source is one input of a
multi-input workstation, this dependency is still checked after the complete
input binding is assembled; see [Joined-input dispatch invariant](#joined-input-dispatch-invariant).

## `SAME_NAME`

Use `SAME_NAME` when one workstation consumes two normal inputs and should fire
only when the selected Work items share the same authored name. The guard
belongs on the input being constrained. `matchInput` names the peer input on
the same workstation by its `workType` value:

```json
{
  "name": "join-plan-and-task",
  "worker": "matcher",
  "inputs": [
    { "workType": "plan", "state": "ready" },
    {
      "workType": "task",
      "state": "ready",
      "guards": [
        {
          "type": "SAME_NAME",
          "matchInput": "plan"
        }
      ]
    }
  ],
  "outputs": [{ "workType": "task", "state": "matched" }]
}
```

This guard only selects a compatible pair. It does not satisfy, replace, or
create a `DEPENDS_ON` prerequisite, and it does not create `PARENT_CHILD`
lineage. The matching Work items may come from different submissions; that
does not widen the same-batch rule for `DEPENDS_ON` or `PARENT_CHILD` relation
endpoints. If either selected Work item has no usable authored name, or the
names differ, the workstation remains disabled.

## Joined-input dispatch invariant

A complete input join is necessary but is not by itself permission to dispatch.
For a workstation with multiple inputs, the scheduler evaluates enablement in
this order:

1. All input arcs and their guards select a complete binding, including any
   `SAME_NAME` match.
2. Every `DEPENDS_ON` relation attached to every selected Work item is checked.
   Each relation target must exist and be in its `requiredState`; a missing
   target or a target in any other state blocks the binding.
3. Only then can the workstation dispatch, and only if its other guards,
   resource capacity, worker capacity, and scheduler rules also permit it.

This means a dependency carried only by a secondary `SAME_NAME`-joined input
still gates the whole workstation. The name match does not hide the secondary
Work's relations, and the primary input's lack of a dependency does not make
the binding eligible.

### Worked two-input example

Assume the `join-plan-and-task` workstation above. The `task` Work is the
secondary input and carries a dependency on a `producer` Work. Submit the
producer and task together so the relation endpoints are in one batch:

```json
{
  "requestId": "joined-task",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "producer",
      "workTypeName": "producer",
      "payload": { "role": "controlled prerequisite" }
    },
    {
      "name": "joined-item",
      "workTypeName": "task",
      "payload": { "role": "secondary join input" }
    }
  ],
  "relations": [
    {
      "type": "DEPENDS_ON",
      "sourceWorkName": "joined-item",
      "targetWorkName": "producer",
      "requiredState": "complete"
    }
  ]
}
```

Then make the primary `plan` Work available under the same authored name. It
can be a separate batch because `SAME_NAME` is a workstation guard, not a
batch relation:

```json
{
  "requestId": "joined-plan",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "joined-item",
      "workTypeName": "plan",
      "payload": { "role": "primary join input" }
    }
  ]
}
```

The two batches each have unique Work names, and the `DEPENDS_ON` source and
target both belong to `joined-task`. Once both `plan` and `task` are in
`ready`, `SAME_NAME` can select them as one complete binding, but the
workstation still waits for `producer`:

| Moment | Relevant state | Dispatch result |
|--------|----------------|-----------------|
| `t0` | `plan` and `task` named `joined-item` are both `ready`; `producer` is `ready` or running | The full join is visible, but the secondary `task` dependency is incomplete. No `join-plan-and-task` dispatch. |
| `t1` | `producer` has been dispatched but has not reached `complete` | The binding remains undispatched; a running or otherwise non-`complete` target does not satisfy `requiredState`. |
| `t2` | `producer` reaches `complete` | The next scheduler evaluation can admit the join, subject to all other guards, capacity, and scheduler rules. |
| `t3` | The join workstation consumes both `joined-item` Work items | One joined dispatch can produce the configured `matched` output. |

The invariant applies to every selected input, not only the input carrying the
`SAME_NAME` guard or the first input listed in the workstation configuration.

## `PARENT_CHILD`

Use `PARENT_CHILD` when a child work item should belong to a parent's child
set for parent-aware fan-in. It records parent lineage on the child Work; it is
not a prerequisite-state gate. Use `DEPENDS_ON` when the child or another Work
must wait for a named state.

```json
{
  "type": "PARENT_CHILD",
  "sourceWorkName": "story-auth",
  "targetWorkName": "story-set"
}
```

Read that as: `story-auth` is a child of `story-set`.

Parent-child batch example:

```json
{
  "requestId": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-set",
      "workTypeName": "story-set",
      "state": "waiting",
      "payload": { "title": "April release story set" }
    },
    {
      "name": "story-auth",
      "workTypeName": "story",
      "payload": { "title": "Harden auth session handling" }
    },
    {
      "name": "story-billing",
      "workTypeName": "story",
      "payload": { "title": "Polish billing retry UX" }
    }
  ],
  "relations": [
    {
      "type": "PARENT_CHILD",
      "sourceWorkName": "story-auth",
      "targetWorkName": "story-set"
    },
    {
      "type": "PARENT_CHILD",
      "sourceWorkName": "story-billing",
      "targetWorkName": "story-set"
    }
  ]
}
```

Place the parent in the waiting `state` consumed by the parent-aware fan-in
workstation in `factory.json`. Children usually start in their work type's
initial state unless you intentionally need non-initial placement.

After validation, the factory attaches each `PARENT_CHILD` relation to the child
work token and sets the child's parent lineage. That lineage is what
parent-aware input guards such as `ALL_CHILDREN_COMPLETE` and
`ANY_CHILD_FAILED` match. See `you docs guards`.

A splitter workstation can also create parent-scoped children at runtime. That
path emits fanout metadata used by `spawnedBy` on input guards. Submitted
`PARENT_CHILD` batches are enough when the full child set is already known up
front.

## `SPAWNED_BY`

`SPAWNED_BY` is usually runtime-derived. It appears on work tokens after a
workstation spawns child work or the factory records spawn lineage during
normalization. Authors do not declare `SPAWNED_BY` in public batch
`relations[]` the way they declare `DEPENDS_ON` or `PARENT_CHILD`.

Template and trace surfaces expose each token's `.Relations` slice. Each entry
includes:

| Field | Description |
|-------|-------------|
| `.Type` | `DEPENDS_ON`, `PARENT_CHILD`, or `SPAWNED_BY` |
| `.TargetWorkID` | Work ID of the related work item |
| `.RequiredState` | State the target must be in for `DEPENDS_ON` |

Use `SPAWNED_BY` when reading spawn lineage in prompts or diagnostics, not
when authoring a new batch file. For batch authoring, use `PARENT_CHILD` when
you need parent membership and `DEPENDS_ON` when you need sibling ordering.
See `you docs templates` for template access to `.Relations`.

## Whole-Batch Validation

The factory validates the full batch before it creates work tokens. Invalid
submissions reject the whole batch. No partial work is created.

Common rejection reasons:

| Problem | Result |
|---------|--------|
| Invalid JSON or retired field aliases | Whole batch rejected |
| Duplicate work names in `works[]` | Whole batch rejected |
| Relation `sourceWorkName` or `targetWorkName` does not match a work name | Whole batch rejected |
| Self-relation (`source` equals `target`) | Whole batch rejected |
| `DEPENDS_ON` cycle between siblings | Whole batch rejected |
| `requiredState` names a state that does not exist on the target work type | Whole batch rejected |
| Unknown `workTypeName` or invalid batch shape | Whole batch rejected |

Declare batch relations by work name. Do not use `targetWorkId` in submitted
batch relations.

## Normalization Outcomes

After a batch passes validation, the factory normalizes it:

1. Missing work IDs are generated as `batch-<requestId>-<work-name>`.
2. Work item tags receive `_work_name` and `_work_type` values.
3. `DEPENDS_ON` relations attach to the blocked source work token.
4. `PARENT_CHILD` relations attach to the child work token and set parent
   lineage for parent-aware guards.
5. Canonical history records a `WORK_REQUEST` event before related work-input
   and relationship-change events.

Independent items in the same batch may dispatch in parallel, subject to the
workflow topology, resource limits, worker capacity, and scheduler rules.

## Parent-Aware Guard Linkage

Parent-aware fan-in pairs a parent input with a guarded child input on the
same workstation. The guard names which parent input to match:

```json
{
  "workType": "task",
  "state": "complete",
  "guards": [
    {
      "type": "ALL_CHILDREN_COMPLETE",
      "parentInput": "story",
      "spawnedBy": "split-story"
    }
  ]
}
```

| Guard need | Submitted batch relation | Runtime splitter path |
|------------|-------------------------|------------------------|
| Parent lineage on children | `PARENT_CHILD` | Child tokens carry parent work ID |
| Known child count for `ALL_CHILDREN_COMPLETE` | Often enough when topology makes the child set explicit | Use `spawnedBy` when the splitter discovers children at execution time |
| Fail parent when any child fails | `PARENT_CHILD` plus `ANY_CHILD_FAILED` on the child input | Same guard types after runtime spawn |

Do not expect `DEPENDS_ON` to create parent lineage. Use `PARENT_CHILD` for
parent-aware guards. Use `DEPENDS_ON` only for sibling prerequisite ordering.

## Batch Relations vs Token `.Relations`

| Surface | What authors write | What runtime exposes |
|---------|-------------------|----------------------|
| Batch file or API `FACTORY_REQUEST_BATCH` | `relations[]` with `DEPENDS_ON` and `PARENT_CHILD` | Normalized attachments on work tokens |
| Factory workstation input | `SAME_NAME` in `workstations[].inputs[].guards[]` | Join selection during scheduler enablement |
| Prompt templates | Not authored directly | `.Relations` on each input token |
| CLI work listings and traces | Not authored directly | Human-readable relation summaries |

Keep detailed batch field tables in `you docs batch-inputs`. Use this
page for relation semantics and scheduling impact.

## Common Mistakes

- Reversing `PARENT_CHILD` direction — keep `sourceWorkName` on the child and
  `targetWorkName` on the parent.
- Using `DEPENDS_ON` where `PARENT_CHILD` is required for parent-aware guards.
- Adding `SAME_NAME` to batch `relations[]` — it belongs on a workstation input
  guard and does not create dependency ordering or parent lineage.
- Assuming a complete `SAME_NAME` join bypasses dependencies on a secondary
  input — every selected Work's `DEPENDS_ON` relations must be satisfied.
- Setting `requiredState` on `PARENT_CHILD` — that field applies only to
  `DEPENDS_ON`.
- Expecting partial work after a validation failure — the whole batch is
  rejected.
- Declaring `SPAWNED_BY` in a batch file instead of using `PARENT_CHILD` or
  runtime spawn paths.

## Related

- `you docs guards`
- `you docs batch-inputs`
- `you docs workstations`
- `you docs config`
- `you docs work`
- `you docs templates`
