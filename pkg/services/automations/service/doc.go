// Package service composes Automations runtime sidecars for script pollers,
// hosted pollers, filesystem watchers, and cron Work generation. Script
// command/source polling is owned by internal/services/script_pollers and
// reached only through this Automations root. Wire supplies explicit
// submitters, clocks, loggers, cancellation, and configuration. Use
// StartSchedulerSidecarsForRuntime as the unified runtime entrypoint for poller
// and cron supervision.
package service
