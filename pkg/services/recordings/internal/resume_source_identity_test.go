package internal

import (
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	resumeSourceSessionID = "550e8400-e29b-41d4-a716-446655440000"
	otherSourceSessionID  = "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba"
)

func TestResumeSourceCanonicalSessionIDUsesRecordingMetadataOverAliases(t *testing.T) {
	metadata := recordings.ReplayInputMetadata{FactorySessionID: resumeSourceSessionID}
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Metadata: &metadata,
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q", got, resumeSourceSessionID)
	}
}

func TestResumeSourceCanonicalSessionIDReadsLegacyEventIdentity(t *testing.T) {
	sessionID := resumeSourceSessionID
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &sessionID}},
				{Context: factorydefinitions.FactoryEventContext{SessionID: &sessionID}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q", got, resumeSourceSessionID)
	}
}

func TestResumeSourceCanonicalSessionIDDoesNotPromoteAlias(t *testing.T) {
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != "" {
		t.Fatalf("source canonical session ID = %q, want empty for alias-only input", got)
	}
}

func TestResumeSourceCanonicalSessionIDUsesAutomaticRecordingPath(t *testing.T) {
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}
	path := filepath.Join(
		t.TempDir(), ".you-agent-factory", "recordings", "2026", "08", "29",
		resumeSourceSessionID+".json",
	)

	got, err := resumeSourceCanonicalSessionIDForPath(input, path)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionIDForPath() error = %v", err)
	}
	if got != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q from automatic path", got, resumeSourceSessionID)
	}
}

func TestResumeSourceCanonicalSessionIDIgnoresUUIDInExplicitPath(t *testing.T) {
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionIDForPath(input, filepath.Join(t.TempDir(), resumeSourceSessionID+".json"))
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionIDForPath() error = %v", err)
	}
	if got != "" {
		t.Fatalf("source canonical session ID = %q, want empty for arbitrary explicit path", got)
	}
}

func TestResumeSourceCanonicalSessionIDRejectsConflictingIdentities(t *testing.T) {
	first := resumeSourceSessionID
	second := otherSourceSessionID
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &first}},
				{Context: factorydefinitions.FactoryEventContext{SessionID: &second}},
			},
		},
	}

	_, err := resumeSourceCanonicalSessionID(input)
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v, want conflicting identity error", err)
	}
}

func TestResumeSourceCanonicalSessionIDAllowsAliasAlongsideCanonicalEventIdentity(t *testing.T) {
	alias := "~default"
	canonical := resumeSourceSessionID
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
				{Context: factorydefinitions.FactoryEventContext{SessionID: &canonical}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != canonical {
		t.Fatalf("source canonical session ID = %q, want %q", got, canonical)
	}
}

func TestLoadResumeInputCarriesV2MetadataIdentityWithoutChangingHistory(t *testing.T) {
	fullInput := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{{Id: "resume-event"}},
		},
	}
	metadataInput := recordings.LoadReplayInputResult{
		Metadata: &recordings.ReplayInputMetadata{FactorySessionID: resumeSourceSessionID},
	}
	loader := replayInputLoaderFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.MetadataOnly {
			return metadataInput, nil
		}
		return fullInput, nil
	})
	service := &combinedService{replayInputs: loader}

	got, err := service.LoadResumeInput(recordings.LoadResumeInputRequest{Path: "source.recording.jsonl"})
	if err != nil {
		t.Fatalf("LoadResumeInput() error = %v", err)
	}
	if got.SourceCanonicalSessionID != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q", got.SourceCanonicalSessionID, resumeSourceSessionID)
	}
	if got.Input.Legacy == nil || len(got.Input.Legacy.Events) != 1 || got.Input.Legacy.Events[0].Id != "resume-event" {
		t.Fatalf("resume history = %#v, want unchanged legacy history", got.Input.Legacy)
	}
}

func TestLoadResumeInputUsesAutomaticPathIdentityForLegacyAliasHistory(t *testing.T) {
	alias := "~default"
	fullInput := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{{
				Id:      "resume-event",
				Context: factorydefinitions.FactoryEventContext{SessionID: &alias},
			}},
		},
	}
	loader := replayInputLoaderFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.MetadataOnly {
			return recordings.LoadReplayInputResult{
				Metadata: &recordings.ReplayInputMetadata{FactorySessionID: alias},
			}, nil
		}
		return fullInput, nil
	})
	service := &combinedService{replayInputs: loader}
	path := filepath.Join(
		t.TempDir(), ".you-agent-factory", "recordings", "2026", "08", "29",
		resumeSourceSessionID+".json",
	)

	got, err := service.LoadResumeInput(recordings.LoadResumeInputRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadResumeInput() error = %v", err)
	}
	if got.SourceCanonicalSessionID != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q from automatic path", got.SourceCanonicalSessionID, resumeSourceSessionID)
	}
}

type replayInputLoaderFunc func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error)

func (loader replayInputLoaderFunc) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	return loader(request)
}
