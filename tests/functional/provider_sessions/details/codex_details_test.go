package details

import (
	"context"
	"encoding/json"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const codexGoldenRolloutFile = "rollout.jsonl"

func codexSessionsRoot(homeDir string) string {
	return filepath.Join(homeDir, ".codex", "sessions")
}

func getCodexProviderSessionDetailErrorBody(
	t *testing.T,
	baseURL, sessionID string,
	wantStatus int,
) string {
	t.Helper()

	response, err := http.Get(codexProviderSessionDetailURL(baseURL, sessionID))
	if err != nil {
		t.Fatalf("GET provider session detail: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read provider session detail body: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf(
			"GET provider session detail status = %d, want %d: %s",
			response.StatusCode,
			wantStatus,
			strings.TrimSpace(string(body)),
		)
	}
	return string(body)
}

func assertCodexProviderSessionErrorBodySafe(t *testing.T, caseID, body, homeDir string) {
	t.Helper()
	if err := support.ValidateProviderSessionFixtureContent(caseID, "provider-session-detail", []byte(body)); err != nil {
		t.Fatalf("provider session response leaked forbidden material: %v\nbody=%s", err, body)
	}
	if strings.Contains(body, homeDir) || strings.Contains(body, codexSessionsRoot(homeDir)) {
		t.Fatalf("provider session response leaked configured host path: %s", body)
	}
}

func codexProviderSessionDetailURL(baseURL, sessionID string) string {
	query := url.Values{}
	query.Set("provider", string(factoryapi.Codex))
	query.Set("kind", string(factoryapi.LoadableProviderSessionKindSessionID))
	query.Set("id", sessionID)
	return strings.TrimSuffix(baseURL, "/") + "/provider-sessions/detail?" + query.Encode()
}

func writeCodexGoldenRolloutFixture(t *testing.T, root, sessionID, content string) {
	t.Helper()
	writeCodexGoldenRolloutFixtureAt(t, root, "2026/07/27", sessionID, content)
}

func writeCodexGoldenRolloutFixtureAt(t *testing.T, root, relativeDir, sessionID, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relativeDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir codex rollout fixture: %v", err)
	}
	path := filepath.Join(dir, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write codex rollout fixture: %v", err)
	}
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

// This API-owned matrix uses real capture/storage through root.BuildProcess and
// Process.Execute. One graph owns the compatible direct cohort. Reload/profile
// selection require separate graphs because their durable roots are immutable.
func TestCodexCapturedDetails(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-codex")
	support.ClearSeedInputs(t, dir)
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: MODEL_WORKER\nmodel: functional-model\nmodelProvider: CODEX\nexecutorProvider: CODEX\nstopToken: COMPLETE\n---\nProcess the public fixture.\n")
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: capturedCodexOutput("captured-direct")}, platformprocess.CommandResult{Stdout: capturedCodexOutput("captured-factory")})
	host := startCapturedCodexHost(t, home, dir, capturedCodexRunner{runner}, nil)
	invokeCapturedCodex(t, host, home, dir, "direct", "")
	detail := awaitCapturedCodexDetail(t, host, "captured-direct")
	assertCapturedCodexDetail(t, detail, "captured-direct")
	t.Run("F-CD2 Factory association", func(t *testing.T) {
		opened := support.OpenFactorySessionAt(t, host.URL(), dir)
		if opened.Session == nil || opened.Session.Id == "" || opened.Session.Id == "~default" {
			t.Fatalf("Factory session not explicit: %#v", opened)
		}
		session := opened.Session.Id
		defer support.CloseFactorySessionAt(t, host.URL(), session)
		name := "captured-factory-work"
		support.SubmitSessionWorkAt(t, host.URL(), session, factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": "public fixture"}})
		support.WaitForSessionTerminalStatus(t, host.URL(), session, 15*time.Second)
		assertCapturedCodexDetail(t, awaitCapturedCodexDetail(t, host, "captured-factory"), "captured-factory")
	})
	t.Run("F-CD4 unknown", func(t *testing.T) {
		for _, id := range []string{"unknown"} {
			body := getCodexProviderSessionDetailErrorBody(t, host.URL(), id, http.StatusNotFound)
			assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeNOTFOUND, home)
		}
	})
	// Joined shutdown precedes copying durable history into a new selected root.
	host.Close(t)
	freshHome, freshDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-reload")
	support.ClearSeedInputs(t, freshDir)
	copyCapturedCodexStore(t, dir, freshDir)
	fresh := startCapturedCodexHost(t, freshHome, freshDir, testutil.NewProviderCommandRunner(), nil)
	reloaded := support.GetJSON[factoryapi.ProviderSessionDetailResponse](t, codexProviderSessionDetailURL(fresh.URL(), "captured-direct"))
	if !reflect.DeepEqual(detail, reloaded) {
		t.Fatalf("F-CD3 reload changed captured detail: %#v / %#v", detail, reloaded)
	}
	otherHome, otherDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-other-profile")
	support.ClearSeedInputs(t, otherDir)
	other := startCapturedCodexHost(t, otherHome, otherDir, testutil.NewProviderCommandRunner(), nil)
	body := getCodexProviderSessionDetailErrorBody(t, other.URL(), "captured-direct", http.StatusNotFound)
	assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeNOTFOUND, otherHome)
}

func startCapturedCodexHost(t *testing.T, home, dir string, runner platformprocess.CommandRunner, readFile func(string) ([]byte, error)) *support.FunctionalAPIServer {
	t.Helper()
	// Codex native roots are inaccessible; capture storage remains real.
	if err := os.WriteFile(filepath.Join(home, ".codex"), []byte("inaccessible native root"), 0600); err != nil {
		t.Fatal(err)
	}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, RecordingReadFile: readFile,
			ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil }},
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, input.WorkingDirectory)
		},
	})
	t.Cleanup(func() { host.Close(t) })
	return host
}

