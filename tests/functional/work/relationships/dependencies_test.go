package relationships

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	dependencyStartWorkstation  = "start"
	dependencyFinishWorkstation = "finish"
	dependencyRequiredState     = "complete"
)

// TestDependentWorkWaitsForPrerequisiteTargetState proves through public Work
// listings and Factory Event dispatch observations that a DEPENDS_ON dependent
// stays undispatched at its initial state until the prerequisite reaches the
// declared requiredState, then proceeds through the public work session once
// that prerequisite target state is satisfied.
func TestDependentWorkWaitsForPrerequisiteTargetState(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteWorkID := "task-prerequisite-a"
	dependentWorkID := "task-dependent-b"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteWorkID,
		Payload:    []byte("prerequisite task"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
	)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)

	assertDependencyWorkLocations(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "complete"):   2,
	})
	if !support.HasWorkAtCustomerState(listed, prerequisiteWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("prerequisite work %q not at %q in public listing: %#v", prerequisiteWorkID, dependencyRequiredState, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q not at %q in public listing: %#v", dependentWorkID, dependencyRequiredState, listed)
	}

	if got := len(support.ProviderCallsForWorker(provider, "starter")); got != 2 {
		t.Fatalf("starter provider calls = %d, want 2 (prerequisite then dependent)", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "finisher")); got != 2 {
		t.Fatalf("finisher provider calls = %d, want 2 (prerequisite then dependent)", got)
	}

	prerequisiteCompleteSequence, dependentStartSequence := dependencyDispatchOrdering(
		t,
		events,
		prerequisiteWorkID,
		dependentWorkID,
	)
	if dependentStartSequence <= prerequisiteCompleteSequence {
		t.Fatalf(
			"dependent %q dispatch at %q sequence = %d, want after prerequisite %q complete sequence %d",
			dependentWorkID,
			dependencyStartWorkstation,
			dependentStartSequence,
			prerequisiteWorkID,
			prerequisiteCompleteSequence,
		)
	}

	if session.Runtime.Progress.Categories.Terminal != 2 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want two terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

// TestDependentWorkDoesNotDispatchAfterPrerequisiteFailure proves through public
// Work listings and Factory Event dispatch observations that a DEPENDS_ON
// dependent never receives a worker dispatch when its prerequisite reaches a
// failed terminal outcome instead of the declared requiredState.
func TestDependentWorkDoesNotDispatchAfterPrerequisiteFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteWorkID := "task-prerequisite-a"
	dependentWorkID := "task-dependent-b"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteWorkID,
		Payload:    []byte("prerequisite task"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		[]error{
			nil,
			errors.New("prerequisite finish failed"),
		},
	)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)

	if !support.HasWorkAtCustomerState(listed, prerequisiteWorkID, support.WorkCustomerLocation("task", "failed")) {
		t.Fatalf("prerequisite work %q not at failed in public listing: %#v", prerequisiteWorkID, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", "failed")) {
		t.Fatalf("dependent work %q not at blocked failed state in public listing: %#v", dependentWorkID, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q reached %q after prerequisite failure: %#v", dependentWorkID, dependencyRequiredState, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", "processing")) {
		t.Fatalf("dependent work %q reached processing after prerequisite failure: %#v", dependentWorkID, listed)
	}

	if got := len(support.ProviderCallsForWorker(provider, "starter")); got != 1 {
		t.Fatalf("starter provider calls = %d, want 1 (prerequisite only)", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "finisher")); got != 1 {
		t.Fatalf("finisher provider calls = %d, want 1 (prerequisite only)", got)
	}

	assertNoDependentStartDispatch(t, events, dependentWorkID)

	if session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 2 {
		t.Fatalf("session progress categories = %+v, want zero terminal and two failed", session.Runtime.Progress.Categories)
	}
}

// TestFanInReleasesOnlyAfterEveryPrerequisite proves through public Work
// listings and Factory Event dispatch observations that a DEPENDS_ON join stays
// undispatched while only a proper subset of prerequisites has reached the
// declared requiredState, then proceeds only after every prerequisite target
// state is satisfied.
func TestFanInReleasesOnlyAfterEveryPrerequisite(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteAWorkID := "task-prerequisite-a"
	prerequisiteBWorkID := "task-prerequisite-b"
	dependentWorkID := "task-dependent-join"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteAWorkID,
		Payload:    []byte("prerequisite task A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteBWorkID,
		Payload:    []byte("prerequisite task B"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("fan-in dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteAWorkID,
				RequiredState: dependencyRequiredState,
			},
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteBWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	provider := newFanInSecondFinisherGateProvider()
	baseURL, daemon := startDependencyFactory(t, dir, provider)
	defer daemon.Stop(t)

	provider.WaitForSecondFinisherGate(t, 15*time.Second)

	partialListed := support.ListDefaultSessionWork(t, baseURL)
	partialEvents := support.GetFactoryEventsAt(t, baseURL)
	assertFanInBlockedAfterPartialPrerequisites(
		t,
		partialListed,
		partialEvents,
		prerequisiteAWorkID,
		prerequisiteBWorkID,
		dependentWorkID,
	)

	provider.Release()
	support.WaitForTerminalStatus(t, baseURL, 15*time.Second)

	session := support.GetDefaultSession(t, baseURL)
	listed := support.ListDefaultSessionWork(t, baseURL)
	events := support.GetFactoryEventsAt(t, baseURL)

	assertDependencyWorkLocations(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "complete"):   3,
	})
	if !support.HasWorkAtCustomerState(listed, prerequisiteAWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("prerequisite A %q not at %q in public listing: %#v", prerequisiteAWorkID, dependencyRequiredState, listed)
	}
	if !support.HasWorkAtCustomerState(listed, prerequisiteBWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("prerequisite B %q not at %q in public listing: %#v", prerequisiteBWorkID, dependencyRequiredState, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q not at %q in public listing: %#v", dependentWorkID, dependencyRequiredState, listed)
	}

	if got := provider.finisherCallCount(); got != 3 {
		t.Fatalf("finisher provider calls = %d, want 3 (both prerequisites and dependent)", got)
	}
	if got := provider.starterCallCount(); got != 3 {
		t.Fatalf("starter provider calls = %d, want 3 (both prerequisites and dependent)", got)
	}

	prerequisiteASequence, prerequisiteBSequence, dependentStartSequence := fanInDispatchOrdering(
		t,
		events,
		prerequisiteAWorkID,
		prerequisiteBWorkID,
		dependentWorkID,
	)
	if dependentStartSequence <= prerequisiteASequence {
		t.Fatalf(
			"dependent %q dispatch at %q sequence = %d, want after prerequisite A %q complete sequence %d",
			dependentWorkID,
			dependencyStartWorkstation,
			dependentStartSequence,
			prerequisiteAWorkID,
			prerequisiteASequence,
		)
	}
	if dependentStartSequence <= prerequisiteBSequence {
		t.Fatalf(
			"dependent %q dispatch at %q sequence = %d, want after prerequisite B %q complete sequence %d",
			dependentWorkID,
			dependencyStartWorkstation,
			dependentStartSequence,
			prerequisiteBWorkID,
			prerequisiteBSequence,
		)
	}

	if session.Runtime.Progress.Categories.Terminal != 3 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want three terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

func assertDependencyWorkLocations(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Fatalf("CountWorkAtCustomerState(%q) = %d, want %d; listed=%#v", location, got, want, listed)
		}
	}
}

func dependencyDispatchOrdering(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteWorkID, dependentWorkID string,
) (prerequisiteCompleteSequence, dependentStartSequence int) {
	t.Helper()

	prerequisiteCompleteSequence = -1
	dependentStartSequence = -1

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != dependencyFinishWorkstation {
				continue
			}
			if !dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteWorkID) {
				continue
			}
			prerequisiteCompleteSequence = event.Context.Sequence
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			if payload.TransitionId != dependencyStartWorkstation {
				continue
			}
			if !dispatchRequestIncludesWork(payload, dependentWorkID) {
				continue
			}
			if prerequisiteCompleteSequence < 0 {
				t.Fatalf(
					"dependent work %q received %q dispatch before prerequisite %q reached %q",
					dependentWorkID,
					dependencyStartWorkstation,
					prerequisiteWorkID,
					dependencyRequiredState,
				)
			}
			if dependentStartSequence < 0 {
				dependentStartSequence = event.Context.Sequence
			}
		}
	}

	if prerequisiteCompleteSequence < 0 {
		t.Fatalf("prerequisite work %q never reached %q through public dispatch", prerequisiteWorkID, dependencyRequiredState)
	}
	if dependentStartSequence < 0 {
		t.Fatalf("dependent work %q never received a public %q dispatch", dependentWorkID, dependencyStartWorkstation)
	}
	return prerequisiteCompleteSequence, dependentStartSequence
}

