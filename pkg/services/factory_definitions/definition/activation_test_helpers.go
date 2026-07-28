package factorydefinition

import (
	"context"
	"sync/atomic"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func newTestService(host Host, gateway ...factoryroot.DefinitionActivationGateway) *Service {
	activationGateway := factoryroot.DefinitionActivationGateway(StubActivationGateway())
	if len(gateway) > 0 && gateway[0] != nil {
		activationGateway = gateway[0]
	}
	return New(host, activationGateway)
}

// trackingActivationGateway records activation gateway calls for characterization tests.
type trackingActivationGateway struct {
	stubActivationGateway

	runSessionID         string
	sessionForActivation *factorydefinitions.DefinitionSession
	persistRoot          string
	folderPath           string
	idleRuntimeErr       error
	idleNamedErr         error
	activateErr          error
	swapErr              error
	saveNow              time.Time
	lockDepth            atomic.Int32
	activateCalls        atomic.Int32
	swapCalls            atomic.Int32
	activatedName        string
	swappedName          string
}

type stubActivationGateway struct{}

func (stubActivationGateway) RunSessionID() string { return "" }

func (stubActivationGateway) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return nil
}

func (stubActivationGateway) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, nil
}

func (stubActivationGateway) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}

func (stubActivationGateway) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "", ""
}

func (stubActivationGateway) SaveNow() time.Time { return time.Time{} }

func (stubActivationGateway) WithActivationLock(fn func() error) error { return fn() }

func (stubActivationGateway) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (stubActivationGateway) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error {
	return nil
}

func (stubActivationGateway) ActivateSessionEditableFactory(context.Context, *factorydefinitions.DefinitionSession, string, string, string, string, string) error {
	return nil
}

func (stubActivationGateway) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorydefinitions.DefinitionSession, string, string, string, string) error {
	return nil
}

func (g *trackingActivationGateway) RunSessionID() string {
	if g.runSessionID != "" {
		return g.runSessionID
	}
	return g.stubActivationGateway.RunSessionID()
}

func (g *trackingActivationGateway) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return g.sessionForActivation
}

func (g *trackingActivationGateway) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return g.persistRoot, g.folderPath
}

func (g *trackingActivationGateway) SaveNow() time.Time {
	if !g.saveNow.IsZero() {
		return g.saveNow
	}
	return g.stubActivationGateway.SaveNow()
}

func (g *trackingActivationGateway) WithActivationLock(fn func() error) error {
	if g.lockDepth.Add(1) > 1 {
		g.lockDepth.Add(-1)
		return context.Canceled
	}
	defer g.lockDepth.Add(-1)
	return fn()
}

func (g *trackingActivationGateway) RequireIdleRuntimeForSession(context.Context, string) error {
	return g.idleRuntimeErr
}

func (g *trackingActivationGateway) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error {
	return g.idleNamedErr
}

func (g *trackingActivationGateway) ActivateSessionEditableFactory(
	_ context.Context,
	_ *factorydefinitions.DefinitionSession,
	_ string,
	_ string,
	_ string,
	name string,
	_ string,
) error {
	g.activateCalls.Add(1)
	g.activatedName = name
	return g.activateErr
}

func (g *trackingActivationGateway) SwapPersistedNamedFactoryRuntime(
	_ context.Context,
	_ string,
	_ *factorydefinitions.DefinitionSession,
	_ string,
	_ string,
	_ string,
	name string,
) error {
	g.swapCalls.Add(1)
	g.swappedName = name
	return g.swapErr
}
