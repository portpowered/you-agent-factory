# Reviewer/Checker Decision Envelope

Reviewer and checker workers that use the decision-envelope contract return one
small JSON object instead of ad hoc text markers. The runtime maps that
envelope directly onto the existing `WorkResult` contract.

Authoritative parsing lives in `pkg/services/factory_definitions/packages/goal/decision_envelope.go`.
Workstations that emit this envelope set `outcomeFormat: "decision-envelope"` in
`factory.json` so the agent executor routes output through the envelope parser
instead of stop-token markers.

Two routing modes share the same JSON shape:

- Standard decision-envelope workstations route on `WorkResult.Outcome`.
- Goal-routing decision-envelope workstations also author
  `classificationRoutes`; those workstations route on the parsed `decision`
  label through `SelectedClassificationLabel` instead of the standard outcome
  vocabulary.

## Envelope shape

| JSON field | Required | Maps to `WorkResult` |
| --- | --- | --- |
| `decision` | yes | `Outcome` |
| `feedback` | yes | `Feedback` |
| `output` | no | `Output` |
| `recorded_output_work` | no | `RecordedOutputWork` |

## Standard outcome vocabulary

Workstations that use the envelope without `classificationRoutes` reuse the
existing work-outcome vocabulary:

| `decision` | Routing meaning |
| --- | --- |
| `ACCEPTED` | Work passed review or check; follow the workstation's accepted path. |
| `CONTINUE` | More executor work is required before the lane can finish. |
| `REJECTED` | Review or check failed; follow the workstation's rejection path. |
| `FAILED` | The reviewer/checker hit a runtime failure while evaluating the work. |

## Goal-routing vocabulary

Packaged `@you/goal` structured review workstations use the same JSON fields,
but when `classificationRoutes` are present the `decision` field must use the
goal-routing labels understood by
`WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed`:

| `decision` | Routing meaning |
| --- | --- |
| `accepted` | Route goal work to the authored complete state. |
| `needs_changes` or `needs-changes` | Route goal work back to the authored rework lane. |
| `tests_failed` or `tests-failed` | Route goal work back to the authored rework lane because verification failed. |
| `needs_human` or `needs-human` | Route goal work to the authored human-escalation state. |
| `blocked` | Route goal work to the authored blocked state. |
| `interrupted` | Route goal work to the authored interrupted state when present. |
| `failed` | Route goal work to the authored failure state. |

Uppercase outcome decisions such as `CONTINUE` and `REJECTED` are valid only
for standard outcome-routing workstations. They are not valid substitutes for
goal-routing labels on a packaged `@you/goal` structured review lane.

## Example

Standard outcome-routing example:

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

Goal-routing example:

```json
{
  "decision": "needs_changes",
  "feedback": "The branch is not mergeable yet; fix the failing runtime test.",
  "output": "Rework required before the goal can complete."
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

Malformed envelopes never masquerade as a valid routing decision for either
mode.

## Where to author responses

- Packaged `@you/goal` authored prompts: `pkg/services/factory_definitions/packages/definitions/goal/prompts/`
- Checker or review-style prompts in other packaged goal factories should use
  this envelope shape and the decision vocabulary that matches their routing
  mode.

When editing prompts, keep customer-facing terms (`Outcome`, `Output`,
`Feedback`, recorded output work) and avoid internal Petri-net terminology.
