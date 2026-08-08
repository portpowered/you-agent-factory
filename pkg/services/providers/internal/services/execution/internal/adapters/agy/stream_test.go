package agy_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type agyRecordedStreamCase struct {
	name             string
	trace            string
	wantResponse     string
	wantDurationMS   int64
	wantDurationText string
	wantTurns        string
	wantUsage        map[string]string
	wantSession      string
}

func TestAgyRootParsesRecordedStreamJSONResultsAndUsage(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(testutil.MustRepoRoot(t), "tests", "functional", "providers", "agy", "testdata")
	tests := []agyRecordedStreamCase{
		{
			name:             "simple text",
			trace:            "agy-trace-simple-text.stream.jsonl",
			wantResponse:     "TRACE_OK",
			wantDurationMS:   1206,
			wantDurationText: "1.2065237",
			wantTurns:        "1",
			wantUsage: map[string]string{
				"input_tokens": "18729", "output_tokens": "7", "thinking_tokens": "0",
				"cache_read_tokens": "0", "total_tokens": "18736",
			},
			wantSession: "0e1ecf6f-7716-4d26-bc56-7b428ab1ff1b",
		},
		{
			name:             "file read",
			trace:            "agy-trace-file-read.stream.jsonl",
			wantResponse:     "**alpha**: `3`",
			wantDurationMS:   2291,
			wantDurationText: "2.2910741",
			wantTurns:        "1",
			wantUsage: map[string]string{
				"input_tokens": "21482", "output_tokens": "314", "thinking_tokens": "0",
				"cache_read_tokens": "44825", "total_tokens": "21796",
			},
			wantSession: "19836153-fd43-4e47-aaad-c1e407c8bf3b",
		},
		{
			name:             "video watch",
			trace:            "agy-trace-video-watch.stream.jsonl",
			wantResponse:     "ambient atmospheric drone",
			wantDurationMS:   23691,
			wantDurationText: "23.6915733",
			wantTurns:        "1",
			wantUsage: map[string]string{
				"input_tokens": "89393", "output_tokens": "4622", "thinking_tokens": "2312",
				"cache_read_tokens": "252517", "total_tokens": "94015",
			},
			wantSession: "bcc76377-b371-4b9e-9856-19786ab7d0e2",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertAgyRecordedStreamCase(t, rootDir, test)
		})
	}
}

func assertAgyRecordedStreamCase(t *testing.T, rootDir string, test agyRecordedStreamCase) {
	t.Helper()

	trace, err := os.ReadFile(filepath.Join(rootDir, test.trace))
	if err != nil {
		t.Fatalf("read recorded trace %q: %v", test.trace, err)
	}
	result, observedSession := executeAgyRecordedStream(t, trace, test)
	assertAgyRecordedResult(t, result, observedSession, test)
}

