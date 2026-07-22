package artifacts

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestBuildRetainsCanonicalSummariesAndOmitsRuntimeDetails(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 7, 12, 16, 0, 1, 0, time.UTC)
	facts := CanonicalFacts{
		SessionID: "session-js-002", Status: "COMPLETED", OrchestratorKind: "JAVASCRIPT",
		SourceRef: "workflow/audit.js", SourceHash: digest('1'), PolicyHash: digest('2'),
		Arguments: map[string]any{"region": "west", "token": "argument-is-digested-not-retained"},
		Artifacts: []CanonicalArtifact{{ID: "artifact-1", Kind: "RESULT", Visibility: "PUBLIC", Label: "Result", ContentHash: digest('3'), SizeBytes: 42, CreatedAt: createdAt, SecretsRedacted: 3}},
		Events: []json.RawMessage{
			json.RawMessage(`{"id":"event-1","type":"SESSION_STARTED","context":{"sequence":0,"eventTime":"2026-07-12T16:00:00Z"},"payload":{"checkpointBody":{"secret":"checkpoint"},"childDispatches":[{"id":"dispatch-secret"}]}}`),
			json.RawMessage(`{"id":"event-2","type":"SESSION_COMPLETED","context":{"sequence":1,"eventTime":"2026-07-12T16:00:02Z"},"payload":{"artifactIds":["artifact-1"],"providerTranscript":"provider-secret"}}`),
		},
		Result: &CanonicalResult{Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"done"}`), ArtifactIDs: []string{"artifact-1"}},
	}

	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if value.ArgumentsDigest == "" || strings.Contains(value.ArgumentsDigest, "argument-is-digested") {
		t.Fatalf("argumentsDigest = %q", value.ArgumentsDigest)
	}
	if value.Redaction.SecretsRedacted != 3 || len(value.Artifacts) != 1 || len(value.Events) != 2 {
		t.Fatalf("recording summaries = %#v", value)
	}
	if len(value.Events[1].ArtifactIDs) != 1 || value.Events[1].ArtifactIDs[0] != "artifact-1" {
		t.Fatalf("event artifact references = %#v", value.Events[1].ArtifactIDs)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, prohibited := range []string{"argument-is-digested-not-retained", `"secret":"checkpoint"`, "dispatch-secret", "provider-secret"} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("portable recording leaked %q: %s", prohibited, encoded)
		}
	}
}

func TestBuildBoundsSecretsRedacted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		counts []int64
	}{
		{name: "at maximum", counts: []int64{MaxSecretsRedacted}},
		{name: "above maximum", counts: []int64{MaxSecretsRedacted + 1}},
		{name: "aggregate overflow", counts: []int64{MaxSecretsRedacted - 1, math.MaxInt64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			facts := minimalCanonicalFacts()
			facts.Artifacts = make([]CanonicalArtifact, len(tt.counts))
			for i, count := range tt.counts {
				facts.Artifacts[i] = CanonicalArtifact{
					ID: fmt.Sprintf("artifact-%d", i), Kind: "RESULT", Visibility: "PUBLIC",
					ContentHash: digest(byte('1' + i)), CreatedAt: time.Now(), SecretsRedacted: count,
				}
			}
			got, err := Build(facts)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if got.Redaction.SecretsRedacted != MaxSecretsRedacted {
				t.Fatalf("SecretsRedacted = %d, want %d", got.Redaction.SecretsRedacted, MaxSecretsRedacted)
			}
		})
	}
}

func TestBuildRetainsCanonicalPrecomputedArgumentsDigest(t *testing.T) {
	t.Parallel()
	facts := minimalCanonicalFacts()
	facts.ArgumentsDigest = digest('9')
	facts.Arguments = map[string]any{"must": "not be re-digested"}

	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if value.ArgumentsDigest != digest('9') {
		t.Fatalf("argumentsDigest = %q, want canonical projection digest", value.ArgumentsDigest)
	}
}

func TestApplyJavaScriptProjectionFactsRetainsPublicCheckpointAndPartialResult(t *testing.T) {
	t.Parallel()
	timestamp := time.Date(2026, 7, 12, 20, 30, 0, 0, time.UTC)
	facts := minimalCanonicalFacts()
	ApplyJavaScriptProjectionFacts(&facts, &interfaces.FactorySessionJavaScriptRuntimeState{
		ArgsDigest: digest('8'),
		Checkpoints: []interfaces.FactorySessionJavaScriptCheckpointRef{{
			ID: "checkpoint-1", Label: "Review", Summary: "Ready for review", Timestamp: timestamp,
			ArtifactRef: &interfaces.JavaScriptCheckpointArtifactRef{ID: "artifact-checkpoint"},
		}},
		ResultStatus: "FAILED_WITH_PARTIAL",
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "partial"},
			{Type: work.WorkContentPartTypeBinary, ArtifactID: "artifact-result"},
		},
	})

	if facts.ArgumentsDigest != digest('8') {
		t.Fatalf("arguments digest = %q", facts.ArgumentsDigest)
	}
	if facts.Checkpoint == nil || facts.Checkpoint.ID != "checkpoint-1" || facts.Checkpoint.ArtifactID != "artifact-checkpoint" || !facts.Checkpoint.Timestamp.Equal(timestamp) {
		t.Fatalf("checkpoint = %#v", facts.Checkpoint)
	}
	if facts.Result == nil || facts.Result.Status != "FAILED_WITH_PARTIAL" || facts.Result.Mode != "partial" || len(facts.Result.PrimaryResult) == 0 {
		t.Fatalf("result = %#v", facts.Result)
	}
	if len(facts.Result.ArtifactIDs) != 1 || facts.Result.ArtifactIDs[0] != "artifact-result" {
		t.Fatalf("result artifact IDs = %#v", facts.Result.ArtifactIDs)
	}
}

func TestApplyJavaScriptProjectionFactsHandlesAbsentAndStatusOnlyProjections(t *testing.T) {
	t.Parallel()
	ApplyJavaScriptProjectionFacts(nil, &interfaces.FactorySessionJavaScriptRuntimeState{})
	facts := minimalCanonicalFacts()
	ApplyJavaScriptProjectionFacts(&facts, nil)
	ApplyJavaScriptProjectionFacts(&facts, &interfaces.FactorySessionJavaScriptRuntimeState{ResultStatus: "FINAL"})
	if facts.Result == nil || facts.Result.Status != "FINAL" || facts.Result.Mode != "final" {
		t.Fatalf("status-only result = %#v", facts.Result)
	}

	facts.Result = nil
	ApplyJavaScriptProjectionFacts(&facts, &interfaces.FactorySessionJavaScriptRuntimeState{})
	if facts.Result != nil {
		t.Fatalf("empty projection manufactured result = %#v", facts.Result)
	}
}

func TestWriteFailureLeavesNoCompleteRecording(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "destination-is-directory")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	value, err := Build(minimalCanonicalFacts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	writer, err := NewAtomicWriter(
		os.MkdirAll,
		func(dir, pattern string) (TemporaryFile, error) { return os.CreateTemp(dir, pattern) },
		os.Remove,
		os.Rename,
	)
	if err != nil {
		t.Fatalf("NewAtomicWriter: %v", err)
	}
	if err := writer.Write(path, value); err == nil {
		t.Fatal("Write succeeded, want publish failure")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("temporary recording remained after failure: %s", entry.Name())
		}
	}
}

func TestNewAtomicWriterRejectsMissingOperations(t *testing.T) {
	t.Parallel()
	writer, err := NewAtomicWriter(nil, nil, nil, nil)
	if writer != nil || err == nil || !strings.Contains(err.Error(), "operations are required") {
		t.Fatalf("NewAtomicWriter = (%#v, %v), want missing operations", writer, err)
	}
}

func TestBuildRejectsMalformedCanonicalEvent(t *testing.T) {
	t.Parallel()
	facts := minimalCanonicalFacts()
	facts.Events = []json.RawMessage{json.RawMessage(`{"id":"event-1"}`)}
	_, err := Build(facts)
	if err == nil || !strings.Contains(err.Error(), "canonical event 0") {
		t.Fatalf("Build error = %v", err)
	}
}

func minimalCanonicalFacts() CanonicalFacts {
	return CanonicalFacts{SessionID: "session-js-002", Status: "COMPLETED", OrchestratorKind: "JAVASCRIPT", SourceRef: "workflow/audit.js", SourceHash: digest('1'), PolicyHash: digest('2'), Result: &CanonicalResult{Status: "FINAL", Mode: "final"}}
}

func digest(character byte) string { return "sha256:" + strings.Repeat(string(character), 64) }
