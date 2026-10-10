package customer_commands_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const diagnosticSecretURL = "https://PRIVATE_USER:PRIVATE_PASS@example.test/provider?token=PRIVATE_QUERY#PRIVATE_FRAGMENT"

func testOutputPrivateUsefulInvocationDiagnostics(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		id        string
		flags     []string
		malformed bool
	}{
		{"D-NORMAL", nil, false},
		{"D-VERBOSE", []string{"--verbose"}, false},
		{"D-DEBUG", []string{"--debug"}, false},
		{"D-PRIMARY", []string{"--json", "--output", "primary"}, false},
		{"D-NDJSON", []string{"--verbose", "--json", "--output", "response-stream"}, false},
		{"D-QUIET", []string{"--quiet"}, false},
		{"D-MALFORMED", []string{"--debug", "--json"}, true},
	} {
		t.Run(cell.id, func(t *testing.T) {
			t.Parallel()
			assertPrivateInvocationFailure(t, cell.flags, cell.malformed)
		})
	}
	t.Run("D-PEER and D-RECOVERY", testPrivateDiagnosticPeerAndRecovery)
	// Reuse the public logical-failure witnesses for both existing structured
	// terminal contracts, and the owned-session conflict witnesses.
	t.Run("D-TERMINAL primary", func(t *testing.T) { t.Parallel(); testOutputCLIJSONFailureRemainsValidJSONCase2(t) })
	t.Run("D-TERMINAL response-stream", testOutputCLINDJSONFailureEndsWithOneTerminalResult)
	for index, flags := range [][]string{{"--quiet", "--json"}, {"--quiet", "--output", "primary"}} {
		t.Run(fmt.Sprintf("D-CONFLICT-%d", index), func(t *testing.T) {
			t.Parallel()
			assertConcurrentOutputConflict(t, 80+index, flags)
		})
	}
}

func assertPrivateInvocationFailure(t *testing.T, flags []string, malformed bool) *support.CapturedInputs {
	t.Helper()
	args := append([]string{"you", "run"}, flags...)
	args = append(args, "--named", goalFactoryName, "--no-record", "private diagnostic input")
	fixture, inputs, _ := newMachineOutputInputs(t, args, goalFactoryName)
	// Initialize this private profile before introducing the unsupported env
	// override. Use the original local resolution path, never remote routing.
	setup := support.FakeInputs(t.Context(), []string{"you", "init", "--provider", "codex", "--model", "gpt-5-codex"})
	setup.Input.Env = append([]string(nil), inputs.Input.Env...)
	setup.Input.WorkingDirectory = inputs.Input.WorkingDirectory
	if err := fixture.process.Execute(setup.Input); err != nil {
		t.Fatalf("initialize private profile: %v", err)
	}
	secret := diagnosticSecretURL
	wantURL := "https://example.test/provider"
	if malformed {
		secret = "https://PRIVATE_USER:PRIVATE_PASS@%zz/provider?token=PRIVATE_QUERY#PRIVATE_FRAGMENT"
		wantURL = "<unavailable>"
	}
	inputs.Input.Env = append(inputs.Input.Env, operatorsettings.EnvDefaultWorkerModelProvider+"="+secret)
	originalArgs := append([]string(nil), inputs.Input.Args...)
	originalEnv := append([]string(nil), inputs.Input.Env...)
	err := fixture.process.Execute(inputs.Input)
	invocation := assertPrivateInvocationFailureType(t, err, secret)
	assertPrivateInvocationFailureOutput(t, inputs, wantURL)
	if !reflect.DeepEqual(inputs.Input.Args, originalArgs) || !reflect.DeepEqual(inputs.Input.Env, originalEnv) || !strings.Contains(invocation.Message, secret) {
		t.Fatal("presentation mutated caller input or original error")
	}
	return inputs
}

func assertPrivateInvocationFailureType(t *testing.T, err error, secret string) *runcli.InvocationError {
	t.Helper()
	var invocation *runcli.InvocationError
	var resolution operatorsettings.ResolutionFailure
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) || !errors.As(err, &resolution) || resolution.Field != "workerModelProvider" || resolution.Message != secret || !errors.As(err, &invocation) || invocation.Code != runcli.InvocationErrorCodeFailed {
		t.Fatalf("want original typed unsupported provider failure; got %T: %v", err, err)
	}
	return invocation
}

func assertPrivateInvocationFailureOutput(t *testing.T, inputs *support.CapturedInputs, wantURL string) {
	t.Helper()
	stdout, stderr := inputs.Stdout(), inputs.Stderr()
	primary, trailing, _ := strings.Cut(stderr, "\n")
	response := decodeSingleJSONErrorResponse(t, primary)
	wantMessage := "operator effective resolution override is unsupported: " + wantURL + " (workerModelProvider)"
	if stdout != "" || response.Code != runcli.InvocationErrorCodeFailed || response.Family != factoryapi.ErrorFamilyInternalServerError || response.Message != wantMessage || strings.Contains(stdout+stderr, "PRIVATE_") {
		t.Fatalf("unsafe or incorrectly framed diagnostic: stdout=%q stderr=%q", stdout, stderr)
	}
	assertPrivateDiagnosticCauses(t, trailing)
	t.Logf("typed unsupported override; empty stdout; primary=%s; safe trailing cause lines=%d", primary, len(nonEmptyStdoutLines(trailing)))
}

