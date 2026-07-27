package output_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	jsonGoalFactoryName          = "@you/goal"
	jsonWantInvocationResultText = "mock worker accepted"
)

// TestCLIJSONSuccessDecodesToPublicInvocationResult proves a successful CLI
// single-JSON run writes exactly one stdout InvocationResponse that decodes
// through the public contract with completed terminal status and no trailing JSON.
func TestCLIJSONSuccessDecodesToPublicInvocationResult(t *testing.T) {
	stdout := runGoalSingleJSON(t)

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var response factoryapi.InvocationResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stdout contains data after InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if got := invocationPrimaryResultText(t, response); got != jsonWantInvocationResultText {
		t.Fatalf("primaryResult = %q, want %q", got, jsonWantInvocationResultText)
	}
}

// TestCLIJSONFailureRemainsValidJSON proves CLI single-JSON failures stay
// machine-parseable: pre-terminal errors emit exactly one stderr ErrorResponse
// with empty stdout, and terminal invocation failures emit one failed
// InvocationResponse on stdout plus one stderr ErrorResponse.
func TestCLIJSONFailureRemainsValidJSON(t *testing.T) {
	t.Run("pre-terminal failure leaves stdout empty with one stderr ErrorResponse", func(t *testing.T) {
		stdout, stderr, err := runSingleJSONInvocation(t, []string{
			"you", "--json", "run", "--named", "@you/missing", "--no-record",
			"deterministic pre-session failure",
		}, "", nil)
		if err == nil {
			t.Fatal("Process.Execute error = nil, want pre-terminal invocation failure")
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty on pre-terminal failure", stdout)
		}
		response := decodeSingleJSONErrorResponse(t, stderr)
		if response.Code != factoryapi.ErrorResponseCode(runcli.InvocationErrorCodeFailed) ||
			response.Family != factoryapi.ErrorFamilyInternalServerError {
			t.Fatalf("ErrorResponse = %#v", response)
		}
	})

	t.Run("terminal failure emits failed InvocationResponse and one stderr ErrorResponse", func(t *testing.T) {
		stdout, stderr, err := runSingleJSONInvocation(t, []string{
			"you", "--json", "run", "--named", jsonGoalFactoryName, "--no-record",
			"deterministic terminal failure",
		}, jsonGoalFactoryName, rejectingGoalMockWorkers())
		if err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		response := decodeSingleJSONInvocationResponse(t, stdout)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusFailed)
		}
		if response.ErrorCode == nil || response.Message == nil {
			t.Fatalf("failed InvocationResponse lacks error detail: %#v", response)
		}
		errorResponse := decodeSingleJSONErrorResponse(t, stderr)
		if errorResponse.Code != factoryapi.ErrorResponseCode(*response.ErrorCode) ||
			errorResponse.Family != factoryapi.ErrorFamilyInternalServerError ||
			!strings.HasPrefix(errorResponse.Message, *response.Message) {
			t.Fatalf("ErrorResponse = %#v, want code %s and message prefix %q", errorResponse, *response.ErrorCode, *response.Message)
		}
	})
}

// TestCLIJSONContainsNoPrivateRuntimeFields proves CLI single-JSON success and
// terminal failure payloads decode through the public contract and omit private
// runtime vocabulary such as Petri internals, retired record shapes, and
// provider-session chunk fields.
func TestCLIJSONContainsNoPrivateRuntimeFields(t *testing.T) {
	t.Run("success stdout stays on public InvocationResponse fields", func(t *testing.T) {
		stdout := runGoalSingleJSON(t)
		assertPublicSingleJSONInvocationPayload(t, stdout, "stdout")
	})

	t.Run("terminal failure stdout and stderr stay on public contract fields", func(t *testing.T) {
		stdout, stderr, err := runSingleJSONInvocation(t, []string{
			"you", "--json", "run", "--named", jsonGoalFactoryName, "--no-record",
			"deterministic terminal failure",
		}, jsonGoalFactoryName, rejectingGoalMockWorkers())
		if err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		assertPublicSingleJSONInvocationPayload(t, stdout, "stdout")
		assertPublicSingleJSONErrorPayload(t, stderr, "stderr")
	})
}