func executeAgyRecordedStream(
	t *testing.T,
	trace []byte,
	test agyRecordedStreamCase,
) (providers.ExecuteResult, providers.SessionRef) {
	t.Helper()

	effect := agy.EffectFunc(func(
		_ context.Context,
		_ execution.ContinuationRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		for _, chunk := range splitAgyTrace(trace, 37) {
			if err := observe(chunk); err != nil {
				return agy.EffectResult{}, err
			}
		}
		return agy.EffectResult{DurationMillis: 999}, nil
	})

	var observedSession providers.SessionRef
	result, err := newAgyRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:  providers.IDAntigravity,
		AttemptID: "agy-recorded-" + strings.ReplaceAll(test.name, " ", "-"),
		SessionObserver: func(reference providers.SessionRef) {
			observedSession = reference
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return result, observedSession
}

func assertAgyRecordedResult(
	t *testing.T,
	result providers.ExecuteResult,
	observedSession providers.SessionRef,
	test agyRecordedStreamCase,
) {
	t.Helper()
	if !strings.Contains(result.Content, test.wantResponse) {
		t.Fatalf("response = %q, want %q", result.Content, test.wantResponse)
	}
	assertAgyRecordedSession(t, result, observedSession, test.wantSession)
	if result.Diagnostics == nil {
		t.Fatal("Diagnostics = nil, want recorded execution facts")
	}
	assertAgyRecordedDiagnostics(t, *result.Diagnostics, test)
}

func assertAgyRecordedSession(
	t *testing.T,
	result providers.ExecuteResult,
	observedSession providers.SessionRef,
	wantSession string,
) {
	t.Helper()
	if result.SessionRef == nil || result.SessionRef.ID != wantSession {
		t.Fatalf("SessionRef = %#v, want %q", result.SessionRef, wantSession)
	}
	if observedSession.ID != wantSession {
		t.Fatalf("observed session = %#v, want %q", observedSession, wantSession)
	}
}

func assertAgyRecordedDiagnostics(
	t *testing.T,
	diagnostics providers.ExecuteDiagnostics,
	test agyRecordedStreamCase,
) {
	t.Helper()
	metadata := diagnostics.Metadata
	if diagnostics.DurationMillis != test.wantDurationMS {
		t.Fatalf("DurationMillis = %d, want %d", diagnostics.DurationMillis, test.wantDurationMS)
	}
	if metadata["duration_seconds"] != test.wantDurationText ||
		metadata["num_turns"] != test.wantTurns || metadata["status"] != "SUCCESS" {
		t.Fatalf("result metadata = %#v, want duration/turn/status facts", metadata)
	}
	for key, want := range test.wantUsage {
		if got := metadata[key]; got != want {
			t.Fatalf("metadata[%q] = %q, want %q; metadata=%#v", key, got, want, metadata)
		}
	}
	if got := progressPhases(diagnostics.Progress); !strings.Contains(got, "run.completed|usage.updated|message.completed") {
		t.Fatalf("progress phases = %q, want terminal and usage facts", got)
	}
}

func TestAgyRootRejectsIncompleteOrMalformedStreamJSON(t *testing.T) {
	t.Parallel()

	validResult := `{"event":"result","result":{"status":"SUCCESS","response":"ok","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0}}}`
	tests := []struct {
		name   string
		stdout string
	}{
		{name: "empty", stdout: ""},
		{name: "malformed line", stdout: "{\"event\":\"init\"}\n{not-json}\n"},
		{name: "missing terminal result", stdout: "{\"event\":\"init\"}\n"},
		{name: "missing response", stdout: `{"event":"result","result":{"status":"SUCCESS","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0}}}`},
		{name: "truncated result", stdout: `{"event":"result","result":{"status":"SUCCESS","response":"ok"`},
		{name: "result is not terminal", stdout: validResult + "\n{\"event\":\"step_update\"}\n"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			effect := agy.EffectFunc(func(
				_ context.Context,
				_ execution.ContinuationRequest,
				observe func([]byte) error,
			) (agy.EffectResult, error) {
				if err := observe([]byte(test.stdout)); err != nil {
					return agy.EffectResult{}, err
				}
				return agy.EffectResult{}, nil
			})
			result, err := newAgyRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
				Provider:  providers.IDAntigravity,
				AttemptID: "agy-invalid-" + test.name,
			})
			if err == nil {
				t.Fatal("Execute() error = nil, want actionable parse failure")
			}
			if result.Content != "" {
				t.Fatalf("result content = %q, want empty result on parse failure", result.Content)
			}
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
			}
			if strings.TrimSpace(failure.Message) == "" {
				t.Fatalf("failure = %#v, want non-empty actionable message", failure)
			}
		})
	}
}

func TestAgyRootPreservesRecordedZeroUsageValues(t *testing.T) {
	t.Parallel()

	stream := []byte("{\"event\":\"init\",\"conversation_id\":\"agy-zero-usage\"}\n" +
		`{"event":"result","result":{"status":"SUCCESS","response":"zero usage is valid","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0}}}` + "\n")
	effect := agy.EffectFunc(func(
		_ context.Context,
		_ execution.ContinuationRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		return agy.EffectResult{}, observe(stream)
	})

	result, err := newAgyRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:  providers.IDAntigravity,
		AttemptID: "agy-zero-usage",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Diagnostics == nil {
		t.Fatal("Diagnostics = nil, want zero-valued usage facts")
	}
	for _, key := range []string{
		"duration_seconds", "num_turns", "input_tokens", "output_tokens",
		"thinking_tokens", "cache_read_tokens", "total_tokens",
	} {
		if got := result.Diagnostics.Metadata[key]; got != "0" {
			t.Fatalf("metadata[%q] = %q, want explicit zero", key, got)
		}
	}
}

func TestAgyRootReturnsRecordedStructuredJSONEnvelope(t *testing.T) {
	t.Parallel()

	trace := readAgyTrace(t, "agy-trace-structured.json")
	var envelope struct {
		StructuredOutput json.RawMessage `json:"structured_output"`
		JSONSchema       json.RawMessage `json:"json_schema"`
	}
	if err := json.Unmarshal(trace, &envelope); err != nil {
		t.Fatalf("decode structured trace: %v", err)
	}
	if len(envelope.JSONSchema) == 0 || len(envelope.StructuredOutput) == 0 {
		t.Fatal("structured trace did not contain schema and structured output")
	}

	effect := agy.EffectFunc(func(
		_ context.Context,
		_ execution.ContinuationRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		return agy.EffectResult{Metadata: map[string]string{"output_format": "json"}}, observe(trace)
	})
	result, err := newAgyRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:     providers.IDAntigravity,
		AttemptID:    "agy-recorded-structured",
		OutputSchema: string(envelope.JSONSchema),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(result.Content), &got); err != nil {
		t.Fatalf("decode provider content: %v", err)
	}
	if err := json.Unmarshal(envelope.StructuredOutput, &want); err != nil {
		t.Fatalf("decode recorded structured output: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("structured content = %#v, want %#v", got, want)
	}
}

