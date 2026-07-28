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

	absentProviderSessionChildLabel  = "association-no-provider-child"
	absentProviderSessionChildPrompt = "fail:association-absent-provider-session"
	absentProviderSessionWorkflow    = `return (async function () {
  const child = await agent.run({
    prompt: "` + absentProviderSessionChildPrompt + `",
    label: "` + absentProviderSessionChildLabel + `",
  });
  return { child };
})();`

	multiDispatchFirstChildLabel  = "association-multi-first"
	multiDispatchFirstChildPrompt = "associate-multi-first-provider-session"
	multiDispatchSecondChildLabel  = "association-multi-second"
	multiDispatchSecondChildPrompt = "associate-multi-second-provider-session"
	multiDispatchWorkflow          = `return (async function () {
  const first = await agent.run({
    prompt: "` + multiDispatchFirstChildPrompt + `",
    label: "` + multiDispatchFirstChildLabel + `",
  });
  const second = await agent.run({
    prompt: "` + multiDispatchSecondChildPrompt + `",
    label: "` + multiDispatchSecondChildLabel + `",
  });
  return { first, second };
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

// TestAbsentProviderSessionIsNotFabricated proves that when a Factory Session
// dispatch completes without establishing a Provider Session, the public dispatch
// listing and dispatch detail surfaces omit fabricated providerSessionRefs rather
// than inventing a synthetic session identity customers could treat as real.
func TestAbsentProviderSessionIsNotFabricated(t *testing.T) {
	t.Parallel()

	dir := scaffoldAbsentProviderSessionWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	executed := startAbsentProviderSessionWorkflow(t, server.URL(), dir)
	if executed.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", executed.Status)
	}
	if strings.TrimSpace(executed.SessionId) == "" {
		t.Fatal("sessionId unexpectedly empty")
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child failure edge", runner.CallCount())
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
		t.Fatalf("dispatch count = %d, want exactly one child dispatch without a Provider Session", len(listed.Dispatches))
	}

	summary := listed.Dispatches[0]
	if strings.TrimSpace(summary.Id) == "" {
		t.Fatal("dispatch id unexpectedly empty on public dispatch summary")
	}
	if summary.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch status = %q, want FAILED", summary.Status)
	}
	if summary.Label == nil || *summary.Label != absentProviderSessionChildLabel {
		t.Fatalf("dispatch label = %#v, want %q", summary.Label, absentProviderSessionChildLabel)
	}
	assertAbsentProviderSessionRefsOnDispatchSummary(t, summary)

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
	assertAbsentProviderSessionRefsOnDispatchDetail(t, detail)
}

// TestMultipleDispatchesKeepDistinctProviderSessionRefs proves that when a
// Factory Session produces multiple provider-backed child dispatches, each
// dispatch keeps its own providerSessionRef on public listing and detail
// surfaces without colliding or sharing a single fabricated identity, and each
// ref continues to join to its owning dispatch and the same Factory Session.
func TestMultipleDispatchesKeepDistinctProviderSessionRefs(t *testing.T) {
	t.Parallel()

	dir := scaffoldMultiDispatchWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	executed := startMultiDispatchWorkflow(t, server.URL(), dir)
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
	if len(listed.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want exactly two provider-backed child dispatches", len(listed.Dispatches))
	}

	firstSummary, secondSummary := findMultiDispatchSummaries(t, listed.Dispatches)
	firstRef := requireProviderSessionRefOnDispatchSummary(t, firstSummary)
	secondRef := requireProviderSessionRefOnDispatchSummary(t, secondSummary)
	if firstRef.Id == secondRef.Id {
		t.Fatalf(
			"providerSessionRef ids collided = %q for dispatches %q and %q",
			firstRef.Id,
			firstSummary.Id,
			secondSummary.Id,
		)
	}
	if firstRef.Id != "fake-provider-session-1" {
		t.Fatalf(
			"first dispatch providerSessionRef id = %q, want runtime-produced fake-provider-session-1",
			firstRef.Id,
		)
	}
	if secondRef.Id != "fake-provider-session-2" {
		t.Fatalf(
			"second dispatch providerSessionRef id = %q, want runtime-produced fake-provider-session-2",
			secondRef.Id,
		)
	}
	assertProviderSessionRefKind(t, firstRef)
	assertProviderSessionRefKind(t, secondRef)

	firstDetail := getAssociationDispatchDetail(t, server.URL(), executed.SessionId, firstSummary.Id)
	secondDetail := getAssociationDispatchDetail(t, server.URL(), executed.SessionId, secondSummary.Id)
	assertDispatchOwnsProviderSessionRef(
		t,
		executed.SessionId,
		firstSummary.Id,
		firstDetail,
		firstRef,
	)
	assertDispatchOwnsProviderSessionRef(
		t,
		executed.SessionId,
		secondSummary.Id,
		secondDetail,
		secondRef,
	)
}

func scaffoldAbsentProviderSessionWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "provider-session-association-absent"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(absentProviderSessionWorkflow), 0o600); err != nil {
		t.Fatalf("write absent provider session workflow: %v", err)
	}
	return dir
}

func startAbsentProviderSessionWorkflow(
	t *testing.T,
	serverURL string,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "provider-session-association-absent",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal absent provider session workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build absent provider session workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start absent provider session workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start absent provider session workflow status = %d: %s", response.StatusCode, body.String())
	}
	var executed factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&executed); err != nil {
		t.Fatalf("decode absent provider session workflow response: %v", err)
	}
	return executed
}

func assertAbsentProviderSessionRefsOnDispatchSummary(
	t *testing.T,
	summary factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if summary.ProviderSessionRefs != nil && len(*summary.ProviderSessionRefs) > 0 {
		t.Fatalf(
			"providerSessionRefs = %#v, want absent public refs without fabricated session identity",
			summary.ProviderSessionRefs,
		)
	}
}

func assertAbsentProviderSessionRefsOnDispatchDetail(
	t *testing.T,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()

	if detail.ProviderSessionRefs != nil && len(*detail.ProviderSessionRefs) > 0 {
		t.Fatalf(
			"providerSessionRefs = %#v, want absent public refs without fabricated session identity",
			detail.ProviderSessionRefs,
		)
	}
}

func scaffoldMultiDispatchWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "provider-session-association-multi"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(multiDispatchWorkflow), 0o600); err != nil {
		t.Fatalf("write multi-dispatch workflow: %v", err)
	}
	return dir
}

func startMultiDispatchWorkflow(
	t *testing.T,
	serverURL string,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "provider-session-association-multi",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal multi-dispatch workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build multi-dispatch workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start multi-dispatch workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start multi-dispatch workflow status = %d: %s", response.StatusCode, body.String())
	}
	var executed factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&executed); err != nil {
		t.Fatalf("decode multi-dispatch workflow response: %v", err)
	}
	return executed
}

func findMultiDispatchSummaries(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) (factoryapi.FactorySessionDispatchSummary, factoryapi.FactorySessionDispatchSummary) {
	t.Helper()

	var firstSummary factoryapi.FactorySessionDispatchSummary
	var secondSummary factoryapi.FactorySessionDispatchSummary
	foundFirst := false
	foundSecond := false
	for _, summary := range dispatches {
		if summary.Label == nil {
			t.Fatalf("dispatch %q label unexpectedly nil", summary.Id)
		}
		switch *summary.Label {
		case multiDispatchFirstChildLabel:
			if foundFirst {
				t.Fatalf("found duplicate first multi-dispatch label %q", multiDispatchFirstChildLabel)
			}
			firstSummary = summary
			foundFirst = true
		case multiDispatchSecondChildLabel:
			if foundSecond {
				t.Fatalf("found duplicate second multi-dispatch label %q", multiDispatchSecondChildLabel)
			}
			secondSummary = summary
			foundSecond = true
		default:
			t.Fatalf("dispatch label = %q, want %q or %q", *summary.Label, multiDispatchFirstChildLabel, multiDispatchSecondChildLabel)
		}
		if strings.TrimSpace(summary.Id) == "" {
			t.Fatal("dispatch id unexpectedly empty on public dispatch summary")
		}
		if summary.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %q status = %q, want COMPLETED", summary.Id, summary.Status)
		}
	}
	if !foundFirst || !foundSecond {
		t.Fatalf(
			"multi-dispatch summaries incomplete: foundFirst=%t foundSecond=%t",
			foundFirst,
			foundSecond,
		)
	}
	return firstSummary, secondSummary
}

func assertProviderSessionRefKind(t *testing.T, ref factoryapi.LoadableProviderSessionRef) {
	t.Helper()

	if ref.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("providerSessionRef kind = %q, want session_id", ref.Kind)
	}
}

func assertDispatchOwnsProviderSessionRef(
	t *testing.T,
	factorySessionID string,
	dispatchID string,
	detail factoryapi.FactoryDispatch,
	expectedRef factoryapi.LoadableProviderSessionRef,
) {
	t.Helper()

	if detail.SessionId != factorySessionID {
		t.Fatalf(
			"dispatch detail sessionId = %q, want owning Factory Session %q",
			detail.SessionId,
			factorySessionID,
		)
	}
	if detail.Id != dispatchID {
		t.Fatalf(
			"dispatch detail id = %q, want same owning dispatch %q from listing",
			detail.Id,
			dispatchID,
		)
	}
	detailRef := requireProviderSessionRefOnDispatchDetail(t, detail)
	if detailRef.Id != expectedRef.Id {
		t.Fatalf(
			"dispatch detail providerSessionRef id = %q, want same ref %q from listing",
			detailRef.Id,
			expectedRef.Id,
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
