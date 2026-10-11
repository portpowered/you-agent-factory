package customer_journeys_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
	if model == "FAILED" && err == nil {
		return platformprocess.CommandResult{Stdout: []byte(`{"type":"error","message":"controlled failure"}` + "\n"), Stderr: []byte("controlled failure"), ExitCode: 1}, nil
	}
	return result, err
}

// One root and immutable edge shape serve parallel Direct attempts. The only
// sequential phase is joined reopen of their common store: recovery requires
// a sole writer. Provider commands are controlled; capture and reads are real.
func TestWorkerSessionCommittedTerminalParity(t *testing.T) {
	t.Parallel()
	states := []string{"COMPLETED", "FAILED", "CANCELED", "TERMINATED"}
	runner := &committedTerminalRunner{slots: make(map[string]*functionalWorkerGate)}
	gates := make(map[string]chan struct{})
	for _, state := range states {
		gates[state] = make(chan struct{})
		runner.slots[state] = newFunctionalWorkerGate(gates[state])
	}
	dir := support.ScaffoldSingleStepFactory(t, "committed-terminal-parity")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + home, "LOCALAPPDATA=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home, "TEMP=" + home, "TMP=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	}
	server := support.StartFunctionalAPIServer(t, config)
	before := make([]factoryapi.WorkerSessionObservation, len(states))
	t.Run("direct", func(t *testing.T) {
		for index, state := range states {
			t.Run(state, func(t *testing.T) {
				t.Parallel()
				id := "terminal-" + state
				admitCommittedTerminal(t, server, id, state)
				runner.slots[state].waitStarted(t)
				if state == "CANCELED" || state == "TERMINATED" {
					action := "cancel"
					if state == "TERMINATED" {
						action = "terminate"
					}
					response := postWorkerSessionControl(t, server.URL(), id, action)
					_ = response.Body.Close()
					if response.StatusCode != http.StatusOK {
						t.Fatalf("stop status=%d", response.StatusCode)
					}
					runner.slots[state].waitCanceled(t)
				} else {
					close(gates[state])
					runner.slots[state].waitCompleted(t)
				}
				waitCapturedTerminal(t, server.URL(), id)
				before[index] = assertCommittedTerminalPublicParity(t, server, id, state)
			})
		}
	})
	server.Close(t)
	reopened := support.StartFunctionalAPIServer(t, config)
	for index, state := range states {
		got := assertCommittedTerminalPublicParity(t, reopened, "terminal-"+state, state)
		if !reflect.DeepEqual(got.Failure, before[index].Failure) || got.ConfirmationState != before[index].ConfirmationState || !reflect.DeepEqual(got.EndedAt, before[index].EndedAt) || !reflect.DeepEqual(got.DurationMillis, before[index].DurationMillis) || !reflect.DeepEqual(got.TerminalCause, before[index].TerminalCause) {
			t.Fatalf("restart changed committed terminal facts for %s: before=%+v after=%+v", state, before[index], got)
		}
	}
}

func admitCommittedTerminal(t *testing.T, server *support.FunctionalAPIServer, id, model string) {
	t.Helper()
	payload := directWorkerSessionPayload(id+"-request", id, id+"-attempt")
	payload.Execution.Model = &model
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
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
	if string(shown.State) != state || shown.ConfirmationState != factoryapi.CONFIRMED || shown.EndedAt == nil || shown.DurationMillis == nil {
		t.Fatalf("unconfirmed committed terminal: %+v", shown)
	}
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
		} else if !reflect.DeepEqual(envelope.Result.Session.Failure, shown.Failure) || envelope.Result.Session.ConfirmationState != shown.ConfirmationState || !reflect.DeepEqual(envelope.Result.Session.TerminalCause, shown.TerminalCause) {
			t.Fatalf("MCP committed facts differ: %+v HTTP=%+v", envelope.Result.Session, shown)
		}
	}
}
