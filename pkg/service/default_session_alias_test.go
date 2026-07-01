package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

func TestFactoryService_DefaultSessionAliasResolvesToUUIDIdentity(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	if defaultSession.ID == factorysessions.DefaultSessionID {
		t.Fatalf("default live session id = %q, want resolved uuid", defaultSession.ID)
	}
	if _, err := uuid.Parse(defaultSession.ID); err != nil {
		t.Fatalf("default live session id = %q, want uuid: %v", defaultSession.ID, err)
	}
	if !defaultSession.IsDefault {
		t.Fatal("default live session IsDefault = false, want true")
	}

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession(~default): %v", err)
	}
	if session.Id != defaultSession.ID {
		t.Fatalf("session.Id = %q, want resolved uuid %q", session.Id, defaultSession.ID)
	}
	if session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("session.Id = %q, want concrete uuid not alias", session.Id)
	}
	if session.Runtime.StreamIdentity == nil || session.Runtime.StreamIdentity.FactorySessionID != defaultSession.ID {
		t.Fatalf("streamIdentity = %#v, want factorySessionId %q", session.Runtime.StreamIdentity, defaultSession.ID)
	}

	preflight, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, nil, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(~default): %v", err)
	}
	if preflight.RequestedSessionId != factorysessions.DefaultSessionID {
		t.Fatalf("requestedSessionId = %q, want %q", preflight.RequestedSessionId, factorysessions.DefaultSessionID)
	}
	if preflight.FactorySessionId == nil || *preflight.FactorySessionId != defaultSession.ID {
		t.Fatalf("factorySessionId = %#v, want %q", preflight.FactorySessionId, defaultSession.ID)
	}
	if preflight.ReasonCode != factoryapi.Ok {
		t.Fatalf("reasonCode = %q, want %q", preflight.ReasonCode, factoryapi.Ok)
	}
}

func TestFactoryService_DefaultSessionAliasAcceptedByUUIDSessionRead(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)

	byUUID, err := harness.svc.GetFactorySession(context.Background(), defaultSession.ID)
	if err != nil {
		t.Fatalf("GetFactorySession(uuid): %v", err)
	}
	byAlias, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession(~default): %v", err)
	}
	if byUUID.Id != byAlias.Id {
		t.Fatalf("uuid read id = %q, alias read id = %q, want same session", byUUID.Id, byAlias.Id)
	}
}

func TestFactoryService_ListFactorySessions_ExposesResolvedDefaultUUID(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	response, err := harness.svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(response.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(response.Sessions))
	}
	summary := response.Sessions[0]
	if summary.Id != defaultSession.ID {
		t.Fatalf("summary.Id = %q, want %q", summary.Id, defaultSession.ID)
	}
	if !summary.IsDefault {
		t.Fatal("summary.IsDefault = false, want true")
	}
	if strings.Contains(summary.Id, factorysessions.DefaultSessionID) {
		t.Fatalf("summary.Id = %q, must not contain alias token", summary.Id)
	}
}
