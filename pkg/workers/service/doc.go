// Package service owns worker-side scheduling supervision for script pollers, hosted
// pollers, and cron workstations. Runtime hosts attach this collaborator with explicit
// submitters, clocks, loggers, cancellation, and config context rather than depending
// on root pkg/service. Use StartSchedulerSidecarsForRuntime as the unified runtime-host
// entrypoint for poller and cron supervision.
package service
