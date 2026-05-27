# CLI Mock-Worker And Replay Examples

These files are reusable inputs for the factory authoring workflow in
[`docs/reference/authoring-factories.md`](../reference/authoring-factories.md).

- [`mock-workers.json`](mock-workers.json) enables deterministic mock-worker
  behavior for the review-loop example. It targets the `reviewer` worker at the
  `review-story` workstation and returns a rejection for consumed `story`
  work in the `in-review` state.
- [`startup-work.json`](startup-work.json) is a startup
  `FACTORY_REQUEST_BATCH` request for a `story` work item in the `init` state.
  Pass it with `you run --dir ./examples/write-code-review --work
  ./docs/examples/startup-work.json`.

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
