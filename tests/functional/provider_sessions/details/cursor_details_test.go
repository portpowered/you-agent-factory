package details

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	acpfixture "github.com/portpowered/infinite-you/tests/functional/workers/invoke_continue/testdata/acpfixture"
)

// The API-owned Cursor journey crosses production capture/storage and a controlled
// ACP peer through root.BuildProcess. No Cursor executable or native database runs.
func TestCursorCapturedDetails(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-cursor")
	support.ClearSeedInputs(t, dir)
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: MODEL_WORKER\nmodel: functional-model\nmodelProvider: CURSOR\nexecutorProvider: CURSOR\nstopToken: COMPLETE\n---\nProcess public input.\n")
	host := startCapturedCursorHost(t, home, dir, false, nil)
	invokeCapturedCursor(t, host, home, dir, "direct")
	direct := awaitCapturedCursorDetail(t, host, "cursor-1")
	assertCapturedCursorDetail(t, direct, "cursor-1")
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	if opened.Session == nil || opened.Session.Id == "~default" {
		t.Fatal("explicit Factory Session missing")
	}
	name := "cursor-work"
	support.SubmitSessionWorkAt(t, host.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": "public input"}})
	support.WaitForSessionTerminalStatus(t, host.URL(), opened.Session.Id, 30*time.Second)
	factory := awaitCapturedCursorDetail(t, host, "cursor-2")
	assertCapturedCursorDetail(t, factory, "cursor-2")
	support.CloseFactorySessionAt(t, host.URL(), opened.Session.Id)
	for _, tuple := range [][2]string{{"cursor", "unknown"}, {"cursor", "native-only"}, {"codex", "cursor-1"}} {
		assertCapturedCursorFailure(t, host, home, tuple[0], tuple[1], http.StatusNotFound)
	}
	body := getAPIProviderSessionDetailErrorBody(t, host.URL(), "cursor", "session_id", " cursor-1 ", http.StatusBadRequest)
	assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeBADREQUEST, home)
	members := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, host.URL()+"/worker-sessions?history=all&scope=all")
	healthyID := ""
	for _, row := range members.Sessions {
		if row.WorkerSessionId != "direct" {
			healthyID = row.WorkerSessionId
		}
	}
	if healthyID == "" {
		t.Fatal("Factory Worker association missing")
	}
	host.Close(t)
	freshHome, freshDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-reload")
	support.ClearSeedInputs(t, freshDir)
	copyCapturedCodexStore(t, dir, freshDir)
	fresh := startCapturedCursorHost(t, freshHome, freshDir, false, nil)
	for id, original := range map[string]factoryapi.ProviderSessionDetailResponse{"cursor-1": direct, "cursor-2": factory} {
		if got := awaitCapturedCursorDetail(t, fresh, id); !reflect.DeepEqual(got, original) {
			t.Fatalf("stopped reload changed %s", id)
		}
	}
	otherHome, otherDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-other-profile")
	support.ClearSeedInputs(t, otherDir)
	other := startCapturedCursorHost(t, otherHome, otherDir, false, nil)
	assertCapturedCursorFailure(t, other, otherHome, "cursor", "cursor-1", http.StatusNotFound)
	assertCapturedCursorFaults(t, dir, healthyID)
}

