# Fix App Storybook React Flow Container Precondition

## Problem

Running `src/App.stories.tsx` through the Storybook Chromium Vitest project can fail every App story before story-specific assertions run:

```text
CurrentActivityGraphEndpointError: React Flow factory graph endpoint error 004:
The React Flow parent container needs a width and a height to render the graph.
```

This blocks browser evidence for unrelated dashboard work, including current-selection editable workstation verification, because the shared workflow graph mounts before the targeted assertions can execute.

## Suggested Direction

- Reproduce against the static Storybook lane with `AGENT_FACTORY_STORYBOOK_PORT=6008 bunx vitest run --config vitest.storybook.config.ts --project=storybook src/App.stories.tsx`.
- Identify whether the App story wrappers, dashboard grid card, or React Flow card needs a stable test/browser height contract.
- Keep the fix at the shared App/Workflow Activity Storybook seam rather than adding per-story workarounds to unrelated current-selection stories.
- Preserve the existing React Flow endpoint validation for real invalid handle and container states; only repair the Storybook harness/container precondition.
