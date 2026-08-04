package response_events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryResponseEventSequenceSurvivesSessionRuntimeReplacement proves
// that when a live, explicitly-opened Factory Session's runtime is replaced
// mid-session (the customer-reachable PUT /factory-sessions/{session_id}/factory
// REPLACE_CURRENT path -- see pkg/services/factory_sessions/internal/
// runtimebinding.Replace, which the running-service definition-activation flow
// drives through pkg/services/factory_sessions/internal/sessionservice's
// SaveReplaceCurrentSnapshotForSession/ActivateSessionEditableFactory), the
// session's public response-event sequence numbers remain globally unique and
// strictly ascending across the replacement instead of colliding.
//
// Replacing a live session's runtime constructs a brand-new
// SessionResponseEventStore for the SAME session identity
// (pkg/services/factory_sessions/internal/runtime/service.go's newLiveSession,
// called again from Service.Register during Replace). For an explicitly
// opened, non-"~default" session this identity is the session's own stable
// ID: livesession.EnsureRuntimeID only mints a fresh synthetic runtime
// identity for the reserved "~default" alias
// (pkg/services/factory_sessions/internal/livesession/session.go), so a named
// or explicitly-opened session's livesession.CanonicalID stays the session's
// own ID across the replacement -- and the Events root's per-session topic
// ("factory-session/<id>/response-events", see responseEventTopic in
// .../services/response_stream/internal/service/service.go) is keyed purely
// by that identity, so it already carries history from the first store
// instance. The new store's own local sequence counter starts back at zero,
// so only PublishThroughAuthority's unconditional adoption of whatever
// sequence the Events root actually assigns
// (pkg/services/factory_sessions/internal/responseeventstore/store.go) keeps
// the second store's publishes continuing past the first store's highest
// sequence instead of renumbering from 1 and colliding with the first
// dispatch's own sequence 1. This test drives that exact construction
// boundary through two ordinary session invocations separated by one runtime
// replacement, all inside a single root.BuildProcess-built process, and
// proves the public response-event history the customer reads back has no
// duplicate or non-ascending sequence across the replacement.
func TestFactoryResponseEventSequenceSurvivesSessionRuntimeReplacement(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, sessionRuntimeReplaceFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)

	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&edges,
		support.NewStaticSuccessCommandRunner("session runtime replace dispatch COMPLETE"),
		nil,
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	baseURL := server.URL()

	// Open a session explicitly instead of using the "~default" alias: only
	// "~default" gets a synthetic per-construction runtime identity
	// substituted in for the Events topic key (see the doc comment above), so
	// only an explicitly opened session's own stable ID reproduces the
	// same-topic reconstruction the delegation's authority-adoption logic
	// exists to handle.
	sessionID := openSessionRuntimeReplaceDefaultTargetSession(t, baseURL, dir)

	firstInvocation := postSessionRuntimeReplaceInvocation(t, baseURL, sessionID, "first dispatch, before replacement")
	assertSessionRuntimeReplaceInvocationCompleted(t, firstInvocation)

	firstEvents := support.GetFactoryResponseEventsAt(t, baseURL, sessionID)
	if len(firstEvents) == 0 {
		t.Fatal("response events after first dispatch = 0, want observable response-stream records")
	}
	assertResponseEventsAscendingSequence(t, firstEvents)
	firstSequences := make(map[int64]bool, len(firstEvents))
	var firstMax int64
	for _, event := range firstEvents {
		firstSequences[event.Sequence] = true
		if event.Sequence > firstMax {
			firstMax = event.Sequence
		}
	}

	replaceSessionRuntimeCurrentFactory(t, baseURL, sessionID, "task")

	secondInvocation := postSessionRuntimeReplaceInvocation(t, baseURL, sessionID, "second dispatch, after replacement")
	assertSessionRuntimeReplaceInvocationCompleted(t, secondInvocation)

	secondEvents := support.GetFactoryResponseEventsAt(t, baseURL, sessionID)
	if len(secondEvents) == 0 {
		t.Fatal("response events after second dispatch = 0, want observable response-stream records")
	}
	assertResponseEventsAscendingSequence(t, secondEvents)

	for _, event := range secondEvents {
		if firstSequences[event.Sequence] {
			t.Fatalf(
				"response event sequence %d (%s) published by the post-replacement session store "+
					"collides with a sequence already used by the pre-replacement store; this is exactly "+
					"the collision PublishThroughAuthority's unconditional authority-sequence adoption "+
					"exists to prevent when a session's Events topic already carries history from a prior "+
					"store instance",
				event.Sequence,
				event.EventId,
			)
		}
		if event.Sequence <= firstMax {
			t.Fatalf(
				"post-replacement response event sequence %d (%s) <= pre-replacement max sequence %d, "+
					"want strictly greater: the second store instance must continue the session's global "+
					"sequence instead of renumbering from its own fresh local counter",
				event.Sequence,
				event.EventId,
				firstMax,
			)
		}
	}
}

