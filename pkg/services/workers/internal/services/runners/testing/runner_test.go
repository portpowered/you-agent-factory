package mockworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	runtimefixtures "github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	work "github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

func TestMockWorkerCommandRunner_DefaultAcceptIncludesConfiguredStopToken(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: NewEmptyMockWorkersConfig(),
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*workerconfig.FactoryWorkerConfig{
				"worker": {Type: workerconfig.WorkerTypeModel, StopToken: "COMPLETE"},
			},
		},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got := string(result.Stdout); got != "mock worker accepted\nCOMPLETE" {
		t.Fatalf("Stdout = %q, want default accepted output with stop token", got)
	}
}

func TestMockWorkerCommandRunner_DefaultAcceptUsesDecisionEnvelopeForStructuredWorkstation(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: NewEmptyMockWorkersConfig(),
		RuntimeConfig: staticRuntimeConfig{
			Workstations: map[string]*workerconfig.FactoryWorkstationConfig{
				"review": {Name: "review", OutcomeFormat: workerconfig.WorkstationOutcomeFormatDecisionEnvelope},
			},
		},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		Command:         "codex",
		WorkstationName: "review",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := string(result.Stdout); !strings.Contains(got, `\"decision\":\"ACCEPTED\"`) ||
		!strings.Contains(got, `\"output\":\"mock worker accepted\"`) {
		t.Fatalf("Stdout = %q, want provider-shaped accepted decision envelope", got)
	}
}

func TestMockWorkerCommandRunner_RejectConfigPreservesObservableOutput(t *testing.T) {
	exitCode := 7
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeReject,
			RejectConfig: &MockWorkerRejectConfig{
				Stdout:   "mock stdout",
				Stderr:   "mock stderr",
				ExitCode: &exitCode,
			},
		}}},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if string(result.Stdout) != "mock stdout" || string(result.Stderr) != "mock stderr" {
		t.Fatalf("result output = stdout %q stderr %q, want configured output", result.Stdout, result.Stderr)
	}
}

func TestMockWorkerCommandRunner_RejectConfigWithZeroExitCodeStillFails(t *testing.T) {
	exitCode := 0
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeReject,
			RejectConfig: &MockWorkerRejectConfig{
				Stdout:   "mock stdout",
				Stderr:   "mock stderr",
				ExitCode: &exitCode,
			},
		}}},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want defensive non-zero reject exit code", result.ExitCode)
	}
	if string(result.Stdout) != "mock stdout" || string(result.Stderr) != "mock stderr" {
		t.Fatalf("result output = stdout %q stderr %q, want configured output", result.Stdout, result.Stderr)
	}
}

func TestMockWorkerCommandRunner_UnmatchedDispatchPassthroughUsesNextRunner(t *testing.T) {
	next := &recordingCommandRunner{}
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{
			UnmatchedDispatchPolicy: MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    MockWorkerRunTypeReject,
				RejectConfig: &MockWorkerRejectConfig{
					Stderr: "matched reject",
				},
			}},
		},
		Next: next,
	}

	req := workerprocess.CommandRequest{
		WorkerType:      "other-worker",
		WorkstationName: "process",
	}
	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(next.requests) != 1 {
		t.Fatalf("next runner call count = %d, want 1 passthrough dispatch", len(next.requests))
	}
	got := next.requests[0]
	if got.WorkerType != req.WorkerType || got.WorkstationName != req.WorkstationName {
		t.Fatalf("next runner request = %#v, want worker %q workstation %q", got, req.WorkerType, req.WorkstationName)
	}
	if result.ExitCode != 0 || string(result.Stdout) != "passthrough" {
		t.Fatalf("result = %#v, want passthrough runner output", result)
	}
}

func TestMockWorkerCommandRunner_UnmatchedDispatchDefaultAcceptSkipsNextRunner(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{
			MockWorkers: []MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    MockWorkerRunTypeReject,
			}},
		},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "other-worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(result.Stdout) != defaultMockWorkerAcceptedOutput {
		t.Fatalf("Stdout = %q, want default accepted output", result.Stdout)
	}
}

