// Package process runs worker-owned subprocess commands with supervised process-tree
// cleanup. ExecCommandRunner configures each command in its own process group (Unix) or
// job object (Windows), captures stdout/stderr, and after the parent exits—on success,
// non-zero exit, cancel, or timeout—attempts best-effort cleanup of remaining members in
// that supervised group.
//
// # Same-group children
//
// Background children that stay in the parent's process group (Unix) or job (Windows)
// are terminated after cmd.Wait returns, including when the parent exits with code 0.
// Post-run cleanup sends SIGTERM, waits up to commandProcessGroupGracePeriod (2s), then
// SIGKILLs the group on Unix; on Windows it polls job active processes, may call
// TerminateJobObject, then closes the job handle (KILL_ON_JOB_CLOSE). Parent stdout,
// stderr, and exit code are captured before cleanup runs and are not changed by cleanup.
//
// # Detached children and escape cases
//
// Cleanup only targets the supervised group created for that command invocation. Children
// may survive after the runner returns if they detach from that group, for example:
//
//   - Unix: setsid(2), starting a new session/process group, or double-fork daemonize so
//     the child is no longer in the parent's PGID
//   - Windows: CREATE_BREAKAWAY_FROM_JOB or processes that move to a different job object
//
// Operators should not rely on post-run cleanup to stop intentionally detached daemons or
// services started outside the supervised group.
package process
