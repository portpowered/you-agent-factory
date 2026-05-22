---
title: Stream script poller stdout instead of waiting for subprocess exit
created: 2026-05-22
status: proposed
---

# Why this matters

The current script poller supervisor reuses `workers.ExecCommandRunner`, which
buffers stdout and stderr until the subprocess exits. Story
`prd-poller-workstation-004` can therefore submit canonical work only after the
poller process returns, even though the intended workstation behavior is a
long-lived ingress daemon.

This is a reusable architecture gap rather than a one-off story detail:

- any future script-backed poller that stays alive cannot emit work
  incrementally through the current runner seam,
- restart supervision and canonical ingress are now correct, but steady-state
  long-lived polling still depends on the script choosing exit boundaries, and
- hosted pollers and future sidecars may want the same streaming subprocess
  primitive for observable incremental output.

# Suggested direction

Add a streaming subprocess execution seam for service-owned sidecars that can:

- read stdout incrementally without waiting for process exit,
- preserve cancellation and process-tree termination behavior,
- distinguish parse failures from process failures in logs, and
- keep ordinary one-shot `SCRIPT_WORKER` execution on the existing buffered
  command runner unless a caller explicitly opts into streaming.

# Relevant files

- `pkg/workers/command.go`
- `pkg/service/poller_watcher.go`
- `pkg/service/poller_watcher_test.go`
