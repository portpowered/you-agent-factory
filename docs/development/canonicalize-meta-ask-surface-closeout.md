# Canonicalize Meta Ask Surface Closeout

Date: 2026-04-30
Scope: final verification for `prd.json` `US-004` on branch `ralph/canonicalize-meta-ask-surface`.

## Summary

This closeout proves the cleanup stayed limited to ask-surface ownership and
control-plane alignment. The branch diff against `main` changes only the
canonical backlog declaration, the legacy redirect stub, the active maintainer
guidance, and the artifact-contract surfaces that define or enforce that same
answer.

Files changed in the reviewable diff:

- `factory/logs/meta/asks.md`
- `factory/logs/meta/view.md`
- `factory/meta/asks.md`
- `factory/workstations/cleaner/AGENTS.md`
- `docs/development/root-factory-artifact-contract-inventory.md`
- `docs/processes/factory-workstation-relevant-files.md`
- `internal/testpath/artifact_contract.go`

The canonical backlog contents remain at `factory/logs/meta/asks.md`. The only
change in that file is the ownership banner that declares it as the source of
truth; the backlog entries themselves were preserved. The former duplicate path
`factory/meta/asks.md` now contains redirect-only language and no longer tracks
independent ask content.

## Validation

Commands run from the repository root:

```powershell
make artifact-contract-closeout
make test
make lint
```

## What This Proves

- The reviewable diff stays narrow: it touches only ask-surface ownership,
  maintainer-control guidance, and the artifact-contract surfaces that keep
  those paths aligned.
- The repository now leaves the meta agent with one trusted checked-in ask
  surface: `factory/logs/meta/asks.md`.
- Silent backlog drift is mechanically prevented because the legacy
  `factory/meta/asks.md` path is a redirect-only stub instead of a peer
  backlog.
