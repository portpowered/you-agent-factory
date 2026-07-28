// Package automations owns functional root.BuildProcess evidence for packaged
// Automations cron scheduling, filesystem watcher preseed, hosted Linear
// polling, script-poller admission, reconciliation/source lifecycle, and
// public-process import boundaries: construction stays inert until runtime
// lifecycle starts scheduler sidecars, preseeds watched inputs, admits
// hosted-source Work, admits polled external Work, or until peers explicitly
// invoke the published Automations Root for reconciliation and source lifecycle
// admission. Proofs import only published Automations contracts plus shared
// functional support; peer_import_boundary_test.go seals the forbidden
// automations/internal, automations/wire, and automations/service imports.
package automations