func dispatchRequestIncludesWork(payload factoryapi.DispatchRequestEventPayload, workID string) bool {
	for _, input := range payload.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}

func dispatchEventIncludesWork(workIDs *[]string, workID string) bool {
	if workIDs == nil {
		return false
	}
	for _, candidate := range *workIDs {
		if candidate == workID {
			return true
		}
	}
	return false
}

func assertNoDependentStartDispatch(t *testing.T, events []factoryapi.FactoryEvent, dependentWorkID string) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			continue
		}
		if payload.TransitionId != dependencyStartWorkstation {
			continue
		}
		if dispatchRequestIncludesWork(payload, dependentWorkID) {
			t.Fatalf(
				"dependent work %q received public %q dispatch after prerequisite failure at sequence %d",
				dependentWorkID,
				dependencyStartWorkstation,
				event.Context.Sequence,
			)
		}
	}
}

func startDependencyFactory(
	t *testing.T,
	dir string,
	provider workerprovider.Provider,
) (baseURL string, daemon *support.ProcessCommand) {
	t.Helper()

	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderOverride: provider,
		APIServerStarter: server.Start,
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	daemon = support.StartProcessCommand(t, process, inputs.Input)
	return server.WaitForURL(t), daemon
}

func assertFanInBlockedAfterPartialPrerequisites(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
	prerequisiteAWorkID, prerequisiteBWorkID, dependentWorkID string,
) {
	t.Helper()

	completeLocation := support.WorkCustomerLocation("task", dependencyRequiredState)
	processingLocation := support.WorkCustomerLocation("task", "processing")
	initLocation := support.WorkCustomerLocation("task", "init")

	aComplete := support.HasWorkAtCustomerState(listed, prerequisiteAWorkID, completeLocation)
	bComplete := support.HasWorkAtCustomerState(listed, prerequisiteBWorkID, completeLocation)
	if aComplete == bComplete {
		t.Fatalf(
			"expected exactly one prerequisite complete during partial fan-in; A complete=%t B complete=%t; listed=%#v",
			aComplete,
			bComplete,
			listed,
		)
	}

	if support.HasWorkAtCustomerState(listed, dependentWorkID, completeLocation) {
		t.Fatalf("dependent %q reached %q before every prerequisite completed: %#v", dependentWorkID, dependencyRequiredState, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, processingLocation) {
		t.Fatalf("dependent %q reached processing before every prerequisite completed: %#v", dependentWorkID, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, initLocation) {
		t.Fatalf("dependent %q not at init while prerequisites are still releasing: %#v", dependentWorkID, listed)
	}

	assertNoDependentStartDispatch(t, events, dependentWorkID)
}

func fanInDispatchOrdering(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteAWorkID, prerequisiteBWorkID, dependentWorkID string,
) (prerequisiteASequence, prerequisiteBSequence, dependentStartSequence int) {
	t.Helper()

	prerequisiteASequence = -1
	prerequisiteBSequence = -1
	dependentStartSequence = -1

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != dependencyFinishWorkstation {
				continue
			}
			if dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteAWorkID) {
				prerequisiteASequence = event.Context.Sequence
			}
			if dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteBWorkID) {
				prerequisiteBSequence = event.Context.Sequence
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			if payload.TransitionId != dependencyStartWorkstation {
				continue
			}
			if !dispatchRequestIncludesWork(payload, dependentWorkID) {
				continue
			}
			if prerequisiteASequence < 0 || prerequisiteBSequence < 0 {
				t.Fatalf(
					"dependent work %q received %q dispatch before both prerequisites reached %q",
					dependentWorkID,
					dependencyStartWorkstation,
					dependencyRequiredState,
				)
			}
			if dependentStartSequence < 0 {
				dependentStartSequence = event.Context.Sequence
			}
		}
	}

	if prerequisiteASequence < 0 {
		t.Fatalf("prerequisite A %q never reached %q through public dispatch", prerequisiteAWorkID, dependencyRequiredState)
	}
	if prerequisiteBSequence < 0 {
		t.Fatalf("prerequisite B %q never reached %q through public dispatch", prerequisiteBWorkID, dependencyRequiredState)
	}
	if dependentStartSequence < 0 {
		t.Fatalf("dependent work %q never received a public %q dispatch", dependentWorkID, dependencyStartWorkstation)
	}
	return prerequisiteASequence, prerequisiteBSequence, dependentStartSequence
}

