# Factory Session API Cleanup Migration Guide

This guide records the maintainer-facing migration notes for the public Agent
Factory API cleanup on branch `ralph/factory-session-api-cleanup`. Use it when
reviewing downstream impact, preparing release notes, or migrating callers that
still depend on the retired route and schema vocabulary.

## Summary

The public API now uses three canonical customer-facing resource families:

- `factories` for persisted factory definitions
- `factory-sessions` for live running factory instances addressed by `session_id`
- `work` and related session-scoped runtime resources nested under
  `factory-sessions`

This cleanup removes the public `~current` route family from the canonical
surface for current-factory editing, replaces the separate
`EditableFactoryDefinition` document with the canonical `Factory` resource, and
requires live-session callers to use `session_id` terminology instead of
`factory_id`.

## Breaking Changes

| Old surface | New canonical surface | Migration note |
| --- | --- | --- |
| `POST /factory` for persisted named-factory activation | `POST /factories` | Persisted factory-definition routes now use the plural `factories` resource family. |
| `/factories/{factory_id}/...` for live runtime session work | `/factory-sessions/{session_id}/...` | Live session routes are now grouped under `factory-sessions` and must use `session_id`. |
| `GET /factory/~current` and `PUT /factory/~current` for current-factory editing | `GET /factory-sessions/{session_id}/factory` and `PUT /factory-sessions/{session_id}/factory` | Session-scoped active-factory reads and full-replacement writes now live under the owning factory session. |
| `EditableFactoryDefinition` response payload | `Factory` | Current-factory reads used for editing now return the canonical `Factory` resource. |
| `SaveEditableFactoryDefinitionRequest` request payload | `Factory` | Current-factory replacement writes now send the canonical `Factory` payload directly. |
| live-session path or parameter names that use `factory_id` | `session_id` | Consumers should treat persisted factory definitions and live factory sessions as different public resources. |

## Canonical Vocabulary

- `Factory` means one persisted factory definition and the one canonical public
  read or write representation for factory editing.
- `Factory Session` means one live running factory instance identified by
  `session_id`.
- `Current Factory` means the active factory configuration for one factory
  session, exposed at `/factory-sessions/{session_id}/factory`.
- `Work` and related runtime resources stay session-scoped under
  `/factory-sessions/{session_id}`.

## Schema Migration

The API no longer publishes a separate editable-definition transport shape for
factory editing. Consumers must read and write the canonical `Factory`
representation instead.

The canonical `Factory` payload now includes server-managed version metadata for
optimistic concurrency. Clients performing replacement writes should echo the
latest `Factory.version` they received and continue to handle machine-readable
stale-version conflict responses.

## Downstream Update Areas

Every downstream surface that consumes the public API should migrate in the
same breaking-change wave:

1. Regenerated Go and TypeScript clients from `api/openapi-main.yaml` and the
   bundled `api/openapi.yaml`.
2. CLI callers and shared session-path helpers that build current-factory,
   session, or work URLs.
3. UI API wrappers and editor flows that previously depended on
   `EditableFactoryDefinition`, `SaveEditableFactoryDefinitionRequest`, or
   unscoped `~current` current-factory routes.

## Verification Checklist

Use this checklist before release:

1. `make generate-api`
2. `make api-smoke`
3. `make typecheck`
4. Run the focused backend, CLI, and UI tests for any touched caller surface
   that consumes current-factory or session-scoped routes.

## Notes

- Compatibility aliases may still exist internally for older flows, but they
  are not part of the canonical public migration target for new callers.
- Release notes should call out both the route rename and the transport-shape
  consolidation, because downstream breakage can come from either path strings
  or schema expectations.
