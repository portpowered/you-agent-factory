package service_test

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
)

func TestService_GetFactorySessionSyncPreflight_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	session := &factorysessions.LiveSession{ID: "sess-preflight"}
	host := &openTestHost{requireSession: session}
	gateway := factorysessionservice.New(host)

	response, err := gateway.GetFactorySessionSyncPreflight(context.Background(), "sess-preflight", nil)
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

	response, err := gateway.GetFactorySessionSyncPreflight(context.Background(), "dur-sess-js-run-n-001", nil)
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
