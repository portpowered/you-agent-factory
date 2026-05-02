package testutil

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/internal/submission"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// ServiceTestHarness wraps a FactoryService built via BuildFactoryService()
// with test-friendly convenience methods. It exercises the full service
// layer: BuildFactoryService → loadWorkersFromConfig → WorkstationExecutor →
// AgentExecutor → mock Provider. This ensures prompt rendering, stop-token
// evaluation, and output parsing are tested.
//
// The harness does not expose the underlying factory or service — all
// operations are available through harness methods.
type ServiceTestHarness struct {
	t               *testing.T
	svc             *service.FactoryService
	mocks           map[string]*MockExecutor
	customExecutors map[string]workers.WorkerExecutor
}

// harnessConfig holds internal configuration for NewServiceTestHarness,
// separating harness-specific flags from the FactoryServiceConfig.
type harnessConfig struct {
	// in this mode, we don't apply any form of mock execution logic, and the only thing we mock is the dispathcer. Everything else is copy straight,.
	// We do not apply any form of custom executor. This MUST never be changed.
	applyFullWorkerPoolAndScriptWrapExecutor bool
	asyncMode                                bool
	serviceConfig                            service.FactoryServiceConfig
}

// ServiceTestHarnessOption configures a ServiceTestHarness.
type ServiceTestHarnessOption func(*harnessConfig)

// WithProvider sets the ProviderOverride on the service config.
func WithProvider(p workers.Provider) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.ProviderOverride = p
	}
}

// WithProviderCommandRunner injects a fake subprocess runner into the real
// ScriptWrapProvider used by MODEL_WORKER executors. Use this for tests whose
// intent is to validate provider CLI construction rather than provider
// replacement at the higher-level Infer API.
func WithProviderCommandRunner(runner workers.CommandRunner) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.ProviderCommandRunnerOverride = runner
	}
}

// WithExtraOptions appends additional factory options to the service config.
func WithExtraOptions(opts ...factory.FactoryOption) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.ExtraOptions = append(cfg.serviceConfig.ExtraOptions, opts...)
	}
}

// WithCommandRunner sets a CommandRunner override on the service config.
// SCRIPT_WORKER executors will use this runner instead of os/exec, allowing
// tests to mock command execution while exercising the full ScriptExecutor
// pipeline (arg templates, env merging, exit-code routing).
func WithCommandRunner(runner workers.CommandRunner) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.CommandRunnerOverride = runner
	}
}

// WithMockWorkersConfig enables service-level mock-worker mode using the
// normalized runtime config supplied by pkg/config.
func WithMockWorkersConfig(mockCfg *factoryconfig.MockWorkersConfig) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.MockWorkersConfig = mockCfg
	}
}

// WithRuntimeLogDir writes service runtime logs under dir for assertions.
func WithRuntimeLogDir(dir string) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.RuntimeLogDir = dir
	}
}

// WithRuntimeLogConfig sets bounded rolling-file policy for service runtime logs.
func WithRuntimeLogConfig(config logging.RuntimeLogConfig) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.RuntimeLogConfig = config
	}
}

// WithRuntimeInstanceID sets a stable runtime log filename for assertions.
func WithRuntimeInstanceID(id string) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.RuntimeInstanceID = id
	}
}

// WithRecordPath enables service record mode for harness runs.
func WithRecordPath(path string) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.RecordPath = path
	}
}

// WithExecutionBaseDir overrides the runtime base directory used to resolve
// relative workstation execution paths.
func WithExecutionBaseDir(dir string) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.ExecutionBaseDir = dir
	}
}

// WithReplayPath enables service replay mode for harness runs.
func WithReplayPath(path string) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.ReplayPath = path
	}
}

// WithRunAsync enables async dispatch mode for tests that use RunUntilComplete
// or RunInBackground. In async mode, the worker pool processes dispatches
// instead of inline execution during Tick. This is the production dispatch path.
func WithRunAsync() ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.asyncMode = true
	}
}

func WithFullWorkerPoolAndScriptWrap() ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.applyFullWorkerPoolAndScriptWrapExecutor = true
	}
}

