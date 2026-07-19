# Fix Storybook Monaco static-output handling on Windows worktrees

## Problem

`bun run build-storybook` fails on Windows worktrees because
`vite-plugin-monaco-editor` tries to create
`.\\.\\storybook-static\\monacoeditorwork` after Storybook has recreated or
removed its output directory. Creating `storybook-static/` before the command
does not help because the build clears it again.

The failure blocks static Storybook browser checks for otherwise isolated UI
stories. Storybook's dev server can publish `index.json`, but the affected
iframe did not finish rendering within the headless verification timeout in
this worktree.

## Suggested direction

Make the Monaco plugin's output-path handling platform-safe (or disable its
write hook for Storybook builds), then add a small Windows-compatible build
regression check that proves Storybook can emit Monaco assets into its static
output directory.
