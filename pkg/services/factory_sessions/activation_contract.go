package factorysessions

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Definition activation root slice freezes session-context lookup, activation
// lock serialization, idle-runtime gating, and editable/named runtime swap
// vocabulary beside the singular Service aggregate. Peers consume this narrow
// gateway without importing private session-runtime registry types or depending
// on the broad SessionHost attach-back bundle:
//
//   - Session context: RunSessionID, SessionForActivation, RequireSession,
//     SessionFactoryPersistRoot, NamedFactoryActivationPaths, SaveNow
//   - Lock serialization: WithActivationLock
//   - Idle gating: RequireIdleRuntimeForSession,
//     RequireIdleBeforeNamedFactoryActivation
//   - Runtime swap: ActivateSessionEditableFactory,
//     SwapPersistedNamedFactoryRuntime
//
// DefinitionActivationGateway intentionally omits AttachFactoryDefinitions and
// other persistence/read collaborators that previously forced a construction-time
// Definitions↔Sessions self-attachment cycle.

// DefinitionActivationGateway is the Factory Sessions-owned activation edge for
// definition save, activate, and swap paths.
type DefinitionActivationGateway interface {
	RunSessionID() string
	SessionForActivation(sessionID string) *factorydefinitions.DefinitionSession
	RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error)
	SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string
	NamedFactoryActivationPaths(session *factorydefinitions.DefinitionSession) (persistRoot, folderPath string)
	SaveNow() time.Time

	WithActivationLock(fn func() error) error
	RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error
	RequireIdleBeforeNamedFactoryActivation(
		ctx context.Context,
		sessionID string,
		session *factorydefinitions.DefinitionSession,
	) error

	ActivateSessionEditableFactory(
		ctx context.Context,
		session *factorydefinitions.DefinitionSession,
		sessionID string,
		sessionRootDir string,
		factoryDir string,
		name string,
		runtimeName string,
	) error
	SwapPersistedNamedFactoryRuntime(
		ctx context.Context,
		sessionID string,
		session *factorydefinitions.DefinitionSession,
		persistRoot string,
		folderPath string,
		factoryDir string,
		name string,
	) error
}
