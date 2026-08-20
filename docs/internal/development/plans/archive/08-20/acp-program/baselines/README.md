# ACP capability baselines

Recorded reference behavior for ACP agents, and the computed comparison against
`you serve acp`.

Produced by `cmd/acpbaseline`. The runbook is
[`docs/internal/development/acp-baselines.md`](../../../development/acp-baselines.md).

## Layout

```
comparison-matrix.md          computed GAP / EXTRA / PARITY across every capture
<agent>/<date>/
  manifest.json               what was run, and what was observed
  capability-matrix.json      the agent's capabilities, in comparable form
  *.digest.jsonl              structural digest of each scenario's transcript
```

## What is here, and what is not

These files are **digests**. Every object key, array length, and
protocol-significant value survives; every other string is replaced by a length
marker. That is what makes them safe to commit and meaningful to diff.

Raw transcripts are **not** here and must never be. They contain full prompt and
response content and stay under `.artifacts/acp-baseline/`, which is gitignored.

`make acp-baseline-check` enforces this. It fails on an undigested string, a
secret pattern, a machine-specific path, or an oversized file.

## Reading a comparison

Verdicts are computed from the captures, not authored, so a row cannot disagree
with the evidence it summarizes. Every **GAP** row is a work item.

Two things to keep in mind:

- Model and option identities are account-entitlement-scoped. Two operators
  legitimately see different option ids, so verdicts key on a capability's
  existence and category rather than on the ids themselves.
- Read the caveats. A capture taken without a model provider configured produces
  no assistant text, and that is not the same as an agent unable to produce
  any — the matrix records the distinction when it applies.

## Retention

At most two dated captures per agent: the latest, and one prior for
release-drift diffing. Older ones are deleted rather than archived; `git log`
is the archive.
