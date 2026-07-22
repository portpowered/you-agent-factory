package run

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestJavaScriptWorkflowPathRecognizesSupportedExtensions(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"workflow.js", "WORKFLOW.MJS", " workflow.cjs "} {
		if !javascriptWorkflowPath(path) {
			t.Fatalf("javascriptWorkflowPath(%q) = false", path)
		}
	}
	if javascriptWorkflowPath("factory.json") {
		t.Fatal("javascriptWorkflowPath accepted a factory config")
	}
	data, err := loadFactoryInvocationHelpData("you", RunConfig{FactoryConfigPath: "workflow.mjs"})
	if err != nil || data != nil {
		t.Fatalf("loadFactoryInvocationHelpData(JavaScript) = (%#v, %v)", data, err)
	}
}

// Work owns ambiguity detection. This transport test verifies only the stable
// CLI representation and observability of the injected role's typed failure.
func TestObserveInvocationRejection_AmbiguousInputRecordsStructuredLogAndMetrics(t *testing.T) {
	resetCleanInvocationMetricsForTest()
	err := MapInvocationInputError(&work.InputError{
		Code:    work.InputErrorCodeSourceConflict,
		Message: "invocation input sources conflict: positional_text, stdin_text",
		ConflictingSources: []work.InputSourceLabel{
			work.InputSourcePositionalText,
			work.InputSourceStdinText,
		},
	})

	core, observed := observer.New(zap.InfoLevel)
	ObserveInvocationRejection(zap.New(core), err)

	entry := observed.FilterMessage(cleanInvocationLogMessageRejected).AllUntimed()
	if len(entry) != 1 {
		t.Fatalf("rejected logs = %d, want 1", len(entry))
	}
	fields := entry[0].ContextMap()
	if fields["mode"] != cleanInvocationModeLabel || fields["reason"] != cleanInvocationRejectReason {
		t.Fatalf("fields = %#v", fields)
	}
	conflictingAny, ok := fields["conflictingSources"].([]interface{})
	if !ok || len(conflictingAny) != 2 || conflictingAny[0] != "positional_prompt" || conflictingAny[1] != "stdin" {
		t.Fatalf("conflictingSources = %#v", fields["conflictingSources"])
	}
	if got := snapshotCleanInvocationMetrics(); got != (CleanInvocationMetricsSnapshot{Attempts: 1, AmbiguityRejected: 1}) {
		t.Fatalf("metrics = %#v", got)
	}
}
