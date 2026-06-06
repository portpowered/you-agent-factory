# Fix current-selection Storybook Monaco startup verification

## Problem

The current-selection editable-configuration Storybook path is now able to render past the React Flow parent-container precondition, but browser verification still cannot prove Monaco palette behavior for prompt or guard-selector editors.

Observed in `you-agent-factory / Workflow Dashboard / Current Selection Editable Configuration Desktop Verification`:

- the play flow reaches later graph assertions instead of crashing on React Flow error `004`
- expanding editable configuration shows prompt and guard-selector fallback copy instead of starting Monaco
- the current story also emits prompt-template and session 404s, which suggests the Storybook runtime mocks for the editable-configuration surface are incomplete for Monaco startup

## Why it matters

Current-selection Monaco stories are now part of multiple browser-verification workflows. If Storybook cannot start those editors in-browser, palette, syntax, accessibility, and save-path checks for prompt and guard-selector work will continue to stall behind Storybook-specific setup issues rather than the feature under test.

## Suggested direction

- audit the current-selection Storybook runtime mocks needed for prompt and guard-selector Monaco startup, including any prompt-template contract and factory-session requests
- add or update a focused Storybook verification story that reaches the editable prompt and guard-selector Monaco surfaces without depending on unrelated graph-editor assertions
- keep the graph-editor browser assertions separate from Monaco verification so one surface failing does not block the other
