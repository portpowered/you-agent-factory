# Repository Maintainer Asks

Local customer-ask backlog for the repository maintainer workflow (gitignored).

Companion surfaces:

- `factory/internal/view.md`
- `factory/internal/progress.md`
- `factory/internal/meta.md`

## Active asks

No active asks recorded.

## service-cleanup-on-success (delivered)

**Ask:** After an agent or script command exits successfully (exit code 0), terminate
background children that were started in the same supervised process group so orphaned
services do not accumulate between runs. Preserve parent stdout, stderr, and exit code;
keep existing cancel/timeout termination behavior; log cleanup without leaking secrets.

**Implementation:** `pkg/workers/process.ExecCommandRunner` runs best-effort post-run
cleanup after every `cmd.Wait` completion (success and non-zero exit) via
`closeCommandProcessTree`, sharing `terminateCommandProcessGroup` / job-object helpers with
the cancel and timeout paths. See package documentation in `pkg/workers/process`.

**Same-group children (expected cleanup):** On Unix, commands run with `Setpgid` so the
parent and typical background children share one process group; post-run cleanup signals
`-pgid` (graceful then force). On Windows, children assigned to the command job object are
terminated when the job is closed or force-terminated after a bounded grace wait.

**Detached children (may escape cleanup):** Children that leave the supervised group are
not guaranteed to be stopped, including:

- Unix: `setsid`, a new session/process group, or double-fork daemonize
- Windows: `CREATE_BREAKAWAY_FROM_JOB` or a different job object

Do not assume post-run cleanup will stop daemons or services that intentionally detach.
