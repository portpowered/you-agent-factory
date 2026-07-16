package composebridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

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

func TestValidateCoreCollaboratorsRequiresEachRuntimeOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		collaborators Collaborators
		wantError     string
	}{
		{name: "sessions", wantError: "Factory Session registry"},
		{
			name:          "runtime build",
			collaborators: Collaborators{Sessions: factorysessions.NewRegistry()},
			wantError:     "runtime build service",
		},
		{
			name: "worker scheduler",
			collaborators: Collaborators{
				Sessions:     factorysessions.NewRegistry(),
				RuntimeBuild: &runtimebuild.Service{},
			},
			wantError: "worker sidecar owner",
		},
		{
			name: "complete",
			collaborators: Collaborators{
				Sessions:         factorysessions.NewRegistry(),
				RuntimeBuild:     &runtimebuild.Service{},
				WorkersScheduler: &workersservice.Service{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateCoreCollaborators(test.collaborators)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateCoreCollaborators: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateCoreCollaborators error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestComposePetriRecordingRuntimeBuildRejectsNonRecordingExecution(t *testing.T) {
	t.Parallel()

	configured, err := composePetriRecordingRuntimeBuild(&runtimebuild.Service{}, factorysessionexecution.NewFakeService())
	if configured != nil {
		t.Fatal("composePetriRecordingRuntimeBuild returned a service without a Petri mutation recorder")
	}
	if err == nil || !strings.Contains(err.Error(), "does not record Petri mutations") {
		t.Fatalf("composePetriRecordingRuntimeBuild error = %v, want missing-recorder diagnostic", err)
	}
}

func TestDurableProjectRootUsesFirstConfiguredRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                  string
		executionBaseDir, configuredDir, root string
		want                                  string
	}{
		{name: "execution base", executionBaseDir: " execution ", configuredDir: "configured", root: "root", want: "execution"},
		{name: "configured directory", configuredDir: " configured ", root: "root", want: "configured"},
		{name: "factory root", root: " root ", want: "root"},
		{name: "empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := durableProjectRoot(test.executionBaseDir, test.configuredDir, test.root); got != test.want {
				t.Fatalf("durableProjectRoot() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestComposeDurableExecutionLoadsWorkerPresets(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "operator-config.json")
	configJSON := `{"workerPresets":[{"id":"research","modelProvider":"codex","model":"gpt-5.4","reasoningEffort":"high"}]}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	execution, persistence, err := composeDurableExecution(&runtimehost.Config{
		Dir:              projectRoot,
		SystemConfigPath: configPath,
	}, Root{}, factory.EnsureClock(nil))
	if err != nil {
		t.Fatalf("composeDurableExecution: %v", err)
	}
	if execution == nil || persistence == nil {
		t.Fatalf("composeDurableExecution = (%v, %v), want execution and persistence", execution, persistence)
	}
}

func TestComposeDurableExecutionRejectsMalformedOperatorConfig(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "operator-config.json")
	if err := os.WriteFile(configPath, []byte(`{"workerPresets":[`), 0o600); err != nil {
		t.Fatalf("write malformed operator config: %v", err)
	}
	execution, persistence, err := composeDurableExecution(&runtimehost.Config{
		Dir:              t.TempDir(),
		SystemConfigPath: configPath,
	}, Root{}, factory.EnsureClock(nil))
	if execution != nil || persistence != nil {
		t.Fatalf("composeDurableExecution = (%v, %v), want no dependencies", execution, persistence)
	}
	if err == nil || !strings.Contains(err.Error(), "compose durable session worker presets") {
		t.Fatalf("composeDurableExecution error = %v, want operator-config diagnostic", err)
	}
}
