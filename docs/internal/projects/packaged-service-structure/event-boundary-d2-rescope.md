# PSS-I05 — D2 event boundary (residual metadata)

`PSS-I05` no longer claims an "event backbone convergence" implementation.
This file is PSS-I05's entire exclusive-path lease: a residual metadata record
of the settled D2 boundary, not a package, service, or generated artifact.

## Settled boundary (see `README.md` § D2 for the full decision)

- **Recordings** remains the canonical JSONL Factory history and replay
  ledger. It cedes nothing to a streaming layer.
- **FND-08** (`event-contract`) retains ownership of Factory event *kinds*
  under `pkg/services/recordings/events/kinds/`.
- **L1 Events** owns the process-local stream: ordering, cursors,
  subscriptions, retention, gaps, backpressure, and Factory Sessions
  response-event extraction.

## Scope fence

PSS-I05 authorizes none of the above implementations. It records that the
three concerns stay separately owned, and its purpose is satisfied once this
record is committed and validated. It introduces no convergence service, no
persistent Events store, no legacy package, no event-kind migration, and no
response-event implementation.
