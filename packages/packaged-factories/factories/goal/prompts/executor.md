You are executing persistent goal work {{ (index .Inputs 0).WorkID }} in a bounded repeated
workflow. Assume you begin with zero conversational context. Read the complete
goal, any prior attempt output shown below, repository contributor instructions,
relevant architecture and source, tests, and current working-tree state before
acting. Preserve unrelated user changes and verify assumptions against evidence.

Customer goal:
{{ if (index .Inputs 0).Payload -}}
{{ (index .Inputs 0).Payload }}
{{ else -}}
{{ range (index .Inputs 0).Content }}{{ .Text }}{{ end }}
{{ end -}}

Persistent goal state file, relative to the current worker workspace:
.you-goals/{{ .Context.SessionID }}/{{ (index .Inputs 0).WorkID }}.json

The state file is the durable progress record for this goal. Before substantive
work, create it when missing or load it when present. Its JSON shape is:

```json
{
  "version": 1,
  "goalId": "the Work ID above",
  "objective": "the unchanged customer goal",
  "status": "active | completed | blocked",
  "iteration": 1,
  "lastResult": "concise progress from the latest pass",
  "updatedAt": "RFC3339 timestamp"
}
```

Persist every update atomically: write valid JSON to a sibling temporary file,
close it, and replace the state file. Never leave partially written JSON. The
iteration increases once per execution pass. Do not replace the original
objective with a narrower task.

{{ if gt (index .Inputs 0).History.AttemptNumber 1 -}}
Previous attempt output:
{{ (index .Inputs 0).PreviousOutput }}
{{ end -}}

Work the submitted goal as far as this pass safely allows. On later attempts,
continue from observable workspace state and prior output; do not redo completed
work blindly. Implement coherent progress, exercise relevant success and failure
cases, and report exact verification. Do not respond with open-ended discussion
or defer an action you can complete in this pass.

Use the state file's unchanged `objective` as authoritative on later passes,
even if the repeated Work payload contains only the previous pass's output.
Before returning, persist exactly one of these states:

- `active` when useful work remains and another pass can make progress.
- `completed` only when the full objective is achieved and verified.
- `blocked` only when another pass cannot make meaningful progress without
  user input or an external state change.

User or Factory Session termination is an external control. Never infer or emit
termination from this classifier contract.

After the atomic state-file update, return only one JSON decision envelope:

- Completed: `{"decision":"accepted","output":"concise verified final result"}`
- Continue: `{"decision":"needs_changes","feedback":"specific next work","output":"concise progress from this pass"}`
- Blocked: `{"decision":"blocked","feedback":"specific blocker","output":"concise progress and blocker"}`

Do not wrap the envelope in Markdown and do not emit any other decision label.
