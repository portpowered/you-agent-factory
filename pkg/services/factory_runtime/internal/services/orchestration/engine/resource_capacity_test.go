package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	workflowruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func TestResourceCapacityDecisionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		current     int
		requested   int
		inUse       int
		wantErr     error
		wantOutcome factory.ResourceCapacityOutcome
		wantAvail   int
	}{
		{name: "increase", current: 2, requested: 4, wantOutcome: factory.ResourceCapacityOutcomeApplied, wantAvail: 4},
		{name: "equal is no-op", current: 2, requested: 2, wantOutcome: factory.ResourceCapacityOutcomeNoOp, wantAvail: 2},
		{name: "decrease removes idle units", current: 4, requested: 2, wantOutcome: factory.ResourceCapacityOutcomeApplied, wantAvail: 2},
		{name: "negative rejected", current: 2, requested: -1, wantErr: factory.ErrResourceCapacityValidation, wantAvail: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eng := newResourceCapacityTestEngine(test.current)
			if test.inUse > 0 {
				markResourceUnitsInUse(eng, test.inUse)
			}
			before := eng.GetMarking()
			result, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{
				ResourceID: "gpu-slot", RequestedCapacity: test.requested,
			})
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("SetResourceCapacity error = %v, want %v", err, test.wantErr)
				}
				beforeMarking := before
				afterMarking := eng.GetMarking()
				if got := len(afterMarking.TokensInPlace("gpu-slot:available")); got != len(beforeMarking.TokensInPlace("gpu-slot:available")) {
					t.Fatalf("available token count after rejection = %d, want %d", got, len(beforeMarking.TokensInPlace("gpu-slot:available")))
				}
				return
			}
			if err != nil {
				t.Fatalf("SetResourceCapacity: %v", err)
			}
			if result.Outcome != test.wantOutcome || result.AvailableCount != test.wantAvail {
				t.Fatalf("result = %#v, want outcome %s and available %d", result, test.wantOutcome, test.wantAvail)
			}
			if got := eng.state.Resources["gpu-slot"].Capacity; got != test.requested {
				t.Fatalf("effective capacity = %d, want %d", got, test.requested)
			}
		})
	}
}

func TestResourceCapacityRejectsReductionBelowInUseWithoutMutation(t *testing.T) {
	eng := newResourceCapacityTestEngine(3)
	markResourceUnitsInUse(eng, 2)
	before := eng.GetRuntimeStateSnapshot()

	result, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 1,
	})
	if !errors.Is(err, factory.ErrResourceCapacityInUse) {
		t.Fatalf("SetResourceCapacity error = %v, want ErrResourceCapacityInUse", err)
	}
	if result.EffectiveCapacity != 3 {
		t.Fatalf("rejected result effective capacity = %d, want 3", result.EffectiveCapacity)
	}
	after := eng.GetRuntimeStateSnapshot()
	if eng.state.Resources["gpu-slot"].Capacity != 3 || len(after.Marking.Tokens) != len(before.Marking.Tokens) {
		t.Fatalf("runtime changed after capacity-in-use rejection: before=%d after=%d capacity=%d", len(before.Marking.Tokens), len(after.Marking.Tokens), eng.state.Resources["gpu-slot"].Capacity)
	}
}

func TestResourceCapacityZeroBlocksUntilRaised(t *testing.T) {
	eng := newResourceCapacityTestEngine(2)
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 0}); err != nil {
		t.Fatalf("set zero capacity: %v", err)
	}
	marking := eng.GetMarking()
	if got := len(marking.TokensInPlace("gpu-slot:available")); got != 0 {
		t.Fatalf("available tokens at zero capacity = %d, want 0", got)
	}
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 1}); err != nil {
		t.Fatalf("raise zero capacity: %v", err)
	}
	marking = eng.GetMarking()
	if got := len(marking.TokensInPlace("gpu-slot:available")); got != 1 {
		t.Fatalf("available tokens after raise = %d, want 1", got)
	}
}

func TestResourceCapacityAdmissionBlocksUntilReleased(t *testing.T) {
	eng := newResourceCapacityTestEngine(1)
	release, err := eng.AcquireResourceCapacityAdmission(context.Background())
	if err != nil {
		t.Fatalf("acquire admission: %v", err)
	}
	defer release()

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 2})
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("capacity mutation completed before release: %v", err)
	default:
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("capacity mutation after release: %v", err)
	}
}

