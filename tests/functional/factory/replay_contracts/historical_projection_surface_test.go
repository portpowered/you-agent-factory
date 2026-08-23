package replay_contracts_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const historicalProjectionWorkflow = `workflow.final("historical projection result");`

// TestRecordedDurableHistoryUsesThePublicProjectionSurface proves that a
// finalized durable session is read through the public historical result and
// dispatch surfaces, and repeated reads remain stable.
func TestRecordedDurableHistoryUsesThePublicProjectionSurface(t *testing.T) {
	factoryConfig := map[string]any{
		"name": "recordings-historical-projection",
		"invocationSignature": map[string]any{
			"parameters": []any{},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   historicalProjectionWorkflow,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
				},
			},
		},
	}
	dir := support.ScaffoldFactory(t, factoryConfig)
	artifactPath := filepath.Join(t.TempDir(), "historical-projection.replay.json")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", artifactPath},
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startHistoricalProjectionSession(t, server.URL())
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("durable session status = %q, want SUCCEEDED", started.Status)
	}
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("durable session id is empty")
	}

	base := strings.TrimSuffix(server.URL(), "/") + "/factory-sessions/" + started.SessionId
	result := support.GetJSON[factoryapi.FactorySessionResult](t, base+"/results")
	if result.SessionId != started.SessionId || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("historical result = %#v, want final result for %q", result, started.SessionId)
	}
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](t, base+"/dispatches")
	if dispatches.SessionId != started.SessionId {
		t.Fatalf("historical dispatch sessionId = %q, want %q", dispatches.SessionId, started.SessionId)
	}
	repeated := support.GetJSON[factoryapi.FactorySessionResult](t, base+"/results")
	if !reflect.DeepEqual(result, repeated) {
		t.Fatalf("repeated historical result changed: first=%#v second=%#v", result, repeated)
	}
	read := support.GetJSON[factoryapi.FactorySessionGetResponse](t, base)
	session, err := read.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session read: %v", err)
	}
	if session.SessionId != started.SessionId {
		t.Fatalf("durable session read id = %q, want %q", session.SessionId, started.SessionId)
	}
}

func startHistoricalProjectionSession(
	t *testing.T,
	serverURL string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	dialect := "you-workflow-v1"
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: "historical-projection-sync",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   historicalProjectionWorkflow,
				},
			},
		},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal historical projection request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/sync",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("start historical projection session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("historical projection session status = %d: %s", response.StatusCode, body)
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode historical projection response: %v", err)
	}
	return started
}
