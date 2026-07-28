package factorydefinitions

import (
	"context"
	"time"
)

// Definition activation root slice freezes session-context lookup, activation
// lock serialization, idle-runtime gating, and editable/named runtime swap
// vocabulary for Factory Definitions activation requesters. Sessions publishes
// the concrete gateway implementation; Definitions consumes this narrow edge
// without depending on the broad SessionHost attach-back bundle:
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

// DefinitionActivationGateway is the Factory Sessions-owned activation edge
// consumed by Definitions save, activate, and swap paths.
type DefinitionActivationGateway interface {
	RunSessionID() string
	SessionForActivation(sessionID string) *DefinitionSession
	RequireSession(sessionID string) (*DefinitionSession, error)
	SessionFactoryPersistRoot(session *DefinitionSession) string
	NamedFactoryActivationPaths(session *DefinitionSession) (persistRoot, folderPath string)
	SaveNow() time.Time

	WithActivationLock(fn func() error) error
	RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error
	RequireIdleBeforeNamedFactoryActivation(
		ctx context.Context,
		sessionID string,
		session *DefinitionSession,
	) error

	ActivateSessionEditableFactory(
		ctx context.Context,
		session *DefinitionSession,
		sessionID string,
		sessionRootDir string,
		factoryDir string,
		name string,
		runtimeName string,
	) error
	SwapPersistedNamedFactoryRuntime(
		ctx context.Context,
		sessionID string,
		session *DefinitionSession,
		persistRoot string,
		folderPath string,
		factoryDir string,
		name string,
	) error
}
