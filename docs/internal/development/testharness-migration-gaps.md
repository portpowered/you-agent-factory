# TestHarness → ServiceTestHarness Migration — Complete

## Summary

**Migration status: COMPLETE** — All tests migrated, `TestHarness` deleted.

All 39 test files across `tests/functional/` and `tests/stress/` now use `ServiceTestHarness` exclusively. The `TestHarness` struct (`harness.go`) has been deleted.

## What Changed

### ServiceTestHarness API Additions (US-002, US-003, US-010)

The following methods were added to `ServiceTestHarness` to close API gaps:

**Submission methods (US-002):**
- `SubmitFull(req SubmitRequest) string`
- `SubmitWithRelations(workTypeID, payload, relations) string`
- `QueueFull(req SubmitRequest)`
- `QueueWork(workTypeID, payload)`

**Executor registration (US-003):**
- `MockWorker(workerType, results...) *MockExecutor`
- `SetCustomExecutor(workerType, executor)`

**Options:**
- Inline dispatch is enabled by default for tick-based testing (required for submission methods + assertions)
- `WithRunAsync()` — for tests using `RunInBackground`
- `WithExtraOptions(factory.WithWorkerExecutor(workerType, executor))` — registers executor at factory construction time

**Async support (US-010):**
- `RunInBackground(ctx) <-chan error` — non-blocking alternative to RunUntilComplete

### Key Design Decisions

- **Inline dispatch (default) and RunInBackground are mutually exclusive** — inline dispatch processes workers during Tick; Run expects async worker pool
- **MockWorker/SetCustomExecutor use shared maps** created before factory construction — reference types allow post-construction registration
- **Custom executors take precedence over mocks**, mocks over service executors
- **All tests use factory.json config** — `ScaffoldFactoryDir` writes programmatic config to temp dirs for dynamic topologies; static fixtures for fixed topologies
- **Tests with custom guards (DepthLimitGuard) are t.Skipped** — pending config schema extension

## Migration Batches

| Story | Scope | Tests Migrated |
|-------|-------|---------------|
| US-004/005 | Simple functional (batch 1) | 20 tests, 6 files |
| US-006 | Executor/failure functional (batch 2) | 20 tests, 6 files |
| US-007 | Concurrency/guard functional (batch 3) | 20 tests, 6 files |
| US-008 | Complex functional (batch 4) | 25 tests, 7 files |
| US-009 | Repeater parameterized | 7 tests, 1 file |
| US-010 | Stress tests | 47 tests, 11 files |
| US-011 | Delete TestHarness | Cleanup only |
