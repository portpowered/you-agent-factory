package customer_journeys_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type committedTerminalRunner struct {
	slots map[string]*functionalWorkerGate
}

func (r *committedTerminalRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	model := copiedLedgerModelArgument(request.Args)
	slot := r.slots[model]
	if slot == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected terminal fixture model %q", model)
	}
	result, err := slot.Run(ctx, request)
	if strings.HasSuffix(model, "FAILED") && err == nil {
		return platformprocess.CommandResult{Stdout: []byte(`{"type":"error","message":"controlled failure"}` + "\n"), Stderr: []byte("controlled failure"), ExitCode: 1}, nil
	}
	if request.Command == "claude" && err == nil {
		result.Stdout = []byte(`{"type":"result","subtype":"success","is_error":false,"result":"functional worker completed. COMPLETE"}` + "\n")
	}
	return result, err
}

type committedTerminalCase struct {
	model, state, scope, dir, id string
	provider                     modelprovider.Provider
}

// One root and immutable edge shape serve parallel Direct and Factory attempts. The only
// sequential phase is joined reopen of their common store: recovery requires
// a sole writer. Provider commands are controlled; capture and reads are real.
func TestWorkerSessionCommittedTerminalParity(t *testing.T) {
	t.Parallel()
	runner, gates, cases, config := committedTerminalFixture(t)
	server := support.StartFunctionalAPIServer(t, config)
	before := make([]factoryapi.WorkerSessionObservation, len(cases))
	t.Run("terminal matrix", func(t *testing.T) {
		for index, cell := range cases {
			t.Run(cell.model, func(t *testing.T) {
				t.Parallel()
				id := cell.id
				if cell.scope == "" {
					admitCommittedTerminal(t, server, id, cell.model, string(cell.provider))
				} else {
					stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), cell.scope))
					name := cell.model
					work := support.SubmitSessionWorkAt(t, server.URL(), cell.scope, factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name}})
					target := waitForRouteCharacterizationAssociation(t, stream, support.StringPointerValue(work.WorkId))
					id = target.workerSessionID
					// Pause dispatch before finishing the physical attempt so existing
					// Factory retry policy cannot admit a second gated command.
					postLiveRuntimeLifecycleControl(t, server.URL(), cell.scope, factoryapi.FactorySessionLifecycleControlKindPause)
				}
				runner.slots[cell.model].waitStarted(t)
				if cell.state == "CANCELED" || cell.state == "TERMINATED" {
					action := "cancel"
					if cell.state == "TERMINATED" {
						action = "terminate"
					}
					response := postWorkerSessionControl(t, server.URL(), id, action)
					_ = response.Body.Close()
					if response.StatusCode != http.StatusOK {
						t.Fatalf("stop status=%d", response.StatusCode)
					}
					runner.slots[cell.model].waitCanceled(t)
				} else {
					close(gates[cell.model])
					runner.slots[cell.model].waitCompleted(t)
				}
				waitCapturedTerminal(t, server.URL(), id)
				before[index] = support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
			})
		}
	})
	if t.Failed() {
		return
	}
	for index, cell := range cases {
		before[index] = assertCommittedTerminalPublicParity(t, server, before[index].WorkerSessionId, cell.state)
		if runner.slots[cell.model].callCount() != 1 {
			t.Fatalf("%s admitted multiple physical executions", cell.model)
		}
	}
	server.Close(t)
	// Reopen only captured reads; don't reactivate the Factory boards or
	// accidentally turn their pending business Work into fresh executions.
	config.BeforeStart = nil
	reopened := support.StartFunctionalAPIServer(t, config)
	for index, cell := range cases {
		got := assertCommittedTerminalPublicParity(t, reopened, before[index].WorkerSessionId, cell.state)
		if !committedTerminalFactsEqual(got, before[index]) {
			t.Fatalf("restart changed committed terminal facts for %s: before=%+v after=%+v", cell.model, before[index], got)
		}
	}
	reopened.Close(t)
	t.Run("opening-only and owner-lost", assertCommittedTerminalPrefixFences)
}

