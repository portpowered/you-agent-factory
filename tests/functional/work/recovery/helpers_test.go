package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startRecoveryAPIServer(
	t *testing.T,
	factoryDir string,
	provider workerexecution.Runner,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            false,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
}

func postMoveWorkStatus(t *testing.T, baseURL, workID, stateName string) (int, string) {
	t.Helper()
	return postMoveWorkStatusWithRequestID(t, baseURL, workID, stateName, "")
}

func postMoveWorkStatusWithRequestID(
	t *testing.T,
	baseURL, workID, stateName, requestID string,
) (int, string) {
	t.Helper()

	request := factoryapi.MoveWorkRequest{StateName: stateName}
	if requestID != "" {
		request.RequestId = &requestID
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal move request: %v", err)
	}
	endpoint := support.DefaultSessionWorkURL(baseURL, "/work/"+workID+"/move")
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(payload)
}

func postMoveWork(t *testing.T, baseURL, workID, stateName string) factoryapi.Work {
	t.Helper()
	return postMoveWorkWithRequestID(t, baseURL, workID, stateName, "")
}

func postMoveWorkWithRequestID(
	t *testing.T,
	baseURL, workID, stateName, requestID string,
) factoryapi.Work {
	t.Helper()

	status, body := postMoveWorkStatusWithRequestID(t, baseURL, workID, stateName, requestID)
	if status != http.StatusOK {
		t.Fatalf("POST /work/%s/move status = %d, want 200: %s", workID, status, body)
	}
	var work factoryapi.Work
	if err := json.Unmarshal([]byte(body), &work); err != nil {
		t.Fatalf("decode move response: %v", err)
	}
	return work
}

func waitForWorkIDsAtState(
	t *testing.T,
	baseURL string,
	workIDs []string,
	stateName string,
	timeout time.Duration,
) {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		found := 0
		for _, item := range listed.Results {
			workID := support.StringPointerValue(item.WorkId)
			if want[workID] && workStateName(item.State) == stateName {
				found++
			}
		}
		if found == len(want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf("timed out waiting for work IDs %v at state %q; last listing: %#v", workIDs, stateName, listed.Results)
}

func waitForWorkIDsComplete(
	t *testing.T,
	baseURL string,
	workIDs []string,
	timeout time.Duration,
) []factoryapi.Work {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		found := make(map[string]factoryapi.Work, len(want))
		for _, item := range listed.Results {
			workID := support.StringPointerValue(item.WorkId)
			if want[workID] && workStateName(item.State) == "complete" {
				found[workID] = item
			}
		}
		if len(found) == len(want) {
			items := make([]factoryapi.Work, 0, len(workIDs))
			for _, workID := range workIDs {
				items = append(items, found[workID])
			}
			return items
		}
		time.Sleep(100 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf("timed out waiting for work IDs %v to complete; last listing: %#v", workIDs, listed.Results)
	return nil
}

func requireWorkByID(t *testing.T, listed factoryapi.ListWorkResponse, workID string) factoryapi.Work {
	t.Helper()
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == workID {
			return item
		}
	}
	t.Fatalf("work ID %q missing from listing: %#v", workID, listed.Results)
	return factoryapi.Work{}
}

func workStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func stringPtr(value string) *string {
	return &value
}

type recoveryRedispatchBlockingProvider struct {
	failWorker   string
	blockWorker  string
	callCounts   map[string]int
	blockStarted chan struct{}
	releaseBlock chan struct{}
	releaseOnce  sync.Once
	mu           sync.Mutex
}

var _ workerexecution.Runner = (*recoveryRedispatchBlockingProvider)(nil)

func newRecoveryRedispatchBlockingProvider(failWorker, blockWorker string) *recoveryRedispatchBlockingProvider {
	return &recoveryRedispatchBlockingProvider{
		failWorker:   failWorker,
		blockWorker:  blockWorker,
		callCounts:   make(map[string]int),
		blockStarted: make(chan struct{}, 1),
		releaseBlock: make(chan struct{}),
	}
}

func (p *recoveryRedispatchBlockingProvider) Execute(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	workerName := req.WorkerType
	if workerName == "" {
		workerName = req.Dispatch.WorkerType
	}
	p.mu.Lock()
	p.callCounts[workerName]++
	callCount := p.callCounts[workerName]
	p.mu.Unlock()

	if workerName == p.failWorker && callCount == 1 {
		return workerexecution.InferenceResponse{}, errors.New("initial terminal failure")
	}
	if workerName == p.blockWorker && callCount >= 1 {
		select {
		case p.blockStarted <- struct{}{}:
		default:
		}
		select {
		case <-p.releaseBlock:
		case <-ctx.Done():
			return workerexecution.InferenceResponse{}, ctx.Err()
		}
		return workerexecution.InferenceResponse{}, errors.New("recovery redispatch failed again")
	}
	return workerexecution.InferenceResponse{Content: "COMPLETE"}, nil
}

func (p *recoveryRedispatchBlockingProvider) CallCount(worker string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCounts[worker]
}

func (p *recoveryRedispatchBlockingProvider) waitForBlockedRedispatch(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-p.blockStarted:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting %s for blocked redispatch provider call", timeout)
	}
}

func (p *recoveryRedispatchBlockingProvider) releaseBlockedRedispatch() {
	p.releaseOnce.Do(func() {
		close(p.releaseBlock)
	})
}

func waitForSessionInFlightDispatches(
	t *testing.T,
	baseURL string,
	want int,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := support.GetDefaultSession(t, baseURL)
		if session.Runtime.Progress.InFlightCount == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	session := support.GetDefaultSession(t, baseURL)
	t.Fatalf("timed out waiting for inFlightCount=%d; session progress=%#v", want, session.Runtime.Progress)
}
