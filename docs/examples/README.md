# CLI Mock-Worker And Replay Examples

These files are reusable inputs for the factory authoring workflow in
`you docs authoring-factories`.

`javascript-workflows/` contains the runnable authoring examples documented by
`you docs javascript-workflows`. Focused runtime tests execute these exact files
so the published primitive usage stays aligned with the shipped host API.

- [`mock-workers.json`](mock-workers.json) enables deterministic mock-worker
  behavior for the review-loop example. It targets the `reviewer` worker at the
  `review-story` workstation and returns a rejection for consumed `story`
  work in the `in-review` state.
- [`mock-workers-script.json`](mock-workers-script.json) runs a local script
  command for the `executor` worker at `execute-story` instead of returning a
  synthetic accept or reject result.
- [`mock-workers-mixed.json`](mock-workers-mixed.json) keeps the reviewer
  rejection mock and sets `unmatchedDispatchPolicy: "passthrough"` so unmatched
  dispatches execute through the normal worker path.
- [`startup-work.json`](startup-work.json) is a startup
  `FACTORY_REQUEST_BATCH` request for a `story` work item in the `init` state.
  Pass it with `you run --dir ./examples/write-code-review --work
  ./docs/examples/startup-work.json`.
- [`factory-validation/unsupported-three-input-join.json`](factory-validation/unsupported-three-input-join.json)
  is a deliberately rejected three-input `SAME_NAME` plus
  `ALL_CHILDREN_COMPLETE` join for the `you docs factory-validation` guide.
- [`factory-validation/supported-two-input-join.json`](factory-validation/supported-two-input-join.json)
  is the corrected two-input boundary for that validation example.

Run the example workflow with mock workers and startup work:

```bash
you run --dir ./examples/write-code-review --with-mock-workers ./docs/examples/mock-workers.json --work ./docs/examples/startup-work.json
```

Record that same run to an explicit replay artifact path:

```bash
you run --dir ./examples/write-code-review --with-mock-workers ./docs/examples/mock-workers.json --work ./docs/examples/startup-work.json --record ./docs/examples/sample-run.replay.json
```

Replay a saved artifact later:

```bash
you run --dir ./examples/write-code-review --replay ./docs/examples/sample-run.replay.json
```

Do not commit generated replay artifacts from real customer runs. Replay files
can contain prompts, payloads, stdout, stderr, and diagnostic metadata.
