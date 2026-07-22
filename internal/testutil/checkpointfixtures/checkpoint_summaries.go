package checkpointfixtures

import factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// CheckpointSummariesFixture returns only the outputs explicitly configured by
// a consumer test. It intentionally contains no checkpoint record reduction or
// projection policy; those invariants belong to the Factory Runtime owner.
type CheckpointSummariesFixture struct {
	BuildResult  *factoryruntime.JavaScriptCheckpointSummary
	LatestResult *factoryruntime.JavaScriptCheckpointSummary
}

var _ factoryruntime.JavaScriptCheckpointSummaries = CheckpointSummariesFixture{}

func (fixture CheckpointSummariesFixture) Build(
	factoryruntime.JavaScriptCheckpointSummaryInput,
) *factoryruntime.JavaScriptCheckpointSummary {
	return cloneCheckpointSummaryResult(fixture.BuildResult)
}

func (fixture CheckpointSummariesFixture) Latest(
	factoryruntime.JavaScriptCheckpointSummaryInput,
) *factoryruntime.JavaScriptCheckpointSummary {
	return cloneCheckpointSummaryResult(fixture.LatestResult)
}

// ResumableCheckpointSummaryResult is fixed contract data used by consumer
// tests whose scenario starts from the standard first durable checkpoint.
func ResumableCheckpointSummaryResult() *factoryruntime.JavaScriptCheckpointSummary {
	return &factoryruntime.JavaScriptCheckpointSummary{
		SchemaVersion:        factoryruntime.JavaScriptCheckpointSummarySchemaVersion,
		Kind:                 factoryruntime.JavaScriptCheckpointSummaryKind,
		CheckpointID:         "checkpoint-1",
		Label:                "after-step-one",
		CompletedDispatchIDs: []string{"dispatch-1"},
		ResumeStrategy:       factoryruntime.JavaScriptResumeStrategy,
		CheckpointState: map[string]any{
			"step":       1,
			"firstLabel": "step-one",
		},
	}
}

// ReplayFirstChildCheckpointSummaryResult is fixed contract data for a
// resumable scenario that replays its first child before reading resume state.
func ReplayFirstChildCheckpointSummaryResult() *factoryruntime.JavaScriptCheckpointSummary {
	return &factoryruntime.JavaScriptCheckpointSummary{
		SchemaVersion:        factoryruntime.JavaScriptCheckpointSummarySchemaVersion,
		Kind:                 factoryruntime.JavaScriptCheckpointSummaryKind,
		CheckpointID:         "checkpoint-1",
		Label:                "after-step-one",
		CompletedDispatchIDs: []string{"dispatch-1"},
		ResumeStrategy:       factoryruntime.JavaScriptResumeStrategy,
	}
}

func cloneCheckpointSummaryResult(
	summary *factoryruntime.JavaScriptCheckpointSummary,
) *factoryruntime.JavaScriptCheckpointSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	cloned.CompletedDispatchIDs = append([]string(nil), summary.CompletedDispatchIDs...)
	cloned.PendingDispatchIDs = append([]string(nil), summary.PendingDispatchIDs...)
	cloned.ArtifactIDs = append([]string(nil), summary.ArtifactIDs...)
	if len(summary.CheckpointState) > 0 {
		cloned.CheckpointState = make(map[string]any, len(summary.CheckpointState))
		for key, value := range summary.CheckpointState {
			cloned.CheckpointState[key] = value
		}
	}
	return &cloned
}
