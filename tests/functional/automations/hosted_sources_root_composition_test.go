package automations

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
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
	failure chan error
	done    chan struct{}
}
type hostedCycleRouter struct{ routes map[string]*hostedCycleRoute }

type hostedSessionScenario struct {
	directory string
	route     *hostedCycleRoute
}

type hostedIngressScenarios struct {
	scenarios         map[string]hostedSessionScenario
	router            hostedCycleRouter
	checkpoints       automations.HostedLinearCheckpointStore
	stoppedAdmissions atomic.Int32
}

func newHostedIngressScenarios(t *testing.T) *hostedIngressScenarios {
	t.Helper()
	h := &hostedIngressScenarios{
		scenarios: map[string]hostedSessionScenario{},
		router:    hostedCycleRouter{routes: map[string]*hostedCycleRoute{}},
	}
	for _, key := range []string{"owned-hosted-success", "owned-hosted-empty", "owned-hosted-stop", "owned-hosted-peer",
		"owned-hosted-retry-secret", "owned-hosted-retry-peer", "owned-hosted-failure-secret", "owned-hosted-failure-peer"} {
		dir, route := newHostedCycleFactory(t, key)
		h.scenarios[key] = hostedSessionScenario{directory: dir, route: route}
		h.router.routes[key] = route
	}
	files := &hostedCheckpointFailure{directory: h.scenarios["owned-hosted-failure-secret"].directory, secret: "owned-hosted-failure-secret"}
	checkpoints, err := automationswire.NewHostedLinearCheckpointStore(files)
	if err != nil {
		t.Fatal(err)
	}
	h.checkpoints = checkpoints
	return h
}

func (h *hostedIngressScenarios) run(t *testing.T, baseURL string) {
	t.Helper()
	t.Run("hosted_request_retry_preserves_Work_identity_while_peer_progresses", func(t *testing.T) {
		t.Parallel()
		source, peer := h.scenarios["owned-hosted-retry-secret"], h.scenarios["owned-hosted-retry-peer"]
		assertHostedRequestRetry(t, baseURL, source.directory, peer.directory, source.route, peer.route)
	})
	t.Run("hosted_failed_checkpoint_save_retains_Work_and_prior_resume_while_peer_progresses", func(t *testing.T) {
		t.Parallel()
		source, peer := h.scenarios["owned-hosted-failure-secret"], h.scenarios["owned-hosted-failure-peer"]
		assertHostedCheckpointRecovery(t, baseURL, source.directory, peer.directory, source.route, peer.route)
	})
	t.Run("hosted_stop_joins_while_peer_progresses_and_restart_admits_eligible_item", func(t *testing.T) {
		t.Parallel()
		source, peer := h.scenarios["owned-hosted-stop"], h.scenarios["owned-hosted-peer"]
		assertHostedIndependentStop(t, baseURL, source.directory, peer.directory, source.route, peer.route, &h.stoppedAdmissions)
	})
	t.Run("hosted_success_normalizes_Work_in_explicit_session", func(t *testing.T) {
		t.Parallel()
		source := h.scenarios["owned-hosted-success"]
		assertHostedSessionIngress(t, baseURL, source.directory, source.route, false)
	})
	t.Run("hosted_completed_empty_cycle_admits_no_Work", func(t *testing.T) {
		t.Parallel()
		source := h.scenarios["owned-hosted-empty"]
		assertHostedSessionIngress(t, baseURL, source.directory, source.route, true)
	})
}

func (r hostedCycleRouter) Do(request *http.Request) (*http.Response, error) {
	route := r.routes[request.Header.Get("Authorization")]
	if route == nil {
		return nil, fmt.Errorf("unowned hosted HTTP route")
	}
	call := hostedCycleRequest{request: request, output: make(chan string, 1), failure: make(chan error, 1), done: make(chan struct{})}
	defer close(call.done)
	select {
	case route.entered <- call:
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
	select {
	case body := <-call.output:
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	case err := <-call.failure:
		return nil, err
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
}

// Observe retry and admission at HTTP/Work boundaries. These observations do
// not establish an externally visible diagnostic or credential redaction.
func assertHostedRequestRetry(t *testing.T, baseURL, sourceDir, peerDir string, source, peer *hostedCycleRoute) {
	t.Helper()
	sourceID := support.OpenFactorySessionAt(t, baseURL, sourceDir).Session.Id
	peerID := support.OpenFactorySessionAt(t, baseURL, peerDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sourceID) })
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, peerID) })
	first := awaitHostedCycleRequest(t, source)
	peerCall := awaitHostedCycleRequest(t, peer)
	secret := first.request.Header.Get("Authorization")
	first.failure <- fmt.Errorf("owned-hosted-request-unavailable: %s", secret)
	retried := awaitHostedCycleRequest(t, source)
	if retried.request.Header.Get("Authorization") != secret || retried.request.URL.String() != first.request.URL.String() {
		t.Fatal("hosted retry changed its logical source route")
	}
	if listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sourceID, "/work")); len(listed.Results) != 0 {
		t.Fatalf("failed hosted request admitted Work: %#v", listed.Results)
	}
	// Keep A's retry outstanding while B completes, proving the retry does not
	// hold a shared admission or HTTP lock.
	peerCall.output <- hostedCycleResponse(false)
	_ = awaitHostedCycleRequest(t, peer)
	assertHostedNormalizedWork(t, support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, peerID, "/work")))
	retried.output <- hostedCycleResponse(false)
	_ = awaitHostedCycleRequest(t, source)
	assertHostedNormalizedWork(t, support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sourceID, "/work")))
}

