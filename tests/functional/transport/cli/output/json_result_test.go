package output_test

import (
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	jsonGoalFactoryName          = "@you/goal"
	jsonWantInvocationResultText = "mock worker accepted"
)

// TestCLIJSONFailureRemainsValidJSON proves CLI JSON failures stay
// machine-parseable: pre-terminal errors emit exactly one stderr ErrorResponse
// with empty stdout, terminal invocation failures end their stdout stream with
// a failed InvocationResponse plus one stderr ErrorResponse, and a later
// successful invocation recovers on the same shared root.
func TestCLIJSONFailureRemainsValidJSON(t *testing.T) {
	t.Parallel()
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
			"you", "--json", "run", "--factory", jsonTerminalFailureFactoryPath(t), "--no-record",
			"deterministic terminal failure",
		}, "", nil)
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
		if string(*response.ErrorCode) != "INVOCATION_RUNTIME_FAILURE" ||
			!strings.HasPrefix(*response.Message, `invocation failed: work "work-1" reached failed state "goal:failed"`) {
			t.Fatalf("InvocationResponse = %#v, want the pinned terminal failure", response)
		}
		errorResponse := decodeSingleJSONErrorResponse(t, stderr)
		if errorResponse.Code != factoryapi.ErrorResponseCode(*response.ErrorCode) ||
			errorResponse.Family != factoryapi.ErrorFamilyInternalServerError ||
			!strings.HasPrefix(errorResponse.Message, *response.Message) {
			t.Fatalf("ErrorResponse = %#v, want code %s and message prefix %q", errorResponse, *response.ErrorCode, *response.Message)
		}
	})

	t.Run("success recovers after terminal failure on the shared root", func(t *testing.T) {
		fixture := sharedMachineOutputFixture(t)
		assertMachineOutputFixtureIdle(t, fixture)
		stdout := runGoalSingleJSON(t)
		response := decodeSingleJSONInvocationResponse(t, stdout)
		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
		}
		if got := invocationPrimaryResultText(t, response); got != jsonWantInvocationResultText {
			t.Fatalf("primaryResult = %q, want %q", got, jsonWantInvocationResultText)
		}
		assertPublicSingleJSONInvocationPayload(t, stdout, "stdout")
		assertMachineOutputFixtureIdle(t, fixture)
	})
}

func jsonTerminalFailureFactoryPath(t *testing.T) string {
	t.Helper()

	// Keep this failure in the logical runtime so worker teardown cannot replace
	// the terminal failure that the stdout/stderr contract is checking.
	return support.ScaffoldFactory(t, map[string]any{
		"name": "json-terminal-failure",
		"workTypes": []any{map[string]any{
			"name":             "goal",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workstations": []map[string]any{{
			"name":    "fail-goal",
			"type":    "LOGICAL_MOVE",
			"inputs":  []any{map[string]any{"workType": "goal", "state": "init"}},
			"outputs": []any{map[string]any{"workType": "goal", "state": "failed"}},
		}},
	})
}