func TestAgyRootReturnsRecordedClipQAStructuredVerdict(t *testing.T) {
	t.Parallel()

	trace := readAgyTrace(t, "agy-trace-clipqa-schema.stream.jsonl")
	var terminal struct {
		Result struct {
			StructuredOutput json.RawMessage `json:"structured_output"`
			JSONSchema       json.RawMessage `json:"json_schema"`
		} `json:"result"`
	}
	lastLine := strings.Split(strings.TrimSpace(string(trace)), "\n")
	if err := json.Unmarshal([]byte(lastLine[len(lastLine)-1]), &terminal); err != nil {
		t.Fatalf("decode clip-QA terminal trace: %v", err)
	}

	effect := agy.EffectFunc(func(
		_ context.Context,
		_ execution.ContinuationRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		for _, chunk := range splitAgyTrace(trace, 53) {
			if err := observe(chunk); err != nil {
				return agy.EffectResult{}, err
			}
		}
		return agy.EffectResult{Metadata: map[string]string{"output_format": "stream-json"}}, nil
	})
	result, err := newAgyRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:     providers.IDAntigravity,
		AttemptID:    "agy-recorded-clipqa",
		OutputSchema: string(terminal.Result.JSONSchema),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
		t.Fatalf("decode clip-QA output: %v", err)
	}
	if output["verdict"] != "pass" || output["audio_content"] != "noise" || output["unexpected_speech"] != false {
		t.Fatalf("clip-QA output = %#v, want pass/noise/no speech", output)
	}
	for _, key := range []string{
		"action_completed", "spec_deviations", "temporal_artifacts",
		"audio_content", "unexpected_speech", "verdict", "confidence",
	} {
		if _, ok := output[key]; !ok {
			t.Fatalf("clip-QA output missing %q: %#v", key, output)
		}
	}
}

func TestAgyRootRejectsRecordedMissingFileWithoutStructuredVerdict(t *testing.T) {
	t.Parallel()

	trace := readAgyTrace(t, "agy-trace-missing-file.stream.jsonl")
	schema := string(readRecordedClipQASchema(t))
	effect := agy.EffectFunc(func(
		_ context.Context,
		_ execution.ContinuationRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		return agy.EffectResult{Metadata: map[string]string{"output_format": "stream-json"}}, observe(trace)
	})
	result, err := newAgyRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:     providers.IDAntigravity,
		AttemptID:    "agy-recorded-missing-file",
		OutputSchema: schema,
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want structured contract failure")
	}
	if result.Content != "" {
		t.Fatalf("result content = %q, want empty on refusal", result.Content)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "structured_output") {
		t.Fatalf("error = %v, want actionable structured-output diagnostic", err)
	}
}

func TestAgyCommandEffectRequiresStructuredOutputAfterExitZero(t *testing.T) {
	t.Parallel()

	trace, err := os.ReadFile(filepath.Join(
		testutil.MustRepoRoot(t),
		"tests",
		"functional",
		"providers",
		"agy",
		"testdata",
		"agy-trace-simple-text.stream.jsonl",
	))
	if err != nil {
		t.Fatalf("read recorded trace: %v", err)
	}
	tests := []struct {
		name       string
		stdout     []byte
		wantResult bool
	}{
		{name: "recorded stream", stdout: trace, wantResult: true},
		{name: "plain exit zero", stdout: []byte("SUCCESS but no stream result"), wantResult: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: test.stdout})
			effect := agy.NewCommandEffect(workers.AdaptCommandRunner(runner))
			result, err := newAgyRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
				Provider:  providers.IDAntigravity,
				AttemptID: "agy-command-parse-" + strings.ReplaceAll(test.name, " ", "-"),
				Model:     "gemini-3.6-flash-low",
			})
			if test.wantResult {
				if err != nil || result.Content != "TRACE_OK" {
					t.Fatalf("Execute() = (%#v, %v), want recorded TRACE_OK", result, err)
				}
				return
			}
			if err == nil {
				t.Fatal("Execute() error = nil, want strict stream parse failure")
			}
			if result.Content != "" {
				t.Fatalf("result content = %q, want empty on strict parse failure", result.Content)
			}
		})
	}
}

func splitAgyTrace(value []byte, size int) [][]byte {
	var chunks [][]byte
	for len(value) > 0 {
		count := size
		if count > len(value) {
			count = len(value)
		}
		chunks = append(chunks, value[:count])
		value = value[count:]
	}
	return chunks
}

func progressPhases(progress []providers.ExecuteProgress) string {
	phases := make([]string, len(progress))
	for index, fact := range progress {
		phases[index] = fact.Phase
	}
	return strings.Join(phases, "|")
}

func readAgyTrace(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), "tests", "functional", "providers", "agy", "testdata", name)
	trace, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recorded trace %q: %v", name, err)
	}
	return trace
}

func readRecordedClipQASchema(t *testing.T) []byte {
	t.Helper()
	trace := readAgyTrace(t, "agy-trace-clipqa-schema.stream.jsonl")
	lines := strings.Split(strings.TrimSpace(string(trace)), "\n")
	var terminal struct {
		Result struct {
			JSONSchema json.RawMessage `json:"json_schema"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &terminal); err != nil {
		t.Fatalf("decode recorded clip-QA schema: %v", err)
	}
	return terminal.Result.JSONSchema
}
