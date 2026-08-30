package internal

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestResumeSourceCanonicalSessionIDForPathRequiresDurableCompletion(t *testing.T) {
	sourceID := "550e8400-e29b-41d4-a716-446655440000"
	input := recordings.LoadReplayInputResult{
		Legacy: &recordings.ReplayArtifact{
			Events: []recordings.FactoryEvent{{
				Type:    factorydefinitions.FactoryEventTypeDispatchResponse,
				Context: factorydefinitions.FactoryEventContext{SessionID: &sourceID},
			}},
		},
	}

	got, err := resumeSourceCanonicalSessionIDForPath(input, "source.recording.json")
	if err != nil {
		t.Fatalf("incomplete replay identity: %v", err)
	}
	if got != "" {
		t.Fatalf("incomplete replay identity = %q, want empty predecessor", got)
	}

	input.Legacy.Events = append(input.Legacy.Events, recordings.FactoryEvent{
		Type:    factorydefinitions.FactoryEventTypeSessionCompleted,
		Context: factorydefinitions.FactoryEventContext{SessionID: &sourceID},
	})
	got, err = resumeSourceCanonicalSessionIDForPath(input, "source.recording.json")
	if err != nil {
		t.Fatalf("closed replay identity: %v", err)
	}
	if got != sourceID {
		t.Fatalf("closed replay identity = %q, want %q", got, sourceID)
	}
}