func TestResourceCapacityLeaseWaitsForLiveIncreaseAndReleasesExactlyOnce(t *testing.T) {
	eng := newResourceCapacityTestEngine(1)
	first, err := eng.AcquireResourceCapacityLease(context.Background(), factory.ResourceCapacityLeaseRequest{
		ResourceID: "gpu-slot",
	})
	if err != nil {
		t.Fatalf("acquire first resource lease: %v", err)
	}

	secondDone := make(chan *factory.ResourceCapacityLease, 1)
	secondErr := make(chan error, 1)
	go func() {
		lease, acquireErr := eng.AcquireResourceCapacityLease(context.Background(), factory.ResourceCapacityLeaseRequest{
			ResourceID: "gpu-slot",
		})
		if acquireErr != nil {
			secondErr <- acquireErr
			return
		}
		secondDone <- lease
	}()
	select {
	case lease := <-secondDone:
		lease.Release()
		t.Fatal("second resource lease completed before capacity increased")
	case err := <-secondErr:
		t.Fatalf("second resource lease failed before capacity increased: %v", err)
	default:
	}

	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 2,
	}); err != nil {
		t.Fatalf("increase resource capacity: %v", err)
	}

	var second *factory.ResourceCapacityLease
	select {
	case second = <-secondDone:
	case err := <-secondErr:
		t.Fatalf("second resource lease after capacity increase: %v", err)
	}
	if second.FactoryRevision != 0 {
		t.Fatalf("second lease factory revision = %d, want initial revision 0", second.FactoryRevision)
	}
	preview, err := eng.PreviewResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 2})
	if err != nil {
		t.Fatalf("preview leased capacity: %v", err)
	}
	if preview.InUseCount != 2 || preview.AvailableCount != 0 {
		t.Fatalf("leased capacity accounting = in-use %d available %d, want 2/0", preview.InUseCount, preview.AvailableCount)
	}

	first.Release()
	first.Release()
	second.Release()
	second.Release()
	preview, err = eng.PreviewResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 2})
	if err != nil {
		t.Fatalf("preview released capacity: %v", err)
	}
	if preview.InUseCount != 0 || preview.AvailableCount != 2 {
		t.Fatalf("released capacity accounting = in-use %d available %d, want 0/2", preview.InUseCount, preview.AvailableCount)
	}
}

func TestResourceCapacityLeasePreventsReductionBelowInUse(t *testing.T) {
	eng := newResourceCapacityTestEngine(1)
	lease, err := eng.AcquireResourceCapacityLease(context.Background(), factory.ResourceCapacityLeaseRequest{ResourceID: "gpu-slot"})
	if err != nil {
		t.Fatalf("acquire resource lease: %v", err)
	}
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 0}); !errors.Is(err, factory.ErrResourceCapacityInUse) {
		t.Fatalf("reduce leased resource error = %v, want ErrResourceCapacityInUse", err)
	}
	lease.Release()
	result, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 0})
	if err != nil || result.Outcome != factory.ResourceCapacityOutcomeApplied {
		t.Fatalf("reduce released resource = %#v, err %v, want applied", result, err)
	}
}

func TestResourceCapacityIncreaseDoesNotReuseLeasedTokenID(t *testing.T) {
	eng := newResourceCapacityTestEngine(2)
	lease, err := eng.AcquireResourceCapacityLease(context.Background(), factory.ResourceCapacityLeaseRequest{ResourceID: "gpu-slot"})
	if err != nil {
		t.Fatalf("acquire resource lease: %v", err)
	}
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 1}); err != nil {
		t.Fatalf("reduce capacity around leased token: %v", err)
	}
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 2}); err != nil {
		t.Fatalf("increase capacity around leased token: %v", err)
	}
	lease.Release()

	marking := eng.GetMarking()
	tokens := marking.TokensInPlace("gpu-slot:available")
	if len(tokens) != 2 {
		t.Fatalf("available resource tokens = %d, want 2", len(tokens))
	}
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if _, exists := seen[token.ID]; exists {
			t.Fatalf("duplicate available resource token ID %q", token.ID)
		}
		seen[token.ID] = struct{}{}
	}
}

func TestResourceCapacityIncreaseDoesNotReuseActiveDispatchTokenID(t *testing.T) {
	eng := newResourceCapacityTestEngine(1)
	markResourceUnitsInUse(eng, 1)
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 2}); err != nil {
		t.Fatalf("increase capacity around active dispatch: %v", err)
	}

	marking := eng.GetMarking()
	tokens := marking.TokensInPlace("gpu-slot:available")
	if len(tokens) != 1 {
		t.Fatalf("available resource tokens = %d, want 1", len(tokens))
	}
	if tokens[0].ID == "gpu-slot:resource:0" {
		t.Fatalf("capacity increase reused active dispatch token ID %q", tokens[0].ID)
	}
}

func TestResourceCapacityLeaseUsesMonotonicFactoryRevision(t *testing.T) {
	eng := newResourceCapacityTestEngine(1)
	eng.SetFactoryRevision(4)
	eng.SetFactoryRevision(2)
	lease, err := eng.AcquireResourceCapacityLease(context.Background(), factory.ResourceCapacityLeaseRequest{ResourceID: "gpu-slot"})
	if err != nil {
		t.Fatalf("acquire revisioned resource lease: %v", err)
	}
	defer lease.Release()
	if lease.FactoryRevision != 4 || eng.CurrentFactoryRevision() != 4 {
		t.Fatalf("resource lease revision = %d, runtime revision = %d, want both 4", lease.FactoryRevision, eng.CurrentFactoryRevision())
	}
}

