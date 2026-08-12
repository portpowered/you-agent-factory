package runtimeopening

import (
	"context"
	"fmt"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// definitionHostHandoff lets Definitions be constructed before the completed
// SessionRuntime exists. The holder is a construction-time seam only: the
// target is bound before the opened runtime is published and all later calls
// delegate to that bound owner.
type definitionHostHandoff struct {
	mu     sync.RWMutex
	target factorydefinitions.SessionHost
}

func (h *definitionHostHandoff) bind(target factorydefinitions.SessionHost) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.target = target
	h.mu.Unlock()
}

func (h *definitionHostHandoff) get() (factorydefinitions.SessionHost, error) {
	if h == nil {
		return nil, fmt.Errorf("Factory Definitions session host handoff is required")
	}
	h.mu.RLock()
	target := h.target
	h.mu.RUnlock()
	if target == nil {
		return nil, fmt.Errorf("Factory Definitions session host is not initialized")
	}
	return target, nil
}

func (h *definitionHostHandoff) PersistRootDir() string {
	target, _ := h.get()
	if target == nil {
		return ""
	}
	return target.PersistRootDir()
}

func (h *definitionHostHandoff) WorkstationLoader() factorydefinitions.WorkstationLoader {
	target, _ := h.get()
	if target == nil {
		return nil
	}
	return target.WorkstationLoader()
}

func (h *definitionHostHandoff) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	target, _ := h.get()
	if target == nil {
		return nil
	}
	return target.CurrentRuntimeConfig()
}

func (h *definitionHostHandoff) WorkflowID() string {
	target, _ := h.get()
	if target == nil {
		return ""
	}
	return target.WorkflowID()
}

func (h *definitionHostHandoff) RequireSession(id string) (*factorydefinitions.DefinitionSession, error) {
	target, err := h.get()
	if err != nil {
		return nil, err
	}
	return target.RequireSession(id)
}

func (h *definitionHostHandoff) SessionRuntimeConfig(id string) (factorydefinitions.LoadedFactorySource, error) {
	target, err := h.get()
	if err != nil {
		return nil, err
	}
	return target.SessionRuntimeConfig(id)
}

func (h *definitionHostHandoff) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	target, _ := h.get()
	if target == nil {
		return ""
	}
	return target.SessionFactoryPersistRoot(session)
}

func (h *definitionHostHandoff) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	target, err := h.get()
	if err != nil {
		return err
	}
	return target.ValidateEditableFactorySnapshot(ctx, snapshot)
}

func (h *definitionHostHandoff) GetCurrentFactorySnapshotForSession(ctx context.Context, id string) (*factorydefinitions.FactorySnapshot, error) {
	target, err := h.get()
	if err != nil {
		return nil, err
	}
	return target.GetCurrentFactorySnapshotForSession(ctx, id)
}

func (h *definitionHostHandoff) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	target, err := h.get()
	if err != nil {
		return nil, err
	}
	return target.ReplaceFactoryLayoutAtDir(targetDir, prepared)
}

// definitionActivationHandoff applies the same construction-time pattern to
// the narrow Sessions-owned activation edge consumed by Definitions.
type definitionActivationHandoff struct {
	mu     sync.RWMutex
	target factorydefinitions.DefinitionActivationGateway
}

func (h *definitionActivationHandoff) bind(target factorydefinitions.DefinitionActivationGateway) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.target = target
	h.mu.Unlock()
}

func (h *definitionActivationHandoff) get() (factorydefinitions.DefinitionActivationGateway, error) {
	if h == nil {
		return nil, fmt.Errorf("Factory Definitions activation handoff is required")
	}
	h.mu.RLock()
	target := h.target
	h.mu.RUnlock()
	if target == nil {
		return nil, fmt.Errorf("Factory Definitions activation gateway is not initialized")
	}
	return target, nil
}

func (h *definitionActivationHandoff) RunSessionID() string {
	target, _ := h.get()
	if target == nil {
		return ""
	}
	return target.RunSessionID()
}

func (h *definitionActivationHandoff) SessionForActivation(id string) *factorydefinitions.DefinitionSession {
	target, _ := h.get()
	if target == nil {
		return nil
	}
	return target.SessionForActivation(id)
}

func (h *definitionActivationHandoff) RequireSession(id string) (*factorydefinitions.DefinitionSession, error) {
	target, err := h.get()
	if err != nil {
		return nil, err
	}
	return target.RequireSession(id)
}

func (h *definitionActivationHandoff) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	target, _ := h.get()
	if target == nil {
		return ""
	}
	return target.SessionFactoryPersistRoot(session)
}

func (h *definitionActivationHandoff) NamedFactoryActivationPaths(session *factorydefinitions.DefinitionSession) (string, string) {
	target, _ := h.get()
	if target == nil {
		return "", ""
	}
	return target.NamedFactoryActivationPaths(session)
}

func (h *definitionActivationHandoff) SaveNow() time.Time {
	target, _ := h.get()
	if target == nil {
		return time.Time{}
	}
	return target.SaveNow()
}

func (h *definitionActivationHandoff) WithActivationLock(fn func() error) error {
	target, err := h.get()
	if err != nil {
		return err
	}
	return target.WithActivationLock(fn)
}

func (h *definitionActivationHandoff) RequireIdleRuntimeForSession(ctx context.Context, id string) error {
	target, err := h.get()
	if err != nil {
		return err
	}
	return target.RequireIdleRuntimeForSession(ctx, id)
}

func (h *definitionActivationHandoff) RequireIdleBeforeNamedFactoryActivation(ctx context.Context, id string, session *factorydefinitions.DefinitionSession) error {
	target, err := h.get()
	if err != nil {
		return err
	}
	return target.RequireIdleBeforeNamedFactoryActivation(ctx, id, session)
}

func (h *definitionActivationHandoff) ActivateSessionEditableFactory(ctx context.Context, session *factorydefinitions.DefinitionSession, sessionID, sessionRootDir, factoryDir, name, runtimeName string) error {
	target, err := h.get()
	if err != nil {
		return err
	}
	return target.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName)
}

func (h *definitionActivationHandoff) SwapPersistedNamedFactoryRuntime(ctx context.Context, sessionID string, session *factorydefinitions.DefinitionSession, persistRoot, folderPath, factoryDir, name string) error {
	target, err := h.get()
	if err != nil {
		return err
	}
	return target.SwapPersistedNamedFactoryRuntime(ctx, sessionID, session, persistRoot, folderPath, factoryDir, name)
}

var _ factorydefinitions.SessionHost = (*definitionHostHandoff)(nil)
var _ factorydefinitions.DefinitionActivationGateway = (*definitionActivationHandoff)(nil)
