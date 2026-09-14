# Project Reconcile

This workstation is the periodic, conservative sweep for the checked-in
Project Lead loop. It runs on a fifteen-minute SCRIPT_RUN cron with no input Work, invoking `factory/scripts/reconcile-projects.py` against the active
Factory Session.

The script uses only the public `you` CLI surfaces:

```text
you --server http://127.0.0.1:7437 --json session show <session-id>
you --server http://127.0.0.1:7437 --json work list --session <session-id>
you --server http://127.0.0.1:7437 --json worker-sessions list --work-id <project-work-id> --session <session-id>
you --server http://127.0.0.1:7437 --json work move <project-work-id> init --session <session-id> --request-id <observation-id>
```

The worker command must pass the session supplied by the runtime:

```text
python3 factory/scripts/reconcile-projects.py --server http://127.0.0.1:7437 --session {{ .Context.SessionID }}
```

The script moves an existing `project` Work only when it is `waiting` and has
no visible same-name `project-cycle`. This is the public-state stranded-lead
signal used by the periodic recovery trigger. A `blocked` Project is
inspect-only: the script never retries it from old terminal or failed child
evidence because this workstation has no durable last-seen ledger.

Unfinished children do not by themselves bar a stranded waiting lead; the lead
can inspect the current public state and choose another ready slice. The normal
active-cycle barrier remains owned by the authored graph: the script skips any
Project that still has a same-name cycle, regardless of that cycle's terminal
state. Leaving the cycle to the authored transition avoids racing `continue-project`,
`complete-project`, `retry-project-after-cycle-failure`, or `block-project` and
prevents overlapping same-name cycles. Moves use a request id derived from the
Project/child public state and lineage. The same observation is therefore
idempotent, while a later relevant state revision gets a new request id. The
script never uses filesystem mtimes or unrelated session-wide changes as retry
authority, submits a new Project or `project-cycle` batch, or marks Work
complete.

The public `work list` CLI follows continuation pages before returning its
`results`; the worker-session query is scoped to the candidate Project and
Factory Session so an unrelated session cannot look like an active lead.

The `project-reconcile` workstation is a `SCRIPT_RUN` cron on
`*/15 * * * *`. Each trigger runs the explicit `project-reconciler` worker with
`{{ .Context.SessionID }}` and completes its `project-reconcile` Work. A
script failure leaves that Work failed and starts the existing `thoughts`
supervisor path. Keep the trigger separate from the existing Project cycle
topology: the sweep repairs stranded waiting Project Leads through the public
move control and does not emit same-name cycle Work. Blocked Projects remain
available for inspection by the supervisor. Use `--dry-run` for local script
probes.

When the Factory is not running, the script reports a successful no-op so a
timer does not turn an unavailable host into a stream of failed reconciliation
Work. A failed CLI read or move returns a non-zero exit code with diagnostics on
stderr; the next scheduled trigger owns the retry.
