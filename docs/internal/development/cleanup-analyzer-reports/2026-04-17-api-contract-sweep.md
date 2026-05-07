# Cleanup Analyzer Report: API Contract Sweep

Date: 2026-04-17

## Scope

Analyzer dispatch over Agent Factory API handlers, generated route wiring, OpenAPI, and route tests.

## Evidence

Commands:

```bash
rg -n "getDashboard|GetState|getWorkflow|listWorkflows|getWorkTrace|getTrace|CreateFactoryRequest|CreateFactoryResponse|/state|/dashboard|/traces|/work/.*/trace|/workflows" libraries/agent-factory -g "!*node_modules*" -g "!*dist*" -g "!*storybook-static*"
rg -n "GetStatus|StatusResponse|StatusCategories|/status" libraries/agent-factory/pkg/api/generated/server.gen.go libraries/agent-factory/pkg/api/handlers.go libraries/agent-factory/api/openapi.yaml
```

Findings:

- Removed handler operation names are absent from active generated route registration.
- Negative-route tests still guard removed `/state`, `/dashboard`, `/dashboard/stream`, trace, and workflow routes.
- The retained CLI status flow still needed a supported replacement for the removed `/state` read model.

## Recommendation

Add a supported `GET /status` runtime read model generated from `api/openapi.yaml`, then route the CLI status command to that supported endpoint instead of `/state`.

## Outcome

Implemented in this sweep:

- Added `GET /status` to `api/openapi.yaml` and regenerated `pkg/api/generated/server.gen.go`.
- Added `Server.GetStatus` using `GetEngineStateSnapshot`.
- Added API route coverage for token categories and resource availability.
- Moved CLI `Status` to `/status`.
