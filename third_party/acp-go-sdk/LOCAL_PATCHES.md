# Local ACP SDK repair

This directory preserves `github.com/coder/acp-go-sdk` v0.13.5 from commit
`0845a3bb9eddda5bfc22a94dd3598c90cb842451`. Its Apache-2.0 license and every
upstream file except `connection.go`, `client.go` and four example imports are unchanged. The original 79-file,
1,075,368-byte inventory and hashes are in `upstream-provenance.json`.

The SDK is part of the root module at
`github.com/portpowered/infinite-you/third_party/acp-go-sdk`. Consumers and the
four upstream examples use that namespace. The original 44-byte module file is
preserved as `upstream-go.mod.txt`; there is no nested module or `replace`
directive. This arrangement supports versioned `go install` of the root CLI.
The upstream dependency and its two checksum entries are removed.

ACP wire schema is unchanged. The additive `Joined` observation on Connection and
ClientSideConnection reports actual resource completion without changing `Done`. Go callers use the
root-owned SDK import namespace. No generated SDK file or model setting changes.
## Repair

- A buffered response survives the receive EOF that follows it. A missing
  response still produces the existing typed disconnect failure.
- A delivered response waits for its already received notification watermark
  across EOF. The existing five-second inbound drain remains unchanged;
  an unfinished drain still fails safely.
- Caller cancellation or deadline takes precedence over a buffered response,
  an already completed or zero notification watermark, and final publication.
  Cancellation during JSON decoding returns a zero result with the caller error.

The response repair changes `connection.go`: `SendRequest`, `SendRequestNoResult`,
`waitForResponse`, `waitNotificationsUpTo`, and the private `callerEndedErr`
helper. Its current SHA256 is `c3bc7c66d3876152f3a272f2e408618eb4129822f6a847f8e16a0e2f79296c7f` (27905 bytes).

## Resource completion observation

- `Connection.Joined` closes after the reader, sequential notification dispatcher,
  cancellation writer, shutdown helpers and admitted inbound handlers return.
  Initial loops are registered before launch and retain their registration while
  admitting child work. Disconnect retains its existing `Done` meaning.
- Response/drain wake helpers join before their owning wait returns. Blocked I/O
  or callbacks keep `Joined` open; this signal does not stop resources itself.
- `ClientSideConnection.Joined` exposes the same signal to the Providers attempt
  owner. `connection_joined_test.go` holds reader, callback and cancellation-write
  gates to protect the distinction. EOF/watermark, cancellation precedence and
  the five-second notification drain policy remain unchanged.

## Evidence and verification

In-memory channel barriers force the response, EOF, and cancellation ordering.
The original SDK discarded 10 of 16 delivered initialize responses. A held
pre-response notification plus final response and EOF failed all 16 trials.
The first EOF repair still falsely succeeded in 14 of 16 already canceled
requests; the completed repair checks caller state before returning results.
These trials establish concrete defects without attributing every earlier
OpenCode timeout or CI failure to them.

`connection_eof_repair_test.go` retains eleven regression cases, including missing
responses, held callbacks, incomplete drain, cancellation, deadline, zero and
completed watermarks, and cancellation during decoding. The full SDK suite and
existing notification-barrier tests passed under the race detector.

Run from the repository root:

```sh
make test-acp-sdk
```

The Backend Unit Coverage job explicitly runs this target because root
`./pkg/...` tests do not discover this preserved dependency package's tests. The compiled
provider EOF boundary is separately exercised with an existing CLI artifact:

```sh
INFINITE_YOU_INTEGRATION_BINARY=/path/to/you make test-acp-provider-prebuilt
```

That target never builds the CLI; the integration lane supplies its previously
built artifact. Source acceptance and the actual compiled-boundary result must
be recorded independently.

## Maintaining the copy

Verify all paths against the original hashes in `upstream-provenance.json`;
only `connection.go`, `client.go`, local repair tests/notes and the four recorded
example import changes should differ.
The module file retains its original bytes under its recorded archival path. Update its local hash and patch scope when
changing this repair. Preserve the upstream license and generated-file bytes.
When a released upstream SDK fixes these boundaries, review that version and
remove or refresh the preserved source copy and namespace migration.

## Lint scope

Pristine v0.13.5 produces twelve `go vet` unreachable-code diagnostics in its
unchanged generated `types_gen.go`. Root vet retains all nine authored Go roots:
`cmd`, `contracts`, `docs`, `internal`, `packages`, `pkg`, `scripts`, `tests`,
and `ui`. Preserved third-party code is treated as a dependency, as it was before
hosting its source locally. The SDK regression suite still runs the race detector
and Go test's default vet analyzers. The authored repair and tests are reviewed
separately; no owned-source lint gate or baseline is relaxed.
