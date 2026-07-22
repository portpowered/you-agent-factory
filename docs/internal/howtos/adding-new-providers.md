# Adding a model provider

> This guide describes the target workflow in
> [`provider-integration-convergence.md`](../../temp/standardized-providers/provider-integration-convergence.md).
> Use it as the implementation checklist while the standardized manifest and
> inference-contract packages are being introduced.

A provider consists of two deliberately separate pieces:

1. A standardized manifest describes identity, technical support level,
   discovery, and the maximum capability set.
2. One Go integration implements request-time capability negotiation and
   invocation through the structured response writer.

Provider-native commands, HTTP/SDK calls, PTY handling, event decoding, session
extraction, and failure classification belong in the provider's package. Shared
orchestration must not switch on the provider ID.

## 1. Author the manifest

For a first-party provider, add
`packages/model-providers/providers/<provider>/provider.yaml`. External
integrations use the same published JSON Schema without editing the first-party
catalog.

An illustrative manifest is:

```yaml
schemaVersion: you-agent-factory.model-provider.v1
id: acme
displayName: Acme Models
description: Runs Acme models through the Acme CLI.
aliases: []
technicalSupportLevel: experimental
implementation: bundled
documentation:
  reference: /providers/acme
discovery:
  executables:
    - acme
  configurationKeys:
    - ACME_API_KEY
capabilities:
  execution:
    - prompt_submission
    - tool_execution
    - session_resume
    - working_directory
  response:
    - native_streaming
    - message_snapshots
    - tool_lifecycle
    - usage
    - stable_item_ids
```

Use `production` only for supported, release-quality integrations;
`experimental` for runnable integrations whose contract or behavior may still
change; and `not-supported` only for catalog entries that document a known gap.
A `not-supported` entry is not runtime-selectable.

The capability lists are maximums, not claims about every request. Do not put
credentials, live readiness, machine-local paths, pricing, or provider-native
event payloads in the manifest. Run the provider-manifest generator and drift
check introduced by the convergence plan, and commit generated artifacts with
the authored manifest.

## 2. Implement the integration

Create `pkg/services/workers/provider/<provider>`. Implement only the public
inference contract:

```go
type Integration interface {
	Manifest() inferencecontract.Manifest
	Capabilities(context.Context, inferencecontract.ProviderInferenceRequest) (inferencecontract.Capabilities, error)
	Invoke(context.Context, inferencecontract.ProviderInferenceRequest, inferencecontract.ResponseWriter) error
}
```

For a built-in, `Manifest()` returns the typed manifest loaded from the embedded
`packages/model-providers` catalog; do not repeat aliases, support level,
discovery data, or maximum capabilities as Go literals. `Capabilities` returns
the subset available for the specific request and environment. It must never
return capabilities outside the manifest maximum.

`Invoke` maps native output to the existing response-event drafts in
`pkg/services/workers/response_drafts.go` and writes them in observation order.
Use the documented response-event vocabulary in
[`response-events.md`](../contract/response-events.md). The runtime owns event
IDs, Factory Session IDs, timestamps, sequence numbers, retention, and
`STREAM_GAP`; provider code must not create them.

Close the response writer exactly once with either the authoritative inference
response or a normalized failure. Stop and return the error if writing an event
fails. Provider failures and diagnostic summaries must be bounded and must not
expose prompts, credentials, environment values, or unsafe native payloads.

## 3. Register it

Bind the manifest and integration by canonical provider ID through the typed
provider-registration edge consumed by `root.BuildProcess`. Production
composition registers a built-in once; customers and tests append named
registrations without replacing unrelated providers.

Do not add the provider to an OpenAPI enum, central switch, alias map,
capability map, CLI catalog, or documentation list. The registry and generated
manifest own those projections. A missing manifest/implementation pair,
duplicate ID or alias, and a negotiated capability outside the manifest must
fail during composition or validation.

## 4. Prove conformance and public behavior

Add sanitized native fixtures and run the shared
`pkg/services/workers/provider/inferencecontract/testkit`. At minimum prove:

- manifest validity and capability-subset enforcement;
- successful invocation, ordered valid drafts, and one terminal completion;
- cancellation, timeout, dependency failure, malformed output, and writer
  failure;
- stable message/tool correlation and correct final-response selection; and
- bounded, safe failures and diagnostics.

Add customer-scale tests under `tests/functional/providers/<provider>/`:

```text
invoke_test.go
failure_test.go
stream_test.go       # only when native streaming is declared
session_test.go      # only when session resume/identity is declared
transport_test.go    # PTY, Windows prompt, negotiation, or other bespoke IO
```

Functional tests enter through `root.BuildProcess` and replace only external
effects through typed edges. They must not import the provider implementation
or construct an alternate service graph. Verify the provider appears in
manifest-backed discovery with the expected support level and capability set,
then prove invocation and failure behavior through the public boundary.

## Completion checklist

- The authored manifest validates and generated catalog drift checks pass.
- Website/reference generation sees the provider without a manual inventory
  edit.
- The Go integration owns all provider-native behavior.
- Negotiated capabilities are a subset of the manifest maximum.
- Response drafts validate and the writer is closed exactly once.
- Shared conformance and provider-scoped functional tests pass.
- No central provider switch, enum, alias list, or capability table changed.
