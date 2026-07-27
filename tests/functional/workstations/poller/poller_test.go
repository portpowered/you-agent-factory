package poller

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	pollerWorkTypeName        = "story"
	pollerOutputStateName     = "queued"
	pollerExternalWorkID      = "external-issue-101"
	pollerExternalRequestID   = "poller-external-batch-1"
	pollerScriptCommand       = "factory/scripts/poller.sh"
	pollerWorkstationName     = "poll-tasks"
	pollerWorkerName          = "script-poller"
)

// TestPollerCreatesWorkFromExternalItems proves a POLLER workstation running
// through root.BuildProcess admits Work when a controlled external poll source
// returns identifiable items. The scenario replaces only the script poller
// command edge and observes admission through the public Work listing.
func TestPollerCreatesWorkFromExternalItems(t *testing.T) {
	dir := scaffoldScriptPollerFactory(t)
	support.ClearSeedInputs(t, dir)

	runner := newPollerIngressCommandRunner(t, pollerExternalWorkRequestJSON(t))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	defer server.Stop(t)

	outputLocation := support.WorkCustomerLocation(pollerWorkTypeName, pollerOutputStateName)
	listed := waitForListedWorkAtCustomerState(t, server.URL(), outputLocation, 1, 10*time.Second)
	if got := support.CountWorkAtCustomerState(listed, outputLocation); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d, want 1; listed=%#v", outputLocation, got, listed)
	}
	if !support.HasWorkAtCustomerState(listed, pollerExternalWorkID, outputLocation) {
		t.Fatalf("listed work = %#v, want work %q at %q", listed.Results, pollerExternalWorkID, outputLocation)
	}
	if runner.callCount() < 1 {
		t.Fatalf("poller command calls = %d, want at least one external poll invocation", runner.callCount())
	}
}

// TestPollerEmptyResultCreatesNoWork proves a POLLER workstation does not admit
// Work when the external poll source returns a successful empty result. The
// scenario replaces only the script poller command edge, waits for the empty
// poll cycle to finish through a deterministic observation edge, and asserts
// the public Work listing at the poller output location is unchanged.
func TestPollerEmptyResultCreatesNoWork(t *testing.T) {
	dir := scaffoldScriptPollerFactory(t)
	support.ClearSeedInputs(t, dir)

	runner := newPollerIngressCommandRunner(t, nil)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	defer server.Stop(t)

	outputLocation := support.WorkCustomerLocation(pollerWorkTypeName, pollerOutputStateName)
	baseline := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(baseline, outputLocation); got != 0 {
		t.Fatalf("baseline CountWorkAtCustomerState(%q) = %d, want 0 before empty poll", outputLocation, got)
	}

	waitForPollCycle(t, runner, 10*time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, outputLocation); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d, want 0 after empty poll; listed=%#v", outputLocation, got, listed.Results)
	}
	if runner.callCount() < 1 {
		t.Fatalf("poller command calls = %d, want at least one empty poll invocation", runner.callCount())
	}
}

func scaffoldScriptPollerFactory(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "poller-external-items",
		"workTypes": []map[string]any{{
			"name": pollerWorkTypeName,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": pollerOutputStateName, "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{
			"name":    pollerWorkerName,
			"type":    "SCRIPT_WORKER",
			"command": pollerScriptCommand,
		}},
		"workstations": []map[string]any{{
			"name":      pollerWorkstationName,
			"behavior":  "POLLER",
			"worker":    pollerWorkerName,
			"inputs":    []map[string]string{{"workType": pollerWorkTypeName, "state": "init"}},
			"outputs":   []map[string]string{{"workType": pollerWorkTypeName, "state": pollerOutputStateName}},
			"onFailure": []map[string]string{{"workType": pollerWorkTypeName, "state": "failed"}},
		}},
	})
}

func pollerExternalWorkRequestJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"requestId": pollerExternalRequestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "external-issue-101",
			"workId":       pollerExternalWorkID,
			"workTypeName": pollerWorkTypeName,
			"payload": map[string]string{
				"id":    "ISSUE-101",
				"title": "External ingress item",
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal poller work request: %v", err)
	}
	return payload
}

func waitForPollCycle(t *testing.T, runner *pollerIngressCommandRunner, timeout time.Duration) {
	t.Helper()

	select {
	case <-runner.pollDone:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for poller poll cycle; calls=%d", runner.callCount())
	}
}

func waitForListedWorkAtCustomerState(
	t *testing.T,
	baseURL string,
	location string,
	wantCount int,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var last factoryapi.ListWorkResponse
	for {
		last = support.ListDefaultSessionWork(t, baseURL)
		if support.CountWorkAtCustomerState(last, location) >= wantCount {
			return last
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf(
				"timed out waiting for %d work at %s; listed=%#v",
				wantCount,
				location,
				last.Results,
			)
		}
	}
}

type pollerIngressCommandRunner struct {
	mu       sync.Mutex
	stdout   []byte
	calls    int
	pollDone chan struct{}
}

func newPollerIngressCommandRunner(t *testing.T, stdout []byte) *pollerIngressCommandRunner {
	t.Helper()
	return &pollerIngressCommandRunner{
		stdout:   append([]byte(nil), stdout...),
		pollDone: make(chan struct{}, 1),
	}
}

func (r *pollerIngressCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	callNumber := r.calls
	stdout := append([]byte(nil), r.stdout...)
	r.mu.Unlock()

	select {
	case r.pollDone <- struct{}{}:
	default:
	}

	if callNumber == 1 {
		return platformprocess.CommandResult{Stdout: stdout}, nil
	}
	return platformprocess.CommandResult{}, nil
}

func (r *pollerIngressCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

var _ platformprocess.CommandRunner = (*pollerIngressCommandRunner)(nil)
