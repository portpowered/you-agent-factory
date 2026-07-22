// Package service owns automation for script pollers, hosted pollers,
// filesystem watchers, and cron Work generation. Wire supplies explicit
// submitters, clocks, loggers, cancellation, and configuration. Use
// StartSchedulerSidecarsForRuntime as the unified runtime entrypoint for poller
// and cron supervision.
package service
