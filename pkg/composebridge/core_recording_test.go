package composebridge

import (
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

func TestValidateCoreCollaboratorsRequiresEachRuntimeOwner(t *testing.T) {
	t.Parallel()

	store, err := runtimepersist.NewProjectStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewProjectStore: %v", err)
	}
	complete := Collaborators{
		Sessions: factorysessions.NewRegistry(), RuntimeBuild: &runtimebuild.Service{},
		WorkersScheduler: &workersservice.Service{}, DurableExecution: factorysessionexecution.NewFakeService(),
		Persistence: factorysessionexecution.EnabledPersistence(store),
	}
	tests := []struct {
		name, wantError string
		collaborators   Collaborators
	}{
		{name: "sessions", wantError: "Factory Session registry"},
		{name: "runtime build", collaborators: Collaborators{Sessions: complete.Sessions}, wantError: "runtime build service"},
		{name: "worker scheduler", collaborators: Collaborators{Sessions: complete.Sessions, RuntimeBuild: complete.RuntimeBuild}, wantError: "worker sidecar owner"},
		{name: "durable execution", collaborators: Collaborators{Sessions: complete.Sessions, RuntimeBuild: complete.RuntimeBuild, WorkersScheduler: complete.WorkersScheduler}, wantError: "durable execution service"},
		{name: "persistence", collaborators: Collaborators{Sessions: complete.Sessions, RuntimeBuild: complete.RuntimeBuild, WorkersScheduler: complete.WorkersScheduler, DurableExecution: complete.DurableExecution}, wantError: "persistence"},
		{name: "complete", collaborators: complete},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			validationErr := validateCoreCollaborators(test.collaborators)
			if test.wantError == "" && validationErr != nil {
				t.Fatalf("validateCoreCollaborators: %v", validationErr)
			}
			if test.wantError != "" && (validationErr == nil || !strings.Contains(validationErr.Error(), test.wantError)) {
				t.Fatalf("validateCoreCollaborators error = %v, want containing %q", validationErr, test.wantError)
			}
		})
	}
}
