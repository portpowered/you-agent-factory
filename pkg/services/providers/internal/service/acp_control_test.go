package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

// acpAwareAttempt is a channel-gated fake acp.Service standing in for one
// live ACP attempt. It reports when Execute started, only becomes
// truthfully cancelable once the test signals openCancelWindow (modeling
// the span between an attempt's session/new and session/prompt returning,
// see acp.Service.Cancelable), and returns the same
// ExecuteFailureKindCanceled outcome the real ACP session/cancel path
// normalizes StopReasonCancelled to once Cancel names its exact attempt
// while cancelable.
type acpAwareAttempt struct {
	provider providers.ID

	started          chan struct{}
	openCancelWindow chan struct{}
	becameCancelable chan struct{}
	canceledSignal   chan struct{}
	release          chan struct{}
	done             chan struct{}

	mu          sync.Mutex
	attemptID   string
	cancelable  bool
	cancelCalls int
}

func newACPAwareAttempt(provider providers.ID) *acpAwareAttempt {
	return &acpAwareAttempt{
		provider:         provider,
		started:          make(chan struct{}),
		openCancelWindow: make(chan struct{}),
		becameCancelable: make(chan struct{}),
		canceledSignal:   make(chan struct{}),
		release:          make(chan struct{}),
		done:             make(chan struct{}),
	}
}

func (a *acpAwareAttempt) Close(context.Context) error                                 { return nil }
func (a *acpAwareAttempt) Configure(context.Context, []providers.ACPIntegration) error { return nil }
func (a *acpAwareAttempt) Integrations() []providers.ACPIntegration                    { return nil }
func (a *acpAwareAttempt) Resolve(id providers.ID) (providers.ID, bool) {
	return a.provider, id == a.provider
}

func (a *acpAwareAttempt) Execute(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	a.mu.Lock()
	a.attemptID = request.AttemptID
	a.mu.Unlock()
	close(a.started)
	defer close(a.done)

	select {
	case <-a.openCancelWindow:
	case <-a.release:
		return providers.ExecuteResult{Content: request.AttemptID}, nil
	}

	a.mu.Lock()
	a.cancelable = true
	a.mu.Unlock()
	close(a.becameCancelable)

	select {
	case <-a.canceledSignal:
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
	case <-a.release:
		return providers.ExecuteResult{Content: request.AttemptID}, nil
	}
}

func (a *acpAwareAttempt) Cancelable(_ providers.ID, attemptID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cancelable && a.attemptID == attemptID
}

func (a *acpAwareAttempt) Cancel(_ context.Context, _ providers.ID, attemptID string) error {
	a.mu.Lock()
	matches := a.cancelable && a.attemptID == attemptID
	if matches {
		a.cancelCalls++
	}
	a.mu.Unlock()
	if !matches {
		return nil
	}
	select {
	case <-a.canceledSignal:
	default:
		close(a.canceledSignal)
	}
	<-a.done
	return nil
}

// multiACPService dispatches to one acpAwareAttempt per configured provider
// identity, so cross-provider isolation can be exercised through one root
// Service the way two configured ACP integrations would be in production.
type multiACPService struct {
	byProvider map[providers.ID]*acpAwareAttempt
}

func (m *multiACPService) Close(context.Context) error                                 { return nil }
func (m *multiACPService) Configure(context.Context, []providers.ACPIntegration) error { return nil }
func (m *multiACPService) Integrations() []providers.ACPIntegration                    { return nil }

func (m *multiACPService) Resolve(id providers.ID) (providers.ID, bool) {
	_, ok := m.byProvider[id]
	return id, ok
}

func (m *multiACPService) Execute(
	ctx context.Context,
	id providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return m.byProvider[id].Execute(ctx, id, request)
}

func (m *multiACPService) Cancelable(id providers.ID, attemptID string) bool {
	target, ok := m.byProvider[id]
	return ok && target.Cancelable(id, attemptID)
}

func (m *multiACPService) Cancel(ctx context.Context, id providers.ID, attemptID string) error {
	target, ok := m.byProvider[id]
	if !ok {
		return nil
	}
	return target.Cancel(ctx, id, attemptID)
}

func mustACPControlRootService(t *testing.T, acpService *acpAwareAttempt) providers.Service {
	t.Helper()
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.NewWithACP(catalogService, executionService, acpService, nil, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}
	return root
}

func TestControlAttempt_ACPCancelUnsupportedBeforeSessionEstablishedThenSucceedsOnceCancelable(t *testing.T) {
	t.Parallel()

	fake := newACPAwareAttempt("cursor-acp")
	root := mustACPControlRootService(t, fake)

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  "cursor-acp",
			AttemptID: "acp-attempt-1",
			Model:     "cursor-model",
		})
		executeDone <- err
	}()
	<-fake.started

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "cursor-acp",
		AttemptID: "acp-attempt-1",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(before session) error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt(before session) outcome = %q, want unsupported", result.Outcome)
	}
	if fake.cancelCalls != 0 {
		t.Fatalf("ACP cancel calls = %d, want 0 before the session is truthfully cancelable", fake.cancelCalls)
	}

	close(fake.openCancelWindow)
	<-fake.becameCancelable

	result, err = root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "cursor-acp",
		AttemptID: "acp-attempt-1",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(cancelable) error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt(cancelable) outcome = %q, want completed", result.Outcome)
	}
	if fake.cancelCalls != 1 {
		t.Fatalf("ACP cancel calls = %d, want 1", fake.cancelCalls)
	}

	executeErr := <-executeDone
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("Execute() error = %v, want ExecuteFailureKindCanceled", executeErr)
	}
}

