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

## Lease-holding states

`active`, `review`, and `integration` hold exclusive path claims. Overlapping
exclusive paths among lease holders must fail validation before a packet may be
treated as dispatched/`active`. Non-holding states (`blocked`, `ready`, `done`)
do not block other packets by path overlap alone.

## Scope fence

FND-10 owns program metadata only. It does not migrate services, invent a
second architecture tree, or take shared Wire/HTTP/CLI/MCP fan-in cutovers
(`PSS-I01`..`PSS-I05`).

## Validation

```bash
go test ./internal/psslease/ -count=1
```
