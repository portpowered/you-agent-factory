# Migrate legacy functional scenarios to customer boundaries

The required-functional guard deliberately quarantines 30 unchanged legacy
functional files while the B00 runtime-harness closure migrates its five named
exceptions. Several of those legacy scenarios still reach
`FunctionalAPIServer` internal service, API-surface, engine-snapshot, or event
history handles, and some rely on polling sleeps for readiness.

Plan a separate, behavior-preserving migration that moves each scenario to the
smallest appropriate owner-package test or to public REST, HTTP, and Factory
Session SSE observations. Retire the legacy server's exported internal capture
and inspection surface only after its callers no longer need it. Keep the
required-functional guard's content-hash quarantine shrinking; do not add new
exceptions or broaden alias allowances.

For every migrated scenario, retain direct owner-package coverage for internal
runtime, projection, and replay invariants, and use bounded cancellation-aware
public readiness/event observations for functional behavior. Verify focused
owner and functional suites plus the full required-functional guard as each
quarantined entry is removed.
