package stress_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type stressProcessHarness struct {
	t       *testing.T
	baseURL string
	cancel  context.CancelFunc
	done    chan error
	workers workerMuxExecutor
}

func startStressProcess(
	t *testing.T,
	dir string,
	provider workerprovider.Provider,
) *stressProcessHarness {
	t.Helper()
	ensureStressProviderDefinitions(t, dir)

	var (
		serverMu sync.Mutex
		server   *httptest.Server
	)
	ready := make(chan struct{})
	edges := serviceedges.Edges{
		ProviderOverride: provider,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			started := httptest.NewServer(request.Handler)
			serverMu.Lock()
			server = started
			serverMu.Unlock()
			close(ready)
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			started.Close()
			return ctx.Err()
		},
	}
	process, err := root.BuildProcess(t.Context(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- process.Execute(root.Input{
			Args: []string{
				"you", "run",
				"--dir", dir,
				"--continuously",
				"--quiet",
				"--no-record",
			},
			Env:              os.Environ(),
			Stdout:           &stdout,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: dir,
		})
	}()
	select {
	case <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("Process.Execute() exited before API readiness: %v; stderr=%s", err, stderr.String())
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatalf("timed out waiting for API readiness; stderr=%s", stderr.String())
	}
	serverMu.Lock()
	baseURL := server.URL
	serverMu.Unlock()
	harness := &stressProcessHarness{
		t: t, baseURL: baseURL, cancel: cancel, done: done,
	}
	harness.waitForSession(5 * time.Second)
	t.Cleanup(harness.Stop)
	return harness
}

func startStressProcessWithWorkerMux(t *testing.T, dir string) *stressProcessHarness {
	t.Helper()
	mux := workerMuxExecutor{}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: mux})
	h.workers = mux
	return h
}

func (h *stressProcessHarness) SetWorkerExecutor(name string, executor workers.WorkerExecutor) {
	h.t.Helper()
	if h.workers == nil {
		h.t.Fatal("stress process was not constructed with a Worker provider mux")
	}
	h.workers[name] = executor
}

func ensureStressProviderDefinitions(t *testing.T, dir string) {
	t.Helper()
	factoryData, err := os.ReadFile(filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatalf("read stress Factory definition: %v", err)
	}
	var definition struct {
		Workers []struct {
			Name      string `json:"name"`
			StopToken string `json:"stopToken"`
		} `json:"workers"`
		Workstations []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Worker string `json:"worker"`
		} `json:"workstations"`
	}
	if err := json.Unmarshal(factoryData, &definition); err != nil {
		t.Fatalf("decode stress Factory definition: %v", err)
	}
	for _, worker := range definition.Workers {
		name := strings.TrimSpace(worker.Name)
		if name == "" {
			t.Fatal("stress Factory contains unnamed Worker")
		}
		workerDir := filepath.Join(dir, "workers", name)
		if err := os.MkdirAll(workerDir, 0o755); err != nil {
			t.Fatalf("create stress Worker directory: %v", err)
		}
		stopToken := ""
		if worker.StopToken != "" {
			stopToken = "\nstopToken: " + worker.StopToken
		}
		agents := `---
type: MODEL_WORKER
model: stress-model
modelProvider: codex` + stopToken + `
---
Process the stress workload.
`
		if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(agents), 0o644); err != nil {
			t.Fatalf("write stress Worker definition: %v", err)
		}
	}
	for _, workstation := range definition.Workstations {
		name := strings.TrimSpace(workstation.Name)
		if name == "" {
			name = strings.TrimSpace(workstation.ID)
		}
		if name == "" {
			t.Fatal("stress Factory contains unnamed workstation")
		}
		workerName := strings.TrimSpace(workstation.Worker)
		if workerName == "" {
			continue
		}
		workstationDir := filepath.Join(dir, "workstations", name)
		if err := os.MkdirAll(workstationDir, 0o755); err != nil {
			t.Fatalf("create stress workstation directory: %v", err)
		}
		content := fmt.Sprintf(`---
type: MODEL_WORKSTATION
worker: %s
---
Process the stress Work item.
`, workerName)
		if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
			t.Fatalf("write stress workstation definition: %v", err)
		}
	}
}

