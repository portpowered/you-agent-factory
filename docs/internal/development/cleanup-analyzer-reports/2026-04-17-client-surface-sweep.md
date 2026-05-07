# Cleanup Analyzer Report: Client Surface Sweep

Date: 2026-04-17

## Scope

Analyzer dispatch over Agent Factory CLI code, command tests, dashboard development proxy configuration, and browser data hooks.

## Evidence

Commands:

```bash
rg -n "state-surfaces|formattraceexplorer|/state|/dashboard|/traces|/work/.*/trace|/workflows" libraries/agent-factory/pkg libraries/agent-factory/ui -g "!*node_modules*" -g "!*dist*" -g "!*storybook-static*"
rg -n "StatusResponse|/status" libraries/agent-factory/pkg/cli
```

Findings:

- Removed CLI audit command surfaces are absent from Cobra registration.
- Dashboard proxy configuration now only forwards `/events`.
- The runtime status contract is API-owned; retired CLI status formatting should stay absent.

## Recommendation

Keep the status read model on the API and avoid reintroducing a separate CLI status formatter.

## Outcome

Implemented in this sweep:

- Removed CLI-local status response/category/resource struct redeclarations.
- Later CLI cleanup retired the status command surface entirely; `/status` remains API-owned.