func TestMockWorkerCommandRunner_RecordsUsageOnceForMatchedRunTypes(t *testing.T) {
	inputTokens, outputTokens := int64(10), int64(5)
	usage := &workers.MockWorkerUsageConfig{
		Provider:     "codex",
		Model:        "gpt-5-codex",
		InputTokens:  &inputTokens,
		OutputTokens: &outputTokens,
	}
	for _, runType := range []workers.MockWorkerRunType{
		workers.MockWorkerRunTypeAccept,
		workers.MockWorkerRunTypeReject,
		workers.MockWorkerRunTypeScript,
	} {
		t.Run(string(runType), func(t *testing.T) {
			capture := &workerexecution.MockWorkerUsageCapture{}
			ctx := workerexecution.WithMockWorkerUsageCapture(context.Background(), capture)
			entry := workers.MockWorkerConfig{WorkerName: "worker", RunType: runType, Usage: usage}
			if runType == workers.MockWorkerRunTypeScript {
				entry.ScriptConfig = &workers.MockWorkerScriptConfig{Command: "mock-script"}
			}
			runner := &MockWorkerCommandRunner{
				Config: &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{entry}},
				Next:   &recordingCommandRunner{},
			}
			if _, err := runner.Run(ctx, workerprocess.CommandRequest{WorkerType: "worker"}); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			got := capture.Usage()
			if got == nil || got.Provider != usage.Provider || got.Model != usage.Model ||
				got.InputTokens == nil || *got.InputTokens != *usage.InputTokens ||
				got.OutputTokens == nil || *got.OutputTokens != *usage.OutputTokens {
				t.Fatalf("captured usage = %#v, want %#v", got, usage)
			}
			*got.InputTokens = 999
			if *capture.Usage().InputTokens != *usage.InputTokens {
				t.Fatal("Usage returned a mutable capture-owned declaration")
			}
		})
	}
}

func TestMockWorkerCommandRunner_UnmatchedDispatchDoesNotRecordUsage(t *testing.T) {
	inputTokens := int64(10)
	capture := &workerexecution.MockWorkerUsageCapture{}
	runner := &MockWorkerCommandRunner{
		Config: &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "matched-worker",
			RunType:    workers.MockWorkerRunTypeAccept,
			Usage: &workers.MockWorkerUsageConfig{
				Provider: "codex", Model: "gpt-5", InputTokens: &inputTokens,
			},
		}}},
		Next: failCommandRunner{t: t},
	}
	ctx := workerexecution.WithMockWorkerUsageCapture(context.Background(), capture)
	if _, err := runner.Run(ctx, workerprocess.CommandRequest{WorkerType: "other-worker"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if capture.Usage() != nil {
		t.Fatalf("captured usage = %#v, want nil for unmatched dispatch", capture.Usage())
	}
}

func TestMockWorkerCommandRunner_SelectsByWorkerWorkstationAndInput(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{
			{
				WorkerName:      "worker",
				WorkstationName: "other",
				RunType:         MockWorkerRunTypeReject,
			},
			{
				WorkerName:      "worker",
				WorkstationName: "process",
				WorkInputs: []MockWorkInputSelector{{
					WorkID:    "work-1",
					WorkType:  "task",
					State:     "init",
					InputName: "work",
					TraceID:   "trace-1",
				}},
				RunType: MockWorkerRunTypeReject,
				RejectConfig: &MockWorkerRejectConfig{
					Stderr: "matched",
				},
			},
		}},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType:      "worker",
		WorkstationName: "process",
		Inputs: []workers.WorkInput{{
			Kind:       string(workers.DataTypeWork),
			State:      "init",
			InputNames: []string{"work"},
			WorkID:     "work-1",
			WorkTypeID: "task",
			Lineage:    workers.WorkLineage{TraceID: "trace-1"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(result.Stderr) != "matched" {
		t.Fatalf("Stderr = %q, want matched selector output", result.Stderr)
	}
}

func TestMockWorkerCommandRunner_NilConfigPassthroughUsesNextRunner(t *testing.T) {
	next := &recordingCommandRunner{}
	runner := &MockWorkerCommandRunner{Next: next}

	req := workerprocess.CommandRequest{WorkerType: "worker"}
	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(next.requests) != 1 {
		t.Fatalf("next runner call count = %d, want 1", len(next.requests))
	}
	if next.requests[0].WorkerType != req.WorkerType {
		t.Fatalf("next runner request worker = %q, want %q", next.requests[0].WorkerType, req.WorkerType)
	}
	if string(result.Stdout) != "passthrough" {
		t.Fatalf("Stdout = %q, want passthrough output", result.Stdout)
	}
}

func TestMockWorkerCommandRunner_ScriptConfigTransformsRequest(t *testing.T) {
	next := &inspectingCommandRunner{}
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeScript,
			ScriptConfig: &MockWorkerScriptConfig{
				Command:          "mock-script",
				Args:             []string{"--flag", "value"},
				Env:              map[string]string{"EXTRA": "overlay", "BASE": "override"},
				WorkingDirectory: "/tmp/mock-worker",
				Stdin:            "script-stdin",
				Timeout:          "50ms",
			},
		}}},
		Next: next,
	}

	_, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
		Command:    "ignored",
		Args:       []string{"ignored"},
		Env:        []string{"BASE=original", "KEEP=value"},
		Stdin:      []byte("upstream-stdin"),
		WorkDir:    "/original",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if next.req.Command != "mock-script" {
		t.Fatalf("Command = %q, want mock-script", next.req.Command)
	}
	if !reflect.DeepEqual(next.req.Args, []string{"--flag", "value"}) {
		t.Fatalf("Args = %#v, want script args", next.req.Args)
	}
	if !reflect.DeepEqual(next.req.Env, []string{
		"BASE=override", "KEEP=value",
		"YOU_MOCK_WORKER_ARGS_JSON=[\"ignored\"]", "YOU_MOCK_WORKER_COMMAND=ignored", "YOU_MOCK_WORKER_TYPE=worker",
		"EXTRA=overlay",
	}) {
		t.Fatalf("Env = %#v, want merged env preserving order with overlays", next.req.Env)
	}
	if string(next.req.Stdin) != "script-stdin" {
		t.Fatalf("Stdin = %q, want script stdin", next.req.Stdin)
	}
	if next.req.WorkDir != "/tmp/mock-worker" {
		t.Fatalf("WorkDir = %q, want /tmp/mock-worker", next.req.WorkDir)
	}
	if !next.hasDeadline {
		t.Fatal("context deadline missing, want timeout-derived deadline")
	}
}

