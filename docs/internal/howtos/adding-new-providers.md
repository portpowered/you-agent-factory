# Implementing a model provider integration

The customer-implementable provider inference boundary is
`pkg/services/workers/provider/inferencecontract`. It is deliberately separate
from built-in provider registration and Factory Session publication. This
guide covers implementing and validating that public Go contract; wiring a new
built-in provider into production is a separate migration.

Provider-native commands, HTTP or SDK calls, event decoding, session
extraction, and failure classification stay in the provider package. Shared
orchestration must not branch on provider identity.

## 1. Implement the public contract

Implement `inferencecontract.Integration`:

```go
type Integration interface {
	Identity() inferencecontract.Identity
	MaximumCapabilities() inferencecontract.CapabilitySet
	Discover(context.Context) (inferencecontract.Discovery, error)
	Capabilities(context.Context, inferencecontract.InvocationRequest) (inferencecontract.CapabilitySet, error)
	Invoke(context.Context, inferencecontract.InvocationRequest, inferencecontract.ResponseWriter) error
}
```

`Identity` is an opaque, stable identifier. Do not use it to select special
orchestration behavior. `MaximumCapabilities` declares the integration's
maximum provider-neutral capability set. `Capabilities` returns the subset
available for one immutable request and must never escalate beyond that
maximum.

`Discover` returns current readiness plus sanitized prerequisites. Report only
bounded setup guidance and prerequisite names or statuses. Never include
credentials, raw environment assignments, machine-local paths, prompts, or
provider-native payloads.

Use the public validators while developing an integration:

```go
if err := inferencecontract.ValidateIdentity(integration.Identity()); err != nil {
	return err
}
maximum := integration.MaximumCapabilities()
if err := inferencecontract.ValidateMaximumCapabilities(maximum); err != nil {
	return err
}
discovery, err := integration.Discover(ctx)
if err != nil {
	return err
}
if err := inferencecontract.ValidateDiscovery(discovery); err != nil {
	return err
}
negotiated, err := integration.Capabilities(ctx, request)
if err != nil {
	return err
}
if err := inferencecontract.ValidateNegotiatedCapabilities(maximum, negotiated); err != nil {
	return err
}
```

## 2. Emit drafts and one completion

`Invoke` receives an immutable `InvocationRequest` and a `ResponseWriter`.
Construct provider-neutral `EventDraft` values with
`inferencecontract.NewEventDraft`, then write them in observation order. The
draft vocabulary is owned by `pkg/services/workers`; see
[`response-events.md`](../contract/response-events.md) for its semantic kinds,
phases, and payloads.

Provider code supplies semantic correlation and provenance, but never Factory
Session envelopes, event IDs, timestamps, publication sequence numbers,
retention metadata, replay gaps, or `STREAM_GAP` drafts. Message and tool
lifecycles retain stable item or tool correlation. A provider may emit at most
one authoritative `MESSAGE/COMPLETED` snapshot representing final response
content.

Close the writer exactly once with either:

- `SuccessfulCompletion(NewResponse(...))`, or
- `FailedCompletion(NewFailure(...))`.

The response is authoritative. If an authoritative completed message was
emitted, its content must agree with that response. A completed success message
cannot be followed by a failure. Stop provider observation and return
immediately when `WriteEvent` fails; later writes or closes cannot replace the
first terminal outcome.

Normalize failures into one of the public failure kinds: authentication,
invalid request, throttling, timeout, cancellation, dependency failure,
malformed provider output, or unknown failure. Messages and diagnostics must
be bounded and safe for customers. Optional `ProviderSession` metadata is
generic and detached; it does not expose transcript history or give the
integration ownership of Provider Session lifecycle.

Shared orchestration must invoke an implementation through
`inferencecontract.ExecuteInvocation`. That boundary validates correlation,
lifecycle order, terminal agreement, sink backpressure, and exactly-once close
before buffered terminal drafts reach orchestration.

## 3. Run the reusable conformance suites

Use `pkg/services/workers/provider/inferencecontract/testkit` with deterministic,
sanitized fixtures. `testkit.Run` requires fresh integration factories for
final-only, streaming, and correlated tool-lifecycle success. Supply at least
two distinct valid identities to prove behavior is identity-neutral:

```go
testkit.Run(t, testkit.Suite{
	Identities: []inferencecontract.Identity{"customer.alpha", "customer-beta"},
	FinalOnly:  finalOnlyFactory,
	Streaming:  streamingFactory,
	Tool:       toolFactory,
	Fixture: testkit.Fixture{
		FinalOnlyRequest: finalOnlyRequest,
		StreamingRequest: streamingRequest,
		ToolRequest:      toolRequest,
		ExpectedResponse: expectedResponse,
	},
})
```

Also run `testkit.RunAdverse` with fresh factories for every field in
`testkit.AdverseSuite`. It proves all normalized failure categories,
cancellation, timeout, response-sink backpressure, double close, write after
close, missing close, response/event disagreement, failure after a represented
success, and conflicting authoritative completed messages. Run the provider
contract tests with the race detector because cancellation, backpressure, and
close behavior exercise concurrency.

The conformance suites require no Factory Session store, provider executable,
credential, worktree, transcript reader, generated API artifact, or process
composition.

## Completion checklist

- Identity, maximum capabilities, discovery, and negotiated capabilities pass
  their public validators.
- Discovery and normalized failure details are bounded and customer-safe.
- Draft lifecycles are ordered and correlated, with no Factory-owned fields.
- The writer closes exactly once and final event content agrees with the
  authoritative response.
- Success and adverse conformance suites pass for at least two opaque
  identities.
- Focused package tests and race-enabled protocol tests pass.
- No built-in provider switch, generated API, Factory Session ownership, or
  production registration migration was added as part of the contract work.
