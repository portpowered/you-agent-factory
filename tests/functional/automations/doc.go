// Package automations owns functional root.BuildProcess evidence for packaged
// Automations cron scheduling, filesystem watcher preseed, hosted Linear
// polling, script-poller admission, and reconciliation/source lifecycle:
// construction stays inert until runtime lifecycle starts scheduler sidecars,
// preseeds watched inputs, admits hosted-source Work, admits polled external
// Work, or until peers explicitly invoke the published Automations Root for
// reconciliation and source lifecycle admission.
package automations
