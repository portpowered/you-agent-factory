# Relationships

Use work-item relationships when one submission should order sibling work,
attach children to a parent for fan-in, or when templates and runtime surfaces
need to read how tokens relate to each other.

`you docs relationships` is the canonical guide for `DEPENDS_ON`,
`PARENT_CHILD`, and `SPAWNED_BY` semantics. See [Guards](guards.md) for
parent-aware input guards that match `PARENT_CHILD` lineage, and
[Batch Inputs](batch-inputs.md) for full batch file field tables and watched
folder placement.

## Quick Choice

| Need | Relation type | Where it is declared |
|------|---------------|---------------------|
| One sibling work item must wait for another sibling to reach a state | `DEPENDS_ON` | `relations[]` on a `FACTORY_REQUEST_BATCH` |
| Child membership under one parent for parent-aware guards | `PARENT_CHILD` | `relations[]` on a `FACTORY_REQUEST_BATCH` |
| Read which workstation spawned a child token in templates or traces | `SPAWNED_BY` | Runtime on work tokens; appears in `.Relations` |
| Gate dispatch until a condition is true | Guards, not relations | [Guards](guards.md) |

`DEPENDS_ON` and `PARENT_CHILD` can appear in the same submitted batch when
the workflow needs both prerequisite ordering and parent membership.

## Source And Target Semantics

| Relation type | Source means | Target means | `requiredState` |
|---------------|--------------|--------------|-----------------|
| `DEPENDS_ON` | The blocked work item | The prerequisite work item | Optional. Defaults to `complete`. Names a state on the target work type. |
| `PARENT_CHILD` | The child work item | The parent work item | Not used. Ignore this field on `PARENT_CHILD`. |
| `SPAWNED_BY` | The spawned child work item | The spawning context (for example a parent work item or fanout source) | Not used on batch files. Runtime records the spawn lineage. |

Read `DEPENDS_ON` as: the source waits for the target. Read `PARENT_CHILD` as:
the source is a child of the target.

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
the target reaches the required state.

## `PARENT_CHILD`

Use `PARENT_CHILD` when a child work item should belong to a parent's child
set for parent-aware fan-in. It records parent lineage on the child token.

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
`ANY_CHILD_FAILED` match. See [Guards](guards.md).

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
See [Templates](templates.md) for template access to `.Relations`.

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
| Prompt templates | Not authored directly | `.Relations` on each input token |
| CLI work listings and traces | Not authored directly | Human-readable relation summaries |

Keep detailed batch field tables in [Batch Inputs](batch-inputs.md). Use this
page for relation semantics and scheduling impact.

## Common Mistakes

- Reversing `PARENT_CHILD` direction — keep `sourceWorkName` on the child and
  `targetWorkName` on the parent.
- Using `DEPENDS_ON` where `PARENT_CHILD` is required for parent-aware guards.
- Setting `requiredState` on `PARENT_CHILD` — that field applies only to
  `DEPENDS_ON`.
- Expecting partial work after a validation failure — the whole batch is
  rejected.
- Declaring `SPAWNED_BY` in a batch file instead of using `PARENT_CHILD` or
  runtime spawn paths.

## Related

- [Guards](guards.md)
- [Batch Inputs](batch-inputs.md)
- [Workstations](workstations.md)
- [Config](config.md)
- [Submitted work](work.md)
- [Templates](templates.md)
