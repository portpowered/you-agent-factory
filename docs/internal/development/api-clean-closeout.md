# API Clean Closeout

This closeout records the repository-root verification bundle for the OpenAPI
schema standardization lane on `ralph/api-clean`.

## Canonical Verification Bundle

Run these commands from the repository root:

```bash
node scripts/run-quiet-api-command.js validate:main ./api/openapi-main.yaml
make api-smoke
make test
make lint
make ui-deps
cd ui && bun run tsc
```

## Notes

- `make api-smoke` is the canonical OpenAPI closeout path for this repo. It now
  validates the authored source tree from the repository root, reruns
  regeneration twice, checks the generated diff is clean, and executes the
  focused bundled-contract and generated-runtime smoke tests without relying on
  legacy checkout-relative paths.
- `make test`, `make lint`, `make ui-deps`, and `cd ui && bun run tsc` provide
  the broader post-regeneration quality evidence for this lane.
