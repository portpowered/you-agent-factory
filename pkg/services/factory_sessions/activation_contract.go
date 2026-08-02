package factorysessions

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// DefinitionActivationGateway is a temporary Sessions-internal construction
// role retained while the P09 runtime is still assembled through the existing
// application graph. It is not consumed by peer services; P12 folds the
// Definitions root and removes this bridge entirely.
type DefinitionActivationGateway interface {
	RunSessionID() string
	SessionForActivation(string) *factorydefinitions.DefinitionSession
	RequireSession(string) (*factorydefinitions.DefinitionSession, error)
	SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string
	NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string)
	SaveNow() time.Time
	WithActivationLock(func() error) error
	RequireIdleRuntimeForSession(context.Context, string) error
	RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error
	ActivateSessionEditableFactory(context.Context, *factorydefinitions.DefinitionSession, string, string, string, string, string) error
	SwapPersistedNamedFactoryRuntime(context.Context, string, *factorydefinitions.DefinitionSession, string, string, string, string) error
}

// DefinitionActivationGatewayProvider exposes the Sessions-owned activation
// gateway for Factory Definitions construction without the attach-capable
// SessionHost bundle.
type DefinitionActivationGatewayProvider interface {
	DefinitionActivationGateway() DefinitionActivationGateway
}