func (h *stressProcessHarness) Stop() {
	if h == nil || h.cancel == nil {
		return
	}
	h.cancel()
	select {
	case err := <-h.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			h.t.Errorf("Process.Execute() shutdown error = %v", err)
		}
	case <-time.After(5 * time.Second):
		h.t.Errorf("timed out waiting for Process.Execute() shutdown")
	}
	h.cancel = nil
}

func (h *stressProcessHarness) waitForSession(timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, err := http.Get(h.sessionURL())
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for default Factory Session")
}

func (h *stressProcessHarness) SubmitFull(
	ctx context.Context,
	requests []work.SubmitRequest,
) {
	h.t.Helper()
	if err := h.SubmitFullError(ctx, requests); err != nil {
		h.t.Fatalf("submit Work request: %v", err)
	}
}

func (h *stressProcessHarness) SubmitFullError(
	ctx context.Context,
	requests []work.SubmitRequest,
) error {
	requestID := "stress-" + uuid.NewString()
	requests = append([]work.SubmitRequest(nil), requests...)
	for i := range requests {
		if requests[i].RequestID == "" {
			requests[i].RequestID = requestID
		}
	}
	request := work.WorkRequestFromSubmitRequests(requests)
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal Work request: %w", err)
	}
	endpoint := h.baseURL + "/factory-sessions/" + factorysessions.DefaultSessionID +
		"/work-requests/" + url.PathEscape(request.RequestID)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build Work request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("submit Work request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"submit Work request status = %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
	return nil
}

func (h *stressProcessHarness) SubmitWork(workTypeID string, payload []byte) {
	h.t.Helper()
	h.SubmitFull(context.Background(), []work.SubmitRequest{{
		WorkTypeID: workTypeID,
		Payload:    payload,
	}})
}

func (h *stressProcessHarness) SubmitError(workTypeID string, payload []byte) error {
	return h.SubmitFullError(context.Background(), []work.SubmitRequest{{
		WorkTypeID: workTypeID,
		Payload:    payload,
	}})
}

func (h *stressProcessHarness) Assert() *testutil.MarkingAssert {
	h.t.Helper()
	return testutil.AssertMarking(h.t, h.Marking())
}

func (h *stressProcessHarness) WaitForTerminalCount(count int, timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := h.Session()
		progress := session.Runtime.Progress
		terminal := progress.Categories.Terminal + progress.Categories.Failed
		if terminal == count && progress.Categories.Initial == 0 &&
			progress.Categories.Processing == 0 && progress.InFlightCount == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := h.Session()
	var marking []factoryapi.TokenResponse
	if session.Runtime.Petri != nil {
		marking = session.Runtime.Petri.Marking
	}
	encodedMarking, _ := json.Marshal(marking)
	h.t.Fatalf(
		"timed out waiting for %d terminal Work items: progress=%+v marking=%s",
		count,
		session.Runtime.Progress,
		encodedMarking,
	)
}