func assertPrivateDiagnosticCauses(t *testing.T, trailing string) {
	t.Helper()
	seen := make(map[string]bool)
	for _, line := range nonEmptyStdoutLines(trailing) {
		if !(strings.HasPrefix(line, "cause[") || strings.HasPrefix(line, "debug: cause[")) || seen[line] {
			t.Fatalf("unexpected or duplicated trailing diagnostic %q", line)
		}
		seen[line] = true
	}
	if len(seen) == 0 {
		t.Fatal("startup failure lost useful safe cause context")
	}
}

func testPrivateDiagnosticPeerAndRecovery(t *testing.T) {
	t.Parallel()
	peer := newConcurrentOutputCall(t, 90, []string{"--quiet"})
	peer.start()
	select {
	case <-peer.runner.entered:
	case <-peer.done:
		t.Fatalf("peer ended before overlap: %v", peer.err)
	case <-peer.ctx.Done():
		t.Fatal(peer.ctx.Err())
	}
	failed := assertPrivateInvocationFailure(t, []string{"--verbose", "--json", "--output", "response-stream"}, false)
	select {
	case <-peer.done:
		t.Fatal("quiet peer ended before failed diagnostic rendered")
	default:
	}
	peer.runner.release()
	peer.join(t)
	assertConcurrentOutputSuccess(t, 0, peer, []*concurrentOutputCall{peer})
	assertInjectedOutputDiagnostics(t, peer)
	if strings.Contains(failed.Stdout()+failed.Stderr(), peer.marker) || strings.Contains(failed.Stderr(), peer.sessionID) || strings.Contains(peer.inputs.Stdout()+peer.inputs.Stderr(), "PRIVATE_") {
		t.Fatal("failure and quiet peer output/correlation crossed")
	}
	recovery := newConcurrentOutputCall(t, 91, []string{"--json", "--output", "primary"})
	recovery.runner.release()
	recovery.start()
	recovery.join(t)
	assertConcurrentOutputSuccess(t, 1, recovery, []*concurrentOutputCall{peer, recovery})
	assertInjectedOutputDiagnostics(t, recovery)
	t.Log("quiet peer remained live throughout failed rendering, then returned its exact raw result; next owned invocation completed on the shared root")
}

const (
	jsonGoalFactoryName          = "@you/goal"
	jsonWantInvocationResultText = "mock worker accepted"
)

// TestCLIJSONFailureRemainsValidJSON proves CLI JSON failures stay
// machine-parseable: pre-terminal errors emit exactly one stderr ErrorResponse
// followed by safe startup causes with empty stdout; terminal invocation failures end their stdout stream with
// a failed InvocationResponse plus one stderr ErrorResponse, and a later
// successful invocation recovers on the same shared root.
func testOutputCLIJSONFailureRemainsValidJSON(t *testing.T) {
	t.Parallel()
	t.Run("pre-terminal failure leaves stdout empty with one stderr ErrorResponse", testOutputCLIJSONFailureRemainsValidJSONCase1)

	t.Run("terminal failure emits failed InvocationResponse and one stderr ErrorResponse", testOutputCLIJSONFailureRemainsValidJSONCase2)

	t.Run("success recovers after terminal failure on the shared root", testOutputCLIJSONFailureRemainsValidJSONCase3)
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

// TestCLIInvalidInputsAreBadRequest proves malformed arguments and output
// selections remain customer-facing bad requests.
func testOutputCLIInvalidInputsAreBadRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		args                []string
		packagedFactoryName string
		wantCode            factoryapi.ErrorResponseCode
	}{
		{
			name:     "quiet and global JSON",
			args:     []string{"you", "--json", "run", "--quiet"},
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
			stdout, stderr, err := runMachineOutputValidationInvocation(t, test.args)
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
		})
	}
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
	return runMachineOutputInvocation(t, args, packagedFactoryName, providerRunner)
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

func testOutputCLIJSONFailureRemainsValidJSONCase1(t *testing.T) {
	t.Helper()

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
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	response := decodeSingleJSONErrorResponse(t, lines[0])
	if len(lines) < 2 || !strings.Contains(stderr, "named factory not found") {
		t.Fatalf("startup failure omitted its cause: %q", stderr)
	}
	for index, line := range lines[1:] {
		if !strings.HasPrefix(line, fmt.Sprintf("cause[%d]=", index)) {
			t.Fatalf("startup stderr contains unexpected framing: %q", stderr)
		}
	}
	if response.Code != factoryapi.ErrorResponseCode(runcli.InvocationErrorCodeFailed) ||
		response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("ErrorResponse = %#v", response)
	}

}

func testOutputCLIJSONFailureRemainsValidJSONCase2(t *testing.T) {
	t.Helper()

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

}

func testOutputCLIJSONFailureRemainsValidJSONCase3(t *testing.T) {
	t.Helper()

	stdout := runGoalSingleJSON(t)
	response := decodeSingleJSONInvocationResponse(t, stdout)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if got := invocationPrimaryResultText(t, response); got != jsonWantInvocationResultText {
		t.Fatalf("primaryResult = %q, want %q", got, jsonWantInvocationResultText)
	}
	assertPublicSingleJSONInvocationPayload(t, stdout, "stdout")

}
