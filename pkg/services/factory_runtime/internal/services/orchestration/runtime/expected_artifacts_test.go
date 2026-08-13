package runtime

import (
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestVerifyRuntimeExpectedArtifactDeclarationsClassifiesWorkspaceResults(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	reports := filepath.Join(workspace, "reports")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(reports, "summary.md"), []byte("summary"), 0o600); err != nil {
		t.Fatalf("WriteFile(summary) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(reports, "empty.md"), nil, 0o600); err != nil {
		t.Fatalf("WriteFile(empty) error = %v", err)
	}

	declarations := []work.ExpectedArtifactDeclaration{
		{Name: "summary", Pattern: "reports/summary.md", NonEmpty: true},
		{Name: "empty", Pattern: "reports/empty.md", NonEmpty: true},
		{Name: "missing", Pattern: "reports/missing.md"},
		{Name: "glob", Pattern: "reports/summary*.md", NonEmpty: true},
		{Name: "unsafe", Pattern: "../outside.md"},
		{Name: "invalid", Pattern: "["},
		{Name: "template", Pattern: "{{.Missing}}"},
	}
	verification := verifyRuntimeExpectedArtifactDeclarations(
		workspace,
		work.WorkDispatch{ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{}},
		declarations,
		platformfilesystem.Local{},
	)
	if verification == nil || len(verification.Entries) != 5 {
		t.Fatalf("verification = %#v, want five unmet declarations", verification)
	}
	wantReasons := map[string]workers.ExpectedArtifactVerificationReason{
		"empty":    workers.ExpectedArtifactVerificationReasonEmpty,
		"missing":  workers.ExpectedArtifactVerificationReasonMissing,
		"unsafe":   workers.ExpectedArtifactVerificationReasonMissing,
		"invalid":  workers.ExpectedArtifactVerificationReasonMissing,
		"template": workers.ExpectedArtifactVerificationReasonMissing,
	}
	for _, entry := range verification.Entries {
		if want := wantReasons[entry.Name]; entry.Reason != want {
			t.Fatalf("entry %q reason = %q, want %q", entry.Name, entry.Reason, want)
		}
		if entry.Name == "unsafe" && entry.Pattern != unsafeExpectedArtifactPattern {
			t.Fatalf("unsafe pattern = %q, want redacted marker", entry.Pattern)
		}
	}
	message := expectedArtifactVerificationMessage(verification)
	if message == "" || !containsExpectedArtifactMessage(message, "EXPECTED_ARTIFACTS_UNSATISFIED") {
		t.Fatalf("verification message = %q, want failure code", message)
	}

	accepted := verifyExpectedArtifactsForDispatch(
		&runtimeConfig{net: &state.Net{}},
		workers.ExecuteRequest{Target: workers.ExecutionTarget{Environment: workers.EnvironmentPolicy{WorkingDirectory: workspace}}},
		workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted},
	)
	if accepted.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("dispatch without declarations outcome = %q, want accepted", accepted.Outcome)
	}
}

func TestExpectedArtifactDeclarationsForDispatchDeduplicatesWorkTypeAndTransitionDeclarations(t *testing.T) {
	t.Parallel()

	declaration := work.ExpectedArtifactDeclaration{Name: "summary", Pattern: "summary.md"}
	cfg := &runtimeConfig{net: &state.Net{
		WorkTypes: map[string]*state.WorkType{
			"report": {ExpectedArtifacts: []work.ExpectedArtifactDeclaration{declaration}},
		},
		Transitions: map[string]*petri.Transition{
			"review": {ExpectedArtifacts: []work.ExpectedArtifactDeclaration{
				declaration,
				{Name: "details", Pattern: "details.md"},
			}},
		},
	}}
	dispatch := work.WorkDispatch{
		TransitionID: "review",
		InputTokens:  workers.InputTokens(workers.Token{Color: workers.Color{WorkTypeID: "report"}}),
	}
	got := expectedArtifactDeclarationsForDispatch(cfg, dispatch)
	if len(got) != 2 || got[0] != declaration || got[1].Name != "details" {
		t.Fatalf("declarations = %#v, want stable deduplicated order", got)
	}
}

func TestSafeRuntimeExpectedArtifactPatternRejectsHostEscapes(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "/absolute/path", "../outside", `C:\\outside`, "["} {
		if normalized, ok := safeRuntimeExpectedArtifactPattern(value); ok || normalized != "" {
			t.Fatalf("safeRuntimeExpectedArtifactPattern(%q) = %q, %t, want rejection", value, normalized, ok)
		}
	}
	if normalized, ok := safeRuntimeExpectedArtifactPattern("reports/*.md"); !ok || normalized != "reports/*.md" {
		t.Fatalf("safeRuntimeExpectedArtifactPattern(valid) = %q, %t", normalized, ok)
	}
}

func containsExpectedArtifactMessage(message, value string) bool {
	for start := 0; start+len(value) <= len(message); start++ {
		if message[start:start+len(value)] == value {
			return true
		}
	}
	return false
}