func TestMockWorkerCommandRunner_ScriptConfigWithoutOverridesKeepsOriginalWorkdir(t *testing.T) {
	next := &inspectingCommandRunner{}
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeScript,
			ScriptConfig: &MockWorkerScriptConfig{
				Command: "mock-script",
				Timeout: "0s",
			},
		}}},
		Next: next,
	}

	_, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
		WorkDir:    "/original",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if next.req.WorkDir != "/original" {
		t.Fatalf("WorkDir = %q, want original workdir", next.req.WorkDir)
	}
	if next.hasDeadline {
		t.Fatal("context deadline present, want none for zero timeout")
	}
}

func TestMockWorkerCommandRunner_ScriptConfigMissingReturnsFailureResult(t *testing.T) {
	result, err := (&MockWorkerCommandRunner{}).runScript(context.Background(), workerprocess.CommandRequest{}, nil)
	if err != nil {
		t.Fatalf("runScript returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
	if string(result.Stderr) != "mock scriptConfig is required" {
		t.Fatalf("Stderr = %q, want missing config error", result.Stderr)
	}
}

func TestMockWorkerCommandRunner_ScriptConfigInvalidTimeoutReturnsFailureResult(t *testing.T) {
	result, err := (&MockWorkerCommandRunner{}).runScript(context.Background(), workerprocess.CommandRequest{}, &MockWorkerScriptConfig{
		Command: "mock-script",
		Timeout: "definitely-not-a-duration",
	})
	if err != nil {
		t.Fatalf("runScript returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
	if got := string(result.Stderr); got == "" || got == "mock scriptConfig is required" {
		t.Fatalf("Stderr = %q, want timeout validation error", got)
	}
}

func TestMockWorkerCommandRunner_RunNextFailsClosedWhenNextMissing(t *testing.T) {
	_, err := (&MockWorkerCommandRunner{}).runNext(context.Background(), workerprocess.CommandRequest{})
	if err == nil || !strings.Contains(err.Error(), "next command runner is required") {
		t.Fatalf("runNext error = %v, want required injected runner", err)
	}
}

func TestRejectResultNilConfigDefaultsToExitCodeOne(t *testing.T) {
	result := rejectResult(nil)
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestCommandRequestInputsProjectToMockInputTokens(t *testing.T) {
	if tokens := commandRequestInputTokens(workerprocess.CommandRequest{}); tokens != nil {
		t.Fatalf("empty projected tokens = %#v, want nil", tokens)
	}

	tokens := commandRequestInputTokens(workerprocess.CommandRequest{
		Inputs: []workers.WorkInput{
			{Kind: string(workers.DataTypeWork), State: "init", WorkID: "work-1", WorkTypeID: "task"},
			{Kind: string(workers.DataTypeWork), State: "ready", WorkID: "work-2", WorkTypeID: "task"},
		},
	})

	if len(tokens) != 2 {
		t.Fatalf("projected token count = %d, want 2", len(tokens))
	}
	if tokens[0].ID != "input-0" || tokens[0].State != "init" || tokens[1].State != "ready" {
		t.Fatalf("projected tokens = %#v, want ordered state facts", tokens)
	}
}

func TestMockWorkInputSelectorMatchesPublicInputsAndSerializedTokens(t *testing.T) {
	payload := []byte("payload")
	sum := sha256.Sum256(payload)
	input := workers.WorkInput{
		Kind:       string(workers.DataTypeWork),
		State:      "ready",
		InputNames: []string{"work"},
		WorkID:     "work-1",
		WorkTypeID: "task",
		Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: string(payload)}},
		Tags:       map[string]string{"channel": "chat"},
		Lineage:    workers.WorkLineage{TraceID: "trace-1"},
	}
	selector := MockWorkInputSelector{
		WorkID:      "work-1",
		WorkType:    "task",
		State:       "ready",
		InputName:   "work",
		TraceID:     "trace-1",
		Channel:     "chat",
		PayloadHash: "sha256:" + hex.EncodeToString(sum[:]),
	}

	if !mockWorkInputSelectorMatches(selector, []workers.WorkInput{
		{Kind: string(workers.DataTypeResource)},
		input,
	}, nil) {
		t.Fatal("selector should match the public work input after skipping a resource")
	}
	if mockWorkInputSelectorMatches(selector, []workers.WorkInput{{Kind: string(workers.DataTypeResource)}}, nil) {
		t.Fatal("selector matched a resource-only input list")
	}

	mismatches := []MockWorkInputSelector{
		{WorkID: "other"},
		{WorkType: "other"},
		{State: "other"},
		{InputName: "other"},
		{TraceID: "other"},
		{Channel: "other"},
		{PayloadHash: "sha256:other"},
	}
	for _, mismatch := range mismatches {
		if mockWorkInputSelectorMatches(mismatch, []workers.WorkInput{input}, nil) {
			t.Fatalf("selector %#v unexpectedly matched public work input", mismatch)
		}
	}

	rawTokens := []map[string]any{{
		"id":    "token-1",
		"state": "ready",
		"color": map[string]any{
			"work_id":      "work-1",
			"work_type_id": "task",
			"data_type":    string(workers.DataTypeWork),
			"trace_id":     "trace-1",
			"tags":         map[string]string{"channel": "chat"},
			"payload":      payload,
		},
	}}
	if !mockWorkInputSelectorMatches(selector, rawTokens, map[string][]string{"work": {"token-1"}}) {
		t.Fatal("selector should match serialized token input")
	}
	if mockWorkInputSelectorMatches(selector, func() {}, nil) {
		t.Fatal("selector matched an unencodable token input")
	}
}

func TestDecodeTokenRejectsMalformedBoundaryValues(t *testing.T) {
	if _, ok := decodeToken(func() {}); ok {
		t.Fatal("decodeToken accepted an unencodable value")
	}
	if _, ok := decodeToken("not a token"); ok {
		t.Fatal("decodeToken accepted a non-object JSON value")
	}
	if got := tokenState("not a token"); got != "" {
		t.Fatalf("tokenState = %q, want empty state for malformed token", got)
	}
}

func TestMockWorkInputSelectorMatchesSkipsResourcesAndChecksAllFields(t *testing.T) {
	payload := []byte("payload")
	sum := sha256.Sum256(payload)
	selector := MockWorkInputSelector{
		WorkID:      "work-1",
		WorkType:    "task",
		State:       "ready",
		InputName:   "work",
		TraceID:     "trace-1",
		Channel:     "chat",
		PayloadHash: "sha256:" + hex.EncodeToString(sum[:]),
	}
	tokens := []mockInputToken{
		{
			ID:    "resource-1",
			State: "ready",
			Color: struct {
				WorkID     string            `json:"work_id"`
				WorkTypeID string            `json:"work_type_id"`
				DataType   string            `json:"data_type"`
				TraceID    string            `json:"trace_id"`
				Tags       map[string]string `json:"tags"`
				Payload    []byte            `json:"payload"`
			}{DataType: string(workers.DataTypeResource)},
		},
		{
			ID:    "token-1",
			State: "ready",
			Color: struct {
				WorkID     string            `json:"work_id"`
				WorkTypeID string            `json:"work_type_id"`
				DataType   string            `json:"data_type"`
				TraceID    string            `json:"trace_id"`
				Tags       map[string]string `json:"tags"`
				Payload    []byte            `json:"payload"`
			}{
				DataType: string(workers.DataTypeWork), WorkID: "work-1", WorkTypeID: "task",
				TraceID: "trace-1", Tags: map[string]string{"channel": "chat"}, Payload: payload,
			},
		},
	}

	if !mockWorkInputSelectorMatches(selector, tokens, map[string][]string{"work": {"token-1"}}) {
		t.Fatal("selector should match non-resource token")
	}
	if mockWorkInputSelectorMatches(selector, tokens[:1], map[string][]string{"work": {"token-1"}}) {
		t.Fatal("selector matched resource token, want false")
	}
}

func TestSelectorHelpersCoverMismatchBranches(t *testing.T) {
	token := mockInputToken{
		ID: "token-1", State: "ready",
		Color: struct {
			WorkID     string            `json:"work_id"`
			WorkTypeID string            `json:"work_type_id"`
			DataType   string            `json:"data_type"`
			TraceID    string            `json:"trace_id"`
			Tags       map[string]string `json:"tags"`
			Payload    []byte            `json:"payload"`
		}{
			DataType: string(workers.DataTypeWork), WorkID: "work-1", WorkTypeID: "task",
			TraceID: "trace-1", Tags: map[string]string{"channel": "chat"}, Payload: []byte("payload"),
		},
	}

	if selectorMatchesToken(MockWorkInputSelector{WorkID: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong work id")
	}
	if selectorMatchesToken(MockWorkInputSelector{WorkType: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong work type")
	}
	if selectorMatchesToken(MockWorkInputSelector{State: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong state")
	}
	if selectorMatchesToken(MockWorkInputSelector{InputName: "work"}, token, map[string][]string{"work": {"other-token"}}) {
		t.Fatal("selector unexpectedly matched wrong input binding")
	}
	if selectorMatchesToken(MockWorkInputSelector{TraceID: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong trace id")
	}
	if selectorMatchesToken(MockWorkInputSelector{Channel: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong channel")
	}
	if selectorMatchesToken(MockWorkInputSelector{PayloadHash: "sha256:other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong payload hash")
	}
}

func TestHelperFunctions(t *testing.T) {
	if bindingContainsToken(nil, "", "token-1") {
		t.Fatal("bindingContainsToken matched empty name")
	}
	if bindingContainsToken(nil, "work", "") {
		t.Fatal("bindingContainsToken matched empty token id")
	}
	if bindingContainsToken(map[string][]string{"work": {"other"}}, "work", "token-1") {
		t.Fatal("bindingContainsToken matched missing token")
	}
	if tokenState(mockInputToken{State: ""}) != "" {
		t.Fatal("tokenState returned non-empty state for empty state fact")
	}
	if payloadHash(nil) != "" {
		t.Fatal("payloadHash returned non-empty hash for empty payload")
	}
	if got := payloadHash([]byte("payload")); got == "" || got == "payload" {
		t.Fatalf("payloadHash = %q, want sha256-prefixed digest", got)
	}
}

type failCommandRunner struct {
	t *testing.T
}

func (r failCommandRunner) Run(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.t.Fatal("next command runner should not be called")
	return workerprocess.CommandResult{}, nil
}

type recordingCommandRunner struct {
	requests []workerprocess.CommandRequest
}

func (r *recordingCommandRunner) Run(_ context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.requests = append(r.requests, req)
	return workerprocess.CommandResult{Stdout: []byte("passthrough")}, nil
}

type inspectingCommandRunner struct {
	req         workerprocess.CommandRequest
	hasDeadline bool
}

func (r *inspectingCommandRunner) Run(ctx context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.req = req
	_, r.hasDeadline = ctx.Deadline()
	return workerprocess.CommandResult{}, nil
}
