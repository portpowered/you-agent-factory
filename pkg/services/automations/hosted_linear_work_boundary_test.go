package automations_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	hostedLinearBoundaryWorkstationName = "linear-boundary-ingress"
	hostedLinearBoundaryWorkerName      = "linear-boundary-poller"
)

func hostedLinearBoundaryIssueResponse() string {
	return `{
		"data": {
			"issues": {
				"nodes": [
					{
						"id": "issue-boundary-new",
						"identifier": "ENG-201",
						"title": "Boundary issue",
						"description": "Hosted linear boundary test",
						"updatedAt": "2026-05-22T08:10:00Z",
						"url": "https://linear.app/example/issue/ENG-201",
						"team": {"id": "team-boundary", "key": "ENG", "name": "Engineering"},
						"state": {"id": "state-boundary", "name": "Todo", "type": "unstarted"},
						"assignee": {"id": "user-boundary", "name": "Alex", "email": "alex@example.com"}
					}
				],
				"pageInfo": {"hasNextPage": false, "endCursor": ""}
			}
		}
	}`
}

func hostedLinearBoundaryWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           hostedLinearBoundaryWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: hostedLinearBoundaryWorkerName,
	}
}

func hostedLinearBoundaryWorker() *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:     hostedLinearBoundaryWorkerName,
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
			TeamIDs:  []string{"team-boundary"},
			StateIDs: []string{"state-boundary"},
			Claim:    &interfaces.HostedLinearWorkerClaimConfig{AssigneeField: "ownerEmail"},
		},
	}
}

func writeHostedLinearBoundarySecret(t *testing.T, factoryDir string) {
	t.Helper()
	secretDir := filepath.Join(factoryDir, "secrets")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "linear-api-key"), []byte("boundary-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
}

func hostedLinearBoundaryRuntimeConfig(
	t *testing.T,
	factoryDir string,
) interfaces.MutableLoadedFactorySource {
	t.Helper()

	poller := hostedLinearBoundaryWorkstation()
	worker := hostedLinearBoundaryWorker()
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []interfaces.FactoryWorkerConfig{{Name: worker.Name}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func newHostedLinearBoundaryAutomationService(
	t *testing.T,
	httpClient *http.Client,
	linearEndpoint string,
) *automationservice.Service {
	t.Helper()

	checkpoints, err := automations.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore: %v", err)
	}
	hostedPollers := automations.NewHostedSourcesFactory(checkpoints)(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		httpClient,
		automations.NewHostedLinearSecretResolver(func(string) string { return "" }, os.ReadFile),
		linearEndpoint,
	)
	return automationservice.New(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		"factory/main",
		"",
		hostedPollers,
		nil,
		nil,
	)
}

func waitForHostedLinearBoundarySubmit(
	t *testing.T,
	submitCalls *atomic.Int32,
	want int32,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if submitCalls.Load() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d hosted linear submission(s); got %d", want, submitCalls.Load())
}

// TestStartHostedLinearPoller_HandsWorkRootRequestToAutomationsSubmitter proves
// hosted Linear pollers construct work.WorkRequest values through Work root
// helpers before handing them to the Automations WorkRequestSubmitter contract.
func TestStartHostedLinearPoller_HandsWorkRootRequestToAutomationsSubmitter(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(hostedLinearBoundaryIssueResponse()))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	writeHostedLinearBoundarySecret(t, factoryDir)
	runtimeCfg := hostedLinearBoundaryRuntimeConfig(t, factoryDir)
	svc := newHostedLinearBoundaryAutomationService(t, server.Client(), server.URL)

	var submitCalls atomic.Int32
	var submitted work.WorkRequest
	submitter := automations.WorkRequestSubmitter(func(_ context.Context, request work.WorkRequest) error {
		submitCalls.Add(1)
		submitted = request
		return nil
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	if err := svc.StartHostedLinearPoller(
		sidecarCtx,
		&sidecars,
		runtimeCfg,
		hostedLinearBoundaryWorkstation(),
		hostedLinearBoundaryWorker(),
		submitter,
	); err != nil {
		t.Fatalf("StartHostedLinearPoller: %v", err)
	}

	waitForHostedLinearBoundarySubmit(t, &submitCalls, 1, time.Second)
	cancel()
	sidecars.Wait()

	if submitCalls.Load() != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls.Load())
	}
	if submitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", submitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if submitted.RequestID == "" {
		t.Fatal("expected non-empty hosted linear batch request ID")
	}
	if len(submitted.Works) != 1 {
		t.Fatalf("works count = %d, want 1", len(submitted.Works))
	}
	workItem := submitted.Works[0]
	if workItem.WorkID != "linear:issue-boundary-new" {
		t.Fatalf("work ID = %q, want linear:issue-boundary-new", workItem.WorkID)
	}
	if workItem.Name != "linear-eng-201" {
		t.Fatalf("work name = %q, want linear-eng-201", workItem.Name)
	}
	if workItem.WorkTypeID != "story" {
		t.Fatalf("work type = %q, want story", workItem.WorkTypeID)
	}
	if workItem.State != "init" {
		t.Fatalf("state = %q, want init", workItem.State)
	}
	if workItem.TraceID != "linear:issue-boundary-new:2026-05-22T08:10:00Z" {
		t.Fatalf("trace ID = %q, want linear issue trace", workItem.TraceID)
	}
	if workItem.Tags["external_source"] != "linear" {
		t.Fatalf("external_source tag = %q, want linear", workItem.Tags["external_source"])
	}
	if workItem.Tags["linear_issue_identifier"] != "ENG-201" {
		t.Fatalf("linear_issue_identifier tag = %q, want ENG-201", workItem.Tags["linear_issue_identifier"])
	}
	if workItem.Tags["poller_workstation"] != hostedLinearBoundaryWorkstationName {
		t.Fatalf("poller_workstation tag = %q, want %q", workItem.Tags["poller_workstation"], hostedLinearBoundaryWorkstationName)
	}

	payloadBytes, ok := workItem.Payload.([]byte)
	if !ok {
		t.Fatalf("payload type = %T, want []byte", workItem.Payload)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	claims, _ := payload["claims"].(map[string]any)
	if claims["ownerEmail"] != "alex@example.com" {
		t.Fatalf("claims = %#v, want ownerEmail claim", claims)
	}

	expected := work.WorkRequestFromSubmitRequests([]work.SubmitRequest{
		{
			RequestID:   submitted.RequestID,
			WorkID:      workItem.WorkID,
			Name:        workItem.Name,
			WorkTypeID:  workItem.WorkTypeID,
			TargetState: "init",
			TraceID:     workItem.TraceID,
			Payload:     payloadBytes,
			Tags:        workItem.Tags,
		},
	})
	if submitted.RequestID != expected.RequestID {
		t.Fatalf("request ID = %q, want WorkRequestFromSubmitRequests %q", submitted.RequestID, expected.RequestID)
	}
	if len(submitted.Works) != len(expected.Works) || submitted.Works[0].WorkID != expected.Works[0].WorkID {
		t.Fatalf("submitted works = %#v, want %#v from Work root helper", submitted.Works, expected.Works)
	}
}

