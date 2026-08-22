# ACP baseline capture runbook

How to record what a real ACP agent does on the wire, and diff it against
`you server acp`.

This exists so "are we correct?" is answered by a computed comparison rather
than an opinion. The captures live in
`docs/internal/projects/acp-program/baselines/`.

## Why a raw driver

`cmd/acpbaseline` drives a target agent with a hand-rolled JSON-RPC client,
deliberately not the ACP SDK. Decoding into SDK structs drops unknown fields
and reorders keys — exactly the signal a comparison against a third party
exists to find. Frames are recorded verbatim through the same
`pkg/platform/wiretranscript` tee the ACP server uses, so a captured third
party and our own server produce comparable transcripts.

## Capture our own server

Needs no external agent and no credentials:

```bash
make acp-baseline-self
```

## Capture a third-party agent

```bash
make acp-baseline-capture ACP_AGENT='cursor-agent acp' ACP_BASELINE_NAME=cursor-agent
```

Exit code `3` means a human must act before capture can run, and the message
says what. Prerequisites per agent:

| Agent | Install | Authenticate |
|---|---|---|
| `cursor-agent` | see `cursor.com/install` | `cursor-agent login`, or export `CURSOR_API_KEY`. Verify with `cursor-agent status`. |
| `claude-code-acp` | `npm i -g @zed-industries/claude-code-acp@<pinned>` — pin it, as `pinned_acpx_test.go` pins `acpx@0.13.0`, or the baseline is not reproducible | reuses existing `claude` credentials; otherwise `claude setup-token` or `ANTHROPIC_API_KEY` |

`cursor-agent acp` is an undocumented subcommand and is absent from `--help`.
It is real; `cursor-agent help acp` confirms it.

The agent runs with an env allowlist (`PATH`, `HOME`, `USER`, `SHELL`,
`TMPDIR`, plus per-agent auth keys) in a fresh copy of a fixed fixture
workspace, so re-captures stay comparable.

## Compare

```bash
make acp-baseline-compare
```

Writes `comparison-matrix.md`. Verdicts are computed, never authored:

- **GAP** — at least one third party exhibits it and we do not. Every GAP row
  is a work item.
- **EXTRA** — only we exhibit it.
- **PARITY** — presence matches, including when nobody has it.

Model and option identities are account-entitlement-scoped, so verdicts key on
a capability's existence and category, never on the exact option ids.

Read the caveats first. A capture environment without a model provider produces
no assistant text, which would otherwise be indistinguishable from an agent
that cannot produce assistant text at all; the matrix says so when it happens.

## What may be committed

Four tiers. Only the last two are ever committed.

| Tier | Where | Committed |
|---|---|---|
| raw — verbatim frames | `.artifacts/acp-baseline/` (gitignored), mode `0600` | **never** — treat as a secret |
| scrubbed — pattern-redacted | same directory | **never** — best effort, not a boundary |
| digest — structural only | `docs/internal/projects/acp-program/baselines/<agent>/<date>/` | yes |
| matrix + manifest | same directory | yes |

The digest preserves every object key, array length, boolean, number, and null,
plus string values at a small allowlist of protocol-significant keys
(`method`, `sessionUpdate`, `kind`, `status`, `stopReason`, …). Every other
string becomes `<str:len=N>` or, past 120 characters, a length-and-hash marker.
Request ids become ordinals and timestamps are dropped, so a re-capture with
unchanged behavior diffs to nothing.

A digest is therefore structurally incapable of carrying a prompt, a file body,
or a credential.

```bash
make acp-baseline-check
```

is the enforcement, not this document: it fails the build on an undigested
string, a secret pattern, a machine-specific path, or a file over 512 KB.
`TestCommittedBaselinesStayPublishable` runs the same check in the unit lane.

Retain at most two dated captures per agent. `git log` is the archive.

## Promoting a finding into a test

A digest is evidence, not a test. When a capture reveals a real behavioral
difference, hand-write the three to eight revealing frames as `Case` entries in
`pkg/transports/acp/internal/testutil/acpfixtures/testdata/`, choosing the
`Expected` deliberately. Do not dump a transcript there: that corpus is a
closed, hand-curated conformance set both ACP directions assert against, and a
few thousand frames would destroy its meaning.
