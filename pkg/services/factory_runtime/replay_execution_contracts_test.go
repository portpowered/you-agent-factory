package factory_test

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/replayhooks"
)

func TestReplayExecutionContractsPublishRuntimeRootHookVocabulary(t *testing.T) {
	t.Parallel()

	if factoryruntime.ReplaySubmissionHookName != "replay-artifact-submissions" {
		t.Fatalf(
			"submission hook name = %q, want replay-artifact-submissions",
			factoryruntime.ReplaySubmissionHookName,
		)
	}
	if factoryruntime.ReplayWorkStateChangeHookName != "replay-artifact-work-state-changes" {
		t.Fatalf(
			"work-state hook name = %q, want replay-artifact-work-state-changes",
			factoryruntime.ReplayWorkStateChangeHookName,
		)
	}
	if replayhooks.Adapt(nil) != nil {
		t.Fatalf("Adapt(nil) = %#v, want nil", replayhooks.Adapt(nil))
	}
}
