package automations

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each hosted scenario owns a real secret file and an immutable HTTP route.
// The next request acknowledges the preceding poll's admission/empty handling.
// HTTP is replaced only at the declared external effect, without a remote call.
type hostedCycleRoute struct{ entered chan hostedCycleRequest }
type hostedCycleRequest struct {
	request *http.Request
	output  chan string
	done    chan struct{}
}
type hostedCycleRouter struct{ routes map[string]*hostedCycleRoute }

func (r hostedCycleRouter) Do(request *http.Request) (*http.Response, error) {
	route := r.routes[request.Header.Get("Authorization")]
	if route == nil {
		return nil, fmt.Errorf("unowned hosted HTTP route")
	}
	call := hostedCycleRequest{request: request, output: make(chan string, 1), done: make(chan struct{})}
	defer close(call.done)
	select {
	case route.entered <- call:
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
	select {
	case body := <-call.output:
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
}

func newHostedCycleFactory(t *testing.T, key string) (string, *hostedCycleRoute) {
	t.Helper()
	dir := support.ScaffoldFactory(t, hostedSourcesFactoryConfig())
	support.ClearSeedInputs(t, dir)
	secretDir := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "linear-api-key"), []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, &hostedCycleRoute{entered: make(chan hostedCycleRequest, 1)}
}

func awaitHostedCycleRequest(t *testing.T, route *hostedCycleRoute) hostedCycleRequest {
	t.Helper()
	select {
	case call := <-route.entered:
		return call
	case <-time.After(10 * time.Second):
		t.Fatal("hosted request did not reach owned HTTP edge")
		return hostedCycleRequest{}
	}
}

func assertHostedSessionIngress(t *testing.T, baseURL, dir string, route *hostedCycleRoute, empty bool) {
	t.Helper()
	sessionID := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sessionID) })
	if sessionID == "" || sessionID == "~default" {
		t.Fatalf("want explicit hosted session, got %q", sessionID)
	}
	first := awaitHostedCycleRequest(t, route)
	first.output <- hostedCycleResponse(empty)
	_ = awaitHostedCycleRequest(t, route)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sessionID, "/work"))
	if empty {
		if len(listed.Results) != 0 {
			t.Fatalf("completed empty hosted cycle admitted Work: %#v", listed.Results)
		}
		return
	}
	assertHostedNormalizedWork(t, listed)
}

func hostedCycleResponse(empty bool) string {
	if empty {
		return `{"data":{"issues":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`
	}
	return `{"data":{"issues":{"nodes":[{"id":"issue-owned","identifier":"ENG-301","title":"Hosted source owned item","description":"Explicit session admission","updatedAt":"2026-05-22T08:10:00Z","url":"https://linear.app/example/issue/ENG-301","team":{"id":"team-owned","key":"ENG","name":"Engineering"},"state":{"id":"state-owned","name":"Todo","type":"unstarted"},"assignee":null}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`
}

func assertHostedIndependentStop(t *testing.T, baseURL, sourceDir, peerDir string,
	source, peer *hostedCycleRoute, stoppedAdmissions *atomic.Int32,
) {
	t.Helper()
	sourceID := support.OpenFactorySessionAt(t, baseURL, sourceDir).Session.Id
	peerID := support.OpenFactorySessionAt(t, baseURL, peerDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, peerID) })
	blocked := awaitHostedCycleRequest(t, source)
	peerCall := awaitHostedCycleRequest(t, peer)
	support.CloseFactorySessionAt(t, baseURL, sourceID)
	select {
	case <-blocked.done:
		if blocked.request.Context().Err() == nil {
			t.Fatal("stopped hosted HTTP request was not canceled")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stopped hosted HTTP request did not join")
	}
	// A late response is owned by the canceled call and cannot enter another
	// session. Positive peer completion gates the stopped-admission observer.
	blocked.output <- strings.ReplaceAll(hostedCycleResponse(false), "issue-owned", "issue-stopped-only")
	peerCall.output <- hostedCycleResponse(false)
	_ = awaitHostedCycleRequest(t, peer)
	assertHostedNormalizedWork(t, support.GetJSON[factoryapi.ListWorkResponse](t,
		support.SessionWorkURL(baseURL, peerID, "/work")))
	if got := stoppedAdmissions.Load(); got != 0 {
		t.Fatalf("stopped hosted source admitted %d late Work items", got)
	}
	restartedID := support.OpenFactorySessionAt(t, baseURL, sourceDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, restartedID) })
	restarted := awaitHostedCycleRequest(t, source)
	restarted.output <- hostedCycleResponse(false)
	_ = awaitHostedCycleRequest(t, source)
	assertHostedNormalizedWork(t, support.GetJSON[factoryapi.ListWorkResponse](t,
		support.SessionWorkURL(baseURL, restartedID, "/work")))
}

func assertHostedNormalizedWork(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if len(listed.Results) != 1 || !support.HasWorkAtCustomerState(listed, "linear:issue-owned", support.WorkCustomerLocation("story", "queued")) {
		t.Fatalf("hosted Work = %#v, want one normalized item at story:queued", listed.Results)
	}
	item := listed.Results[0]
	if item.RequestId == nil || *item.RequestId == "" || item.Tags == nil {
		t.Fatalf("hosted Work missing request/source correlation: %#v", item)
	}
	for key, want := range map[string]string{"external_source": "linear", "linear_issue_id": "issue-owned", "linear_issue_identifier": "ENG-301", "poller_workstation": "poll-linear", "poller_worker": "linear-poller"} {
		if got := (*item.Tags)[key]; got != want {
			t.Fatalf("hosted tag %s=%q, want %q", key, got, want)
		}
	}
	payload, ok := item.Payload.(map[string]any)
	if !ok || payload["source"] != "linear" {
		t.Fatalf("hosted payload=%#v", item.Payload)
	}
	issue, ok := payload["issue"].(map[string]any)
	if !ok || issue["id"] != "issue-owned" || issue["identifier"] != "ENG-301" || issue["title"] != "Hosted source owned item" {
		t.Fatalf("normalized issue=%#v", payload["issue"])
	}
}

func hostedSourcesFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "queued", "type": "PROCESSING"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":     "linear-poller",
			"type":     "HOSTED_WORKER",
			"provider": "LINEAR",
			"auth":     map[string]string{"secretRef": "secrets/linear-api-key"},
			"linear": map[string]any{
				"pollInterval": "100ms",
				// This source-only cell leaves normalized Work observable without
				// dispatching hosted inference, which belongs to another owner.
				"mapping": map[string]string{"workType": "story", "state": "queued"},
			},
		}},
		"workstations": []map[string]any{{
			"name":     "poll-linear",
			"behavior": "POLLER",
			"worker":   "linear-poller",
			"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "story", "state": "queued"}},
			"onFailure": []map[string]string{{
				"workType": "story",
				"state":    "failed",
			}},
		}},
	}
}
