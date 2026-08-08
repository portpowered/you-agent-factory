package guards

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	secondaryJoinProducerWorkID = "producer-prerequisite"
	secondaryJoinPlanWorkID     = "plan-joined"
	secondaryJoinTaskWorkID     = "task-joined"
	secondaryJoinName           = "joined-item"
	secondaryJoinProduce        = "produce-prerequisite"
	secondaryJoinTransition     = "join-items"
)

// TestDependsOnSecondaryJoinedInput proves through public Factory Events that
// a SAME_NAME binding remains undispatched while a DEPENDS_ON relation on its
// secondary input is blocked, then dispatches exactly once after that
// prerequisite reaches its required terminal state.
func TestDependsOnSecondaryJoinedInput(t *testing.T) {
	dir := support.ScaffoldFactory(t, secondaryDependencyJoinFactoryConfig())
	support.WriteAgentConfig(t, dir, "producer", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "matcher", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "producer",
		WorkID:     secondaryJoinProducerWorkID,
		WorkTypeID: "producer",
		Payload:    []byte(`{"role":"controlled prerequisite"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       secondaryJoinName,
		WorkID:     secondaryJoinPlanWorkID,
		WorkTypeID: "plan",
		Payload:    []byte(`{"role":"primary join input"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       secondaryJoinName,
		WorkID:     secondaryJoinTaskWorkID,
		WorkTypeID: "task",
		Payload:    []byte(`{"role":"secondary join input"}`),
		Relations: []work.Relation{{
			Type:          work.RelationDependsOn,
			TargetWorkID:  secondaryJoinProducerWorkID,
			RequiredState: "complete",
		}},
	})

	provider := newSecondaryJoinProvider()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	provider.WaitForProducer(t, 15*time.Second)

	blockedEvents := server.GetFactoryEvents(t)
	assertNoTransitionDispatch(t, blockedEvents, secondaryJoinTransition)

	provider.ReleaseProducer()
	provider.WaitForJoinedDispatch(t, 15*time.Second)
	support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)

	finalEvents := server.GetFactoryEvents(t)
	joinedRequests := dispatchRequestsForTransition(t, finalEvents, secondaryJoinTransition)
	if len(joinedRequests) != 1 {
		t.Fatalf("%s dispatch requests = %d, want exactly one; events=%#v", secondaryJoinTransition, len(joinedRequests), finalEvents)
	}
	assertJoinedInputBinding(t, joinedRequests[0], secondaryJoinPlanWorkID, secondaryJoinTaskWorkID)

	producerCompleteSequence := dispatchResponseSequenceForWork(
		t,
		finalEvents,
		secondaryJoinProduce,
		secondaryJoinProducerWorkID,
	)
	if joinedRequests[0].Context.Sequence <= producerCompleteSequence {
		t.Fatalf(
			"joined dispatch sequence = %d, want after producer completion sequence %d",
			joinedRequests[0].Context.Sequence,
			producerCompleteSequence,
		)
	}
}

