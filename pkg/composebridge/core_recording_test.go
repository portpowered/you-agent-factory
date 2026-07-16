package composebridge

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"go.uber.org/zap"
)

func TestNewCollaboratorsRejectsMissingSessionRegistry(t *testing.T) {
	t.Parallel()

	collaborators, err := NewCollaborators(nil, factory.EnsureClock(nil), zap.NewNop(), nil)
	if collaborators != (Collaborators{}) || err == nil || !strings.Contains(err.Error(), "registry is required") {
		t.Fatalf("NewCollaborators() = (%#v, %v), want empty collaborators and registry error", collaborators, err)
	}
}

type recordingDurableExecution struct {
	factorysessionexecution.Service
	recordedSessionID string
	recorded          []interfaces.TokenMutationRecord
}

func (r *recordingDurableExecution) RecordPetriTokenMutations(sessionID string, records []interfaces.TokenMutationRecord) error {
	r.recordedSessionID = sessionID
	r.recorded = append([]interfaces.TokenMutationRecord(nil), records...)
	return nil
}

func TestComposePetriRecordingRuntimeBuildUsesDurableExecutionOwner(t *testing.T) {
	t.Parallel()

	var built runtimebuild.SessionBuildSpec
	build, err := runtimebuild.New(
		runtimebuild.Config{},
		factory.EnsureClock(nil),
		zap.NewNop(),
		func(_ context.Context, spec runtimebuild.SessionBuildSpec) (any, error) {
			built = spec
			return struct{}{}, nil
		},
	)
	if err != nil {
		t.Fatalf("runtimebuild.New: %v", err)
	}
	owner := &recordingDurableExecution{Service: factorysessionexecution.NewFakeService()}
	configured, err := composePetriRecordingRuntimeBuild(build, owner)
	if err != nil {
		t.Fatalf("composePetriRecordingRuntimeBuild: %v", err)
	}
	if _, err := configured.Build(context.Background(), runtimebuild.SessionBuildSpec{SessionID: "session-owned"}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	cfg := &factory.FactoryConfig{}
	for _, option := range built.AdditionalFactoryOpts {
		option(cfg)
	}
	if cfg.PetriMutationRecorder == nil {
		t.Fatal("runtime build omitted the canonical durable execution recorder")
	}
	want := []interfaces.TokenMutationRecord{{TransitionID: "completed"}}
	if err := cfg.PetriMutationRecorder("session-owned", want); err != nil {
		t.Fatalf("PetriMutationRecorder: %v", err)
	}
	if owner.recordedSessionID != "session-owned" || len(owner.recorded) != 1 || owner.recorded[0].TransitionID != "completed" {
		t.Fatalf("durable owner recording = (%q, %#v), want session-owned mutation", owner.recordedSessionID, owner.recorded)
	}
}
