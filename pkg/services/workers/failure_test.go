package workers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func TestParseMockWorkersConfigWithDiagnosticsPreservesKnownBehavior(t *testing.T) {
	t.Parallel()

	config, diagnostics, err := ParseMockWorkersConfigWithDiagnostics([]byte(`{
		"mockWorkers": [{
			"id": "reviewer",
			"runType": "accept",
			"futureEntry": {"secret": "do-not-log"}
		}],
		"futureTopLevel": true
	}`))
	if err != nil {
		t.Fatalf("ParseMockWorkersConfigWithDiagnostics() error = %v", err)
	}
	if config == nil || len(config.MockWorkers) != 1 || config.MockWorkers[0].ID != "reviewer" {
		t.Fatalf("config = %#v, want known mock-worker fields preserved", config)
	}
	wantPaths := []string{"$.futureTopLevel", "$.mockWorkers[0].futureEntry"}
	if got := diagnostics.Paths(); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("diagnostic paths = %#v, want %#v", got, wantPaths)
	}
	if strings.Contains(strings.Join(diagnostics.Paths(), "\n"), "secret") {
		t.Fatal("diagnostics retained an ignored field value")
	}
}

func TestParseMockWorkersConfigWithDiagnosticsRemainsStrictForKnownAndTrailingInput(t *testing.T) {
	t.Parallel()

	for name, payload := range map[string]string{
		"invalid run type":  `{"mockWorkers":[{"runType":"future"}]}`,
		"trailing document": `{"mockWorkers":[]} {"later":true}`,
		"malformed json":    `{"mockWorkers":[` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := ParseMockWorkersConfigWithDiagnostics([]byte(payload)); err == nil {
				t.Fatal("ParseMockWorkersConfigWithDiagnostics() error = nil, want reject")
			}
		})
	}
}

func TestMockWorkerCommandRunnerUsesGoalRoutingEnvelopePolicy(t *testing.T) {
	t.Parallel()

	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "goal-executor", WorkstationName: "execute-goal", RunType: MockWorkerRunTypeAccept,
		}}},
		OutputPolicy: OutputPolicy{Format: "decision-envelope", DecisionEnvelope: true, GoalRoutingDecisionEnvelope: true},
	}
	result, err := runner.Run(nil, CommandRequest{Command: "codex", WorkerType: "goal-executor", WorkstationName: "execute-goal"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	var decision struct {
		Decision string `json:"decision"`
		Output   string `json:"output"`
	}
	for _, line := range strings.Split(strings.TrimSpace(string(result.Stdout)), "\n") {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("mock provider event %q is invalid JSON: %v", line, err)
		}
		if event.Type == "item.completed" {
			if err := json.Unmarshal([]byte(event.Item.Text), &decision); err != nil {
				t.Fatalf("mock agent message %q is not a decision envelope: %v", event.Item.Text, err)
			}
			break
		}
	}
	if decision.Decision != "accepted" || decision.Output != defaultMockWorkerAcceptedOutput {
		t.Fatalf("mock decision envelope = %#v, want lower-case accepted routing label", decision)
	}
}

func TestMockWorkersConfigLoaderValidatesAndReadsDetachedConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := NewMockWorkersConfigLoader(nil); err == nil {
		t.Fatal("NewMockWorkersConfigLoader(nil) error = nil")
	}
	loader, err := NewMockWorkersConfigLoader(mockWorkersConfigReader(func(string) ([]byte, error) {
		return []byte(`{"mockWorkers":[{"workerName":"worker","runType":"script","scriptConfig":{"command":"echo","args":["ok"]}}]}`), nil
	}))
	if err != nil {
		t.Fatalf("NewMockWorkersConfigLoader() error = %v", err)
	}
	assertMockWorkersConfigLoaderResults(t, loader)
	futureLoader, err := NewMockWorkersConfigLoader(mockWorkersConfigReader(func(string) ([]byte, error) {
		return []byte(`{"mockWorkers":[{"id":"future-compatible","runType":"accept","futureEntry":{"secret":"do-not-log"}}],"futureTopLevel":true}`), nil
	}))
	if err != nil {
		t.Fatalf("NewMockWorkersConfigLoader(future fields) error = %v", err)
	}
	futureConfig, err := futureLoader("future.json")
	if err != nil || futureConfig == nil || len(futureConfig.MockWorkers) != 1 || futureConfig.MockWorkers[0].ID != "future-compatible" {
		t.Fatalf("future loader result = %#v, %v, want known fields preserved", futureConfig, err)
	}

	readErr := errors.New("read failed")
	failingLoader, err := NewMockWorkersConfigLoader(mockWorkersConfigReader(func(string) ([]byte, error) {
		return nil, readErr
	}))
	if err != nil {
		t.Fatalf("NewMockWorkersConfigLoader(failing) error = %v", err)
	}
	if _, err := failingLoader("config.json"); !errors.Is(err, readErr) {
		t.Fatalf("loader(read failure) error = %v, want %v", err, readErr)
	}
}

