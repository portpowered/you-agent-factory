package factorysessions

import (
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestDefinitionRuntimeRouter_IsolatesBindingsAndRetainsTheOtherSession(t *testing.T) {
	router := NewDefinitionRuntimeRouter()
	alphaHost := definitionRouterHostStub{sessionID: "alpha"}
	alphaGateway := definitionRouterGatewayStub{sessionID: "alpha"}
	betaHost := definitionRouterHostStub{sessionID: "beta"}
	betaGateway := definitionRouterGatewayStub{sessionID: "beta"}

	if err := router.Bind("alpha", alphaHost, alphaGateway); err != nil {
		t.Fatalf("Bind(alpha) error = %v", err)
	}
	if err := router.Bind("beta", betaHost, betaGateway); err != nil {
		t.Fatalf("Bind(beta) error = %v", err)
	}

	alpha, err := router.Host().RequireSession("alpha")
	if err != nil || alpha == nil || alpha.ID != "alpha" {
		t.Fatalf("Host().RequireSession(alpha) = %#v, %v", alpha, err)
	}
	beta, err := router.Host().RequireSession("beta")
	if err != nil || beta == nil || beta.ID != "beta" {
		t.Fatalf("Host().RequireSession(beta) = %#v, %v", beta, err)
	}
	if got := router.ActivationGateway().RunSessionID(); got != "alpha" {
		t.Fatalf("ActivationGateway().RunSessionID() = %q, want first bound session", got)
	}

	router.Unbind("alpha")
	if _, err := router.Host().RequireSession("alpha"); !errors.Is(err, ErrDefinitionRuntimeUnavailable) {
		t.Fatalf("Host().RequireSession(alpha after Unbind) error = %v, want unavailable", err)
	}
	if got := router.ActivationGateway().RunSessionID(); got != "beta" {
		t.Fatalf("ActivationGateway().RunSessionID() after alpha cleanup = %q, want beta", got)
	}
	if retained, err := router.Host().RequireSession("beta"); err != nil || retained == nil || retained.ID != "beta" {
		t.Fatalf("Host().RequireSession(beta after alpha cleanup) = %#v, %v", retained, err)
	}
}

func TestDefinitionRuntimeRouter_EquivalentBindIsIdempotentAndConflictDoesNotReplace(t *testing.T) {
	router := NewDefinitionRuntimeRouter()
	host := definitionRouterHostStub{sessionID: "session"}
	gateway := definitionRouterGatewayStub{sessionID: "session"}
	if err := router.Bind("session", host, gateway); err != nil {
		t.Fatalf("Bind(first) error = %v", err)
	}
	if err := router.Bind("session", host, gateway); err != nil {
		t.Fatalf("Bind(equivalent) error = %v, want idempotent success", err)
	}
	if err := router.Bind("session", definitionRouterHostStub{sessionID: "replacement"}, gateway); !errors.Is(err, ErrDefinitionRuntimeAlreadyBound) {
		t.Fatalf("Bind(conflicting) error = %v, want already-bound conflict", err)
	}
	resolved, err := router.Host().RequireSession("session")
	if err != nil || resolved == nil || resolved.ID != "session" {
		t.Fatalf("conflicting Bind replaced target: %#v, %v", resolved, err)
	}
}

type definitionRouterHostStub struct {
	factorydefinitions.SessionHost
	sessionID string
}

func (s definitionRouterHostStub) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{ID: s.sessionID}, nil
}

type definitionRouterGatewayStub struct {
	factorydefinitions.DefinitionActivationGateway
	sessionID string
}

func (s definitionRouterGatewayStub) RunSessionID() string { return s.sessionID }

func (s definitionRouterGatewayStub) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return &factorydefinitions.DefinitionSession{ID: s.sessionID}
}

func (s definitionRouterGatewayStub) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{ID: s.sessionID}, nil
}

var _ DefinitionHost = definitionRouterHostStub{}
var _ DefinitionActivationGateway = definitionRouterGatewayStub{}