// TestStartHostedLinearPoller_PreservesDeterministicWorkRootIdentityForEquivalentInputs
// proves equivalent hosted Linear issue inputs still produce the same observable
// Work Request identity fields before submitter handoff.
func TestStartHostedLinearPoller_PreservesDeterministicWorkRootIdentityForEquivalentInputs(t *testing.T) {
	t.Parallel()

	capture := func(t *testing.T) work.WorkRequest {
		t.Helper()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(hostedLinearBoundaryIssueResponse()))
		}))
		t.Cleanup(server.Close)

		factoryDir := t.TempDir()
		writeHostedLinearBoundarySecret(t, factoryDir)
		runtimeCfg := hostedLinearBoundaryRuntimeConfig(t, factoryDir)
		svc := newHostedLinearBoundaryAutomationService(t, server.Client(), server.URL)

		var submitCalls atomic.Int32
		var submitted work.WorkRequest
		submitter := automations.WorkRequestSubmitter(func(_ context.Context, request work.WorkRequest) error {
			submitCalls.Add(1)
			submitted = request
			return nil
		})

		sidecarCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() {
			cancel()
		})
		var sidecars sync.WaitGroup
		if err := svc.StartHostedLinearPoller(
			sidecarCtx,
			&sidecars,
			runtimeCfg,
			hostedLinearBoundaryWorkstation(),
			hostedLinearBoundaryWorker(),
			submitter,
		); err != nil {
			t.Fatalf("StartHostedLinearPoller: %v", err)
		}
		waitForHostedLinearBoundarySubmit(t, &submitCalls, 1, time.Second)
		cancel()
		sidecars.Wait()
		return submitted
	}

	first := capture(t)
	second := capture(t)
	if first.RequestID != second.RequestID {
		t.Fatalf("request ID changed: first=%q second=%q", first.RequestID, second.RequestID)
	}
	if len(first.Works) != 1 || len(second.Works) != 1 {
		t.Fatal("expected one work item per hosted linear batch request")
	}
	if first.Works[0].WorkID != second.Works[0].WorkID {
		t.Fatalf("work ID changed: first=%q second=%q", first.Works[0].WorkID, second.Works[0].WorkID)
	}
	if first.Works[0].Name != second.Works[0].Name {
		t.Fatalf("work name changed: first=%q second=%q", first.Works[0].Name, second.Works[0].Name)
	}
	if first.Works[0].TraceID != second.Works[0].TraceID {
		t.Fatalf("trace ID changed: first=%q second=%q", first.Works[0].TraceID, second.Works[0].TraceID)
	}
}
