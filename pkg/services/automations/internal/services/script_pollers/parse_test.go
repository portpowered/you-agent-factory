package script_pollers_test

import (
	"encoding/json"
	"strings"
	"testing"

	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
)

func TestParseScriptPollerOutput_RejectsMalformedStdoutShapes(t *testing.T) {
	t.Parallel()

	rawEventJSON, err := json.Marshal(map[string]any{
		"events": []map[string]any{{
			"type": "WORK_REQUEST",
		}},
	})
	if err != nil {
		t.Fatalf("marshal raw event payload: %v", err)
	}

	tests := []struct {
		name             string
		stdout           []byte
		wantHasOutput    bool
		wantErrSubstring string
	}{
		{
			name:             "non-json stdout",
			stdout:           []byte("submitted work\n"),
			wantHasOutput:    true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "malformed work request json in request envelope",
			stdout:           []byte(`{"request":{"requestId":`),
			wantHasOutput:    true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "malformed bare work request json",
			stdout:           []byte(`{"requestId":"x","type":`),
			wantHasOutput:    true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "unsupported raw factory events",
			stdout:           rawEventJSON,
			wantHasOutput:    true,
			wantErrSubstring: "unsupported raw factory events",
		},
		{
			name:             "mixed request and submissions",
			stdout:           []byte(`{"request":{"requestId":"a","type":"FACTORY_REQUEST_BATCH","works":[]},"submissions":[]}`),
			wantHasOutput:    true,
			wantErrSubstring: "either request or submissions",
		},
		{
			name:             "unsupported work request type",
			stdout:           []byte(`{"requestId":"x","type":"UNSUPPORTED","works":[]}`),
			wantHasOutput:    true,
			wantErrSubstring: "unsupported work request type",
		},
		{
			name:             "missing request id",
			stdout:           []byte(`{"requestId":"","type":"FACTORY_REQUEST_BATCH","works":[{"name":"w","workTypeName":"task"}]}`),
			wantHasOutput:    true,
			wantErrSubstring: "requestId",
		},
		{
			name:             "malformed submissions decode",
			stdout:           []byte(`{"submissions":[{"requestId":`),
			wantHasOutput:    true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "empty submissions array",
			stdout:           []byte(`{"submissions":[]}`),
			wantHasOutput:    true,
			wantErrSubstring: "submissions must contain at least one item",
		},
		{
			name:             "submissions missing shared request id",
			stdout:           []byte(`{"submissions":[{"requestId":"","workId":"w1","name":"w","workTypeName":"task"}]}`),
			wantHasOutput:    true,
			wantErrSubstring: "requestId",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, hasOutput, parseErr := scriptpollers.ParseScriptPollerOutput(tc.stdout)
			if hasOutput != tc.wantHasOutput {
				t.Fatalf("hasOutput = %v, want %v", hasOutput, tc.wantHasOutput)
			}
			if parseErr == nil {
				t.Fatal("expected malformed-output parse error")
			}
			if !strings.Contains(parseErr.Error(), tc.wantErrSubstring) {
				t.Fatalf("parse error = %v, want substring %q", parseErr, tc.wantErrSubstring)
			}
		})
	}
}

func TestParseScriptPollerOutput_EmptyStdoutIsNotOutput(t *testing.T) {
	t.Parallel()

	_, hasOutput, err := scriptpollers.ParseScriptPollerOutput(nil)
	if hasOutput || err != nil {
		t.Fatalf("empty stdout = hasOutput %v err %v, want no output", hasOutput, err)
	}
}

func TestParseScriptPollerStdout_ExtractsOpaqueRecoveryFacts(t *testing.T) {
	t.Parallel()

	stdout := []byte(`{
		"requestId":"linear-issue-batch-cursor",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-cursor","workTypeName":"task"}],
		"cursor":"opaque-cursor-9",
		"checkpoint":"checkpoint-9"
	}`)
	parsed, err := scriptpollers.ParseScriptPollerStdout(stdout)
	if err != nil {
		t.Fatalf("ParseScriptPollerStdout() error = %v", err)
	}
	if !parsed.HasRequest || !parsed.AdvancesPosition {
		t.Fatalf("parsed = %+v, want request and position advancement", parsed)
	}
	if parsed.AdvancedCursor != "opaque-cursor-9" || parsed.Checkpoint != "checkpoint-9" {
		t.Fatalf("parsed recovery = cursor %q checkpoint %q", parsed.AdvancedCursor, parsed.Checkpoint)
	}
}
