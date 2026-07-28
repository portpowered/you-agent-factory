# Carry Provider Session references on Providers execution failures

## Problem

`providers.ExecuteResult` can return a detached `SessionRef` on success, but
`providers.ExecuteFailure` has no equivalent field. A provider may establish a
new resumable session before an attempt fails, so peers translating a typed
failure cannot retain that identity unless it was already present on the input
request.

The Workers Agent Runner can preserve an input resume session on failure, but it
cannot recover a new failure-time Provider Session without inspecting
provider-private state or reparsing provider-native diagnostics. Both would
violate the Providers ownership boundary.

## Proposed direction

- Add an optional detached `SessionRef` to the Providers one-attempt failure
  contract.
- Normalize and validate it beside failure diagnostics in the Providers
  execution service, including same-provider enforcement and clone isolation.
- Update provider adapters only through their existing Providers-owned
  normalization path.
- Add contract and conformance coverage proving failure session identity is
  detached, safe, provider-matched, and available to Workers without a
  backquery or private downcast.

Keep retry policy outside Providers and Workers Runners; this field is
correlation data, not permission to retry.