// NewServiceTestHarness builds a FactoryService from the given directory
// (which must contain factory.json and workers/{name}/AGENTS.md files) and
// returns a test harness that drives the engine step-by-step.
//
// By default, the harness enables inline dispatch so that worker executors
// (built by BuildFactoryService from AGENTS.md configs) run during each
// Tick — no goroutines or channels required in tests.
//
// Pass WithRunAsync() for tests that use RunUntilComplete or RunInBackground,
// which require the real async worker pool.
func NewServiceTestHarness(t *testing.T, dir string, opts ...ServiceTestHarnessOption) *ServiceTestHarness {
	t.Helper()

	// Pre-create maps so the inline dispatch closure can reference them.
	// MockWorker/SetCustomExecutor modify these maps post-construction;
	// the closure sees updates because maps are reference types.
	mocks := make(map[string]*MockExecutor)
	customExecs := make(map[string]workers.WorkerExecutor)

	cfg := &harnessConfig{
		serviceConfig: service.FactoryServiceConfig{
			Dir: dir,
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.asyncMode {
		// Async mode: wrap all registered executors with the mock/custom
		// override chain. The delegating executors check mock/custom maps
		// at execution time, so MockWorker() and SetCustomExecutor() called
		// after construction still take effect. Appended as LAST extra option
		// so it runs after loadWorkersFromConfig and sees the fully populated
		// WorkerExecutors map.
		cfg.serviceConfig.ExtraOptions = append(cfg.serviceConfig.ExtraOptions, buildAsyncMockOverrides(mocks, customExecs))
	} else if cfg.applyFullWorkerPoolAndScriptWrapExecutor {
		// In this mode, we don't apply any form of mock execution logic, and the only thing we mock is the dispathcer. Everything else is copy straight,.
	} else {
		// Default: enable inline dispatch for tick-based testing. Appended
		// as LAST extra option so it runs after loadWorkersFromConfig and
		// sees the fully populated WorkerExecutors map.
		cfg.serviceConfig.ExtraOptions = append(cfg.serviceConfig.ExtraOptions, buildInlineDispatch(mocks, customExecs))
	}

	svc, err := service.BuildFactoryService(context.Background(), &cfg.serviceConfig)
	if err != nil {
		t.Fatalf("NewServiceTestHarness: BuildFactoryService failed: %v", err)
	}

	return &ServiceTestHarness{
		t:               t,
		svc:             svc,
		mocks:           mocks,
		customExecutors: customExecs,
	}
}

// --- internal routing helpers ---

// submit delegates to the underlying FactoryService.
func (h *ServiceTestHarness) submit(ctx context.Context, reqs []interfaces.SubmitRequest) error {
	request := submission.WorkRequestFromSubmitRequests(reqs)
	_, err := h.svc.SubmitWorkRequest(ctx, request)
	return err
}

// getMarking delegates to the canonical engine-state snapshot.
func (h *ServiceTestHarness) getMarking(ctx context.Context) (*petri.MarkingSnapshot, error) {
	snap, err := h.svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &snap.Marking, nil
}

// waitToComplete delegates to the underlying FactoryService.
func (h *ServiceTestHarness) waitToComplete() <-chan struct{} {
	return h.svc.WaitToComplete()
}

// run delegates to the underlying FactoryService.
func (h *ServiceTestHarness) run(ctx context.Context) error {
	return h.svc.Run(ctx)
}

// buildInlineDispatch returns a FactoryOption that enables inline dispatch
// with the full override chain: customExecutors → mocks → service executors.
// It wraps all registered worker executors with delegating executors (same
// pattern as buildAsyncMockOverrides) and enables inline dispatch mode so
// the runtime calls executors synchronously during ticks.
func buildInlineDispatch(mocks map[string]*MockExecutor, customExecs map[string]workers.WorkerExecutor) factory.FactoryOption {
	return func(c *factory.FactoryConfig) {
		// Wrap all registered executors with the mock/custom override chain.
		for workerType, original := range c.WorkerExecutors {
			c.WorkerExecutors[workerType] = &delegatingExecutor{
				workerType: workerType,
				mocks:      mocks,
				customs:    customExecs,
				fallback:   original,
			}
		}
		// Enable inline dispatch: the runtime will call executors synchronously
		// during ticks instead of routing through the async worker pool.
		factory.WithInlineDispatch()(c)
	}
}

// delegatingExecutor wraps a base WorkerExecutor and checks mock/custom maps
// at execution time. This allows MockWorker() and SetCustomExecutor() to
// override executors in both inline and async dispatch modes.
type delegatingExecutor struct {
	workerType string
	mocks      map[string]*MockExecutor
	customs    map[string]workers.WorkerExecutor
	fallback   workers.WorkerExecutor
}

func (d *delegatingExecutor) Execute(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	if custom, ok := d.customs[d.workerType]; ok {
		return custom.Execute(ctx, dispatch)
	}
	if mock, ok := d.mocks[d.workerType]; ok {
		return mock.Execute(ctx, dispatch)
	}
	if d.fallback != nil {
		return d.fallback.Execute(ctx, dispatch)
	}
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeFailed,
		Error:        fmt.Sprintf("no executor registered for worker type %q (transition %s)", d.workerType, dispatch.TransitionID),
	}, nil
}

// buildAsyncMockOverrides returns a FactoryOption that wraps all registered
// WorkerExecutors with the mock/custom override chain. Unlike buildInlineDispatch
// (which enables synchronous inline dispatch), this preserves async dispatch
// through the worker pool while allowing MockWorker() and SetCustomExecutor()
// to intercept executions at runtime.
func buildAsyncMockOverrides(mocks map[string]*MockExecutor, customExecs map[string]workers.WorkerExecutor) factory.FactoryOption {
	return func(c *factory.FactoryConfig) {
		for workerType, original := range c.WorkerExecutors {
			c.WorkerExecutors[workerType] = &delegatingExecutor{
				workerType: workerType,
				mocks:      mocks,
				customs:    customExecs,
				fallback:   original,
			}
		}
	}
}

// MockWorker registers a MockExecutor for the given worker type and returns it.
// If the worker type was already registered, returns the existing mock.
// In inline mode, mocks execute during Tick. In async mode (WithRunAsync),
// mocks execute in the worker pool via the delegating executor.
func (h *ServiceTestHarness) MockWorker(workerType string, results ...interfaces.WorkResult) *MockExecutor {
	if existing, ok := h.mocks[workerType]; ok {
		return existing
	}
	mock := NewMockExecutor(results...)
	h.mocks[workerType] = mock
	return mock
}

// SetCustomExecutor registers a custom WorkerExecutor for a worker type.
// Custom executors take precedence over mock executors. This is useful
// when a test needs dynamic behavior that depends on the dispatch inputs
// (e.g., setting ParentID on spawned tokens based on the input token's WorkID).
// Works in both inline and async (WithRunAsync) dispatch modes.
func (h *ServiceTestHarness) SetCustomExecutor(workerType string, executor workers.WorkerExecutor) {
	h.customExecutors[workerType] = executor
}

// RunUntilComplete starts the factory's Run loop in a background goroutine
// and blocks until all tokens reach terminal/failed places or timeout elapses.
// It fails the test on timeout or factory error.
//
// Works with both inline dispatch (default) and async dispatch (WithRunAsync).
// Delegates to FactoryService.Run().
func (h *ServiceTestHarness) RunUntilComplete(t *testing.T, timeout time.Duration) {
	t.Helper()
	if err := h.RunUntilCompleteError(timeout); err != nil {
		t.Fatalf("RunUntilComplete: %v", err)
	}
}

// RunUntilCompleteError is the error-returning form of RunUntilComplete. Tests
// that intentionally exercise timeout behavior can assert on the returned
// diagnostic message.
func (h *ServiceTestHarness) RunUntilCompleteError(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.run(ctx)
	}()

	select {
	case <-h.waitToComplete():
		cancel()
	case <-ctx.Done():
		cancel()
		<-errCh
		return fmt.Errorf("timed out waiting for factory to complete within %s\n", timeout)
	}

	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("factory run error: %w", err)
	}
	return nil
}

