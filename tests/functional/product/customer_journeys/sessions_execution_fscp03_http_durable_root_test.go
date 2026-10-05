package customer_journeys_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFSCP03HTTPDurableStartUsesCurrentFactoryRoot(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)
	factoryDir := scaffoldFSCP03ProbeFactory(t)
	workflowDir := filepath.Join(factoryDir, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create Factory workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "http-root.workflow.js"), []byte(`return "http durable root";`), 0o600); err != nil {
		t.Fatalf("write Factory workflow: %v", err)
	}
	home := t.TempDir()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges: serviceedges.Edges{
			FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			BrowserOpener:                      func(context.Context, string) error { return nil },
		},
	})
	defer server.Close(t)
	workflowName := "http-root"
	requestBody, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "fscp03-http-durable-root",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
	})
	if err != nil {
		t.Fatalf("marshal durable request: %v", err)
	}
	endpoint := strings.TrimSuffix(server.URL(), "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("build durable request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST durable request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("durable start status = %d: %s", response.StatusCode, body)
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode durable response: %v", err)
	}
	if !strings.HasPrefix(started.SessionId, "dur-sess-") || string(started.Status) != "SUCCEEDED" {
		t.Fatalf("durable start = %#v", started)
	}
	if got := started.ResolvedSource; got.SourceRef == nil || *got.SourceRef == "" {
		t.Fatalf("durable source identity missing: %#v", got)
	}
}
