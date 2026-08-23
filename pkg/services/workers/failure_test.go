package workers

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func TestParseMockWorkersConfigWithDiagnosticsPreservesKnownBehavior(t *testing.T) {
	t.Parallel()

	config, diagnostics, err := (MockWorkersConfigCodec{}).ParseWithDiagnostics([]byte(`{
		"mockWorkers": [{
			"id": "reviewer",
			"runType": "accept",
			"futureEntry": {"secret": "do-not-log"}
		}],
		"futureTopLevel": true
	}`))
	if err != nil {
		t.Fatalf("ParseWithDiagnostics() error = %v", err)
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
			if _, _, err := (MockWorkersConfigCodec{}).ParseWithDiagnostics([]byte(payload)); err == nil {
				t.Fatal("ParseWithDiagnostics() error = nil, want reject")
			}
		})
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
	config, diagnostics, err := (MockWorkersConfigCodec{}).ParseWithDiagnostics([]byte(`{"unexpected":true}`))
	if err != nil || config == nil {
		t.Fatalf("ParseWithDiagnostics(unknown field) = %#v, %v, want accepted config", config, err)
	}
	if got := diagnostics.Paths(); len(got) != 1 || got[0] != "$.unexpected" {
		t.Fatalf("unknown field paths = %#v, want [$.unexpected]", got)
	}
}

type mockWorkersConfigReader func(string) ([]byte, error)

func (reader mockWorkersConfigReader) ReadFile(path string) ([]byte, error) { return reader(path) }

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
