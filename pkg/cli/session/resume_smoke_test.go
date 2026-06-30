package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func TestCLIResumeSmoke_InterruptedJavaScriptFactorySessionResumesThroughSharedSessionCommands(t *testing.T) {
	harness := newCLIResumeSmokeHarness(t)
	sessionID := harness.startInterruptedSession(t)

	before := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
	if before.SessionId != sessionID {
		t.Fatalf("pre-resume sessionId = %q, want %q", before.SessionId, sessionID)
	}
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-resume status = %q, want INTERRUPTED", before.Status)
	}
	if before.Lifecycle == nil || before.Lifecycle.InterruptedAt == nil {
		t.Fatalf("pre-resume lifecycle = %#v, want interruptedAt", before.Lifecycle)
	}

	resumeResponse := resumeSessionViaCLI(t, harness.serverURL, sessionID)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", resumeResponse.Operation)
	}
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}
	if resumeResponse.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", resumeResponse.SessionId, sessionID)
	}

	after := waitForDurableSessionStatusViaCLI(
		t,
		harness.serverURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	if after.SessionId != sessionID {
		t.Fatalf("post-resume sessionId = %q, want %q", after.SessionId, sessionID)
	}
	if after.Lifecycle == nil || after.Lifecycle.InterruptedAt == nil || after.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt", after.Lifecycle)
	}
	if after.ResultSummary == nil || after.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("post-resume resultSummary = %#v, want FINAL", after.ResultSummary)
	}
	if harness.provider.callCount() < 3 {
		t.Fatalf("provider infer calls = %d, want at least 3 after resume completion", harness.provider.callCount())
	}
}

type cliResumeSmokeHarness struct {
	serverURL string
	service   fse.Service
	provider  *cliResumeSmokeBlockingProvider
}

func newCLIResumeSmokeHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "resumable-two-step-fake-children.workflow.js", workflowName)
	provider := newCLIResumeSmokeBlockingProvider(workflowName)

	runtimeService := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		PersistSessions:   true,
	})

	server := httptest.NewServer(api.NewServer(&testutil.MockFactory{
		DurableExecutionService: runtimeService,
	}, 0, zap.NewNop()).Handler())
	t.Cleanup(server.Close)

	return &cliResumeSmokeHarness{
		serverURL: server.URL,
		service:   runtimeService,
		provider:  provider,
	}
}

func (h *cliResumeSmokeHarness) startInterruptedSession(t *testing.T) string {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	started, err := h.service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-cli-resume-smoke-start-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: workflowName,
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	sessionID := started.SessionID
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}

	waitForCLIResumeSmokeDispatchStatus(
		t,
		h.service,
		sessionID,
		"dispatch-1",
		fse.DispatchStatusCompleted,
		5*time.Second,
	)
	waitForCLIResumeSmokeDispatchStatus(
		t,
		h.service,
		sessionID,
		"dispatch-2",
		fse.DispatchStatusRunning,
		5*time.Second,
	)

	_, err = h.service.InterruptDispatch(context.Background(), sessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "cli resume smoke interrupt"},
		DispatchID:     "dispatch-2",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	h.provider.waitForCanceledInfer(t, 5*time.Second)
	waitForCLIResumeSmokeSessionStatus(t, h.service, sessionID, fse.LifecycleStatusInterrupted, 5*time.Second)
	return sessionID
}

func readDurableSessionViaCLI(
	t *testing.T,
	serverURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	var out bytes.Buffer
	if err := sessioncli.Show(sessioncli.ShowConfig{
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("session show: %v", err)
	}

	var session factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &session); err != nil {
		t.Fatalf("decode session show JSON: %v\n%s", err, out.String())
	}
	return session
}

func resumeSessionViaCLI(
	t *testing.T,
	serverURL string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	var out bytes.Buffer
	if err := sessioncli.Resume(sessioncli.LifecycleControlConfig{
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("session resume: %v", err)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil {
		t.Fatalf("decode session resume JSON: %v\n%s", err, out.String())
	}
	return response
}

func waitForDurableSessionStatusViaCLI(
	t *testing.T,
	serverURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableSessionViaCLI(t, serverURL, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableSessionViaCLI(t, serverURL, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func setupCLIResumeSmokeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()

	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func waitForCLIResumeSmokeDispatchStatus(
	t *testing.T,
	svc fse.Service,
	sessionID string,
	dispatchID string,
	want fse.DispatchStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed, err := svc.ListDispatches(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListDispatches: %v", err)
		}
		for _, dispatch := range listed.Dispatches {
			if dispatch.ID != dispatchID {
				continue
			}
			if dispatch.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach %s within %s", dispatchID, want, timeout)
}

func waitForCLIResumeSmokeSessionStatus(
	t *testing.T,
	svc fse.Service,
	sessionID string,
	want fse.LifecycleStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		read, err := svc.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	read, err := svc.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	t.Fatalf("session status = %q, want %q within %s", read.Status, want, timeout)
}

type cliResumeSmokeBlockingProvider struct {
	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled int
	workflowName    string
}

func newCLIResumeSmokeBlockingProvider(workflowName string) *cliResumeSmokeBlockingProvider {
	return &cliResumeSmokeBlockingProvider{workflowName: workflowName}
}

func (p *cliResumeSmokeBlockingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *cliResumeSmokeBlockingProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		return interfaces.InferenceResponse{
			Content: fmt.Sprintf(`{"text":"live:%s:step-one:step-one:workflows","label":"step-one"}`, p.workflowName),
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return interfaces.InferenceResponse{}, ctx.Err()
	}

	return interfaces.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"live:%s:step-two:step-two:workflows","label":"step-two"}`, p.workflowName),
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}, nil
}

func (p *cliResumeSmokeBlockingProvider) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled
		p.mu.Unlock()
		if canceled > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("provider Infer did not observe canceled workflow context")
}
