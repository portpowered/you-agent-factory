package automations

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestBuildProcessRemainsHostedSourcesInertBeforeRuntimeLifecycle(t *testing.T) {
	t.Parallel()

	var submissionsMu sync.Mutex
	var submissions []work.FactorySubmissionRecord
	recorder := func(record work.FactorySubmissionRecord) {
		submissionsMu.Lock()
		submissions = append(submissions, record)
		submissionsMu.Unlock()
	}

	_ = support.BuildProcess(t, serviceedges.Edges{
		SubmissionRecorder: recorder,
	})

	submissionsMu.Lock()
	defer submissionsMu.Unlock()
	if len(submissions) != 0 {
		t.Fatalf(
			"BuildProcess() submitted %d Work records, want zero before runtime lifecycle",
			len(submissions),
		)
	}
}

// TestAutomationsHostedSourcesActivateThroughRuntimeLifecycle proves hosted automation
// sources admit Work through the runtime lifecycle after BuildProcess composition.
func TestAutomationsHostedSourcesActivateThroughRuntimeLifecycle(t *testing.T) {
	linearServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"issues": {
					"nodes": [{
						"id": "issue-functional-new",
						"identifier": "ENG-301",
						"title": "Hosted sources functional proof",
						"description": "Activate through public process",
						"updatedAt": "2026-05-22T08:10:00Z",
						"url": "https://linear.app/example/issue/ENG-301",
						"team": {"id": "team-functional", "key": "ENG", "name": "Engineering"},
						"state": {"id": "state-functional", "name": "Todo", "type": "unstarted"},
						"assignee": null
					}],
					"pageInfo": {"hasNextPage": false, "endCursor": ""}
				}
			}
		}`))
	}))
	t.Cleanup(linearServer.Close)

	dir := support.ScaffoldFactory(t, hostedSourcesFactoryConfig())
	writeHostedSourcesLinearSecret(t, dir)

	observedSubmissions := make(chan work.FactorySubmissionRecord, 8)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			HostedHTTPClient:     linearServer.Client(),
			HostedLinearEndpoint: linearServer.URL,
			SubmissionRecorder: func(record work.FactorySubmissionRecord) {
				select {
				case observedSubmissions <- record:
				default:
					t.Fatalf("hosted sources submission channel overflow")
				}
			},
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	record := waitForHostedSourcesSubmission(t, observedSubmissions, "story", 10*time.Second)
	if record.Request.WorkTypeID != "story" {
		t.Fatalf("hosted sources submission work type = %q, want story", record.Request.WorkTypeID)
	}
	if record.Request.Tags["external_source"] != "linear" {
		t.Fatalf("hosted sources external_source tag = %q, want linear", record.Request.Tags["external_source"])
	}
	if record.Request.Tags["poller_workstation"] != "poll-linear" {
		t.Fatalf(
			"hosted sources poller_workstation tag = %q, want poll-linear",
			record.Request.Tags["poller_workstation"],
		)
	}
}

func hostedSourcesFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "queued", "type": "PROCESSING"},
			},
		}},
		"workers": []map[string]any{{
			"name":     "linear-poller",
			"type":     "HOSTED_WORKER",
			"provider": "LINEAR",
			"auth":     map[string]string{"secretRef": "secrets/linear-api-key"},
			"linear": map[string]any{
				"pollInterval": "100ms",
				"mapping":      map[string]string{"workType": "story", "state": "init"},
			},
		}},
		"workstations": []map[string]any{{
			"name":     "poll-linear",
			"behavior": "POLLER",
			"worker":   "linear-poller",
			"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "story", "state": "queued"}},
		}},
	}
}

func writeHostedSourcesLinearSecret(t *testing.T, factoryDir string) {
	t.Helper()
	secretDir := filepath.Join(factoryDir, "secrets")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(secretDir, "linear-api-key"),
		[]byte("functional-hosted-linear-key\n"),
		0o600,
	); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
}

func waitForHostedSourcesSubmission(
	t *testing.T,
	submissions <-chan work.FactorySubmissionRecord,
	workType string,
	timeout time.Duration,
) work.FactorySubmissionRecord {
	t.Helper()

	deadline := time.After(timeout)
	for {
		select {
		case record := <-submissions:
			if record.Request.WorkTypeID != workType {
				continue
			}
			return record
		case <-deadline:
			t.Fatalf("timed out waiting for hosted sources submission for work type %q", workType)
		}
	}
}