func assertCapturedCursorFaults(t *testing.T, dir, healthyID string) {
	t.Helper()
	// Independent stopped-copy faults have separate selected profiles and real storage.
	for name, damage := range map[string]func([]byte) []byte{
		"missing terminal": func(data []byte) []byte {
			return bytes.ReplaceAll(bytes.ReplaceAll(data, []byte(`"phase":"COMPLETED"`), []byte(`"phase":"UPDATED"`)), []byte(`"status":"COMPLETED"`), []byte(`"status":"RUNNING"`))
		},
		"corrupt":   func(data []byte) []byte { return append(data, []byte("{broken}\n")...) },
		"torn":      func(data []byte) []byte { return append(data, []byte("{\"uncommitted\":")...) },
		"truncated": func(data []byte) []byte { return data[:len(data)-10] },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			faultHome, faultDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, name)
			support.ClearSeedInputs(t, faultDir)
			copyCapturedCodexStore(t, dir, faultDir)
			mutateCapturedCodexJournals(t, faultDir, damage)
			fault := startCapturedCursorHost(t, faultHome, faultDir, false, nil)
			assertCapturedCursorFailure(t, fault, faultHome, "cursor", "cursor-1", http.StatusInternalServerError)
		})
	}
	t.Run("healthy sibling", func(t *testing.T) {
		t.Parallel()
		faultHome, faultDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-sibling")
		support.ClearSeedInputs(t, faultDir)
		copyCapturedCodexStore(t, dir, faultDir)
		mutateCapturedCodexJournals(t, faultDir, func(data []byte) []byte {
			if bytes.Contains(data, []byte(`"workerSessionId":"direct"`)) {
				return append(data, []byte("{broken}\n")...)
			}
			return data
		})
		fault := startCapturedCursorHost(t, faultHome, faultDir, false, nil)
		assertCapturedCursorFailure(t, fault, faultHome, "cursor", "cursor-2", http.StatusInternalServerError)
		page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, fault.URL()+"/worker-sessions/"+healthyID+"/logs")
		if page.WorkerSessionId != healthyID || len(page.Events) == 0 {
			t.Fatal("healthy sibling lost captured logs")
		}
		response, err := http.Get(fault.URL() + "/worker-sessions/direct/logs")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var failure factoryapi.ErrorResponse
		if err := json.NewDecoder(response.Body).Decode(&failure); err != nil || response.StatusCode != http.StatusServiceUnavailable || failure.Code != factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGUNAVAILABLE {
			t.Fatalf("damaged ID outcome: %d %#v %v", response.StatusCode, failure, err)
		}
	})
	t.Run("configured redaction", func(t *testing.T) { t.Parallel(); assertCapturedCursorRedaction(t, dir) })
	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		faultHome, faultDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-read-fault")
		support.ClearSeedInputs(t, faultDir)
		copyCapturedCodexStore(t, dir, faultDir)
		fault := startCapturedCursorHost(t, faultHome, faultDir, false, func(string) ([]byte, error) { return nil, privateStorageFault() })
		assertCapturedCursorFailure(t, fault, faultHome, "cursor", "cursor-1", http.StatusInternalServerError)
	})
}

func TestCursorCapturedDetailsAmbiguity(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-ambiguity")
	support.ClearSeedInputs(t, dir)
	host := startCapturedCursorHost(t, home, dir, true, nil)
	invokeCapturedCursor(t, host, home, dir, "first")
	awaitCapturedCursorDetail(t, host, "cursor-1")
	invokeCapturedCursor(t, host, home, dir, "second")
	host.Close(t)
	freshHome, freshDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-ambiguous-reload")
	support.ClearSeedInputs(t, freshDir)
	copyCapturedCodexStore(t, dir, freshDir)
	fresh := startCapturedCursorHost(t, freshHome, freshDir, false, nil)
	assertCapturedCursorFailure(t, fresh, freshHome, "cursor", "cursor-1", http.StatusInternalServerError)
}

func startCapturedCursorHost(t *testing.T, home, dir string, shared bool, readFile func(string) ([]byte, error), empty ...bool) *support.FunctionalAPIServer {
	t.Helper()
	// A file blocks native root traversal. The planted marker must never be disclosed.
	if err := os.WriteFile(filepath.Join(home, ".cursor"), []byte("native-private-history"), 0600); err != nil {
		t.Fatal(err)
	}
	var sequence atomic.Int32
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: testutil.NewProviderCommandRunner(), RecordingReadFile: readFile,
			PlatformProcessCommandFactory: acpfixture.CommandFactory(), ProvidersExecutableLocator: cursorCapturedLocator{},
			ProvidersStdioPipeFactory: acpfixture.StdioFactory(func(_ string, reader io.Reader, writer io.Writer) {
				id := sequence.Add(1)
				if shared {
					id = 1
				}
				serveCapturedCursor(reader, writer, fmt.Sprintf("cursor-%d", id), len(empty) > 0 && empty[0])
			}),
		},
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, input.WorkingDirectory)
		},
	})
	t.Cleanup(func() { host.Close(t) })
	return host
}