// TestCLIInvalidInputsAreBadRequest proves the bounded pre-activation input
// batch keeps every malformed argument and output-selector contract distinct.
// The seven invocations share one root only while CLI parsing/output
// validation rejects them; each retains its own streams, working directory,
// provider route, and public error assertion.
func TestCLIInvalidInputsAreBadRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		args                []string
		packagedFactoryName string
		wantCode            factoryapi.ErrorResponseCode
	}{
		{
			name: "unknown argument in normal JSON mode",
			args: []string{
				"you", "--json", "run", "--named", "@you/plan-parallel", "--no-record",
				"--planer-model", "bad-model", "--to", "reproduce the typo",
			},
			packagedFactoryName: "@you/plan-parallel",
			wantCode:            factoryapi.ErrorResponseCode("INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT"),
		},
		{
			name: "unknown argument in quiet mode",
			args: []string{
				"you", "run", "--named", "@you/plan-parallel", "--quiet", "--no-record",
				"--planer-model", "bad-model", "--to", "reproduce the typo",
			},
			packagedFactoryName: "@you/plan-parallel",
			wantCode:            factoryapi.ErrorResponseCode("INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT"),
		},
		{
			name: "missing value before next invocation flag",
			args: []string{
				"you", "run", "--named", "@you/plan-parallel", "--quiet", "--no-record",
				"--planner-model", "--to", "request without a planner model",
			},
			packagedFactoryName: "@you/plan-parallel",
			wantCode:            factoryapi.ErrorResponseCode("INVOCATION_ARGUMENT_MISSING_VALUE"),
		},
		{
			name:     "missing value for run flag",
			args:     []string{"you", "run", "--quiet", "--factory", "--no-record"},
			wantCode: factoryapi.ErrorResponseCode("INVOCATION_ARGUMENT_MISSING_VALUE"),
		},
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

	fixture := sharedMachineOutputFixture(t)
	invocations := make([]machineOutputPreActivationInvocation, len(tests))
	providerRunners := make([]*testutil.ProviderCommandRunner, len(tests))
	for index, test := range tests {
		_, inputs := newMachineOutputInputs(t, test.args, test.packagedFactoryName)
		providerRunners[index] = testutil.NewProviderCommandRunner()
		invocations[index] = machineOutputPreActivationInvocation{
			inputs:         inputs,
			providerRunner: providerRunners[index],
		}
	}
	var effects atomic.Int32
	batch := fixture.executePreActivationBatch(invocations, &effects)
	t.Logf("pre-activation batch: cases=%d max_concurrent=%d provider_calls=0 product_effects=%d routes_after=0 active_after=0", len(invocations), batch.maxConcurrent, effects.Load())
	if batch.maxConcurrent < 2 {
		t.Fatalf("pre-activation batch concurrency = %d, want at least 2", batch.maxConcurrent)
	}
	assertMachineOutputFixtureIdle(t, fixture)
	if got := effects.Load(); got != 0 {
		t.Fatalf("product effects after pre-activation batch = %d, want 0", got)
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := invocations[index].inputs
			stdout, stderr, err := inputs.Stdout(), inputs.Stderr(), batch.errors[index]
			if err == nil {
				t.Fatalf("Process.Execute(%v) succeeded; stdout=%q stderr=%q", test.args, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty for usage failure", stdout)
			}
			response := decodeSingleJSONErrorResponse(t, stderr)
			if response.Code != test.wantCode || response.Family != factoryapi.ErrorFamilyBadRequest {
				t.Fatalf("ErrorResponse = %#v, want code %s and family BAD_REQUEST", response, test.wantCode)
			}
			if providerRunners[index].CallCount() != 0 {
				t.Fatalf("provider dispatch calls = %d, want 0", providerRunners[index].CallCount())
			}
		})
	}
}

// TestCLIJSONFailureContainsNoPrivateRuntimeFields proves terminal JSON failure
// payloads decode through the public contract and omit private runtime
// vocabulary such as Petri internals and provider-session chunk fields.
func TestCLIJSONFailureContainsNoPrivateRuntimeFields(t *testing.T) {
	t.Parallel()
	t.Run("terminal failure stdout and stderr stay on public contract fields", func(t *testing.T) {
		stdout, stderr, err := runSingleJSONInvocation(t, []string{
			"you", "--json", "run", "--named", jsonGoalFactoryName, "--no-record",
			"--executor-provider", "codex", "--executor-model", "gpt-5-codex",
			"deterministic terminal failure",
		}, jsonGoalFactoryName, machineOutputRejectedProviderRunner())
		if err == nil {
			t.Fatal("Process.Execute error = nil, want terminal invocation failure")
		}
		assertPublicSingleJSONInvocationPayload(t, stdout, "stdout")
		assertPublicSingleJSONErrorPayload(t, stderr, "stderr")
	})
}

func runGoalSingleJSON(t *testing.T) string {
	t.Helper()

	args := []string{
		"you", "--json", "run", "--named", jsonGoalFactoryName,
		"--executor-provider", "codex", "--executor-model", "gpt-5-codex",
		"--no-record",
		"deterministic single-json success contract",
	}
	stdout, stderr, err := runMachineOutputInvocation(
		t,
		args,
		jsonGoalFactoryName,
		machineOutputAcceptedProviderRunner(),
		nil,
	)
	if err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", stderr)
	}
	return stdout
}

func runSingleJSONInvocation(
	t *testing.T,
	args []string,
	packagedFactoryName string,
	providerRunner platformprocess.CommandRunner,
) (stdout, stderr string, err error) {
	t.Helper()
	return runMachineOutputInvocation(t, args, packagedFactoryName, providerRunner, nil)
}

func decodeSingleJSONInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()
	records := decodeNDJSONRecords(t, stdout)
	if len(records) == 0 {
		t.Fatal("stdout contains no JSON records")
	}
	record := records[len(records)-1]
	if record.RecordType != invocationResultType {
		t.Fatalf("terminal recordType = %q, want %q", record.RecordType, invocationResultType)
	}
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(record.Payload, &response); err != nil {
		t.Fatalf("decode InvocationResponse: %v\nstdout:\n%s", err, stdout)
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
	records := decodeNDJSONRecords(t, payload)
	if len(records) == 0 || records[len(records)-1].RecordType != invocationResultType {
		t.Fatalf("%s has no terminal invocation_result", stream)
	}
	_ = decodeSingleJSONInvocationResponse(t, payload)
	assertNoPrivateRuntimeKeysInJSON(t, string(records[len(records)-1].Payload), stream+" terminal response")
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
