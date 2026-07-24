# Packaged Service Structure — Changed-Path Lease and Packet-State Manifest

Program metadata owned by **FND-10**. Planners update packet leases and states
here (and in co-located validator fixtures) without editing generated OpenAPI,
CLI manifests/generators, or provider registry/conductor surfaces.

## Authority

| Artifact | Role |
| --- | --- |
| [`path-lease-packet-manifest.json`](./path-lease-packet-manifest.json) | Machine-readable lease + packet-state ledger |
| `internal/psslease` | Decode/validate contract; focused fixture tests |
| `docs/temp/projects/packaged-service-structure/plan.md` | Source plan prose and Changed-Path Lease Matrix |

`docs/temp/**` remains local planner working state (gitignored). The committed
ledger under this directory is the reviewable program-metadata source for
scheduling evidence.

## Packet record shape

Each packet entry requires:

- `packetId` — stable catalog ID (for example `FND-10`)
- `exclusivePaths` — non-empty exclusive changed-path set the packet may edit
- `state` — exactly one of `blocked`, `ready`, `active`, `review`,
  `integration`, `done`
- `prerequisites` — optional list of packet IDs that must be satisfied first
- `leaseClass` — optional Changed-Path Lease Matrix label for reviewers

Undispatched packets use `blocked` or `ready` according to prerequisite
readiness. Do not place packets into lease-holding states until a planner
intentionally dispatches them.

## Required catalog (Phase 0 + shared lanes)

The committed ledger must include every Phase 0 `FND-01`..`FND-12` foundation
packet plus shared foundation/integration lane IDs `PSS-F01`, `PSS-F02`, and
`PSS-I01`..`PSS-I05`. `internal/psslease.ValidateCatalog` rejects manifests that
omit any of those IDs, duplicate IDs, omit exclusive paths, or omit state.

Exclusive path sets follow the plan’s Changed-Path Lease Matrix wording for each
packet (for example FND-10 remains program-metadata files under this directory
and `internal/psslease/`).

## Lease-holding states

`active`, `review`, and `integration` hold exclusive path claims. Overlapping
exclusive paths among lease holders must fail validation before a packet may be
treated as dispatched/`active`. Non-holding states (`blocked`, `ready`, `done`)
do not block other packets by path overlap alone.

## Path-overlap rule

`ValidateLeaseHolders` and `ValidateDispatchCandidate` treat exclusive paths as
slash-normalized prefixes/files. Two claims overlap when they are equal, or when
one is a path-segment prefix of the other (so `pkg/foo/` covers
`pkg/foo/bar`, and `pkg/foo` covers `pkg/foo/bar`, but `pkg/foo` does not cover
`pkg/foobar`). Before promoting a packet into a lease-holding state, call
`ValidateDispatchCandidate` so overlapping holders are rejected before the
packet is treated as active/dispatched.

## Scope fence

FND-10 owns program metadata only. It does not migrate services, invent a
second architecture tree, or take shared Wire/HTTP/CLI/MCP fan-in cutovers
(`PSS-I01`..`PSS-I05`). Shared Wire/HTTP/CLI/MCP composition remains owned by
those integration packets; do not edit generated OpenAPI bundles, CLI
manifests/generators, or provider registry/conductor surfaces to record lease
or packet-state evidence.

## Planner state updates

Update scheduling evidence by editing only this program-metadata ledger (and,
when needed, co-located `internal/psslease` fixtures). Representative lifecycle:

1. `blocked` or `ready` — undispatched; path overlap does not hold a lease
2. `active` — dispatched / lease-holding (run `ValidateDispatchCandidate` first)
3. `review` or `integration` — still lease-holding while exclusive paths are claimed
4. `done` — lease released; packet no longer blocks overlapping dispatch

In code, `psslease.SetPacketState` applies the same gate then writes the new
state. Planners may equivalently edit `state` in
`path-lease-packet-manifest.json` by hand and re-run validation.

Cross-links: plan/checklist lease matrix under
`docs/temp/projects/packaged-service-structure/` (local planner mirror) and the
committed ledger in this directory.

## Validation

```bash
go test ./internal/psslease/ -count=1
```
