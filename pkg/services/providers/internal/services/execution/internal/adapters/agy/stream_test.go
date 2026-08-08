package agy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
)

func TestAgyRootParsesRecordedStreamJSONResultsAndUsage(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(testutil.MustRepoRoot(t), "docs", "temp", "agy-traces")
	tests := []struct {
		name             string
		trace            string
		wantResponse     string
		wantDurationMS   int64
		wantDurationText string
		wantTurns        string
		wantUsage        map[string]string
		wantSession      string
	}{
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

			trace, err := os.ReadFile(filepath.Join(rootDir, test.trace))
			if err != nil {
				t.Fatalf("read recorded trace %q: %v", test.trace, err)
			}
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
			if !strings.Contains(result.Content, test.wantResponse) {
				t.Fatalf("response = %q, want %q", result.Content, test.wantResponse)
			}
			if result.SessionRef == nil || result.SessionRef.ID != test.wantSession {
				t.Fatalf("SessionRef = %#v, want %q", result.SessionRef, test.wantSession)
			}
			if observedSession.ID != test.wantSession {
				t.Fatalf("observed session = %#v, want %q", observedSession, test.wantSession)
			}
			if result.Diagnostics == nil {
				t.Fatal("Diagnostics = nil, want recorded execution facts")
			}
			if result.Diagnostics.DurationMillis != test.wantDurationMS {
				t.Fatalf("DurationMillis = %d, want %d", result.Diagnostics.DurationMillis, test.wantDurationMS)
			}
			metadata := result.Diagnostics.Metadata
			if metadata["duration_seconds"] != test.wantDurationText ||
				metadata["num_turns"] != test.wantTurns || metadata["status"] != "SUCCESS" {
				t.Fatalf("result metadata = %#v, want duration/turn/status facts", metadata)
			}
			for key, want := range test.wantUsage {
				if got := metadata[key]; got != want {
					t.Fatalf("metadata[%q] = %q, want %q; metadata=%#v", key, got, want, metadata)
				}
			}
			if got := progressPhases(result.Diagnostics.Progress); !strings.Contains(got, "run.completed|usage.updated|message.completed") {
				t.Fatalf("progress phases = %q, want terminal and usage facts", got)
			}
		})
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