// SubmitWork injects a new work token into the factory's submission queue.
// The token is processed when the engine runs (via RunUntilComplete or
// RunInBackground). Returns an error if submission fails (also fatals
// the test). Callers that need token IDs should inspect the marking
// after the engine has run.
func (h *ServiceTestHarness) SubmitWork(workTypeID string, payload []byte) error {
	h.t.Helper()

	if err := h.SubmitError(workTypeID, payload); err != nil {
		h.t.Fatalf("ServiceTestHarness.SubmitWork: failed to submit: %v", err)
		return err
	}

	return nil
}

// SubmitError is the nonfatal form of Submit. Use it for tests that assert
// validation failures at the submit boundary.
func (h *ServiceTestHarness) SubmitError(workTypeID string, payload []byte) error {
	h.t.Helper()

	return h.submit(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: workTypeID,
		Payload:    payload,
		TraceID:    fmt.Sprintf("trace-%s-%d", workTypeID, time.Now().UnixNano()),
	}})
}

// SubmitFull submits one or more work items with full control over WorkID,
// TraceID, Tags, Relations, and other SubmitRequest fields. It wraps those
// fields into a canonical WorkRequest before calling the service ingress.
func (h *ServiceTestHarness) SubmitFull(ctx context.Context, reqs []interfaces.SubmitRequest) error {
	h.t.Helper()

	if err := h.SubmitFullError(ctx, reqs); err != nil {
		h.t.Fatalf("ServiceTestHarness.SubmitFull: failed to submit: %v", err)
		return err
	}
	return nil
}

