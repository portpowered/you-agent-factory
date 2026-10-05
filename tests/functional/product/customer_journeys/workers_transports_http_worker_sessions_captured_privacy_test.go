package customer_journeys_test

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