func committedTerminalFixture(t *testing.T) (*committedTerminalRunner, map[string]chan struct{}, []committedTerminalCase, support.FunctionalAPIServerConfig) {
	t.Helper()
	states := []string{"COMPLETED", "FAILED", "CANCELED", "TERMINATED"}
	runner := &committedTerminalRunner{slots: make(map[string]*functionalWorkerGate)}
	gates := make(map[string]chan struct{})
	var cases []committedTerminalCase
	for _, origin := range []string{"direct", "factory"} {
		for _, provider := range []modelprovider.Provider{modelprovider.ProviderCodex, modelprovider.ProviderClaude} {
			for _, state := range states {
				model := origin + "-" + string(provider) + "-" + state
				cell := committedTerminalCase{model: model, state: state, id: "terminal-" + model, provider: provider}
				if origin == "factory" {
					cell.scope, cell.dir = uuid.NewString(), support.ScaffoldSingleStepFactory(t, model)
					support.ClearSeedInputs(t, cell.dir)
					support.WriteAgentConfig(t, cell.dir, "processor", support.BuildModelWorkerConfig(provider, model))
				}
				cases = append(cases, cell)
				gates[model] = make(chan struct{})
				runner.slots[model] = newFunctionalWorkerGate(gates[model])
			}
		}
	}
	dir := support.ScaffoldSingleStepFactory(t, "committed-terminal-parity")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + home, "LOCALAPPDATA=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home, "TEMP=" + home, "TMP=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
		BeforeStart: func(tb testing.TB, process support.Process, _ root.Input) {
			for _, cell := range cases {
				if cell.scope == "" {
					continue
				}
				_, err := process.(support.ApplicationProcess).FactorySessions().Start(tb.Context(), factorysessions.SessionStartRequest{
					SessionID: cell.scope, Mode: factorysessions.SessionOperationModeLive, FolderPath: cell.dir, ActivationOnly: true,
					RuntimeSelection: &factorysessions.SessionRuntimeSelection{
						Mode: factorysessions.SessionRuntimeModeService, SystemConfigHome: home,
						DefinitionSourcePath: filepath.Join(cell.dir, "factory.json"), ExecutionBaseDir: cell.dir, RuntimeInstanceID: uuid.NewString(),
						Recording: factorysessions.SessionRecordingSelection{RecordPath: filepath.Join(cell.dir, cell.scope+".json")},
					},
				})
				if err != nil {
					tb.Fatal(err)
				}
			}
		},
	}
	return runner, gates, cases, config
}

func assertCommittedTerminalPrefixFences(t *testing.T) {
	t.Helper()
	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	dir, home := support.ScaffoldSingleStepFactory(t, "terminal-prefix"), t.TempDir()
	config := support.FunctionalAPIServerConfig{FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + home, "LOCALAPPDATA=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home, "TEMP=" + home, "TMP=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)}}
	server := support.StartFunctionalAPIServer(t, config)
	admitCommittedTerminal(t, server, "prefix-worker", "test-model", "codex")
	runner.waitStarted(t)
	prefix := assertCapturedLogsCLIHTTPParity(t, server, "prefix-worker")
	assertCapturedActiveHead(t, server, prefix)
	active := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/prefix-worker")
	if prefix.Health != factoryapi.INCOMPLETE || active.EndedAt != nil || active.TokenUsage != nil || active.TerminalCause != nil {
		t.Fatalf("active opening invented terminal facts: %+v %+v", prefix, active)
	}
	response, err := http.Get(server.URL() + "/worker-sessions/prefix-worker/transcript")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("active transcript status=%d", response.StatusCode)
	}
	server.Close(t)
	// Keep only the actually committed opening in a test-owned copy. This
	// controlled former-owner fixture cannot retain the joined shutdown terminal.
	archiveRoot := t.TempDir()
	path, journal := copyCurrentCapturedJournal(t, config, archiveRoot)
	writeCapturedJournal(t, path, journal[:1])
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(archiveRoot)
	archived := support.StartFunctionalAPIServer(t, config)
	page := assertRestoredLogsCLIHTTPParity(t, archived, "prefix-worker")
	if page.Health != factoryapi.INCOMPLETE || page.NextToken == nil || len(page.Events) != 1 || !reflect.DeepEqual(page.Events, prefix.Events) {
		t.Fatalf("owner-lost opening changed: %+v", page)
	}
	assertCapturedActiveHead(t, archived, page)
	shown := assertTerminalReadParity(t, archived, "prefix-worker")
	assertOwnerLostPrefixFacts(t, shown)
	assertIncompleteCapturedReplay(t, archived, page)
}

func assertOwnerLostPrefixFacts(t *testing.T, shown factoryapi.WorkerSessionObservation) {
	t.Helper()
	if shown.ConfirmationState != factoryapi.UNCONFIRMED || shown.EndedAt != nil || shown.DurationMillis != nil || shown.TokenUsage != nil || shown.Revivable != nil && *shown.Revivable {
		t.Fatalf("owner-lost prefix invented facts or authority: %+v", shown)
	}
}