func invokeCapturedCodex(t *testing.T, host *support.FunctionalAPIServer, home, dir, id, session string) {
	t.Helper()
	document := map[string]any{"requestId": id + "-request", "workerSessionId": id,
		"execution": map[string]any{"workstationName": "direct", "workingDirectory": dir,
			"factorySessionId": session, "workerType": "direct-worker", "runnerId": "codex",
			"executorProvider": "codex", "modelProvider": "codex", "model": "functional-model",
			"userMessage": "public input", "dispatch": map[string]any{"dispatchId": id + "-attempt", "workstationName": "direct", "workerType": "direct-worker"}}}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, mustMarshalJSON(document), 0600); err != nil {
		t.Fatal(err)
	}
	input := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
	input.Input.Env, input.Input.WorkingDirectory = []string{"HOME=" + home, "USERPROFILE=" + home}, dir
	if err := host.Execute(t, input.Input); err != nil {
		t.Fatalf("invoke: %v stdout=%s stderr=%s", err, input.Stdout(), input.Stderr())
	}
	var result struct {
		Accepted bool   `json:"accepted"`
		State    string `json:"state"`
	}
	if err := json.Unmarshal([]byte(input.Stdout()), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.State != "COMPLETED" {
		t.Fatalf("invoke did not join completion: %s", input.Stdout())
	}
}

func capturedCodexOutput(id string) []byte {
	return []byte(`{"type":"thread.started","thread_id":"` + id + `"}
{"type":"item.completed","item":{"id":"answer","type":"agent_message","text":"captured answer COMPLETE"}}
{"type":"item.started","item":{"id":"tool","type":"command_execution","command":"inspect public fixture"}}
{"type":"item.updated","item":{"id":"tool","type":"command_execution","aggregated_output":"public result"}}
{"type":"item.completed","item":{"id":"tool","type":"command_execution","command":"inspect public fixture","aggregated_output":"public result","exit_code":0}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":3,"output_tokens":4}}
`)
}

func awaitCapturedCodexDetail(t *testing.T, host *support.FunctionalAPIServer, id string) factoryapi.ProviderSessionDetailResponse {
	t.Helper()
	// Capture commits asynchronously after execution joins; the public endpoint
	// is the only observer, so poll its completion instead of sleeping or probing internals.
	detail, err := support.WaitForObservation(15*time.Second, func() (factoryapi.ProviderSessionDetailResponse, error) {
		response, err := http.Get(codexProviderSessionDetailURL(host.URL(), id))
		if err != nil {
			return factoryapi.ProviderSessionDetailResponse{}, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return factoryapi.ProviderSessionDetailResponse{}, nil
		}
		var detail factoryapi.ProviderSessionDetailResponse
		err = json.NewDecoder(response.Body).Decode(&detail)
		return detail, err
	}, func(detail factoryapi.ProviderSessionDetailResponse) bool { return detail.ProviderSession.Id == id })
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

func assertCapturedCodexDetail(t *testing.T, detail factoryapi.ProviderSessionDetailResponse, id string) {
	t.Helper()
	assertProviderSessionDetailIdentity(t, detail, id, factoryapi.Codex, factoryapi.LoadableProviderSessionKindSessionID)
	if len(detail.Transcript) != 3 || detail.Transcript[2].Text == nil || *detail.Transcript[2].Text != "captured answer COMPLETE" {
		t.Fatalf("captured message/tools lost: %#v", detail.Transcript)
	}
	for i, entry := range detail.Transcript {
		if entry.Order != i+1 || entry.Timestamp == nil || entry.LineNumber != nil {
			t.Fatalf("captured entry facts fabricated/lost: %#v", entry)
		}
	}
	if detail.Source.RelativePath != "" || detail.Source.SizeBytes != 0 || detail.Source.ModifiedAt != nil || detail.Parse.LineCount != 0 {
		t.Fatalf("uncaptured native facts fabricated: %#v", detail)
	}
	usage := detail.Parse.TokenUsage
	if usage == nil || usage.InputTokens == nil || *usage.InputTokens != 10 || usage.OutputTokens == nil || *usage.OutputTokens != 4 || usage.CachedInputTokens == nil || *usage.CachedInputTokens != 3 {
		t.Fatalf("captured usage lost: %#v", usage)
	}
	if len(detail.Parse.FunctionCalls) != 2 || detail.Transcript[0].Name == nil || detail.Transcript[1].Output == nil {
		t.Fatalf("captured tool facts lost: %#v", detail)
	}
}

func assertCapturedCodexFailure(t *testing.T, body string, code factoryapi.ErrorResponseCode, home string) {
	t.Helper()
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(body), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != code {
		t.Fatalf("error = %#v, want %s", failure, code)
	}
	for _, forbidden := range []string{`"transcript"`, `"providerSession"`, "native-private-history", "captured answer", home} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("failure leaked content: %s", body)
		}
	}
}

func copyCapturedCodexStore(t *testing.T, source, target string) {
	t.Helper()
	source = filepath.Join(source, ".you-agent-factory", "worker-recordings")
	target = filepath.Join(target, ".you-agent-factory", "worker-recordings")
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Streaming is the provider command effect that records progress observations.
// The queued command result remains the sole controlled external dependency.
type capturedCodexRunner struct {
	*testutil.ProviderCommandRunner
}

func (runner capturedCodexRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}
