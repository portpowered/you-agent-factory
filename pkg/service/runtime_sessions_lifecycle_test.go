package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	api "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"go.uber.org/zap"
)

func TestFactoryService_CancelDurableFactorySession_RuntimeBackedSession(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	ctx := context.Background()

	started, err := fs.StartDurableFactorySessionAsync(ctx, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	response, err := fs.CancelDurableFactorySession(ctx, started.SessionId, factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("CancelDurableFactorySession: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("operation = %q, want CANCEL", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		t.Fatalf("status = %q, want CANCELING", response.Status)
	}
}

func TestFactoryService_CancelDurableFactorySession_HTTPUsesProductionRuntime(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	server := httptest.NewServer(api.NewServer(fs, 0, zap.NewNop()).Handler())
	defer server.Close()

	started, err := fs.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-http-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	url := server.URL + "/factory-sessions/" + started.SessionId + "/cancel"
	resp, err := http.Post(url, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("operation = %q, want CANCEL", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func newFactoryServiceForDurableLifecycleTest(t *testing.T, fixtureName, workflowName string) *FactoryService {
	t.Helper()
	projectRoot := setupDurableLifecycleWorkflowFixture(t, fixtureName, workflowName)
	return &FactoryService{
		cfg: &FactoryServiceConfig{
			Dir: projectRoot,
		},
		factoryRootDir: projectRoot,
	}
}

func setupDurableLifecycleWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func strPtr(value string) *string {
	return &value
}