func TestControlAttempt_ACPTerminateAndPauseAreNeverSupportedAndLeaveAttemptRunning(t *testing.T) {
	t.Parallel()

	fake := newACPAwareAttempt("cursor-acp")
	root := mustACPControlRootService(t, fake)

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  "cursor-acp",
			AttemptID: "acp-attempt-2",
			Model:     "cursor-model",
		})
		executeDone <- err
	}()
	<-fake.started
	close(fake.openCancelWindow)
	<-fake.becameCancelable

	for _, action := range []providers.ControlAction{providers.ControlActionTerminate, providers.ControlActionPause} {
		result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
			Provider:  "cursor-acp",
			AttemptID: "acp-attempt-2",
			Action:    action,
		})
		if err != nil {
			t.Fatalf("ControlAttempt(%q) error = %v, want nil", action, err)
		}
		if result.Outcome != providers.ControlOutcomeUnsupported {
			t.Fatalf("ControlAttempt(%q) outcome = %q, want unsupported", action, result.Outcome)
		}
	}
	if fake.cancelCalls != 0 {
		t.Fatalf("ACP cancel calls = %d, want 0 for terminate/pause", fake.cancelCalls)
	}

	// The unsupported terminate/pause attempts must not have consumed or
	// corrupted the live registration: a supported cancel on the same
	// identity must still reach it.
	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "cursor-acp",
		AttemptID: "acp-attempt-2",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(cancel) error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt(cancel) outcome = %q, want completed", result.Outcome)
	}

	executeErr := <-executeDone
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("Execute() error = %v, want ExecuteFailureKindCanceled", executeErr)
	}
}

// blockingACPAttempt is a dedicated, single-purpose fake acp.Service (not
// acpAwareAttempt) whose Execute only returns after the test explicitly
// releases it once cancellation has been observed, so ControlAttempt's
// blocking-until-terminal contract can be proven deterministically (no
// sleep-based timing involved), the same way
// TestControlAttempt_BlocksUntilSignaledNativeAttemptReturns proves it for
// the native control path.
type blockingACPAttempt struct {
	started        chan struct{}
	cancelledSeen  chan struct{}
	releaseAttempt chan struct{}
	mu             sync.Mutex
	attemptID      string
	cancelable     bool
}

func newBlockingACPAttempt() *blockingACPAttempt {
	return &blockingACPAttempt{
		started:        make(chan struct{}),
		cancelledSeen:  make(chan struct{}),
		releaseAttempt: make(chan struct{}),
	}
}

func (a *blockingACPAttempt) Close(context.Context) error                                 { return nil }
func (a *blockingACPAttempt) Configure(context.Context, []providers.ACPIntegration) error { return nil }
func (a *blockingACPAttempt) Integrations() []providers.ACPIntegration                    { return nil }
func (a *blockingACPAttempt) Resolve(id providers.ID) (providers.ID, bool) {
	return "cursor-acp", id == "cursor-acp"
}

func (a *blockingACPAttempt) Execute(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	a.mu.Lock()
	a.attemptID = request.AttemptID
	a.cancelable = true
	a.mu.Unlock()
	close(a.started)

	<-a.cancelledSeen
	<-a.releaseAttempt
	return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
}

func (a *blockingACPAttempt) Cancelable(_ providers.ID, attemptID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cancelable && a.attemptID == attemptID
}

func (a *blockingACPAttempt) Cancel(_ context.Context, _ providers.ID, attemptID string) error {
	a.mu.Lock()
	matches := a.cancelable && a.attemptID == attemptID
	a.mu.Unlock()
	if !matches {
		return nil
	}
	close(a.cancelledSeen)
	<-a.releaseAttempt
	return nil
}

func TestControlAttempt_ACPBlocksUntilSignaledAttemptReturns(t *testing.T) {
	t.Parallel()

	fake := newBlockingACPAttempt()
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.NewWithACP(catalogService, executionService, fake, nil, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  "cursor-acp",
			AttemptID: "acp-attempt-blocks",
			Model:     "cursor-model",
		})
		executeDone <- err
	}()
	<-fake.started

	controlDone := make(chan providers.ControlAttemptResult, 1)
	go func() {
		result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
			Provider:  "cursor-acp",
			AttemptID: "acp-attempt-blocks",
			Action:    providers.ControlActionCancel,
		})
		if err != nil {
			t.Errorf("ControlAttempt() error = %v, want nil", err)
		}
		controlDone <- result
	}()

	<-fake.cancelledSeen
	select {
	case <-controlDone:
		t.Fatal("ControlAttempt() returned before the attempt observed its terminal behavior")
	default:
	}

	close(fake.releaseAttempt)
	result := <-controlDone
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt() outcome = %q, want completed", result.Outcome)
	}
	executeErr := <-executeDone
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("Execute() error = %v, want ExecuteFailureKindCanceled", executeErr)
	}
}