type cursorCapturedLocator struct{}

func (cursorCapturedLocator) LookPath(file string) (string, error) { return file, nil }

const cursorCapturedConfig = `[{"id":"model","name":"Model","category":"model","type":"select","currentValue":"functional-model","options":[{"value":"functional-model","name":"Functional model"}]}]`

func serveCapturedCursor(reader io.Reader, writer io.Writer, id string, empty bool) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var call struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &call) != nil {
			return
		}
		result := json.RawMessage(`{}`)
		switch call.Method {
		case "initialize":
			result = json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{}}`)
		case "session/new":
			result = mustMarshalJSON(map[string]any{"sessionId": id, "configOptions": json.RawMessage(cursorCapturedConfig)})
		case "session/set_config_option":
			result = mustMarshalJSON(map[string]any{"configOptions": json.RawMessage(cursorCapturedConfig)})
		case "session/prompt":
			updates := []map[string]any{
				{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "captured Cursor answer COMPLETE"}},
				{"sessionUpdate": "tool_call", "toolCallId": "call", "title": "Read public fixture", "kind": "read", "status": "pending", "rawInput": map[string]string{"path": "public", "api_key": "cursor-secret-marker"}},
				{"sessionUpdate": "tool_call_update", "toolCallId": "call", "status": "completed", "rawOutput": "public result"},
				{"sessionUpdate": "usage_update", "used": 14, "size": 1000},
			}
			if empty {
				updates = []map[string]any{{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "empty fixture COMPLETE"}}}
			}
			for _, update := range updates {
				if json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": id, "update": update}}) != nil {
					return
				}
			}
			result = json.RawMessage(`{"stopReason":"end_turn"}`)
		}
		if len(call.ID) > 0 && json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result}) != nil {
			return
		}
	}
}

func invokeCapturedCursor(t *testing.T, host *support.FunctionalAPIServer, home, dir, id string) {
	t.Helper()
	document := map[string]any{"requestId": id + "-request", "workerSessionId": id, "execution": map[string]any{
		"workstationName": "direct", "workingDirectory": dir, "workerType": "direct-worker", "runnerId": "cursor", "executorProvider": "cursor", "modelProvider": "cursor", "model": "functional-model", "userMessage": "public input",
		"dispatch": map[string]any{"dispatchId": id + "-attempt", "workstationName": "direct", "workerType": "direct-worker"}}}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, mustMarshalJSON(document), 0600); err != nil {
		t.Fatal(err)
	}
	input := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
	input.Input.Env, input.Input.WorkingDirectory = []string{"HOME=" + home, "USERPROFILE=" + home}, dir
	if err := host.Execute(t, input.Input); err != nil {
		t.Fatalf("invoke: %v %s %s", err, input.Stdout(), input.Stderr())
	}
	var result struct {
		Accepted bool   `json:"accepted"`
		State    string `json:"state"`
	}
	if err := json.Unmarshal([]byte(input.Stdout()), &result); err != nil || !result.Accepted || result.State != "COMPLETED" {
		t.Fatalf("invoke did not join: %s", input.Stdout())
	}
}

func awaitCapturedCursorDetail(t *testing.T, host *support.FunctionalAPIServer, id string) factoryapi.ProviderSessionDetailResponse {
	t.Helper()
	// Capture commit follows joined execution asynchronously; observe only the public endpoint.
	detail, err := support.WaitForObservation(30*time.Second, func() (factoryapi.ProviderSessionDetailResponse, error) {
		response, err := http.Get(providerSessionDetailURL(host.URL(), "cursor", "session_id", id))
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

func assertCapturedCursorDetail(t *testing.T, detail factoryapi.ProviderSessionDetailResponse, id string) {
	t.Helper()
	assertProviderSessionDetailIdentity(t, detail, id, factoryapi.Cursor, factoryapi.LoadableProviderSessionKindSessionID)
	if len(detail.Transcript) != 3 || detail.Transcript[2].Text == nil || *detail.Transcript[2].Text != "captured Cursor answer COMPLETE" || len(detail.Parse.FunctionCalls) != 2 {
		t.Fatalf("captured facts lost: %#v", detail)
	}
	for i, entry := range detail.Transcript {
		if entry.Order != i+1 || entry.Timestamp == nil || entry.LineNumber != nil {
			t.Fatalf("invalid captured entry: %#v", entry)
		}
	}
	if detail.Source.RelativePath != "" || detail.Source.SizeBytes != 0 || detail.Source.ModifiedAt != nil || detail.Parse.LineCount != 0 || detail.Parse.TokenUsage == nil || detail.Parse.TokenUsage.TotalTokens == nil || *detail.Parse.TokenUsage.TotalTokens != 14 {
		t.Fatalf("usage/source facts: %#v", detail)
	}
}

func assertCapturedCursorFailure(t *testing.T, host *support.FunctionalAPIServer, home, provider, id string, status int) {
	t.Helper()
	body := getAPIProviderSessionDetailErrorBody(t, host.URL(), provider, "session_id", id, status)
	if status == http.StatusInternalServerError {
		assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeINTERNALERROR, home)
	} else {
		assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeNOTFOUND, home)
	}
}

// Like Codex, Cursor execution needs output. Clear message text only in a stopped
// test-owned copy to exercise an honest complete-ended-empty historical capture.
func TestCursorCapturedDetailsCompleteEmpty(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-empty")
	support.ClearSeedInputs(t, dir)
	host := startCapturedCursorHost(t, home, dir, false, nil, true)
	invokeCapturedCursor(t, host, home, dir, "empty")
	awaitCapturedCursorDetail(t, host, "cursor-1")
	host.Close(t)
	freshHome, freshDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-empty-reload")
	support.ClearSeedInputs(t, freshDir)
	copyCapturedCodexStore(t, dir, freshDir)
	mutateCapturedCodexJournals(t, freshDir, func(data []byte) []byte { return bytes.ReplaceAll(data, []byte("empty fixture COMPLETE"), nil) })
	fresh := startCapturedCursorHost(t, freshHome, freshDir, false, nil)
	detail := awaitCapturedCursorDetail(t, fresh, "cursor-1")
	if len(detail.Transcript) != 0 || detail.Parse.TokenUsage != nil || detail.Parse.FunctionCalls == nil || detail.Parse.ParseErrors == nil || detail.Parse.Reasoning == nil || detail.Parse.Turns == nil || detail.Parse.UnknownEvents == nil {
		t.Fatalf("complete empty capture: %#v", detail)
	}
}

// The existing declared-field redactor prepares a stopped fixture. This protects
// stored redaction through HTTP projection; provider classification is unchanged.
func assertCapturedCursorRedaction(t *testing.T, dir string) {
	t.Helper()

	home, target := t.TempDir(), support.ScaffoldSingleStepFactory(t, "cursor-redacted")
	support.ClearSeedInputs(t, target)
	copyCapturedCodexStore(t, dir, target)
	mutateCapturedCodexJournals(t, target, func(data []byte) []byte {
		lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
		for i, line := range lines {
			if bytes.Contains(line, []byte("cursor-secret-marker")) {
				safe, err := recordings.RedactDeclaredSecretText(recordings.RecordingRedactionRequest{Payload: line, Secrets: []recordings.RecordingSecret{{JSONPointer: "/record/Payload/payload/argumentsSummary/api_key", Provenance: recordings.RecordingSecretProvenanceDeclared}}})
				if err != nil {
					t.Fatal(err)
				}
				lines[i] = safe.Payload
			}
		}
		return append(bytes.Join(lines, []byte{'\n'}), '\n')
	})
	host := startCapturedCursorHost(t, home, target, false, nil)
	detail := awaitCapturedCursorDetail(t, host, "cursor-1")
	if bytes.Contains(mustMarshalJSON(detail), []byte("cursor-secret-marker")) || !bytes.Contains(mustMarshalJSON(detail), []byte("redacted")) {
		t.Fatal("configured redaction lost in projection")
	}
}
