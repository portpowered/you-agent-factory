package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

// acpAwareGeneration is the opaque per-claim capability acpAwareAttempt hands
// out from Claim, mirroring cancelwindow.Session's role for the real
// implementation: TryCancel only acts when the generation it is handed is
// still the one Claim captured (checked by identity via owner+attemptID),
// never by re-deriving liveness from provider/attemptID strings alone.
type acpAwareGeneration struct {
	owner     *acpAwareAttempt
	attemptID string
}

// acpAwareAttempt is a channel-gated fake acp.Service standing in for one
// live ACP attempt. It reports when Execute started, only becomes
// truthfully cancelable once the test signals openCancelWindow (modeling
// the span between an attempt's session/new and session/prompt returning,
// see acp.Service.Claim), and returns the same ExecuteFailureKindCanceled
// outcome the real ACP session/cancel path normalizes StopReasonCancelled to
// once Cancel names its exact attempt while cancelable.
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

func (a *acpAwareAttempt) NegotiatedCapabilities(providers.ID) (acpsdk.AgentCapabilities, bool) {
	return acpsdk.AgentCapabilities{}, false
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

func (a *acpAwareAttempt) Claim(_ providers.ID, attemptID string) (acp.Generation, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.cancelable || a.attemptID != attemptID {
		return nil, false
	}
	return acpAwareGeneration{owner: a, attemptID: attemptID}, true
}

func (a *acpAwareAttempt) TryCancel(_ context.Context, generation acp.Generation) (bool, error) {
	gen, generationOK := generation.(acpAwareGeneration)
	a.mu.Lock()
	matches := generationOK && gen.owner == a && a.cancelable && a.attemptID == gen.attemptID
	if matches {
		a.cancelCalls++
	}
	a.mu.Unlock()
	if !matches {
		return false, nil
	}
	select {
	case <-a.canceledSignal:
	default:
		close(a.canceledSignal)
	}
	<-a.done
	return true, nil
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

func (m *multiACPService) NegotiatedCapabilities(providers.ID) (acpsdk.AgentCapabilities, bool) {
	return acpsdk.AgentCapabilities{}, false
}

func (m *multiACPService) Execute(
	ctx context.Context,
	id providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return m.byProvider[id].Execute(ctx, id, request)
}

func (m *multiACPService) Claim(id providers.ID, attemptID string) (acp.Generation, bool) {
	target, ok := m.byProvider[id]
	if !ok {
		return nil, false
	}
	return target.Claim(id, attemptID)
}

func (m *multiACPService) TryCancel(ctx context.Context, generation acp.Generation) (bool, error) {
	gen, ok := generation.(acpAwareGeneration)
	if !ok || gen.owner == nil {
		return false, nil
	}
	return gen.owner.TryCancel(ctx, generation)
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

func (a *blockingACPAttempt) NegotiatedCapabilities(providers.ID) (acpsdk.AgentCapabilities, bool) {
	return acpsdk.AgentCapabilities{}, false
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

func (a *blockingACPAttempt) Claim(_ providers.ID, attemptID string) (acp.Generation, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.cancelable || a.attemptID != attemptID {
		return nil, false
	}
	return acpAwareGeneration{owner: nil, attemptID: attemptID}, true
}

func (a *blockingACPAttempt) TryCancel(_ context.Context, generation acp.Generation) (bool, error) {
	gen, generationOK := generation.(acpAwareGeneration)
	a.mu.Lock()
	matches := generationOK && a.cancelable && a.attemptID == gen.attemptID
	a.mu.Unlock()
	if !matches {
		return false, nil
	}
	close(a.cancelledSeen)
	<-a.releaseAttempt
	return true, nil
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
// signal-delivery failure from TryCancel (for example a broken ACP
// connection) while still letting the underlying attempt observe the real
// cancellation path, proving ControlAttempt surfaces a genuine operation
// failure as an error distinct from the successful unsupported-capability
// result, per story 004's "control context cancellation or a real adapter
// signaling failure remains distinguishable from an unsupported capability
// result" requirement. It wraps cancelErr in providers.ErrControlSignalFailed
// itself, matching the contract the real acp.Service implementation upholds
// (see acp/internal/service/cancel.go's tryCancel): TryCancel's non-context
// errors are already classified as genuine delivery failures by the time
// they reach acpAttemptControl.signal.
type failingCancelACPService struct {
	*acpAwareAttempt
	cancelErr error
}

func (f *failingCancelACPService) TryCancel(ctx context.Context, generation acp.Generation) (bool, error) {
	_, _ = f.acpAwareAttempt.TryCancel(ctx, generation)
	return false, fmt.Errorf("%w: %v", providers.ErrControlSignalFailed, f.cancelErr)
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

// raceLostACPService reports a stale Claim()-succeeds pre-check (as a real
// ACP session can look live a moment before it naturally finishes) while
// TryCancel grounds its answer in the real recorded outcome and reports the
// race was lost to natural completion. This is the root-level regression for
// the ACP Claim/TryCancel TOCTOU this story's review required: a claimed
// control (registry.claim consulted the stale Claim pre-check) must still
// report Unsupported, never a false Completed, when the underlying
// acp.Service.TryCancel says the cancellation was not actually accepted.
type raceLostACPService struct {
	*acpAwareAttempt
}

func (r *raceLostACPService) Claim(providers.ID, string) (acp.Generation, bool) {
	return acpAwareGeneration{owner: r.acpAwareAttempt}, true
}

func (r *raceLostACPService) TryCancel(context.Context, acp.Generation) (bool, error) {
	return false, nil
}

func TestControlAttempt_ACPClaimedControlLosingRaceToNaturalCompletionReturnsUnsupported(t *testing.T) {
	t.Parallel()

	fake := newACPAwareAttempt("cursor-acp")
	racing := &raceLostACPService{acpAwareAttempt: fake}

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.NewWithACP(catalogService, executionService, racing, nil, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  "cursor-acp",
			AttemptID: "acp-race-lost",
			Model:     "cursor-model",
		})
		executeDone <- err
	}()
	<-fake.started
	close(fake.openCancelWindow)
	<-fake.becameCancelable

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "cursor-acp",
		AttemptID: "acp-race-lost",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt() outcome = %q, want unsupported (claimed control lost the race to natural completion)", result.Outcome)
	}

	// The underlying attempt must have actually completed normally, not via
	// cancellation: proves the race-lost path did not corrupt the attempt's
	// real outcome even though the registration was claimed and removed.
	close(fake.release)
	if err := <-executeDone; err != nil {
		t.Fatalf("Execute() error = %v, want nil (normal completion, not cancellation)", err)
	}
}

// sequentialGeneration is one live execution generation for
// sequentialACPService, mirroring cancelwindow.Session: cancelled is closed
// by TryCancel to request cancellation, done is closed by Execute with the
// real recorded outcome once this exact generation ends, and accepted holds
// that real outcome. Because each Execute call allocates a fresh
// *sequentialGeneration even when the caller reuses the same attemptID
// string, a delayed TryCancel holding a stale generation pointer can never
// be satisfied by a later, unrelated generation.
type sequentialGeneration struct {
	attemptID string
	cancelled chan struct{}
	done      chan struct{}
	accepted  bool
}

// sequentialACPService is a fake acp.Service supporting multiple sequential
// executions that reuse the same provider/attemptID identity, modeling the
// real daemon's single-slot cancelwindow.Window across generations. Claim
// captures whichever *sequentialGeneration is currently open; TryCancel's
// delivery can be paused mid-flight via armTryCancelGate so a test can
// deterministically interleave "claim generation A", "A completes and fully
// releases", "generation B opens with the same identity", and "A's delayed
// signal resumes" without any sleep-based timing.
type sequentialACPService struct {
	provider providers.ID

	mu         sync.Mutex
	attemptID  string
	generation *sequentialGeneration
	release    chan struct{}
	ready      chan struct{}

	tryCancelEntered chan struct{}
	tryCancelGate    chan struct{}
}

func newSequentialACPService(provider providers.ID) *sequentialACPService {
	return &sequentialACPService{provider: provider}
}

func (s *sequentialACPService) Close(context.Context) error { return nil }
func (s *sequentialACPService) Configure(context.Context, []providers.ACPIntegration) error {
	return nil
}
func (s *sequentialACPService) Integrations() []providers.ACPIntegration { return nil }
func (s *sequentialACPService) Resolve(id providers.ID) (providers.ID, bool) {
	return s.provider, id == s.provider
}

func (s *sequentialACPService) NegotiatedCapabilities(providers.ID) (acpsdk.AgentCapabilities, bool) {
	return acpsdk.AgentCapabilities{}, false
}

// beginExecute arms this service for the next Execute call: the caller must
// call this before starting that Execute call (in a goroutine), then wait on
// the returned ready channel before claiming a control against the
// generation Execute opens.
func (s *sequentialACPService) beginExecute() (release chan struct{}, ready <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release = make(chan struct{})
	readyCh := make(chan struct{})
	s.release, s.ready = release, readyCh
	return release, readyCh
}

func (s *sequentialACPService) Execute(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	s.mu.Lock()
	release, ready := s.release, s.ready
	generation := &sequentialGeneration{attemptID: request.AttemptID, cancelled: make(chan struct{}), done: make(chan struct{})}
	s.attemptID, s.generation = request.AttemptID, generation
	s.mu.Unlock()
	close(ready)

	var cancelled bool
	select {
	case <-release:
	case <-generation.cancelled:
		cancelled = true
	}

	s.mu.Lock()
	if s.generation == generation {
		s.generation = nil
	}
	s.mu.Unlock()
	generation.accepted = cancelled
	close(generation.done)

	if cancelled {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
	}
	return providers.ExecuteResult{Content: request.AttemptID}, nil
}

// Claim captures whichever generation is currently open for attemptID, if
// any - the fake's analog of cancelwindow.Window.Claim.
func (s *sequentialACPService) Claim(_ providers.ID, attemptID string) (acp.Generation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == nil || s.attemptID != attemptID {
		return nil, false
	}
	return s.generation, true
}

// armTryCancelGate installs a one-shot gate: the next TryCancel call closes
// the returned entered channel the moment it is invoked, then blocks until
// releaseGate is called. Only the single next TryCancel call is gated; later
// calls proceed immediately.
func (s *sequentialACPService) armTryCancelGate() (entered <-chan struct{}, releaseGate func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enteredCh := make(chan struct{})
	gateCh := make(chan struct{})
	s.tryCancelEntered, s.tryCancelGate = enteredCh, gateCh
	var once sync.Once
	return enteredCh, func() { once.Do(func() { close(gateCh) }) }
}

// TryCancel delivers only to the exact generation captured by a prior Claim
// call, never re-deriving liveness from provider/attemptID strings - the
// fake's analog of cancelwindow.Session.TryCancel.
func (s *sequentialACPService) TryCancel(ctx context.Context, generationValue acp.Generation) (bool, error) {
	s.mu.Lock()
	entered, gate := s.tryCancelEntered, s.tryCancelGate
	s.tryCancelEntered, s.tryCancelGate = nil, nil
	s.mu.Unlock()
	if entered != nil {
		close(entered)
	}
	if gate != nil {
		<-gate
	}

	generation, ok := generationValue.(*sequentialGeneration)
	if !ok || generation == nil {
		return false, nil
	}
	select {
	case <-generation.done:
		// Already terminal by the time this delivery ran: a stale claim,
		// must not be reinterpreted against whatever is live now.
		return false, nil
	default:
	}
	close(generation.cancelled)
	select {
	case <-generation.done:
		return generation.accepted, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// TestControlAttempt_ACPDelayedControlCannotRedirectToReplacementGenerationAfterIdentityReuse
// is the root-level deterministic ABA regression required by
// ACP-L2-FIX-PRV-CONTROL-GENERATION-002: claim a control for generation A,
// let A complete and fully release (both the Providers live-attempt registry
// entry and the ACP cancel-window ownership), bind and open generation B
// with the identical canonical provider and attempt ID, then resume A's
// already-claimed but delayed signal. Generation B must receive zero
// notifications, complete normally on its own terms, and remain
// independently controllable; A's delayed control must report Unsupported
// and never derive a false Completed from B's terminal outcome. Every step
// is gated by real channels (armTryCancelGate, beginExecute's ready channel,
// Execute/ControlAttempt done channels) - no sleep-based timing is used.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestControlAttempt_ACPDelayedControlCannotRedirectToReplacementGenerationAfterIdentityReuse(t *testing.T) {
	t.Parallel()

	const identity = "acp-identity-reused"
	fake := newSequentialACPService("cursor-acp")

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

	// --- Generation A opens and its control is claimed.
	releaseA, readyA := fake.beginExecute()
	executeADone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider: "cursor-acp", AttemptID: identity, Model: "cursor-model",
		})
		executeADone <- err
	}()
	<-readyA

	tryCancelEntered, releaseTryCancelGate := fake.armTryCancelGate()
	controlAResult := make(chan providers.ControlAttemptResult, 1)
	controlAErr := make(chan error, 1)
	go func() {
		result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
			Provider: "cursor-acp", AttemptID: identity, Action: providers.ControlActionCancel,
		})
		controlAErr <- err
		controlAResult <- result
	}()
	// Once this fires, the Providers registry entry for `identity` is already
	// gone (registry.claim removed it before ControlAttempt ever called
	// signal), and this control's delivery is paused mid-flight.
	<-tryCancelEntered

	// --- A completes naturally (not via this control) and fully releases:
	// its registry entry is already gone, and closing releaseA now also
	// closes A's cancel-window generation (done).
	close(releaseA)
	if err := <-executeADone; err != nil {
		t.Fatalf("Execute(A) error = %v, want nil (natural completion)", err)
	}

	// --- Generation B opens, reusing the identical canonical provider and
	// attempt ID, now that A's registry and cancel-window ownership are both
	// released.
	releaseB, readyB := fake.beginExecute()
	executeBDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider: "cursor-acp", AttemptID: identity, Model: "cursor-model",
		})
		executeBDone <- err
	}()
	<-readyB

	// --- Resume A's delayed signal.
	releaseTryCancelGate()
	if err := <-controlAErr; err != nil {
		t.Fatalf("ControlAttempt(A, delayed) error = %v, want nil", err)
	}
	resultA := <-controlAResult
	if resultA.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt(A, delayed) outcome = %q, want unsupported: must not reach or derive completed from generation B", resultA.Outcome)
	}

	// --- B must have received zero notifications: it completes on its own
	// terms (not cancellation) once released, exactly as if A's delayed
	// control never existed.
	close(releaseB)
	if err := <-executeBDone; err != nil {
		t.Fatalf("Execute(B) error = %v, want nil: A's delayed control must not have reached B", err)
	}

	// --- B must remain independently, correctly claimable/cancelable: prove
	// this with a fresh generation C using the same identity. C is expected
	// to end via the cancellation below, not via release.
	_, readyC := fake.beginExecute()
	executeCDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider: "cursor-acp", AttemptID: identity, Model: "cursor-model",
		})
		executeCDone <- err
	}()
	<-readyC

	resultC, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider: "cursor-acp", AttemptID: identity, Action: providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(C) error = %v, want nil", err)
	}
	if resultC.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt(C) outcome = %q, want completed: the identity must remain independently controllable after the stale A claim", resultC.Outcome)
	}
	if err := <-executeCDone; err != nil {
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
			t.Fatalf("Execute(C) error = %v, want ExecuteFailureKindCanceled", err)
		}
	} else {
		t.Fatal("Execute(C) error = nil, want ExecuteFailureKindCanceled")
	}
}