// TestCLIJSONOutputSelectionFailsBeforeProductActivation proves invalid CLI
// output selectors fail before product activation with one stderr ErrorResponse
// and no product side effects.
func TestCLIJSONOutputSelectionFailsBeforeProductActivation(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode factoryapi.ErrorResponseCode
	}{
		{
			name:     "quiet and global JSON",
			args:     []string{"you", "--json", "run", "--quiet"},
			wantCode: runcli.InvocationOutputConflictCode,
		},
		{
			name:     "quiet and explicit output",
			args:     []string{"you", "run", "--quiet", "--output", "primary"},
			wantCode: runcli.InvocationOutputConflictCode,
		},
		{
			name:     "unsupported explicit output",
			args:     []string{"you", "run", "--output", "provider-chunks"},
			wantCode: runcli.InvocationOutputUnsupportedCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var effects atomic.Int32
			edges := serviceedges.Edges{
				APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
					effects.Add(1)
					return nil
				},
				BrowserOpener: func(context.Context, string) error {
					effects.Add(1)
					return nil
				},
				RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
					effects.Add(1)
				},
				FactorySessionIDGenerator: func() string {
					effects.Add(1)
					return "unexpected-session"
				},
			}
			inputs := support.FakeInputs(t.Context(), test.args)
			inputs.Input.WorkingDirectory = t.TempDir()

			err := support.BuildProcess(t, edges).Execute(inputs.Input)
			if err == nil {
				t.Fatal("Process.Execute error = nil, want output-selection failure")
			}
			if inputs.Stdout() != "" {
				t.Fatalf("stdout = %q, want empty", inputs.Stdout())
			}
			response := decodeSingleJSONErrorResponse(t, inputs.Stderr())
			if response.Code != test.wantCode || response.Family != factoryapi.ErrorFamilyBadRequest {
				t.Fatalf("ErrorResponse = %#v, want code %s", response, test.wantCode)
			}
			if effects.Load() != 0 {
				t.Fatalf("product effects = %d, want 0", effects.Load())
			}
		})
	}
}

func runGoalSingleJSON(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, jsonGoalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	args := []string{
		"you", "--json", "run", "--named", jsonGoalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		"deterministic single-json success contract",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
	return inputs.Stdout()
}

func runSingleJSONInvocation(
	t *testing.T,
	args []string,
	packagedFactoryName string,
	mockWorkers *workers.MockWorkersConfig,
) (stdout, stderr string, err error) {
	t.Helper()

	homeDir := t.TempDir()
	if packagedFactoryName != "" {
		support.InstallPackagedFactory(t, homeDir, packagedFactoryName)
	}
	if mockWorkers != nil {
		mockWorkersPath := support.WriteMockWorkersConfig(t, mockWorkers)
		outputFlag := len(args) - 1
		args = append(args[:outputFlag], append([]string{"--with-mock-workers", mockWorkersPath}, args[outputFlag:]...)...)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	err = support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
}

func decodeSingleJSONInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var response factoryapi.InvocationResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stdout contains data after InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	return response
}

func decodeSingleJSONErrorResponse(t *testing.T, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stderr))
	var response factoryapi.ErrorResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stderr contains data after ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	return response
}

var singleJSONForbiddenLiterals = []string{
	"FactoryResponseEvent", "response_event", "provider_session", "providerSession",
	"textDelta", "toolCallId", "toolCalls",
	`"recordType":"progress"`, `"recordType":"compaction"`, `"recordType":"primary_result"`,
	`"primary_result":`, `"marking":`, `"placeId":`, `"Petri":`,
}

func assertPublicSingleJSONInvocationPayload(t *testing.T, payload, stream string) {
	t.Helper()
	_ = decodeSingleJSONInvocationResponse(t, payload)
	assertNoPrivateRuntimeKeysInJSON(t, payload, stream)
}

func assertPublicSingleJSONErrorPayload(t *testing.T, payload, stream string) {
	t.Helper()
	_ = decodeSingleJSONErrorResponse(t, payload)
	assertNoPrivateRuntimeKeysInJSON(t, payload, stream)
}

func assertNoPrivateRuntimeKeysInJSON(t *testing.T, payload, stream string) {
	t.Helper()
	var raw any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("decode %s JSON: %v\n%s", stream, err, payload)
	}
	if key := singleJSONPrivateRuntimeKey(raw); key != "" {
		t.Fatalf("%s exposes private runtime field %q:\n%s", stream, key, payload)
	}
	for _, forbidden := range singleJSONForbiddenLiterals {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("%s contains forbidden private runtime literal %q:\n%s", stream, forbidden, payload)
		}
	}
}

func singleJSONPrivateRuntimeKey(value any) string {
	switch value := value.(type) {
	case map[string]any:
		for _, key := range []string{
			"diagnostics", "response", "providerSession", "provider_session",
			"textDelta", "toolCallId", "toolCalls",
			"marking", "placeId", "Petri",
			"progress", "compaction", "primary_result",
			"recordType", "factory_event", "response_event",
		} {
			if _, exists := value[key]; exists {
				return key
			}
		}
		for _, child := range value {
			if key := singleJSONPrivateRuntimeKey(child); key != "" {
				return key
			}
		}
	case []any:
		for _, child := range value {
			if key := singleJSONPrivateRuntimeKey(child); key != "" {
				return key
			}
		}
	}
	return ""
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text content part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text content: %v", err)
	}
	return part.Text
}
