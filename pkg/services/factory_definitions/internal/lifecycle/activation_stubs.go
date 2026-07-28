package lifecycle

import (
	"context"
	"time"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// stubActivationGateway is the default no-op activation gateway for lifecycle
// characterization. Host-specific tests can override individual methods on a copy.
type stubActivationGateway struct{}

func (stubActivationGateway) RunSessionID() string { return "" }

func (stubActivationGateway) SessionForActivation(string) *factoryroot.DefinitionSession {
	return nil
}

func (stubActivationGateway) RequireSession(string) (*factoryroot.DefinitionSession, error) {
	return nil, nil
}

func (stubActivationGateway) SessionFactoryPersistRoot(*factoryroot.DefinitionSession) string {
	return ""
}

func (stubActivationGateway) NamedFactoryActivationPaths(*factoryroot.DefinitionSession) (string, string) {
	return "", ""
}

func (stubActivationGateway) SaveNow() time.Time { return time.Time{} }

func (stubActivationGateway) WithActivationLock(fn func() error) error { return fn() }

func (stubActivationGateway) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (stubActivationGateway) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factoryroot.DefinitionSession) error {
	return nil
}

func (stubActivationGateway) ActivateSessionEditableFactory(context.Context, *factoryroot.DefinitionSession, string, string, string, string, string) error {
	return nil
}

func (stubActivationGateway) SwapPersistedNamedFactoryRuntime(context.Context, string, *factoryroot.DefinitionSession, string, string, string, string) error {
	return nil
}

// StubActivationGateway returns the default no-op activation gateway for tests.
func StubActivationGateway() factoryroot.DefinitionActivationGateway {
	return stubActivationGateway{}
}
