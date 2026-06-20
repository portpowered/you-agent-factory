# Reviewer/Checker Decision Envelope

Packaged `@you/goal` reviewer and checker workers return one small JSON object
instead of ad hoc text markers. The runtime maps that envelope directly onto the
existing `WorkResult` contract.

Authoritative parsing lives in `pkg/packagedfactories/goal/decision_envelope.go`.

## Envelope shape

| JSON field | Required | Maps to `WorkResult` |
| --- | --- | --- |
| `decision` | yes | `Outcome` |
| `feedback` | yes | `Feedback` |
| `output` | no | `Output` |
| `recorded_output_work` | no | `RecordedOutputWork` |

Accepted `decision` values reuse the existing work-outcome vocabulary:

| `decision` | Routing meaning |
| --- | --- |
| `ACCEPTED` | Work passed review or check; follow the workstation's accepted path. |
| `CONTINUE` | More executor work is required before the lane can finish. |
| `REJECTED` | Review or check failed; follow the workstation's rejection path. |
| `FAILED` | The reviewer/checker hit a runtime failure while evaluating the work. |

## Example

```json
{
  "decision": "ACCEPTED",
  "feedback": "All acceptance criteria pass. PR is ready to merge.",
  "output": "Merged PR #842 after green checks.",
  "recorded_output_work": [
    {
      "id": "work-task-24",
      "workTypeId": "task",
      "state": "to-complete",
      "traceId": "trace-example-001"
    }
  ]
}
```

`output` and `recorded_output_work` are optional. Omit them when the decision
and feedback are sufficient.

`recorded_output_work` entries use the same work-item JSON shape as submitted
work (`workTypeId`, `traceId`, and related fields).

## Malformed envelope behavior

Invalid reviewer/checker output must not be silently coerced into a routing
decision.

| Input problem | Result |
| --- | --- |
| Output is empty or not valid JSON | `WorkResult.Outcome` is `FAILED`. `WorkResult.Error` carries actionable parse text. |
| JSON parses but `decision` is missing or unknown | `WorkResult.Outcome` is `FAILED`. `WorkResult.Error` names the decision problem. When the JSON shape parsed, any reviewer `feedback` is preserved in `WorkResult.Feedback` for inspection. |

Malformed envelopes never masquerade as `ACCEPTED`, `CONTINUE`, or `REJECTED`.

## Where to author responses

- Review workstation prompt: `factory/workstations/review/AGENTS.md`
- Checker or review-style prompts in other packaged goal factories should use
  the same envelope and vocabulary.

When editing prompts, keep customer-facing terms (`Outcome`, `Output`,
`Feedback`, recorded output work) and avoid internal Petri-net terminology.
