# Implementing a model provider integration

The provider-owned execution boundary is `pkg/services/providers`. A native
integration exposes one normalized attempt through `providers.Service.Execute`;
Providers also owns catalog selection, provider-session identity, and
continuation. Workers owns work scheduling, retries, throttling, and output
policy around that boundary.

Provider-native commands, HTTP or SDK calls, event decoding, session
extraction, and failure classification stay in the provider adapter. Shared
orchestration must not branch on provider identity.

## 1. Implement one normalized execution attempt

The parent-private adapter seam is:

```go
type Attempt func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)
```

Build the request from provider-neutral fields and return a detached
`providers.ExecuteResult`. Content, an optional `SessionRef`, and bounded
`ExecuteDiagnostics` are the only successful attempt facts that cross the
boundary. A provider may report bounded live progress through
`ExecuteRequest.ProgressObserver` and its final diagnostics.

Validate provider identity and attempt metadata before invoking native I/O.
Do not put retry, fallback, scheduling, or throttle policy in the adapter.
Provider-specific failures must be normalized to `providers.ExecuteFailure`
with a safe message and one of the public `ExecuteFailureKind` values. Never
return raw command output, credentials, prompts, machine-local paths, or live
process objects as diagnostics.

## 2. Use Providers execution and continuation contracts

Callers invoke an adapter only through the Providers root:

```go
result, err := service.Execute(ctx, providers.ExecuteRequest{
	Provider:  providerID,
	AttemptID: attemptID,
	UserMessage: prompt,
})
```

`Execute` owns exactly one normalized attempt and returns typed cancellation,
timeout, capability-mismatch, and normalized execution failures. It does not
accept a caller-supplied provider session. Native adapters report a detached
session with `ExecuteRequest.SessionObserver` or `ExecuteResult.SessionRef`.

Resume an exact provider session through `Service.Continue` or
`Service.ContinueReference`. Continuation validates provider identity and
session lineage before adapter I/O, and returns a typed resumed or unsupported
outcome. It never falls back to ordinary `Execute`.

Structured output remains part of the provider-neutral execution request and
result path; it does not require a response-stream executor or a second
inference contract.

## 3. Run native behavioral tests

Use the parent-private Providers execution conformance package at
`pkg/services/providers/internal/testutil/execution`. It exercises detached
success results, optional sessions, ordered bounded progress, normalized
declared and parse failures, cancellation, deadlines, cleanup, and late
success suppression through `providers.Service`.

Provider-specific behavior belongs beside the adapter under
`pkg/services/providers/internal/services/execution/internal/adapters/<id>`.
Keep root contract assertions in `pkg/services/providers` and composition
assertions in `pkg/services/providers/wire`. Prefer observable execution
results, typed failures, progress observations, session references, and
continuation outcomes over source-shape checks.

Run focused package tests with the race detector when an adapter owns
concurrent process, cancellation, progress, or cleanup behavior.

## Completion checklist

- The provider has a catalog descriptor and truthful capability facts.
- One native adapter attempt returns detached `ExecuteResult` values.
- Failures are normalized, typed, bounded, and customer-safe.
- Progress and session observations are ordered, bounded, and detached.
- Structured output, cancellation, and continuation are covered through the
  Providers contracts.
- Focused adapter, root contract, composition, and conformance tests pass.
- No Workers provider registry, response-stream executor, legacy inference
  contract, Factory Session ownership, or generated API artifact is added.
