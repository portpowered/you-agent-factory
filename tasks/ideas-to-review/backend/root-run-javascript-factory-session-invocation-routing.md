# Route root-run JavaScript Factory invocation through the JavaScript session owner

## Problem

The production-shaped root-run HTTP host can load a Factory whose orchestrator
is JavaScript, but `POST /factory-sessions/~default/invocations` is currently
routed through `runtimehost.sessionInvocationAPI` and the Petri invocation
owner. The request fails with:

`resolve invocation work type: expected exactly one work type with handlingBehavior DEFAULT for simplified prompt runs`

The compatibility `FactoryService` path detects a JavaScript orchestrator and
uses the durable JavaScript execution service instead, so the two composed API
surfaces disagree.

## Why this matters

Packaged JavaScript Factories such as `@you/deep-research` cannot be exercised
through `RootRunFunctionalHost` plus the documented invocation REST operation.
This blocks migration away from the legacy functional API server and represents
a real customer-boundary behavior mismatch.

## Proposed outcome

- Give the root-run API surface the same orchestrator-aware invocation routing
  as the canonical Factory Session owner.
- Keep JavaScript execution behind the durable session execution contract; do
  not add JavaScript policy to the HTTP handler.
- Add a root-run REST test using a deterministic injected provider edge that
  invokes a JavaScript Factory and confirms its terminal response and dispatch
  read through public APIs.
- Retain a failure test proving Petri and JavaScript invocation errors preserve
  their documented public taxonomy without leaking internal diagnostics.