func sessionRuntimeReplaceFactoryConfig() map[string]any {
	return map[string]any{
		"name": "session-runtime-replace",
		"id":   "session-runtime-replace",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

// openSessionRuntimeReplaceDefaultTargetSession opens an explicit,
// non-"~default" Factory Session against the same root factory directory
// through the public POST /factory-sessions boundary, returning its assigned
// session ID.
func openSessionRuntimeReplaceDefaultTargetSession(t *testing.T, baseURL, folderPath string) string {
	t.Helper()

	body, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: folderPath,
		Target: &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		},
	})
	if err != nil {
		t.Fatalf("marshal open factory session request: %v", err)
	}
	endpoint := baseURL + "/factory-sessions"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, payload)
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		t.Fatalf("decode open factory session response: %v", err)
	}
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open factory session response = %#v, want session id", opened)
	}
	return opened.Session.Id
}

func postSessionRuntimeReplaceInvocation(
	t *testing.T,
	baseURL, sessionID, text string,
) factoryapi.InvocationResponse {
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
	request := factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, payload)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func assertSessionRuntimeReplaceInvocationCompleted(t *testing.T, response factoryapi.InvocationResponse) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
}

// replaceSessionRuntimeCurrentFactory drives the customer-facing
// PUT /factory-sessions/{session_id}/factory REPLACE_CURRENT path (mode
// omitted defaults to REPLACE_CURRENT) against the running explicitly-opened
// Factory Session, which the running service must have idle after the prior
// invocation returned a terminal status. This is the one production call
// site (pkg/services/factory_sessions/internal/sessionservice/save.go's
// SaveReplaceCurrentSnapshotForSession -> ActivateSessionEditableFactory ->
// ReplaceSessionRuntime -> runtimebinding.Replace) that reconstructs a second
// SessionResponseEventStore for an already-live session ID.
func replaceSessionRuntimeCurrentFactory(t *testing.T, baseURL, sessionID, workType string) {
	t.Helper()

	current := getSessionRuntimeReplaceCurrentFactory(t, baseURL, sessionID)
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for replacement save")
	}
	if current.Id == nil || *current.Id == "" {
		t.Fatal("current factory id = nil, want a durable runtime id to echo back on replacement save")
	}

	nextVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  current.Version.Logical + 1,
		Physical: current.Version.Physical.UTC().Add(time.Millisecond),
	}
	body := sessionRuntimeReplaceFactorySaveBody(*current.Id, workType, nextVersion)

	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/factory"
	request, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("build replace-current factory request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("PUT %s status = %d, want 200: %s", endpoint, response.StatusCode, payload)
	}
}

func getSessionRuntimeReplaceCurrentFactory(t *testing.T, baseURL, sessionID string) factoryapi.Factory {
	t.Helper()

	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/factory"
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s status = %d, want 200: %s", endpoint, response.StatusCode, payload)
	}
	var current factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		t.Fatalf("decode current factory response: %v", err)
	}
	return current
}

func sessionRuntimeReplaceFactorySaveBody(id, workType string, version factoryapi.HybridLogicalTimestamp) string {
	factoryJSON := `{
		"name":"UNDEFINED",
		"id":"` + id + `",
		"version":{"physical":"` + version.Physical.UTC().Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(version.Logical.Int64(), 10) + `"},
		"workTypes":[{"name":"` + workType + `","handlingBehavior":["DEFAULT"],"states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CODEX","executorProvider":"SCRIPT_WRAP","model":"gpt-5-codex"}],
		"workstations":[{"name":"process","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"worker-a","body":"Do the work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"complete"}],"onFailure":[{"workType":"` + workType + `","state":"failed"}]}]
	}`
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}
