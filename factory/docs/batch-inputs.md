# Batch Inputs

This directory records a human-readable example for ideafy/meta-planner batch
submission. These files are documentation, not live factory inputs.

The source of truth for the live file-listener schema is:

```sh
you docs batch-inputs
```

The example JSON uses the canonical `FACTORY_REQUEST_BATCH` shape from
`you docs batch-inputs`:

* submit 3-5 `idea` work items per batch
* submit one loopback `thoughts` work item
* make the loopback depend on the ideas through `DEPENDS_ON` relations
* use `workTypeName`, not `workType`
* use `works[]`, not `items[]`
* prefer `you submit batch <path>` for autonomous meta-planner submission when
  the factory is already running

Before submitting a real batch, run:

```sh
you submit batch --dry-run factory/docs/batch-input-example.json
```

