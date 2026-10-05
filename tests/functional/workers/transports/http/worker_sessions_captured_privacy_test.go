package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type capturedPrivacyCommand struct{ *functionalWorkerGate }

func (runner capturedPrivacyCommand) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	result, err := runner.functionalWorkerGate.Run(ctx, request)
	if err == nil {
		result.Stdout = append([]byte(`{"type":"item.completed","item":{"id":"visible-call","type":"command_execution","aggregated_output":"visible-tool visible-result classified-tool-token classified-environment-token"}}`+"\n"), support.CodexSuccessStdout("visible completion. COMPLETE")...)
	}
	return result, err
}

// Direct admission is the behavior here: converting it to Factory dispatch
// would miss the ID-only live stream. This immutable command shape owns a
// separate root, home and store; no native files, executable or paid provider.
func TestWorkerSessionCapturedLogsLivePrivacy(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	runner := capturedPrivacyCommand{newFunctionalWorkerGate(release)}
	dir := support.ScaffoldSingleStepFactory(t, "captured-live-privacy")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	}
	server := support.StartFunctionalAPIServer(t, config)
	payload := directWorkerSessionPayload("privacy-request", "privacy-worker", "privacy-dispatch")
	message, prompt := "classified-tool-token", "classified-environment-token"
	payload.Execution.UserMessage, payload.Execution.SystemPrompt = &message, &prompt
	payload.Execution.EnvVars = &map[string]string{"CAPTURE_SECRET": prompt, "TOOL_SECRET": message}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL()+"/worker-sessions", "application/json", strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("private Direct admission status=%d", response.StatusCode)
	}
	runner.waitStarted(t)
	assertCapturedPrivateCursorError(t, server, "privacy-worker")
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/worker-sessions/privacy-worker/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live private stream status=%d", response.StatusCode)
	}
	// Response headers establish subscription before the controlled command
	// returns. The stream observes publication through its terminal frame.
	close(release)
	frames, err := readWorkerSessionEventStream(response)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(frames)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPublicPrivacy(t, string(data))
	ended := waitCapturedTerminal(t, server.URL(), "privacy-worker")
	logs := assertCapturedLogsCLIHTTPParity(t, server, "privacy-worker")
	data, err = json.Marshal(logs)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPublicPrivacy(t, string(data))
	assertCapturedReplayTimes(t, server.URL(), ended)
	server.Close(t)
	restarted := support.StartFunctionalAPIServer(t, config)
	logs = assertCapturedLogsCLIHTTPParity(t, restarted, "privacy-worker")
	data, err = json.Marshal(logs)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPublicPrivacy(t, string(data))
}

// Classified publication output seeds the supported recording representation.
// CLI/HTTP logs are customer observers of recovered history, not a
// claim that a native provider classifies its output. Each reconstruction owns
// its home/store and joins the previous server before reusing those files.
func assertCapturedPublishedPrivacy(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	for _, test := range []struct {
		name     string
		kind     workers.Kind
		phase    workers.Phase
		payload  string
		pointers []string
	}{
		{"tool", workers.KindTool, workers.PhaseCompleted,
			`{"toolCallId":"visible-call","toolName":"visible-tool","argumentsSummary":{"environment":"classified-environment-token"},"resultSummary":{"secret":"classified-tool-token","neighbor":"visible-result"}}`,
			[]string{"/argumentsSummary/environment", "/resultSummary/secret"}},
		{"oversized_tool", workers.KindTool, workers.PhaseCompleted,
			`{"toolCallId":"visible-call","toolName":"visible-tool","argumentsSummary":{"environment":"classified-environment-token"},"resultSummary":{"secret":"classified-tool-token","neighbor":"visible-result ` + strings.Repeat("x", 3<<20) + `"}}`,
			[]string{"/argumentsSummary/environment", "/resultSummary/secret"}},
		{"tool_delta", workers.KindTool, workers.PhaseDelta,
			`{"toolCallId":"visible-tool visible-result","outputDelta":"classified-tool-token"}`,
			[]string{"/outputDelta"}},
		{"progress", workers.KindProgress, workers.PhaseUpdated,
			`{"label":"visible-tool visible-result","message":"classified-tool-token"}`,
			[]string{"/message"}},
		{"error", workers.KindError, workers.PhaseFailed,
			`{"code":"visible-tool visible-result","message":"classified-tool-token","retryable":true}`,
			[]string{"/message"}},
	} {
		t.Run("recovered_privacy_"+test.name, func(t *testing.T) {
			t.Parallel()
			assertCapturedPublishedDraftPrivacy(t, config, current, workers.Draft{
				Kind: test.kind, Phase: test.phase,
				Provenance: workers.Provenance{Provider: "codex", NativeEventType: "classified"},
				Payload:    json.RawMessage(test.payload), DeclaredSecretJSONPointers: test.pointers,
			})
		})
	}
}

func assertCapturedPublishedDraftPrivacy(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage, draft workers.Draft) {
	t.Helper()
	home := t.TempDir()
	config.Env = []string{"HOME=" + home, "USERPROFILE=" + home}
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
	var published []workers.ProgressFragment
	publisher := workersessions.RuntimeProgressPublisher(func(context.Context, workersessions.RuntimeAttemptKey, workers.ProgressFragment, workers.ProgressPublisher) error {
		return workersessions.ErrRuntimeProgressUnsupervised
	}).ForRuntime(t.Context(), "privacy-runtime", func(fragment workers.ProgressFragment) {
		published = append(published, fragment)
	})
	publisher(workers.CanonicalDraftFragment("privacy-dispatch", draft))
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
	// The recording fixture stores the public map representation, whose JSON
	// field order differs from Draft's struct encoding. Compare retrieval to
	// those exact stored bytes rather than re-encoding the producer struct.
	data, err = json.Marshal(page.Events[position].Event.Payload)
	if err != nil {
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
		assertCapturedSecretsAbsent(t, string(encoded))
		if logs.Events[position].Event.Truncated != nil && *logs.Events[position].Event.Truncated {
			ref := assertCapturedPayloadMetadata(t, logs.Events[position].Event, data)
			assertCapturedPayloadRoundTrip(t, server, page.WorkerSessionId, ref, data)
			assertCapturedPublicPrivacy(t, string(data))
		} else {
			assertCapturedPublicPrivacy(t, string(encoded))
		}
		assertCapturedPrivateCursorError(t, server, page.WorkerSessionId)
		server.Close(t)
	}
}

func assertCapturedPublicPrivacy(t *testing.T, output string) {
	t.Helper()
	assertCapturedSecretsAbsent(t, output)
	for _, visible := range []string{"visible-tool", "visible-result", "redacted"} {
		if !strings.Contains(output, visible) {
			t.Fatalf("public captured read lost %s", visible)
		}
	}
}

func assertCapturedSecretsAbsent(t *testing.T, output string) {
	t.Helper()
	for _, secret := range []string{"classified-environment-token", "classified-tool-token", "DeclaredSecretJSONPointers"} {
		if strings.Contains(output, secret) {
			t.Fatalf("public captured read exposed planted fixture %s", secret)
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
