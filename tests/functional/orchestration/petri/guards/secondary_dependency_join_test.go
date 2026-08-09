package guards

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

// TestDependsOnSecondaryJoinedInput proves through the injected dispatch edge
// that a SAME_NAME binding remains undispatched while a DEPENDS_ON relation on
// its secondary input is blocked, then dispatches exactly once after that
// prerequisite reaches its required terminal state. The application is built
// and executed through the same root process used by the customer CLI.
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

	provider := newSecondaryJoinCommandRunner()
	dispatches := newSecondaryJoinDispatchRecorder()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: provider,
		DispatchRecorder:      dispatches.Record,
	})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	support.StartProcessCommand(t, process, inputs.Input)

	<-provider.producerStarted
	blocked := dispatches.Snapshot()
	if got := countDispatches(blocked, secondaryJoinTransition); got != 0 {
		t.Fatalf("joined dispatches before producer release = %d, want zero; dispatches=%#v", got, blocked)
	}
	if got := provider.CallCount(); got != 1 {
		t.Fatalf("provider command calls before producer release = %d, want one controlled producer call", got)
	}

	provider.ReleaseProducer()
	<-provider.producerResponseReturned
	joined := <-dispatches.joined

	allDispatches := dispatches.Snapshot()
	if got := countDispatches(allDispatches, secondaryJoinTransition); got != 1 {
		t.Fatalf("joined dispatches after producer completion = %d, want exactly one; dispatches=%#v", got, allDispatches)
	}
	producer, ok := dispatchForTransition(allDispatches, secondaryJoinProduce)
	if !ok {
		t.Fatalf("missing producer dispatch in %#v", allDispatches)
	}
	if joined.CreatedTick <= producer.CreatedTick {
		t.Fatalf("joined dispatch tick = %d, want after producer dispatch tick %d", joined.CreatedTick, producer.CreatedTick)
	}
	assertJoinedInputBinding(t, joined, secondaryJoinPlanWorkID, secondaryJoinTaskWorkID)
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

type secondaryJoinCommandRunner struct {
	mu sync.Mutex

	requests []platformprocess.CommandRequest

	producerStarted          chan struct{}
	producerResponseReturned chan struct{}
	releaseProducer          chan struct{}
	producerOnce             sync.Once
	responseOnce             sync.Once
	releaseOnce              sync.Once
}

func newSecondaryJoinCommandRunner() *secondaryJoinCommandRunner {
	return &secondaryJoinCommandRunner{
		producerStarted:          make(chan struct{}),
		producerResponseReturned: make(chan struct{}),
		releaseProducer:          make(chan struct{}),
	}
}

func (r *secondaryJoinCommandRunner) Run(
	ctx context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	call := len(r.requests)
	r.requests = append(r.requests, req)
	r.mu.Unlock()

	if call == 0 {
		r.producerOnce.Do(func() { close(r.producerStarted) })
		select {
		case <-r.releaseProducer:
			r.responseOnce.Do(func() { close(r.producerResponseReturned) })
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}

	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")}, nil
}

func (r *secondaryJoinCommandRunner) ReleaseProducer() {
	r.releaseOnce.Do(func() { close(r.releaseProducer) })
}

func (r *secondaryJoinCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

type secondaryJoinDispatchRecorder struct {
	mu      sync.Mutex
	records []recordings.FactoryDispatchRecord
	joined  chan recordings.FactoryDispatchRecord
}

func newSecondaryJoinDispatchRecorder() *secondaryJoinDispatchRecorder {
	return &secondaryJoinDispatchRecorder{
		joined: make(chan recordings.FactoryDispatchRecord, 4),
	}
}

func (r *secondaryJoinDispatchRecorder) Record(record recordings.FactoryDispatchRecord) {
	record = cloneDispatchRecord(record)
	r.mu.Lock()
	r.records = append(r.records, record)
	r.mu.Unlock()

	if record.Dispatch.TransitionID == secondaryJoinTransition {
		r.joined <- record
	}
}

func (r *secondaryJoinDispatchRecorder) Snapshot() []recordings.FactoryDispatchRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]recordings.FactoryDispatchRecord, len(r.records))
	for index, record := range r.records {
		result[index] = cloneDispatchRecord(record)
	}
	return result
}

func cloneDispatchRecord(record recordings.FactoryDispatchRecord) recordings.FactoryDispatchRecord {
	record.Dispatch = work.CloneWorkDispatch(record.Dispatch)
	record.ConsumedTokens = append([]string(nil), record.ConsumedTokens...)
	return record
}

func countDispatches(records []recordings.FactoryDispatchRecord, transitionID string) int {
	count := 0
	for _, record := range records {
		if record.Dispatch.TransitionID == transitionID {
			count++
		}
	}
	return count
}

func dispatchForTransition(
	records []recordings.FactoryDispatchRecord,
	transitionID string,
) (recordings.FactoryDispatchRecord, bool) {
	for _, record := range records {
		if record.Dispatch.TransitionID == transitionID {
			return record, true
		}
	}
	return recordings.FactoryDispatchRecord{}, false
}

func assertJoinedInputBinding(
	t *testing.T,
	record recordings.FactoryDispatchRecord,
	planWorkID string,
	taskWorkID string,
) {
	t.Helper()
	seen := make(map[string]bool, len(record.Dispatch.Execution.WorkIDs))
	for _, workID := range record.Dispatch.Execution.WorkIDs {
		seen[workID] = true
	}
	if !seen[planWorkID] || !seen[taskWorkID] || len(seen) != 2 {
		t.Fatalf(
			"joined dispatch Work IDs = %#v, want exactly %q and %q",
			record.Dispatch.Execution.WorkIDs,
			planWorkID,
			taskWorkID,
		)
	}
}

var _ platformprocess.CommandRunner = (*secondaryJoinCommandRunner)(nil)