func TestControlAttempt_ACPCrossProviderIdentityIsolation(t *testing.T) {
	t.Parallel()

	target := newACPAwareAttempt("cursor-acp")
	bystander := newACPAwareAttempt("claude-acp")
	multi := &multiACPService{byProvider: map[providers.ID]*acpAwareAttempt{
		"cursor-acp": target,
		"claude-acp": bystander,
	}}
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.NewWithACP(catalogService, executionService, multi, nil, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}

	targetDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  "cursor-acp",
			AttemptID: "attempt-x",
			Model:     "cursor-model",
		})
		targetDone <- err
	}()
	<-target.started
	close(target.openCancelWindow)
	<-target.becameCancelable

	bystanderDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  "claude-acp",
			AttemptID: "attempt-x",
			Model:     "claude-model",
		})
		bystanderDone <- err
	}()
	<-bystander.started
	close(bystander.openCancelWindow)
	<-bystander.becameCancelable

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "cursor-acp",
		AttemptID: "attempt-x",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt() outcome = %q, want completed", result.Outcome)
	}
	var failure providers.ExecuteFailure
	if !errors.As(<-targetDone, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatal("target Execute() did not observe cancellation")
	}
	if bystander.cancelCalls != 0 {
		t.Fatalf("bystander ACP cancel calls = %d, want 0 (same attempt id, different provider)", bystander.cancelCalls)
	}

	close(bystander.release)
	if err := <-bystanderDone; err != nil {
		t.Fatalf("bystander Execute() error = %v, want nil (unrelated provider must be unaffected)", err)
	}
}

// failingCancelACPService wraps acpAwareAttempt to report a genuine
// signal-delivery failure from Cancel (for example a broken ACP connection)
// while still letting the underlying attempt observe the real cancellation
// path, proving ControlAttempt surfaces a genuine operation failure as an
// error distinct from the successful unsupported-capability result, per
// story 004's "control context cancellation or a real adapter signaling
// failure remains distinguishable from an unsupported capability result"
// requirement.
type failingCancelACPService struct {
	*acpAwareAttempt
	cancelErr error
}

func (f *failingCancelACPService) Cancel(ctx context.Context, id providers.ID, attemptID string) error {
	_ = f.acpAwareAttempt.Cancel(ctx, id, attemptID)
	return f.cancelErr
}

func TestControlAttempt_ACPSignalFailureIsDistinguishableFromUnsupportedAndClearsRegistration(t *testing.T) {
	t.Parallel()

	fake := newACPAwareAttempt("cursor-acp")
	failing := &failingCancelACPService{acpAwareAttempt: fake, cancelErr: errors.New("broken acp connection")}

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	logger := &recordingControlLogger{}
	root, err := providerservice.NewWithACP(catalogService, executionService, failing, nil, logger)
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  "cursor-acp",
			AttemptID: "acp-signal-failure",
			Model:     "cursor-model",
		})
		executeDone <- err
	}()
	<-fake.started
	close(fake.openCancelWindow)
	<-fake.becameCancelable

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "cursor-acp",
		AttemptID: "acp-signal-failure",
		Action:    providers.ControlActionCancel,
	})
	if err == nil {
		t.Fatal("ControlAttempt() error = nil, want a genuine signal-delivery failure")
	}
	if !errors.Is(err, providers.ErrControlSignalFailed) {
		t.Fatalf("ControlAttempt() error = %v, want errors.Is ErrControlSignalFailed", err)
	}
	if result != (providers.ControlAttemptResult{}) {
		t.Fatalf("ControlAttempt() result = %#v, want the zero value alongside a genuine failure error", result)
	}

	var failure providers.ExecuteFailure
	if !errors.As(<-executeDone, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatal("target Execute() did not still observe cancellation despite the reported signal failure")
	}

	outcome := logger.entriesFor("provider control attempt outcome")
	if len(outcome) != 1 || outcome[0].fields["outcome"] != "failed" {
		t.Fatalf("outcome log = %#v, want a single entry with outcome=failed", outcome)
	}
	assertNoUnsafeControlLogFields(t, outcome[0].fields)

	// The live registration must already be gone (claimed atomically before
	// signal() was even invoked, not stale): a second control for the same
	// identity is unsupported, and the adapter is not signaled again.
	second, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "cursor-acp",
		AttemptID: "acp-signal-failure",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("second ControlAttempt() error = %v, want nil", err)
	}
	if second.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("second ControlAttempt() outcome = %q, want unsupported (registration already claimed, not stale)", second.Outcome)
	}
	if fake.cancelCalls != 1 {
		t.Fatalf("ACP cancel calls = %d, want exactly 1 (no duplicate signal from the second control)", fake.cancelCalls)
	}
}
