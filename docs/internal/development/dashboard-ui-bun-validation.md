---
author: ralph agent
last modified: 2026, april, 12
doc-id: AGF-DEV-006
---

# Dashboard UI Bun Validation

This validation records the Agent Factory dashboard UI Bun compatibility gate for the build and test workflow. It compares the required UI commands from the baseline against Bun execution before later stories switch or document default command surfaces.

## Scope

Run the validation from `libraries/agent-factory/ui` unless a command explicitly uses the parent `libraries/agent-factory` module.

The required checks are:

- `bun install --frozen-lockfile`
- `bun run tsc`
- `bun run build`
- `bun run test`
- `bun run build-storybook`
- `go test ./ui` from `libraries/agent-factory`

## Result

Validated on 2026-04-12 in the `ralph/agent-factory-bun-switch` worktree with Bun 1.3.12.

| Check | Result | Evidence |
| --- | --- | --- |
| Dependency install | Passed | `bun install --frozen-lockfile` reported no lockfile or dependency changes. |
| TypeScript check | Passed | `bun run tsc` completed `tsc -b --noEmit --pretty false`. |
| Production build | Passed | `bun run build` completed TypeScript build plus Vite production build. |
| Vitest suite | Passed | `bun run test` reported 15 test files and 66 tests passed. |
| Storybook static build | Passed | `bun run build-storybook` completed Storybook's static Vite build. |
| Embedded asset visibility | Passed | `go test ./ui` compiled the Go embed package against the generated `ui/dist` output. |

## Generated Assets

The Bun production build generated the embedded dashboard assets at `libraries/agent-factory/ui/dist`:

- `dist/index.html`
- `dist/assets/index-CiGZ_MhP.js`
- `dist/assets/index-fX_CPwxP.css`

These files are under the path embedded by `libraries/agent-factory/ui/embed.go`, so the Go dashboard embed package can see the Bun-generated production output.

## Notes

Vite and Storybook both reported the existing chunk-size warning for the main dashboard JavaScript bundle. The warning did not fail the build and is not a Bun compatibility blocker, but it is worth tracking separately if dashboard bundle size becomes a review concern.

No compatibility blocker was found. Later stories may switch the remaining supported command and documentation surfaces to Bun.
