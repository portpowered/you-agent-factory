package service_test

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
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
	if response.Reason != factorysessions.SyncPreflightReasonOK {
		t.Fatalf("reason = %q, want ok", response.Reason)
	}
	if response.BackendScopeID == nil || *response.BackendScopeID != "runtime-test" {
		t.Fatalf("backend scope ID = %#v, want runtime-test", response.BackendScopeID)
	}
}

func TestService_GetFactorySessionSyncPreflight_RejectsDurableSessionID(t *testing.T) {
	t.Parallel()

	gateway := factorysessionservice.New(&openTestHost{})

	response, err := gateway.GetFactorySessionSyncPreflight(context.Background(), "dur-sess-js-run-n-001", nil, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if response.Reason != factorysessions.SyncPreflightReasonSessionNotFound {
		t.Fatalf("reason = %q, want session_not_found", response.Reason)
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
	if len(response) != 1 || response[0].Context.Session.ID != "~default" {
		t.Fatalf("sessions = %#v, want one default session", response)
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
	if session.Session.ID != "sess-get" {
		t.Fatalf("session id = %q, want sess-get", session.Session.ID)
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

	response, err := gateway.OpenFactorySession(context.Background(), factorysessions.OpenRequest{
		FolderPath:   t.TempDir(),
		ValidateOnly: validateOnly,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if response == nil || !response.InitsNewFactory {
		t.Fatalf("initsNewFactory = %#v, want true", response.InitsNewFactory)
	}
}