// SubmitFullError is the nonfatal form of SubmitFull for tests that need to
// assert validation errors without failing the test immediately.
func (h *ServiceTestHarness) SubmitFullError(ctx context.Context, reqs []interfaces.SubmitRequest) error {
	h.t.Helper()

	return h.submit(ctx, reqs)
}

// SubmitWorkRequest submits a canonical work request batch with full control
// over request IDs, work item names, payloads, tags, and relations.
func (h *ServiceTestHarness) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) error {
	h.t.Helper()

	if _, err := h.svc.SubmitWorkRequest(ctx, request); err != nil {
		h.t.Fatalf("ServiceTestHarness.SubmitWorkRequest: failed to submit: %v", err)
		return err
	}
	return nil
}

// Marking returns the current marking snapshot.
func (h *ServiceTestHarness) Marking() *petri.MarkingSnapshot {
	snap, err := h.getMarking(context.Background())
	if err != nil {
		h.t.Fatalf("ServiceTestHarness.Marking: %v", err)
	}
	return snap
}

// Assert returns a MarkingAssert for the current marking.
func (h *ServiceTestHarness) Assert() *MarkingAssert {
	h.t.Helper()
	return AssertMarking(h.t, h.Marking())
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight.
func (h *ServiceTestHarness) WaitToComplete() <-chan struct{} {
	return h.waitToComplete()
}

// GetEngineStateSnapshot returns a unified EngineStateSnapshot combining runtime
// state, factory lifecycle, session metrics, and uptime.
func (h *ServiceTestHarness) GetEngineStateSnapshot() (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return h.svc.GetEngineStateSnapshot(context.Background())
}

// GetFactoryEvents returns the canonical factory event history recorded by the service.
func (h *ServiceTestHarness) GetFactoryEvents(ctx context.Context) ([]factoryapi.FactoryEvent, error) {
	return h.svc.GetFactoryEvents(ctx)
}

// RunInBackground starts the factory's Run loop in a background goroutine
// and returns the error channel. Unlike RunUntilComplete, this does NOT
// block — the caller controls when to stop (via context cancel) and can
// submit work or query state while the engine runs.
//
// Delegates to FactoryService.Run().
func (h *ServiceTestHarness) RunInBackground(ctx context.Context) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.run(ctx)
	}()
	return errCh
}