func TestJavaScriptParallelResourceChildrenWakeOnLiveCapacityIncrease(t *testing.T) {
	eng := newResourceCapacityTestEngine(1)
	eng.SetFactoryRevision(4)
	releaseFirst := make(chan struct{})
	children := &resourceBoundJavaScriptChildren{
		engine:         eng,
		firstAdmitted:  make(chan struct{}),
		secondAdmitted: make(chan struct{}),
		releaseFirst:   releaseFirst,
	}
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 2
	policy.Concurrency = 2

	done := make(chan workflowruntime.Outcome, 1)
	errDone := make(chan error, 1)
	go func() {
		outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
			Source:    `return parallel([{prompt: "one", resourceId: "gpu-slot"}, {prompt: "two", resourceId: "gpu-slot"}]);`,
			SessionID: "session-resource-parallel",
			Policy:    policy,
		}, workflowruntime.Hooks{
			NewChildExecutor: func(string, workflowruntime.ChildRecordSink, workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
				return children
			},
		})
		done <- outcome
		errDone <- err
	}()

	<-children.firstAdmitted
	select {
	case <-children.secondAdmitted:
		t.Fatal("second JavaScript resource child admitted before capacity increase")
	default:
	}
	eng.SetFactoryRevision(5)
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 2}); err != nil {
		t.Fatalf("increase JavaScript resource capacity: %v", err)
	}
	<-children.secondAdmitted
	if children.peakActive() != 2 {
		t.Fatalf("peak JavaScript resource admissions = %d, want 2", children.peakActive())
	}
	close(releaseFirst)

	outcome := <-done
	if err := <-errDone; err != nil || !outcome.OK {
		t.Fatalf("JavaScript resource workflow outcome=%#v err=%v", outcome, err)
	}
	children.mu.Lock()
	revisions := append([]int(nil), children.revisions...)
	children.mu.Unlock()
	if len(revisions) != 2 || revisions[0] != 4 || revisions[1] != 5 {
		t.Fatalf("JavaScript child admission revisions = %v, want prior/new [4 5]", revisions)
	}
}

type resourceBoundJavaScriptChildren struct {
	engine         *FactoryEngine
	firstAdmitted  chan struct{}
	secondAdmitted chan struct{}
	releaseFirst   <-chan struct{}
	mu             sync.Mutex
	active         int
	peak           int
	admitted       int
	revisions      []int
}

func (e *resourceBoundJavaScriptChildren) Execute(
	ctx context.Context,
	req workflowruntime.ChildExecutionRequest,
) (workflowruntime.ChildExecutionResult, error) {
	lease, err := e.engine.AcquireResourceCapacityLease(ctx, factory.ResourceCapacityLeaseRequest{ResourceID: req.ResourceID})
	if err != nil {
		return workflowruntime.ChildExecutionResult{}, err
	}
	defer lease.Release()
	e.mu.Lock()
	e.admitted++
	e.active++
	if e.active > e.peak {
		e.peak = e.active
	}
	e.revisions = append(e.revisions, lease.FactoryRevision)
	which := e.admitted
	if which == 1 {
		close(e.firstAdmitted)
	} else if which == 2 {
		close(e.secondAdmitted)
	}
	e.mu.Unlock()
	if which == 1 {
		<-e.releaseFirst
	}
	e.mu.Lock()
	e.active--
	e.mu.Unlock()
	return workflowruntime.ChildExecutionResult{
		Status:        workflowruntime.ChildDispatchStatusCompleted,
		ExecutionMode: workflowruntime.ChildExecutionModeLive,
		Request:       req,
	}, nil
}

func (e *resourceBoundJavaScriptChildren) peakActive() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.peak
}

func newResourceCapacityTestEngine(capacity int) *FactoryEngine {
	net := buildTestNet()
	net.Resources = map[string]*state.ResourceDef{
		"gpu-slot": {ID: "gpu-slot", Name: "GPU slot", Capacity: capacity},
	}
	place, tokens := state.GenerateResourcePlaces(net.Resources["gpu-slot"], time.Unix(0, 0))
	net.Places[place.ID] = place
	marking := petri.NewMarking(net.ID)
	for _, token := range tokens {
		marking.AddToken(token)
	}
	return newTestFactoryEngine(net, marking, nil)
}

func markResourceUnitsInUse(eng *FactoryEngine, count int) {
	marking := eng.GetMarking()
	available := marking.TokensInPlace("gpu-slot:available")
	if count > len(available) {
		count = len(available)
	}
	eng.mu.Lock()
	defer eng.mu.Unlock()
	for _, token := range available[:count] {
		eng.runtimeState.Marking.RemoveToken(token.ID)
	}
	consumed := make([]factorytoken.Token, 0, count)
	for _, token := range available[:count] {
		token.PlaceID = "gpu-slot:held"
		consumed = append(consumed, token)
	}
	eng.runtimeState.Dispatches["dispatch-resource"] = &interfaces.DispatchEntry{ConsumedTokens: consumed}
}