func committedTerminalFactsEqual(left, right factoryapi.WorkerSessionObservation) bool {
	return reflect.DeepEqual(left.Failure, right.Failure) && left.ConfirmationState == right.ConfirmationState &&
		reflect.DeepEqual(left.EndedAt, right.EndedAt) && reflect.DeepEqual(left.DurationMillis, right.DurationMillis) &&
		reflect.DeepEqual(left.TerminalCause, right.TerminalCause)
}

func admitCommittedTerminal(t *testing.T, server *support.FunctionalAPIServer, id, model, provider string) {
	t.Helper()
	payload := directWorkerSessionPayload(id+"-request", id, id+"-attempt")
	payload.Execution.Model = &model
	payload.Execution.ModelProvider, payload.Execution.RunnerId = &provider, &provider
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL()+"/worker-sessions", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("admission status=%d", response.StatusCode)
	}
}

func assertCommittedTerminalPublicParity(t *testing.T, server *support.FunctionalAPIServer, id, state string) factoryapi.WorkerSessionObservation {
	t.Helper()
	shown := assertTerminalReadParity(t, server, id)
	if string(shown.State) != state || shown.ConfirmationState != factoryapi.CONFIRMED || shown.EndedAt == nil || shown.DurationMillis == nil {
		t.Fatalf("unconfirmed committed terminal: %+v", shown)
	}
	if shown.Failure != nil && (shown.Failure.ProviderFailureKind != nil || shown.Failure.ProviderContinuationFailureKind != nil || shown.Failure.ProviderContinuationOutcome != nil) {
		t.Fatalf("invented uncommitted provider classification: %+v", shown.Failure)
	}
	if state == "FAILED" && shown.Failure == nil {
		t.Fatal("committed FAILED lost failure cause")
	}
	return shown
}

func assertTerminalReadParity(t *testing.T, server *support.FunctionalAPIServer, id string) factoryapi.WorkerSessionObservation {
	t.Helper()
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
	list := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions?history=all")
	assertTerminalListed(t, list, shown)
	for _, args := range [][]string{{"show", "--worker-session-id", id}, {"list", "--history", "all"}} {
		input := support.FakeInputs(t.Context(), append([]string{"you", "--server", server.URL(), "worker-sessions"}, append(args, "--output", "json")...))
		if err := server.Execute(t, input.Input); err != nil {
			t.Fatalf("CLI read: %v %s", err, input.Stderr())
		}
		if args[0] == "show" {
			var got factoryapi.WorkerSessionObservation
			if err := json.Unmarshal([]byte(input.Stdout()), &got); err != nil || !reflect.DeepEqual(got, shown) {
				t.Fatalf("CLI summary differs: %+v %v", got, err)
			}
		} else {
			var got factoryapi.ListWorkerSessionsResponse
			if err := json.Unmarshal([]byte(input.Stdout()), &got); err != nil {
				t.Fatal(err)
			}
			assertTerminalListed(t, got, shown)
		}
	}
	assertCommittedTerminalMCPParity(t, server, shown)
	return shown
}

func assertTerminalListed(t *testing.T, list factoryapi.ListWorkerSessionsResponse, shown factoryapi.WorkerSessionObservation) {
	t.Helper()
	for _, row := range list.Sessions {
		if row.WorkerSessionId == shown.WorkerSessionId {
			if !reflect.DeepEqual(row, shown) {
				t.Fatalf("LIST/READ differ: list=%+v read=%+v", row, shown)
			}
			return
		}
	}
	t.Fatalf("terminal %s absent from list", shown.WorkerSessionId)
}

func assertCommittedTerminalMCPParity(t *testing.T, server *support.FunctionalAPIServer, shown factoryapi.WorkerSessionObservation) {
	t.Helper()
	connection, ctx, cleanup := archivedReplayMCP(t, server)
	defer cleanup()
	for _, args := range []map[string]any{{"action": "LIST", "history": "all"}, {"action": "READ", "view": "summary", "workerSessionId": shown.WorkerSessionId}} {
		result, err := connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
		if err != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("MCP terminal read: %+v %v", result, err)
		}
		var envelope struct {
			Result struct {
				Session  factoryapi.WorkerSessionObservation   `json:"session"`
				Sessions []factoryapi.WorkerSessionObservation `json:"sessions"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
			t.Fatal(err)
		}
		if args["action"] == "LIST" {
			assertTerminalListed(t, factoryapi.ListWorkerSessionsResponse{Sessions: envelope.Result.Sessions}, shown)
		} else if !committedTerminalFactsEqual(envelope.Result.Session, shown) || envelope.Result.Session.State != shown.State || !reflect.DeepEqual(envelope.Result.Session.TokenUsage, shown.TokenUsage) {
			t.Fatalf("MCP committed facts differ: %+v HTTP=%+v", envelope.Result.Session, shown)
		}
	}
}
