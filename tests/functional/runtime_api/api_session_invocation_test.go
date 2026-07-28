package runtime_api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSessionInvocationAPI_AcceptsStructuredArgsWithActiveSignature proves the
// public session invocation API accepts structured args when an active signature
// is configured and returns the completed primary result text.
func TestSessionInvocationAPI_AcceptsStructuredArgsWithActiveSignature(t *testing.T) {
	dir := scaffoldStructuredArgsInvocationFactory(t)
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(support.NewStaticSuccessCommandRunner("structured primary COMPLETE"), nil))

	response := postInvocation(t, server.URL(), factoryapi.InvocationRequest{
		Args: &map[string]any{"input": "structured invoke"},
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	if part.Text != "structured primary COMPLETE" {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, "structured primary COMPLETE")
	}
}

func scaffoldStructuredArgsInvocationFactory(t *testing.T) string {
	t.Helper()

	cfg := simplePipelineConfig()
	cfg["invocationSignature"] = map[string]any{
		"parameters": []any{
			map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			},
		},
	}
	workTypes := cfg["workTypes"].([]map[string]any)
	for i := range workTypes {
		if name, _ := workTypes[i]["name"].(string); name == "task" {
			workTypes[i]["handlingBehavior"] = []string{"DEFAULT"}
		}
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func scaffoldInvocationFactory(t *testing.T, overrides map[string]any) string {
	t.Helper()

	cfg := simplePipelineConfig()
	for key, value := range overrides {
		cfg[key] = value
	}
	workTypes := cfg["workTypes"].([]map[string]any)
	for i := range workTypes {
		if name, _ := workTypes[i]["name"].(string); name == "task" {
			workTypes[i]["handlingBehavior"] = []string{"DEFAULT"}
		}
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func textInvocationRequest(t *testing.T, text string, timeoutMillis *int64) factoryapi.InvocationRequest {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind:    &sourceKind,
		Content:       &content,
		TimeoutMillis: timeoutMillis,
	}
}

func postInvocation(t *testing.T, serverURL string, request factoryapi.InvocationRequest) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/~default/invocations status = %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}
