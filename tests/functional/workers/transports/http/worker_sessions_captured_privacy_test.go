package http_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Classified publication output seeds the supported recording representation.
// CLI/HTTP logs are customer observers of recovered history, not a
// claim that a native provider classifies its output. Each reconstruction owns
// its home/store and joins the previous server before reusing those files.
func assertCapturedPublishedPrivacy(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	page := current
	page.Events = append([]factoryapi.WorkerSessionEvent(nil), current.Events...)
	position := -1
	for index, frame := range page.Events {
		if frame.Event.Payload["kind"] == "MESSAGE" {
			position = index
			break
		}
	}
	if position < 0 {
		t.Fatal("recording fixture has no message position")
	}
	draft := workers.Draft{
		Kind: workers.KindTool, Phase: workers.PhaseCompleted,
		Provenance:                 workers.Provenance{Provider: "codex", NativeEventType: "tool.completed"},
		Payload:                    json.RawMessage(`{"toolCallId":"visible-call","toolName":"visible-tool","argumentsSummary":{"environment":"classified-environment-token"},"resultSummary":{"secret":"classified-tool-token","neighbor":"visible-result"}}`),
		DeclaredSecretJSONPointers: []string{"/argumentsSummary/environment", "/resultSummary/secret"},
	}
	var published []workers.ProgressFragment
	publisher := workersessions.NewProviderSessionObservationPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, fragment)
	}).WithUnassociatedProgressFallback()
	publisher.Publish(workers.CanonicalDraftFragment("privacy-dispatch", draft))
	if len(published) != 1 {
		t.Fatal("classified publication produced no safe recording fixture")
	}
	safe, ok := published[0].CanonicalDraft.(workers.Draft)
	if !ok || workers.ValidateDraft(safe) != nil {
		t.Fatal("classified publication did not preserve a valid Worker draft")
	}
	data, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	page.Events[position].Event.Payload = nil
	if err := json.Unmarshal(data, &page.Events[position].Event.Payload); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeLegacyCapturedFixture(t, root, page)
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(root)
	for range 2 {
		server := support.StartFunctionalAPIServer(t, config)
		logs := assertLegacyLogsCLIHTTPParity(t, server, page.WorkerSessionId)
		encoded, err := json.Marshal(logs)
		if err != nil {
			t.Fatal(err)
		}
		assertCapturedPublicPrivacy(t, string(encoded))
		assertCapturedPrivateCursorError(t, server, page.WorkerSessionId)
		server.Close(t)
	}
}

func assertCapturedPublicPrivacy(t *testing.T, output string) {
	t.Helper()
	for _, secret := range []string{"classified-environment-token", "classified-tool-token", "DeclaredSecretJSONPointers"} {
		if strings.Contains(output, secret) {
			t.Fatal("public captured read exposed classified content")
		}
	}
	for _, visible := range []string{"visible-tool", "visible-result", "redacted"} {
		if !strings.Contains(output, visible) {
			t.Fatalf("public captured read lost %s", visible)
		}
	}
}

func assertCapturedPrivateCursorError(t *testing.T, server *support.FunctionalAPIServer, id string) {
	t.Helper()
	response, err := http.Get(server.URL() + "/worker-sessions/" + url.PathEscape(id) + "/logs?nextToken=classified-tool-token")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("private cursor error status=%d error=%v", response.StatusCode, err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--next-token", "classified-tool-token", "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err == nil {
		t.Fatal("CLI accepted private malformed cursor")
	}
	for _, output := range []string{string(data), inputs.Stdout() + inputs.Stderr()} {
		if strings.Contains(output, "classified-tool-token") || strings.Contains(output, "visible-result") {
			t.Fatal("cursor rejection exposed token or captured content")
		}
	}
}