func assertMockWorkersConfigLoaderResults(t *testing.T, loader MockWorkersConfigLoader) {
	t.Helper()
	empty, err := loader("")
	if err != nil || empty == nil || len(empty.MockWorkers) != 0 {
		t.Fatalf("loader(empty) = %#v, %v, want empty config", empty, err)
	}
	loaded, err := loader("config.json")
	if err != nil || loaded == nil || loaded.MockWorkers[0].ScriptConfig.Command != "echo" {
		t.Fatalf("loader(config) = %#v, %v, want parsed script config", loaded, err)
	}
	for name, data := range map[string][]byte{
		"trailing JSON":    []byte(`{"mockWorkers":[]} {}`),
		"invalid run type": []byte(`{"mockWorkers":[{"runType":"unknown"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseMockWorkersConfig(data); err == nil {
				t.Fatal("ParseMockWorkersConfig() error = nil")
			}
		})
	}
	config, diagnostics, err := ParseMockWorkersConfigWithDiagnostics([]byte(`{"unexpected":true}`))
	if err != nil || config == nil {
		t.Fatalf("ParseMockWorkersConfigWithDiagnostics(unknown field) = %#v, %v, want accepted config", config, err)
	}
	if got := diagnostics.Paths(); len(got) != 1 || got[0] != "$.unexpected" {
		t.Fatalf("unknown field paths = %#v, want [$.unexpected]", got)
	}
}

func TestMockWorkerCommandRunnerExecutesScriptAndRejectRoutes(t *testing.T) {
	t.Parallel()

	var scriptRequest CommandRequest
	next := mockCommandRunnerFunc(func(ctx context.Context, request CommandRequest) (CommandResult, error) {
		scriptRequest = request
		if err := ctx.Err(); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Stdout: []byte("script output")}, nil
	})
	runner := &MockWorkerCommandRunner{Next: next, Config: &MockWorkersConfig{
		MockWorkers: []MockWorkerConfig{{
			RunType: MockWorkerRunTypeScript,
			ScriptConfig: &MockWorkerScriptConfig{
				Command:          "script-command",
				Args:             []string{"--flag"},
				Env:              map[string]string{"MOCK": "yes"},
				WorkingDirectory: "workdir",
				Stdin:            "stdin",
			},
		}},
	}}
	result, err := runner.Run(context.Background(), CommandRequest{
		Command: "original", Args: []string{"arg"}, Env: []string{"BASE=1"}, WorkerType: "worker",
	})
	if err != nil || string(result.Stdout) != "script output" || scriptRequest.Command != "script-command" {
		t.Fatalf("script result/request = %#v, %v / %#v", result, err, scriptRequest)
	}
	if scriptRequest.WorkDir != "workdir" || string(scriptRequest.Stdin) != "stdin" || !strings.Contains(strings.Join(scriptRequest.Env, "\n"), "YOU_MOCK_WORKER_COMMAND=original") {
		t.Fatalf("script request = %#v, want transformed working directory, stdin, and command metadata", scriptRequest)
	}
	assertMockWorkerRejectRoutes(t)
}

func assertMockWorkerRejectRoutes(t *testing.T) {
	t.Helper()
	reject := &MockWorkerCommandRunner{Config: &MockWorkersConfig{
		MockWorkers: []MockWorkerConfig{{
			RunType:      MockWorkerRunTypeReject,
			RejectConfig: &MockWorkerRejectConfig{Stdout: "rejected", ExitCode: func() *int { value := 7; return &value }()},
		}},
	}}
	if result, err := reject.Run(context.Background(), CommandRequest{Command: "other"}); err != nil || result.ExitCode != 7 || string(result.Stdout) != "rejected" {
		t.Fatalf("reject result = %#v, %v", result, err)
	}
	if result, err := reject.Run(context.Background(), CommandRequest{Command: "codex"}); err != nil || result.ExitCode != 0 || !strings.Contains(string(result.Stdout), "turn.failed") {
		t.Fatalf("codex reject result = %#v, %v", result, err)
	}
}

func TestMockWorkerCommandRunnerUsesUnmatchedPoliciesAndOutputFormats(t *testing.T) {
	t.Parallel()

	next := mockCommandRunnerFunc(func(_ context.Context, request CommandRequest) (CommandResult, error) {
		return CommandResult{Stdout: []byte("next:" + request.Command)}, nil
	})
	passthrough := &MockWorkerCommandRunner{
		Next: next,
		Config: &MockWorkersConfig{
			UnmatchedDispatchPolicy: MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers:             []MockWorkerConfig{{WorkerName: "other", RunType: MockWorkerRunTypeAccept}},
		},
	}
	result, err := passthrough.Run(context.Background(), CommandRequest{Command: "other-command", WorkerType: "worker"})
	if err != nil || string(result.Stdout) != "next:other-command" {
		t.Fatalf("passthrough unmatched result = %#v, %v", result, err)
	}
	if _, err := (&MockWorkerCommandRunner{}).Run(context.Background(), CommandRequest{}); err == nil {
		t.Fatal("nil next runner error = nil")
	}

	for _, test := range []struct {
		name   string
		policy OutputPolicy
		cmd    string
		want   string
	}{
		{name: "generic decision envelope", policy: OutputPolicy{Format: "decision-envelope"}, cmd: "claude", want: "ACCEPTED"},
		{name: "stop token", policy: OutputPolicy{StopToken: "<DONE>"}, cmd: "codex", want: "DONE"},
		{name: "default output", policy: OutputPolicy{}, cmd: "other", want: defaultMockWorkerAcceptedOutput},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &MockWorkerCommandRunner{
				Config:       &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{RunType: MockWorkerRunTypeAccept}}},
				OutputPolicy: test.policy,
			}
			result, err := runner.Run(context.Background(), CommandRequest{Command: test.cmd})
			if err != nil || !strings.Contains(string(result.Stdout), test.want) {
				t.Fatalf("Run() = %#v, %v, want %q", result, err, test.want)
			}
		})
	}
}

func TestMockWorkerCommandRunnerRejectsInvalidScriptConfiguration(t *testing.T) {
	t.Parallel()

	runner := &MockWorkerCommandRunner{Next: mockCommandRunnerFunc(func(context.Context, CommandRequest) (CommandResult, error) {
		return CommandResult{Stdout: []byte("unexpected")}, nil
	})}
	for _, test := range []struct {
		name   string
		config *MockWorkerScriptConfig
		want   string
	}{
		{name: "missing config", want: "scriptConfig is required"},
		{name: "invalid timeout", config: &MockWorkerScriptConfig{Command: "script", Timeout: "not-a-duration"}, want: "invalid mock script timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := runner.runScript(context.Background(), CommandRequest{}, test.config)
			if err != nil || result.ExitCode != 1 || !strings.Contains(string(result.Stderr), test.want) {
				t.Fatalf("runScript() = %#v, %v, want failure %q", result, err, test.want)
			}
		})
	}
}

type mockWorkersConfigReader func(string) ([]byte, error)

func (reader mockWorkersConfigReader) ReadFile(path string) ([]byte, error) { return reader(path) }

type mockCommandRunnerFunc func(context.Context, CommandRequest) (CommandResult, error)

func (runner mockCommandRunnerFunc) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	return runner(ctx, request)
}

func TestContainsStopToken_CompleteMarkerMustBeFinalNonEmptyLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "final marker", output: "finished\n<COMPLETE>\n", want: true},
		{name: "continue wins", output: "completion uses <COMPLETE>\n<CONTINUE>"},
		{name: "inline mention", output: "finished with <COMPLETE> in prose"},
		{name: "trailing prose", output: "<COMPLETE>\nadditional caveat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ContainsStopToken(tc.output, "<COMPLETE>"); got != tc.want {
				t.Fatalf("ContainsStopToken() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestFailureDecisionFromMetadata_ExpectedArtifactsIsTerminal(t *testing.T) {
	t.Parallel()
	decision := FailureDecisionFromMetadata(&WorkFailureMetadata{
		Family: WorkFailureFamilyTerminal,
		Type:   WorkFailureTypeExpectedArtifactsUnsatisfied,
	})
	if !decision.Terminal || decision.Retryable || decision.TriggersThrottlePause {
		t.Fatalf("decision = %#v, want terminal non-retryable artifact failure", decision)
	}
}

func TestContainsStopToken_LegacyTokensRetainSubstringSemantics(t *testing.T) {
	t.Parallel()
	if !ContainsStopToken("Work done. COMPLETE", "COMPLETE") {
		t.Fatal("plain legacy stop token did not match inline output")
	}
	if !ContainsStopToken("prefix <result>ACCEPTED</result> suffix", "<result>ACCEPTED</result>") {
		t.Fatal("structured legacy stop token did not match inline output")
	}
}

func TestNormalizeProviderExecutionError_PreservesBoundedProviderSessionInspectionCause(t *testing.T) {
	err := &providersessions.LookupError{
		Provider:  providersessions.ProviderCodex,
		SessionID: "rollout-resource-limit",
		Err:       errors.New("rollout contents must not be copied into the worker error"),
	}
	err.Err = errors.Join(providersessions.ErrResourceLimitExceeded, err.Err)

	normalized := NormalizeProviderExecutionError(err)
	if normalized == nil {
		t.Fatal("NormalizeProviderExecutionError() = nil, want a typed provider error")
	}
	if normalized.Type != WorkFailureTypeUnknown || normalized.Family != WorkFailureFamilyTerminal {
		t.Fatalf("normalized = %#v, want terminal unknown classification", normalized)
	}
	if normalized.Message != "provider session inspection reached its configured limit" {
		t.Fatalf("normalized.Message = %q, want bounded resource-limit cause", normalized.Message)
	}
	if !errors.Is(normalized, providersessions.ErrResourceLimitExceeded) {
		t.Fatal("normalized error did not retain the typed inspection-limit cause")
	}
	if normalized.Diagnostics == nil || normalized.Diagnostics.Provider == nil {
		t.Fatalf("normalized.Diagnostics = %#v, want provider diagnostics", normalized.Diagnostics)
	}
	if normalized.Continuation == nil || normalized.Continuation.ProviderSessionID != "rollout-resource-limit" {
		t.Fatalf("normalized.Continuation = %#v, want stable provider session identity", normalized.Continuation)
	}
	metadata := normalized.Diagnostics.Provider.ResponseMetadata
	if metadata[ProviderResponseMetadataFailureOperation] != "provider_session_ingestion" ||
		metadata[ProviderResponseMetadataFailureClassification] != "resource_limit" ||
		metadata["provider_session_id"] != "rollout-resource-limit" {
		t.Fatalf("inspection diagnostics = %#v, want stable operation/classification", metadata)
	}
	if normalized.Error() == "" || normalized.Error() == err.Error() {
		t.Fatalf("normalized.Error() = %q, want safe bounded text", normalized.Error())
	}
}

func TestNormalizeProviderExecutionError_ClassifiesBareProviderSessionCancellation(t *testing.T) {
	normalized := NormalizeProviderExecutionError(providersessions.ErrOperationCanceled)
	if normalized == nil {
		t.Fatal("NormalizeProviderExecutionError() = nil, want a typed cancellation error")
	}
	if normalized.Type != WorkFailureTypeUnknown || normalized.Family != WorkFailureFamilyTerminal {
		t.Fatalf("normalized = %#v, want terminal unknown cancellation classification", normalized)
	}
	if normalized.Message != "provider session inspection was canceled" {
		t.Fatalf("normalized.Message = %q, want safe cancellation cause", normalized.Message)
	}
	if normalized.Diagnostics == nil || normalized.Diagnostics.Provider == nil ||
		normalized.Diagnostics.Provider.ResponseMetadata[ProviderResponseMetadataFailureClassification] != "canceled" {
		t.Fatalf("normalized.Diagnostics = %#v, want canceled inspection classification", normalized.Diagnostics)
	}
}

func TestFailureDecisionFromMetadata_ClassifiesStructuredSchemaViolationAsTerminal(t *testing.T) {
	decision := FailureDecisionFromMetadata(&WorkFailureMetadata{
		Family: WorkFailureFamilyTerminal,
		Type:   WorkFailureTypeStructuredOutputSchemaViolation,
	})
	if decision.Retryable || !decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("FailureDecisionFromMetadata() = %#v, want terminal non-retryable non-throttle", decision)
	}
}
