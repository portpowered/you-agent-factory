package run

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

func TestEmitHistoricalReplayInspectionStableFactoryFactsAndSafeQuoting(t *testing.T) {
	t.Parallel()

	workA := work.FactoryWorkItem{ID: "work-a", WorkTypeID: "task", State: "done", TraceID: "trace-a"}
	workZ := work.FactoryWorkItem{
		ID: "work-z", WorkTypeID: "review", State: "failed", TraceID: "trace-z", ParentID: "work-a",
		CurrentChainingTraceID: "chain-z", PreviousChainingTraceIDs: []string{"trace-z", "trace-a", "trace-z"},
	}
	state := recordings.FactoryWorldState{
		WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{
			"request-z": {
				RequestID: "request-z", Type: work.WorkRequestTypeFactoryRequestBatch,
				WorkItems: []work.FactoryWorkItem{workZ, workA},
			},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"dispatch-only": {ID: "dispatch-only", WorkTypeID: "internal", State: "running"},
			workZ.ID:        workZ,
			workA.ID:        workA,
		},
		RelationsByWorkID: map[string][]work.FactoryRelation{
			"dispatch-only": {{Type: "INTERNAL", TargetWorkID: "work-a"}},
			workZ.ID:        {{Type: "DEPENDS_ON", SourceWorkID: workZ.ID, TargetWorkID: workA.ID, RequiredState: "done", RequestID: "request-z", TraceID: "trace-z"}},
		},
		FailureDetailsByWorkID: map[string]recordings.FactoryWorldFailureDetail{
			workZ.ID: {
				DispatchID: "dispatch-z", TransitionID: "transition-z", WorkItem: workZ,
				FailureDetail: &workers.FailureDetail{
					Reason: workers.WorkFailureTypeUnknown, Message: "recorded failure\nwith safe quoting",
				},
			},
		},
		SessionBracket: &recordings.FactoryWorldSessionBracketState{
			LifecycleControlStatus: "RUNNING", Terminal: true, FinalStatus: "SUCCEEDED",
		},
	}
	inspection := factorysessions.HistoricalReplayInspection{
		Session: factorysessions.SessionReadResult{SessionID: "legacy-session", Status: factorysessions.LifecycleStatusSucceeded},
		FactoryProjection: factorysessions.HistoricalReplayFactoryProjection{
			Availability: factorysessions.HistoricalReplayFactoryProjectionAvailable,
			State:        &state,
		},
	}

	var first string
	for attempt := 0; attempt < 20; attempt++ {
		var output bytes.Buffer
		if err := emitHistoricalReplayInspection(&output, inspection); err != nil {
			t.Fatalf("emitHistoricalReplayInspection() attempt %d: %v", attempt, err)
		}
		if attempt == 0 {
			first = output.String()
		} else if output.String() != first {
			t.Fatalf("historical replay output changed on attempt %d:\nfirst=%s\ncurrent=%s", attempt, first, output.String())
		}
	}

	for _, want := range []string{
		`Factory projection: AVAILABLE (reason=)`,
		`Session lifecycle: control="RUNNING" terminal=true final="SUCCEEDED"`,
		`Work: id="work-a" type="task" state="done" trace="trace-a" parent=""`,
		`Work: id="work-z" type="review" state="failed" trace="trace-z" parent="work-a"`,
		`Lineage: work="work-z" current="chain-z" previous="trace-a,trace-z"`,
		`Relation: source="work-z" type="DEPENDS_ON" target="work-a" required="done" request="request-z" trace="trace-z"`,
		`Failure: work="work-z" dispatch="dispatch-z" reason="unknown" message="recorded failure\nwith safe quoting"`,
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("historical replay output = %q, want %q", first, want)
		}
	}
	if strings.Contains(first, "dispatch-only") {
		t.Fatalf("historical replay output fabricated dispatch-only Work facts: %q", first)
	}
	if strings.Index(first, `Work: id="work-a"`) > strings.Index(first, `Work: id="work-z"`) {
		t.Fatalf("Work rows are not sorted: %q", first)
	}
}

func TestClassifyRunInputFailurePreservesTypedFirstCorruptReplayDiagnostic(t *testing.T) {
	t.Parallel()

	diagnostic := recordings.ReplayArtifactDiagnostic{
		Code:    recordings.ReplayArtifactDiagnosticMalformed,
		Area:    "events",
		Path:    "events[3]",
		Message: `event "event-3" is malformed`,
		Action:  recordings.ReplayArtifactStructuralRepairAction,
	}
	artifactErr := &recordings.ReplayArtifactError{
		Kind:       recordings.ReplayArtifactErrorCorruptInput,
		Diagnostic: diagnostic,
		Cause:      recordings.ErrCorruptReplayInput,
	}
	inputErr := &recordings.ReplayInputError{
		Family:     recordings.ReplayInputFamilyLegacy,
		Diagnostic: diagnostic,
		Cause:      artifactErr,
	}

	got := classifyRunInputFailure(RunConfig{ReplayPath: `C:\private\recording.jsonl`}, inputErr)
	want := `Error: MALFORMED_REPLAY_ARTIFACT area=events path=events[3] event="event-3" action="REPLACE_OR_REGENERATE_RECORDING": event "event-3" is malformed`
	if got.Error() != want {
		t.Fatalf("classified replay error = %q, want %q", got.Error(), want)
	}
	if !clidiag.HasCodedDiagnostic(got) {
		t.Fatal("classified replay error has no CLI diagnostic")
	}
	if !errors.Is(got, recordings.ErrCorruptReplayInput) {
		t.Fatal("classified replay error did not preserve corruption cause")
	}
}

func TestClassifyRunInputFailurePreservesForeignReplayDiagnostic(t *testing.T) {
	t.Parallel()

	diagnostic := recordings.ReplayArtifactDiagnostic{
		Code:    recordings.ReplayArtifactDiagnosticForeignReference,
		Area:    "events",
		Path:    "events[4]",
		Message: `event "dispatch-response" has a foreign reference`,
		Action:  recordings.ReplayArtifactStructuralRepairAction,
	}
	err := &recordings.ReplayInputError{
		Family:     recordings.ReplayInputFamilyLegacy,
		Diagnostic: diagnostic,
		Cause: &recordings.ReplayArtifactError{
			Kind:       recordings.ReplayArtifactErrorForeign,
			Diagnostic: diagnostic,
			Cause:      recordings.ErrForeignPortableArtifact,
		},
	}

	got := classifyRunInputFailure(RunConfig{ReplayPath: "recording.jsonl"}, err)
	want := `Error: FOREIGN_REPLAY_ARTIFACT_REFERENCE area=events path=events[4] event="dispatch-response" action="REPLACE_OR_REGENERATE_RECORDING": event "dispatch-response" has a foreign reference`
	if got.Error() != want {
		t.Fatalf("classified foreign replay error = %q, want %q", got.Error(), want)
	}
}
