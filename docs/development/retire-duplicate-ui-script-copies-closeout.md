# Retire Duplicate UI Script Copies Closeout

This closeout records the repository-root verification bundle for the
`ralph/retire-duplicate-ui-script-copies` cleanup lane.

## Scope

This lane is intentionally limited to retiring tracked duplicate UI workflow
scripts that are not part of the package-owned command surface:

- remove `ui/scripts/normalize-dist-output copy.mjs`
- remove `ui/scripts/write-replay-coverage-report copy.ts`
- keep `ui/package.json` pointed at
  `ui/scripts/normalize-dist-output.mjs` and
  `ui/scripts/write-replay-coverage-report.ts`
- keep checked-in maintainer docs aligned to the canonical script paths

The lane does not introduce alias scripts, compatibility wrappers, or changes
to embedded UI assets beyond proving the canonical workflows still run.

## Canonical Verification Bundle

Run these commands from the repository root:

```bash
cd ui && bun run tsc
cd ui && bun run test
cd ui && bun run build
cd ui && bun run replay:coverage:check
```

## Notes

- `bun run tsc` is the focused static verification pass for the canonical UI
  script entrypoints and their package wiring.
- `bun run test` exercises the existing UI unit and integration suite without
  adding source-topology guards for the deleted duplicate files.
- `bun run build` proves the production Vite build still completes through
  `ui/scripts/normalize-dist-output.mjs`.
- `bun run replay:coverage:check` proves replay metadata validation still
  completes through `ui/scripts/write-replay-coverage-report.ts`.
- `bun run build` refreshes `ui/dist/**` and `ui/dist_stamp.go`; restore those
  generated artifacts before committing when the cleanup lane is otherwise
  dead-code-only.
