package service

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestFactoryService_GetFactorySession_ProjectsLegacyPetriRuntime(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		namedFactories: []string{"alpha"},
		defaultFactory: "alpha",
	})
	defer harness.stop(t)

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Id != defaultFactorySessionID {
		t.Fatalf("session id = %q, want %q", session.Id, defaultFactorySessionID)
	}
	if session.Runtime.OrchestratorKind != factoryapi.PETRI {
		t.Fatalf("orchestrator kind = %q, want PETRI", session.Runtime.OrchestratorKind)
	}
	if session.Runtime.Petri == nil {
		t.Fatal("petri projection is nil")
	}
	if session.Runtime.Javascript != nil {
		t.Fatalf("javascript projection = %#v, want nil for Petri session", session.Runtime.Javascript)
	}
	if session.Runtime.Lifecycle.StartedAt.IsZero() || session.Runtime.Lifecycle.UpdatedAt.IsZero() {
		t.Fatalf("lifecycle = %#v, want startedAt and updatedAt", session.Runtime.Lifecycle)
	}
	if session.Runtime.Lifecycle.UpdatedAt.Before(session.Runtime.Lifecycle.StartedAt.Add(-time.Minute)) {
		t.Fatalf("lifecycle ordering = %#v", session.Runtime.Lifecycle)
	}
}

func TestFactoryService_ListFactorySessions_IncludesRuntimeProjection(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		namedFactories: []string{"alpha"},
		defaultFactory: "alpha",
	})
	defer harness.stop(t)

	listed, err := harness.svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed.Sessions) == 0 {
		t.Fatal("expected at least one live session")
	}
	found := false
	for _, summary := range listed.Sessions {
		if summary.Id != defaultFactorySessionID {
			continue
		}
		found = true
		if summary.Runtime == nil {
			t.Fatal("default session summary missing runtime projection")
		}
		if summary.Runtime.OrchestratorKind != factoryapi.PETRI {
			t.Fatalf("orchestrator kind = %q, want PETRI", summary.Runtime.OrchestratorKind)
		}
	}
	if !found {
		t.Fatalf("sessions = %#v, want default session", listed.Sessions)
	}
}