type fanInSecondFinisherGateProvider struct {
	secondFinisherReached chan struct{}
	release               chan struct{}
	releaseOnce           sync.Once
	mu                    sync.Mutex
	finisherCalls         int
	starterCalls          int
}

var _ workerprovider.Provider = (*fanInSecondFinisherGateProvider)(nil)

func newFanInSecondFinisherGateProvider() *fanInSecondFinisherGateProvider {
	return &fanInSecondFinisherGateProvider{
		secondFinisherReached: make(chan struct{}, 1),
		release:               make(chan struct{}),
	}
}

func (p *fanInSecondFinisherGateProvider) Infer(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	workerType := req.WorkerType
	if workerType == "" {
		workerType = req.Dispatch.WorkerType
	}

	p.mu.Lock()
	switch workerType {
	case "starter":
		p.starterCalls++
	case "finisher":
		p.finisherCalls++
		blockSecondFinisher := p.finisherCalls == 2
		p.mu.Unlock()

		if blockSecondFinisher {
			select {
			case p.secondFinisherReached <- struct{}{}:
			default:
			}
			select {
			case <-p.release:
			case <-ctx.Done():
				return workerexecution.InferenceResponse{}, ctx.Err()
			}
		}
		return workerexecution.InferenceResponse{Content: "COMPLETE"}, nil
	default:
		p.mu.Unlock()
		return workerexecution.InferenceResponse{}, errors.New("unexpected worker type: " + workerType)
	}

	p.mu.Unlock()
	return workerexecution.InferenceResponse{Content: "COMPLETE"}, nil
}

func (p *fanInSecondFinisherGateProvider) WaitForSecondFinisherGate(t *testing.T, timeout time.Duration) {
	t.Helper()

	select {
	case <-p.secondFinisherReached:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting %s for second finisher gate", timeout)
	}
}

func (p *fanInSecondFinisherGateProvider) Release() {
	p.releaseOnce.Do(func() {
		close(p.release)
	})
}

func (p *fanInSecondFinisherGateProvider) finisherCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finisherCalls
}

func (p *fanInSecondFinisherGateProvider) starterCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.starterCalls
}
