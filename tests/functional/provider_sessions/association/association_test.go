// Package association owns functional coverage for Provider Session ref correlation
// on public Factory Session dispatch projections.
package association_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	associationChildLabel  = "association-child"
	associationChildPrompt = "associate-provider-session"
	associationWorkflow    = `return (async function () {
  const child = await agent.run({
    prompt: "` + associationChildPrompt + `",
    label: "` + associationChildLabel + `",
  });
  return { child };
})();`
)

// TestProviderSessionRefAssociatesWithOwningDispatchAndFactorySession proves that
// after a Factory Session produces a provider-backed child dispatch, the public
// dispatch listing and dispatch detail surfaces expose providerSessionRefs that
// join to the owning dispatch identifier and the same Factory Session identifier.
func TestProviderSessionRefAssociatesWithOwningDispatchAndFactorySession(t *testing.T) {
	t.Parallel()

	dir := scaffoldAssociationWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	executed := startAssociationWorkflow(t, server.URL(), dir)
	if executed.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", executed.Status)
	}
	if strings.TrimSpace(executed.SessionId) == "" {
		t.Fatal("sessionId unexpectedly empty")
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	listed := listAssociationDispatches(t, server.URL(), executed.SessionId)
	if listed.SessionId != executed.SessionId {
		t.Fatalf(
			"dispatch list sessionId = %q, want owning Factory Session %q",
			listed.SessionId,
			executed.SessionId,
		)
	}
	if len(listed.Dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want exactly one provider-backed child dispatch", len(listed.Dispatches))
	}

	summary := listed.Dispatches[0]
	if strings.TrimSpace(summary.Id) == "" {
		t.Fatal("dispatch id unexpectedly empty on public dispatch summary")
	}
	if summary.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch status = %q, want COMPLETED", summary.Status)
	}
	if summary.Label == nil || *summary.Label != associationChildLabel {
		t.Fatalf("dispatch label = %#v, want %q", summary.Label, associationChildLabel)
	}

	providerRef := requireProviderSessionRefOnDispatchSummary(t, summary)
	if providerRef.Id != "fake-provider-session-1" {
		t.Fatalf(
			"providerSessionRef id = %q, want runtime-produced fake-provider-session-1",
			providerRef.Id,
		)
	}
	if providerRef.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("providerSessionRef kind = %q, want session_id", providerRef.Kind)
	}

	detail := getAssociationDispatchDetail(t, server.URL(), executed.SessionId, summary.Id)
	if detail.SessionId != executed.SessionId {
		t.Fatalf(
			"dispatch detail sessionId = %q, want owning Factory Session %q",
			detail.SessionId,
			executed.SessionId,
		)
	}
	if detail.Id != summary.Id {
		t.Fatalf(
			"dispatch detail id = %q, want same owning dispatch %q from listing",
			detail.Id,
			summary.Id,
		)
	}
	detailRef := requireProviderSessionRefOnDispatchDetail(t, detail)
	if detailRef.Id != providerRef.Id {
		t.Fatalf(
			"dispatch detail providerSessionRef id = %q, want same ref %q from listing",
			detailRef.Id,
			providerRef.Id,
		)
	}
}

func scaffoldAssociationWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "provider-session-association"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(associationWorkflow), 0o600); err != nil {
		t.Fatalf("write association workflow: %v", err)
	}
	return dir
}

func startAssociationWorkflow(
	t *testing.T,
	serverURL string,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "provider-session-association",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal association workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build association workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start association workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start association workflow status = %d: %s", response.StatusCode, body.String())
	}
	var executed factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&executed); err != nil {
		t.Fatalf("decode association workflow response: %v", err)
	}
	return executed
}

func listAssociationDispatches(
	t *testing.T,
	serverURL string,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	return support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches",
	)
}

func getAssociationDispatchDetail(
	t *testing.T,
	serverURL string,
	sessionID string,
	dispatchID string,
) factoryapi.FactoryDispatch {
	t.Helper()

	return support.GetJSON[factoryapi.FactoryDispatch](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches/"+dispatchID,
	)
}

func requireProviderSessionRefOnDispatchSummary(
	t *testing.T,
	summary factoryapi.FactorySessionDispatchSummary,
) factoryapi.LoadableProviderSessionRef {
	t.Helper()

	if summary.ProviderSessionRefs == nil || len(*summary.ProviderSessionRefs) != 1 {
		t.Fatalf("providerSessionRefs = %#v, want exactly one public ref", summary.ProviderSessionRefs)
	}
	ref := (*summary.ProviderSessionRefs)[0]
	if strings.TrimSpace(ref.Id) == "" {
		t.Fatalf("providerSessionRef = %#v, want non-empty public session identity", ref)
	}
	return ref
}

func requireProviderSessionRefOnDispatchDetail(
	t *testing.T,
	detail factoryapi.FactoryDispatch,
) factoryapi.LoadableProviderSessionRef {
	t.Helper()

	if detail.ProviderSessionRefs == nil || len(*detail.ProviderSessionRefs) != 1 {
		t.Fatalf("providerSessionRefs = %#v, want exactly one public ref", detail.ProviderSessionRefs)
	}
	ref := (*detail.ProviderSessionRefs)[0]
	if strings.TrimSpace(ref.Id) == "" {
		t.Fatalf("providerSessionRef = %#v, want non-empty public session identity", ref)
	}
	return ref
}
