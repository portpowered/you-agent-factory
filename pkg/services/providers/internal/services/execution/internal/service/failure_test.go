package service

import (
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

func TestLifecycleStageDiagnosticsCarriesExistingDiagnosticsWithoutStage(t *testing.T) {
	failure := execution.AttemptFailure{
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 15,
			Progress: []providers.ExecuteProgress{
				{Phase: "flush", Detail: "already observed"},
			},
		},
	}

	got := lifecycleStageDiagnostics(failure, false)

	if got == nil {
		t.Fatalf("expected diagnostics to be preserved when no stage error is set")
	}
	if got.DurationMillis != 15 || len(got.Progress) != 1 {
		t.Fatalf("expected cloned diagnostics content to be preserved, got %+v", got)
	}
	if _, hasStage := got.Metadata["failure_stage"]; hasStage {
		t.Fatalf("expected no failure_stage metadata without a stage error, got %+v", got.Metadata)
	}
}

func TestMergeLifecycleDiagnosticsNilInputsReturnNil(t *testing.T) {
	if got := mergeLifecycleDiagnostics(nil, nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestMergeLifecycleDiagnosticsDeclaredNilReturnsLifecycleClone(t *testing.T) {
	lifecycle := &providers.ExecuteDiagnostics{
		DurationMillis: 42,
		Metadata:       map[string]string{"stage": "decode"},
	}

	got := mergeLifecycleDiagnostics(nil, lifecycle)

	if got == nil {
		t.Fatalf("expected non-nil diagnostics")
	}
	if got.DurationMillis != 42 || got.Metadata["stage"] != "decode" {
		t.Fatalf("expected cloned lifecycle diagnostics, got %+v", got)
	}
	got.Metadata["stage"] = "mutated"
	if lifecycle.Metadata["stage"] != "decode" {
		t.Fatalf("expected detached clone, mutation leaked into source")
	}
}

func TestMergeLifecycleDiagnosticsLifecycleNilReturnsDeclaredClone(t *testing.T) {
	declared := &providers.ExecuteDiagnostics{
		DurationMillis: 7,
		Metadata:       map[string]string{"kind": "declared"},
	}

	got := mergeLifecycleDiagnostics(declared, nil)

	if got == nil {
		t.Fatalf("expected non-nil diagnostics")
	}
	if got.DurationMillis != 7 || got.Metadata["kind"] != "declared" {
		t.Fatalf("expected cloned declared diagnostics, got %+v", got)
	}
	got.Metadata["kind"] = "mutated"
	if declared.Metadata["kind"] != "declared" {
		t.Fatalf("expected detached clone, mutation leaked into source")
	}
}

func TestMergeLifecycleDiagnosticsMergesBothSources(t *testing.T) {
	declared := &providers.ExecuteDiagnostics{
		DurationMillis:          5,
		ProgressAlreadyObserved: false,
	}
	overlay := &providers.ExecuteDiagnostics{
		DurationMillis:          20,
		ProgressAlreadyObserved: true,
		Progress: []providers.ExecuteProgress{
			{Phase: "flush", Detail: "lifecycle progress"},
		},
		Metadata: map[string]string{"failure_stage": "flush"},
	}

	got := mergeLifecycleDiagnostics(declared, overlay)

	if got == nil {
		t.Fatalf("expected non-nil diagnostics")
	}
	if got.DurationMillis != 20 {
		t.Fatalf("expected overlay duration to win when larger, got %d", got.DurationMillis)
	}
	if len(got.Progress) != 1 || got.Progress[0].Phase != "flush" {
		t.Fatalf("expected overlay progress to be appended, got %+v", got.Progress)
	}
	if got.Metadata["failure_stage"] != "flush" {
		t.Fatalf("expected overlay metadata to be merged, got %+v", got.Metadata)
	}
	if !got.ProgressAlreadyObserved {
		t.Fatalf("expected ProgressAlreadyObserved to be true when either source sets it")
	}
}

func TestMergeLifecycleDiagnosticsKeepsDeclaredDurationWhenLarger(t *testing.T) {
	declared := &providers.ExecuteDiagnostics{DurationMillis: 100}
	overlay := &providers.ExecuteDiagnostics{DurationMillis: 10}

	got := mergeLifecycleDiagnostics(declared, overlay)

	if got == nil || got.DurationMillis != 100 {
		t.Fatalf("expected declared duration to be kept, got %+v", got)
	}
}