func (h *stressProcessHarness) Session() factoryapi.FactorySession {
	h.t.Helper()
	response, err := http.Get(h.sessionURL())
	if err != nil {
		h.t.Fatalf("GET default Factory Session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		h.t.Fatalf("GET default Factory Session status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		h.t.Fatalf("decode default Factory Session: %v", err)
	}
	session, err := envelope.AsFactorySession()
	if err != nil {
		h.t.Fatalf("decode Factory Session response: %v", err)
	}
	return session
}

func (h *stressProcessHarness) Marking() *factoryruntime.PetriMarkingSnapshot {
	h.t.Helper()
	session := h.Session()
	snapshot := &factoryruntime.PetriMarkingSnapshot{
		Tokens:      make(map[string]*factoryruntime.RuntimeToken),
		PlaceTokens: make(map[string][]string),
	}
	if session.Runtime.Petri == nil {
		return snapshot
	}
	for _, token := range session.Runtime.Petri.Marking {
		history := factoryruntime.RuntimeTokenHistory{}
		if token.History != nil {
			history.LastError = stringValue(token.History.LastError)
			history.ConsecutiveFailures = integerMapValue(token.History.ConsecutiveFailures)
			history.PlaceVisits = integerMapValue(token.History.PlaceVisits)
			history.TotalVisits = integerMapValue(token.History.TotalVisits)
		}
		tags := map[string]string{}
		if token.Tags != nil {
			for key, value := range *token.Tags {
				tags[key] = value
			}
		}
		runtimeToken := &factoryruntime.RuntimeToken{
			ID:        token.Id,
			PlaceID:   token.PlaceId,
			CreatedAt: token.CreatedAt,
			EnteredAt: token.EnteredAt,
			History:   history,
			Color: factoryruntime.RuntimeTokenColor{
				DataType:   factoryruntime.RuntimeTokenDataTypeWork,
				WorkID:     token.WorkId,
				WorkTypeID: token.WorkType,
				TraceID:    token.TraceId,
				Tags:       tags,
			},
		}
		snapshot.Tokens[token.Id] = runtimeToken
		snapshot.PlaceTokens[token.PlaceId] = append(snapshot.PlaceTokens[token.PlaceId], token.Id)
	}
	return snapshot
}

func integerMapValue(values *factoryapi.IntegerMap) map[string]int {
	if values == nil {
		return nil
	}
	cloned := make(map[string]int, len(*values))
	for key, value := range *values {
		cloned[key] = value
	}
	return cloned
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (h *stressProcessHarness) sessionURL() string {
	return fmt.Sprintf(
		"%s/factory-sessions/%s",
		strings.TrimSuffix(h.baseURL, "/"),
		factorysessions.DefaultSessionID,
	)
}

type workerExecutorProvider struct {
	executor workers.WorkerExecutor
}

func (provider workerExecutorProvider) Infer(
	ctx context.Context,
	request workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	result, err := provider.executor.Execute(ctx, request.Dispatch)
	if err != nil {
		return workers.InferenceResponse{}, err
	}
	content := result.Output
	switch result.Outcome {
	case workers.OutcomeAccepted:
		if content == "" {
			content = "COMPLETE"
		}
	case workers.OutcomeContinue:
		content = "<CONTINUE>"
	case workers.OutcomeRejected:
		if content == "" {
			content = "REJECTED"
		}
	case workers.OutcomeFailed:
		return workers.InferenceResponse{}, errors.New(result.Error)
	}
	return workers.InferenceResponse{Content: content}, nil
}

type workerMuxExecutor map[string]workers.WorkerExecutor

func (mux workerMuxExecutor) Execute(
	ctx context.Context,
	dispatch work.WorkDispatch,
) (workers.WorkResult, error) {
	executor := mux[dispatch.WorkerType]
	if executor == nil {
		return workers.WorkResult{}, fmt.Errorf("no stress executor for Worker %q", dispatch.WorkerType)
	}
	return executor.Execute(ctx, dispatch)
}

type acceptedCountingExecutor struct {
	mu    sync.Mutex
	calls int
}

func (executor *acceptedCountingExecutor) Execute(
	_ context.Context,
	dispatch work.WorkDispatch,
) (workers.WorkResult, error) {
	executor.mu.Lock()
	executor.calls++
	executor.mu.Unlock()
	return workers.WorkResult{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workers.OutcomeAccepted,
	}, nil
}

func (executor *acceptedCountingExecutor) callCount() int {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return executor.calls
}

var (
	_ workers.WorkerExecutor = workerMuxExecutor{}
	_ workers.WorkerExecutor = (*acceptedCountingExecutor)(nil)
)
