package service_test

import (
	"context"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factory/sessions/service"
)

func TestService_GetFactorySessionSyncPreflight_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	session := &factorysessions.LiveSession{ID: "sess-preflight"}
	host := &openTestHost{requireSession: session}
	gateway := factorysessionservice.New(host)

	response, err := gateway.GetFactorySessionSyncPreflight(context.Background(), "sess-preflight", nil, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if response.ReasonCode != factoryapi.Ok {
		t.Fatalf("reasonCode = %q, want ok", response.ReasonCode)
	}
	if response.BackendScopeId == nil || *response.BackendScopeId != "runtime-test" {
		t.Fatalf("backendScopeId = %#v, want runtime-test", response.BackendScopeId)
	}
}

func TestService_GetFactorySessionSyncPreflight_RejectsDurableSessionID(t *testing.T) {
	t.Parallel()

	gateway := factorysessionservice.New(&openTestHost{})

	response, err := gateway.GetFactorySessionSyncPreflight(context.Background(), "dur-sess-js-run-n-001", nil, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if response.ReasonCode != factoryapi.SessionNotFound {
		t.Fatalf("reasonCode = %q, want session_not_found", response.ReasonCode)
	}
}

func TestService_GetFactorySessionResult_RejectsNonJavaScriptFactory(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		requireSession: &factorysessions.LiveSession{ID: "sess-petri"},
	}
	gateway := factorysessionservice.New(host)

	_, err := gateway.GetFactorySessionResult(context.Background(), "sess-petri")
	if err == nil {
		t.Fatal("GetFactorySessionResult = nil, want result unavailable")
	}
}

func TestService_GetFactorySessionPartialResult_RejectsNonJavaScriptFactory(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		requireSession: &factorysessions.LiveSession{ID: "sess-petri"},
	}
	gateway := factorysessionservice.New(host)

	_, err := gateway.GetFactorySessionPartialResult(context.Background(), "sess-petri")
	if err == nil {
		t.Fatal("GetFactorySessionPartialResult = nil, want result unavailable")
	}
}

func TestService_ListFactorySessions_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		sessionIDs: []string{"~default"},
		sessions: map[string]*factorysessions.LiveSession{
			"~default": {ID: "~default", IsDefault: true},
		},
	}
	gateway := factorysessionservice.New(host)

	response, err := gateway.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].Id != "~default" {
		t.Fatalf("sessions = %#v, want one default session", response.Sessions)
	}
}

func TestService_GetFactorySession_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		requireSession: &factorysessions.LiveSession{ID: "sess-get"},
	}
	gateway := factorysessionservice.New(host)

	session, err := gateway.GetFactorySession(context.Background(), "sess-get")
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Id != "sess-get" {
		t.Fatalf("session id = %q, want sess-get", session.Id)
	}
}

func TestService_ListFactorySessions_RequiresGateway(t *testing.T) {
	t.Parallel()

	var gateway *factorysessionservice.Service
	_, err := gateway.ListFactorySessions(context.Background())
	if err == nil {
		t.Fatal("ListFactorySessions = nil, want gateway required")
	}
}

func TestService_OpenFactorySession_MapsInitNewFactoryHint(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		discoverErr: factorysessions.NewValidationError(
			factorysessions.ValidationReasonNotRunnable,
			"folderPath",
			errors.New("no runnable targets"),
		),
	}
	gateway := factorysessionservice.New(host)
	validateOnly := true

	response, err := gateway.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:   t.TempDir(),
		ValidateOnly: &validateOnly,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if response.InitsNewFactory == nil || !*response.InitsNewFactory {
		t.Fatalf("initsNewFactory = %#v, want true", response.InitsNewFactory)
	}
}
