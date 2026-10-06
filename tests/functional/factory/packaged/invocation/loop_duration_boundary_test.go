package invocation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type loopBoundaryHTTPResult struct {
	response *http.Response
	err      error
}

// Duration acceptance is observed through admitted scheduled Work. The separate
// overlap journey proves the bounded terminal response. A 20 ms response timeout
// must not cancel this long-lived controller before its first schedule is visible.
func invokeLoopUntilFirstScheduledWork(t *testing.T, scenario *loopScenario, args map[string]any, observer *loopPhaseObserver) work.FactorySubmissionRecord {
	t.Helper()
	requestID := fmt.Sprintf("packaged-loop-boundary-%d", time.Now().UnixNano())
	payload, err := json.Marshal(factoryapi.InvocationRequest{RequestId: &requestID, Args: &args})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	endpoint := scenario.fixture.baseURL + "/factory-sessions/" + scenario.sessionID + "/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	results := make(chan loopBoundaryHTTPResult, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		results <- loopBoundaryHTTPResult{response: response, err: err}
	}()
	defer joinLoopBoundaryRequest(t, observer, cancel, results)
	return waitForLoopSubmission(observer, scenario.fixture.submissions, "scheduled-execution")
}

func joinLoopBoundaryRequest(t *testing.T, observer *loopPhaseObserver, cancel context.CancelFunc, results <-chan loopBoundaryHTTPResult) {
	t.Helper()
	cancel()
	ctx, finish := observer.phaseContext("duration-boundary request cancellation", "waiting for owned HTTP call", loopInvocationRequestBudget)
	defer finish()
	select {
	case result := <-results:
		if result.err != nil && !errors.Is(result.err, context.Canceled) {
			t.Errorf("duration-boundary HTTP request: %v", result.err)
		}
		if result.response != nil {
			defer result.response.Body.Close()
			if result.response.StatusCode != http.StatusOK {
				t.Errorf("duration-boundary invocation status = %d, want 200", result.response.StatusCode)
			}
		}
	case <-ctx.Done():
		observer.fail(loopInvocationRequestBudget, ctx.Err())
	}
}
