package mock

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	rootMockWorkType       = "root-mock-task"
	rootMockWorker         = "root-mock-worker"
	rootMockWorkstation    = "root-mock-process"
	rootMockWorkID         = "root-mock-work"
	rootMockAcceptedOutput = "mock worker accepted"
)

// TestMockWorkerSelectedThroughCustomerProcess proves explicit mock
// composition is selected by the customer --with-mock-workers input and still
// publishes the correlated terminal dispatch through the public process.
func TestMockWorkerSelectedThroughCustomerProcess(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{{
			"name": rootMockWorkType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": rootMockWorker}},
		"workstations": []map[string]any{{
			"name":      rootMockWorkstation,
			"worker":    rootMockWorker,
			"inputs":    []map[string]string{{"workType": rootMockWorkType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": rootMockWorkType, "state": "done"}},
			"onFailure": []map[string]string{{"workType": rootMockWorkType, "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, rootMockWorker, "---\n"+
		"type: MODEL_WORKER\n"+
		"model: root-mock-model\n"+
		"---\n")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     rootMockWorkID,
		WorkTypeID: rootMockWorkType,
		TraceID:    "root-mock-trace",
		Payload:    []byte("root mock payload"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		MockWorkersConfig: &workers.MockWorkersConfig{
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      rootMockWorker,
				WorkstationName: rootMockWorkstation,
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
	})
	defer server.Stop(t)

	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, rootMockWorkType+":done"); got != 1 {
		t.Fatalf("completed mock Work = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, rootMockWorkType+":init"); got != 0 {
		t.Fatalf("pending mock Work = %d, want zero; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, rootMockWorkType+":failed"); got != 0 {
		t.Fatalf("failed mock Work = %d, want zero; listed=%#v", got, listed)
	}

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 1 {
		t.Fatalf("mock dispatch count = %d, want one; dispatches=%#v", len(dispatches), dispatches)
	}
	dispatch := dispatches[0]
	if dispatch.Request.TransitionId != rootMockWorkstation {
		t.Fatalf("mock dispatch transition = %q, want %q", dispatch.Request.TransitionId, rootMockWorkstation)
	}
	if !support.DispatchObservationIncludesWork(dispatch, rootMockWorkID) {
		t.Fatalf("mock dispatch = %#v, want work %q correlation", dispatch, rootMockWorkID)
	}
	if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
		response := "<nil>"
		if dispatch.Response != nil {
			response = string(dispatch.Response.Outcome)
		}
		t.Fatalf("mock dispatch outcome = %s, want ACCEPTED", response)
	}
	if got := support.StringPointerValue(dispatch.Response.Output); got != rootMockAcceptedOutput {
		t.Fatalf("mock dispatch output = %q, want %q", got, rootMockAcceptedOutput)
	}
}
