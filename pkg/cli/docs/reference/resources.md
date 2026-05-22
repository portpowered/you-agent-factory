---
author: Agent Factory Team
last-modified: 2026-05-21
doc-id: agent-factory/reference/resources
---

# Resources

`you docs resources` stays available as the stable packaged bounded-concurrency
quick reference. Use
[`docs/reference/resources.md`](../../../docs/reference/resources.md) for the
maintained resource guide.

Resources are bounded capacity pools that limit concurrent dispatches across
workstations. They are declared once at the top level and then consumed by
workstations while a you-agent-factory dispatch is active.

## Example

```json
{
  "resources": [
    { "name": "agent-slot", "capacity": 2 }
  ],
  "workstations": [
    {
      "name": "execute-story",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    }
  ]
}
```

## How Resources Work

- Each declared resource creates `<resource>:available` tokens equal to its
  `capacity`.
- A workstation `resources` entry consumes that capacity while the dispatch is
  running.
- The runtime returns the capacity when the dispatch completes, fails, rejects,
  or emits generated work and exits the in-flight path.

## Resource Fields

| Field | Description |
|-------|-------------|
| `resources[].name` | Stable resource name. |
| `resources[].capacity` | Total available capacity for that resource. |
| `workstations[].resources[].name` | Resource name consumed by the workstation. |
| `workstations[].resources[].capacity` | Capacity units held during dispatch. |

## Authoring Rules

- Declare resources at the top level before referencing them from workers or
  workstations.
- Use positive capacity values.
- Use resources for concurrency limits, not for loop breaking or timeouts.

## Related

- `you docs config`
- `you docs workstation`
- `you docs workers`
