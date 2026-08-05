# Event Streams

The product ships two different event contracts. They serve different
lifetimes and must not be combined into a third, normalized hierarchy.

## Worker observations

Workers publish source-native observations through the public `workers` contract
(`pkg/services/workers/response_drafts.go`). An observation is a `workers.Draft`:
its provider provenance identifies the original source, while its `Kind`,
`Phase`, and typed JSON payload describe the observation made available to a
consumer.

The declared `workers.Kind` values are:

- `SESSION`
- `RUN`
- `TURN`
- `MESSAGE`
- `REASONING`
- `TOOL`
- `FILE_CHANGE`
- `PLAN`
- `PROGRESS`
- `USAGE`
- `ERROR`
- `STREAM_GAP`

The declared `workers.Phase` values are `STARTED`, `DELTA`, `UPDATED`,
`COMPLETED`, `FAILED`, and `CANCELED`.

`Kind` and `Phase` are not a Cartesian-product taxonomy. Each emitted
observation uses the combination and typed payload supported by its source and
Worker contract. For example, a `MESSAGE` observation can carry either a
`MessagePayload` snapshot or a `MessageDeltaPayload`; a `TOOL` observation can
carry `ToolPayload` or `ToolDeltaPayload`; and `STREAM_GAP` carries
`StreamGapPayload`. Consumers must preserve the accompanying `Provenance` and
interpret the payload for the specific emitted combination rather than assume
that every kind accepts every phase or payload.

The Events service delivers these observations through a process-local,
in-memory, session-scoped stream. It retains observations only for the active
process/session scope and relays source-native JSON; it does not create another
kind union, durable journal, or replay history.

## Canonical Factory history

Recordings separately owns the canonical Factory history. Its `FactoryEvent`
values are recorded in canonical order and written to Factory Event JSONL
artifacts. Recordings uses that history for replay, historical inspection, and
derived projections of Factory state.

Factory history describes durable Factory facts. It is not a retention layer
for Worker observations, and an Events stream is not a replayable Factory
ledger. Conversely, `FactoryEvent` values are not a replacement vocabulary for
source-native Worker observation payloads.

## Superseded outline

The former nested Factory/Worker/User/Agent/Tool event tree was conceptual
material only. It is superseded and is not a supported event contract. New
consumers must use the `workers.Kind`/`workers.Phase` vocabulary for Worker
observations and Recordings `FactoryEvent` history for canonical Factory
replay.
