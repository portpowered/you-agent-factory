# Remove Dead Contract Adapters And Generated-Surface Shims Closeout

Date: 2026-05-05

## Scope

This closeout records the `US-003` contract-lane cleanup completed in the May
2026 dead-code batch.

The change removed config-local duplicate ownership for public workstation-kind
boundary normalization. Generated/OpenAPI JSON decoding now uses the
interfaces-owned strict workstation-kind helper, while runtime and export
canonicalization continue to use the permissive interfaces-owned helper.

## Canonical Owners

| Behavior lane | Canonical surviving owner | Removed or collapsed shadow owner |
| --- | --- | --- |
| Generated factory JSON workstation-kind validation | `pkg/interfaces/workstation_kind_public.go:StrictPublicWorkstationKind` | `pkg/config/public_factory_enums.go` local workstation-kind alias table and config-local normalization branch |
| Runtime/export workstation-kind canonicalization | `pkg/interfaces/workstation_kind_public.go:CanonicalPublicWorkstationKind` | none; retained as the explicit permissive owner |

## Behavior Preservation

- Generated factory JSON still accepts canonical public workstation kinds such
  as `STANDARD`, `REPEATER`, and `CRON`.
- Generated factory JSON still rejects lowercase runtime workstation kinds at
  the public boundary, so consumers do not gain a new implicit compatibility
  mode.
- Runtime and event/export code paths still canonicalize internal workstation
  kinds onto the public generated enum values without reintroducing a second
  config-owned owner.

## Verification

- `go test ./pkg/interfaces ./pkg/config`
- `make typecheck`
- `make lint`
- `make test`