// The production checkpoint adapter still performs real atomic file IO. Only
// the second commit in this scenario's directory fails, after a prior commit.
type hostedCheckpointFailure struct {
	platformfilesystem.Local
	directory string
	secret    string
	commits   atomic.Int32
}

func (f *hostedCheckpointFailure) Rename(from, to string) error {
	if strings.HasPrefix(filepath.Clean(to), filepath.Clean(f.directory)+string(filepath.Separator)) && f.commits.Add(1) == 2 {
		return fmt.Errorf("owned-hosted-checkpoint-unavailable: %s", f.secret)
	}
	return f.Local.Rename(from, to)
}

func assertHostedCheckpointRecovery(t *testing.T, baseURL, sourceDir, peerDir string, source, peer *hostedCycleRoute) {
	t.Helper()
	sourceID := support.OpenFactorySessionAt(t, baseURL, sourceDir).Session.Id
	peerID := support.OpenFactorySessionAt(t, baseURL, peerDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sourceID) })
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, peerID) })
	first := awaitHostedCycleRequest(t, source)
	peerCall := awaitHostedCycleRequest(t, peer)
	first.output <- hostedCycleResponse(false)
	next := awaitHostedCycleRequest(t, source)
	assertHostedNormalizedWork(t, support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sourceID, "/work")))
	newer := strings.ReplaceAll(hostedCycleResponse(false), "issue-owned", "issue-before-save-failure")
	newer = strings.ReplaceAll(newer, "08:10:00Z", "08:11:00Z")
	next.output <- newer
	resumed := awaitHostedCycleRequest(t, source)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sourceID, "/work"))
	location := support.WorkCustomerLocation("story", "queued")
	if len(listed.Results) != 2 || !support.HasWorkAtCustomerState(listed, "linear:issue-owned", location) ||
		!support.HasWorkAtCustomerState(listed, "linear:issue-before-save-failure", location) {
		t.Fatalf("failed checkpoint save lost admitted Work: %#v", listed.Results)
	}
	// A response containing the exact prior issue ID and timestamp is a resume
	// boundary. With the failed newer checkpoint, this page would instead admit
	// the older item and follow its pagination cursor. Observe HTTP and Work,
	// never decode a private checkpoint file as composed recovery evidence.
	var page map[string]any
	if err := json.Unmarshal([]byte(hostedCycleResponse(false)), &page); err != nil {
		t.Fatal(err)
	}
	issues := page["data"].(map[string]any)["issues"].(map[string]any)
	nodes := issues["nodes"].([]any)
	older := map[string]any{}
	for key, value := range nodes[0].(map[string]any) {
		older[key] = value
	}
	older["id"], older["updatedAt"] = "issue-older-than-prior", "2026-05-22T08:09:00Z"
	issues["nodes"] = append(nodes, older)
	issues["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "must-not-page-past-prior"}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	resumed.output <- string(body)
	following := awaitHostedCycleRequest(t, source)
	var query struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(following.request.Body).Decode(&query); err != nil {
		t.Fatal(err)
	}
	if after := query.Variables["after"]; after != nil && after != "" {
		t.Fatalf("resume paged past the exact prior checkpoint: after=%#v", after)
	}
	listed = support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sourceID, "/work"))
	if len(listed.Results) != 2 || support.HasWorkAtCustomerState(listed, "linear:issue-older-than-prior", location) {
		t.Fatalf("resume did not retain the prior checkpoint boundary: %#v", listed.Results)
	}
	peerCall.output <- hostedCycleResponse(false)
	_ = awaitHostedCycleRequest(t, peer)
	assertHostedNormalizedWork(t, support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, peerID, "/work")))
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
	case <-time.After(support.ScaledTimeout(10 * time.Second)):
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
	case <-time.After(support.ScaledTimeout(10 * time.Second)):
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