func secondaryDependencyJoinFactoryConfig() map[string]any {
	return map[string]any{
		"name": "secondary-dependency-join",
		"workTypes": []map[string]any{
			{
				"name": "producer",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
			{
				"name": "plan",
				"states": []map[string]string{
					{"name": "ready", "type": "INITIAL"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "ready", "type": "INITIAL"},
					{"name": "matched", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "producer"},
			{"name": "matcher"},
		},
		"workstations": []map[string]any{
			{
				"name":   secondaryJoinProduce,
				"worker": "producer",
				"inputs": []map[string]string{{"workType": "producer", "state": "init"}},
				"outputs": []map[string]string{{
					"workType": "producer",
					"state":    "complete",
				}},
			},
			{
				"name":   secondaryJoinTransition,
				"worker": "matcher",
				"inputs": []map[string]any{
					{"workType": "plan", "state": "ready"},
					{
						"workType": "task",
						"state":    "ready",
						"guards": []map[string]string{{
							"type":       "SAME_NAME",
							"matchInput": "plan",
						}},
					},
				},
				"outputs": []map[string]string{{
					"workType": "task",
					"state":    "matched",
				}},
			},
		},
	}
}

type secondaryJoinProvider struct {
	producerStarted chan struct{}
	joinedStarted   chan struct{}
	releaseProducer chan struct{}
	producerOnce    sync.Once
	joinedOnce      sync.Once
	releaseOnce     sync.Once
}

var _ workerprovider.Provider = (*secondaryJoinProvider)(nil)

func newSecondaryJoinProvider() *secondaryJoinProvider {
	return &secondaryJoinProvider{
		producerStarted: make(chan struct{}),
		joinedStarted:   make(chan struct{}),
		releaseProducer: make(chan struct{}),
	}
}

func (p *secondaryJoinProvider) Infer(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	workerType := req.WorkerType
	if workerType == "" {
		workerType = req.Dispatch.WorkerType
	}
	switch workerType {
	case "producer":
		p.producerOnce.Do(func() { close(p.producerStarted) })
		select {
		case <-p.releaseProducer:
		case <-ctx.Done():
			return workerexecution.InferenceResponse{}, ctx.Err()
		}
		return workerexecution.InferenceResponse{Content: "producer complete: COMPLETE"}, nil
	case "matcher":
		p.joinedOnce.Do(func() { close(p.joinedStarted) })
		return workerexecution.InferenceResponse{Content: "joined: COMPLETE"}, nil
	default:
		return workerexecution.InferenceResponse{}, errors.New("unexpected worker type: " + workerType)
	}
}

func (p *secondaryJoinProvider) WaitForProducer(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-p.producerStarted:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for controlled producer dispatch within %s", timeout)
	}
}

func (p *secondaryJoinProvider) ReleaseProducer() {
	p.releaseOnce.Do(func() { close(p.releaseProducer) })
}

func (p *secondaryJoinProvider) WaitForJoinedDispatch(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-p.joinedStarted:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for joined dispatch within %s", timeout)
	}
}

func assertNoTransitionDispatch(t *testing.T, events []factoryapi.FactoryEvent, transitionID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch request %q: %v", event.Id, err)
		}
		if payload.TransitionId == transitionID {
			t.Fatalf("%s dispatched before producer release at sequence %d", transitionID, event.Context.Sequence)
		}
	}
}

func dispatchRequestsForTransition(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	transitionID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	var matches []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch request %q: %v", event.Id, err)
		}
		if payload.TransitionId == transitionID {
			matches = append(matches, event)
		}
	}
	return matches
}

func assertJoinedInputBinding(t *testing.T, event factoryapi.FactoryEvent, planWorkID, taskWorkID string) {
	t.Helper()
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode joined dispatch request %q: %v", event.Id, err)
	}
	seen := make(map[string]bool, len(payload.Inputs))
	for _, input := range payload.Inputs {
		seen[input.WorkId] = true
	}
	if !seen[planWorkID] || !seen[taskWorkID] || len(seen) != 2 {
		t.Fatalf("joined dispatch inputs = %#v, want exactly %q and %q", payload.Inputs, planWorkID, taskWorkID)
	}
}

func dispatchResponseSequenceForWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	transitionID string,
	workID string,
) int {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response %q: %v", event.Id, err)
		}
		if payload.TransitionId != transitionID || !eventContainsWorkID(event.Context.WorkIds, workID) {
			continue
		}
		if payload.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf(
				"%s response for work %q outcome = %q, want accepted",
				transitionID,
				workID,
				payload.Outcome,
			)
		}
		return event.Context.Sequence
	}
	t.Fatalf("missing accepted %s response for work %q", transitionID, workID)
	return 0
}

func eventContainsWorkID(workIDs *[]string, want string) bool {
	if workIDs == nil {
		return false
	}
	for _, workID := range *workIDs {
		if workID == want {
			return true
		}
	}
	return false
}
