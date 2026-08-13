package factorysessions

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

var (
	// ErrDefinitionRuntimeAlreadyBound reports a conflicting registration for
	// one Factory Session identity. The existing capability is never replaced.
	ErrDefinitionRuntimeAlreadyBound = errors.New("Factory Definitions runtime is already bound")
	// ErrDefinitionRuntimeUnavailable reports a Definitions operation that has
	// no opened Factory Session capability to route to.
	ErrDefinitionRuntimeUnavailable = errors.New("Factory Definitions runtime is unavailable")
)

// DefinitionRuntimeRouter routes the process-scoped Definitions root to the
// session-owned host and activation capabilities that become available when a
// Factory Session is opened. The router contains only runtime capability
// state; it does not construct a service or another dependency graph. Its
// zero value is ready for use by the process composition layer.
type DefinitionRuntimeRouter struct {
	mu      sync.RWMutex
	current string
	targets map[string]definitionRuntimeTarget
}

type definitionRuntimeTarget struct {
	host    DefinitionHost
	gateway DefinitionActivationGateway
}

func (r *DefinitionRuntimeRouter) Host() DefinitionHost {
	if r == nil {
		return nil
	}
	return definitionHostRouter{router: r}
}

func (r *DefinitionRuntimeRouter) ActivationGateway() DefinitionActivationGateway {
	if r == nil {
		return nil
	}
	return definitionActivationGatewayRouter{router: r}
}

func (r *DefinitionRuntimeRouter) Bind(
	sessionID string,
	host DefinitionHost,
	gateway DefinitionActivationGateway,
) error {
	if r == nil {
		return fmt.Errorf("%w: router is required", ErrDefinitionRuntimeUnavailable)
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("%w: session ID is required", ErrDefinitionRuntimeUnavailable)
	}
	if host == nil {
		return fmt.Errorf("%w: session host is required", ErrDefinitionRuntimeUnavailable)
	}
	if gateway == nil {
		return fmt.Errorf("%w: activation gateway is required", ErrDefinitionRuntimeUnavailable)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.targets == nil {
		r.targets = make(map[string]definitionRuntimeTarget)
	}
	if existing, ok := r.targets[sessionID]; ok {
		if existing.host == host && existing.gateway == gateway {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrDefinitionRuntimeAlreadyBound, sessionID)
	}
	r.targets[sessionID] = definitionRuntimeTarget{host: host, gateway: gateway}
	if r.current == "" {
		r.current = sessionID
	}
	return nil
}

func (r *DefinitionRuntimeRouter) Unbind(sessionID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.targets, sessionID)
	if r.current != sessionID {
		return
	}
	r.current = ""
	ids := make([]string, 0, len(r.targets))
	for id := range r.targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > 0 {
		r.current = ids[0]
	}
}

func (r *DefinitionRuntimeRouter) target(sessionID string) (definitionRuntimeTarget, error) {
	if r == nil {
		return definitionRuntimeTarget{}, ErrDefinitionRuntimeUnavailable
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if sessionID = strings.TrimSpace(sessionID); sessionID == "" {
		sessionID = r.current
	}
	target, ok := r.targets[sessionID]
	if !ok && sessionID != DefaultSessionID {
		// The process-scoped default host owns the live session registry and can
		// serve explicitly identified sessions opened through that registry. A
		// separately bound session still wins above; this fallback keeps the
		// singular Definitions root usable for the common default-host layout
		// without weakening explicit binding isolation.
		target, ok = r.targets[DefaultSessionID]
	}
	if !ok {
		return definitionRuntimeTarget{}, fmt.Errorf("%w: %s", ErrDefinitionRuntimeUnavailable, sessionID)
	}
	return target, nil
}

type definitionHostRouter struct {
	router *DefinitionRuntimeRouter
}

func (h definitionHostRouter) target(sessionID string) (DefinitionHost, error) {
	target, err := h.router.target(sessionID)
	if err != nil {
		return nil, err
	}
	return target.host, nil
}

func (h definitionHostRouter) PersistRootDir() string {
	target, _ := h.target("")
	if target == nil {
		return ""
	}
	return target.PersistRootDir()
}

func (h definitionHostRouter) WorkstationLoader() factorydefinitions.WorkstationLoader {
	target, _ := h.target("")
	if target == nil {
		return nil
	}
	return target.WorkstationLoader()
}

func (h definitionHostRouter) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	target, _ := h.target("")
	if target == nil {
		return nil
	}
	return target.CurrentRuntimeConfig()
}

func (h definitionHostRouter) WorkflowID() string {
	target, _ := h.target("")
	if target == nil {
		return ""
	}
	return target.WorkflowID()
}

func (h definitionHostRouter) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	target, err := h.target(sessionID)
	if err != nil {
		return nil, err
	}
	return target.RequireSession(sessionID)
}

func (h definitionHostRouter) SessionRuntimeConfig(sessionID string) (factorydefinitions.LoadedFactorySource, error) {
	target, err := h.target(sessionID)
	if err != nil {
		return nil, err
	}
	return target.SessionRuntimeConfig(sessionID)
}

func (h definitionHostRouter) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	if session == nil {
		return ""
	}
	target, _ := h.target(session.ID)
	if target == nil {
		return ""
	}
	return target.SessionFactoryPersistRoot(session)
}

func (h definitionHostRouter) ValidateEditableFactorySnapshot(
	ctx context.Context,
	snapshot *factorydefinitions.FactorySnapshot,
) error {
	target, err := h.target("")
	if err != nil {
		return err
	}
	return target.ValidateEditableFactorySnapshot(ctx, snapshot)
}

func (h definitionHostRouter) GetCurrentFactorySnapshotForSession(
	ctx context.Context,
	sessionID string,
) (*factorydefinitions.FactorySnapshot, error) {
	target, err := h.target(sessionID)
	if err != nil {
		return nil, err
	}
	return target.GetCurrentFactorySnapshotForSession(ctx, sessionID)
}

func (h definitionHostRouter) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	target, err := h.target("")
	if err != nil {
		return nil, err
	}
	return target.ReplaceFactoryLayoutAtDir(targetDir, prepared)
}

type definitionActivationGatewayRouter struct {
	router *DefinitionRuntimeRouter
}

func (g definitionActivationGatewayRouter) target(sessionID string) (DefinitionActivationGateway, error) {
	target, err := g.router.target(sessionID)
	if err != nil {
		return nil, err
	}
	return target.gateway, nil
}

func (g definitionActivationGatewayRouter) RunSessionID() string {
	target, _ := g.target("")
	if target == nil {
		return ""
	}
	return target.RunSessionID()
}

func (g definitionActivationGatewayRouter) SessionForActivation(sessionID string) *factorydefinitions.DefinitionSession {
	target, _ := g.target(sessionID)
	if target == nil {
		return nil
	}
	return target.SessionForActivation(sessionID)
}

func (g definitionActivationGatewayRouter) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	target, err := g.target(sessionID)
	if err != nil {
		return nil, err
	}
	return target.RequireSession(sessionID)
}

func (g definitionActivationGatewayRouter) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	if session == nil {
		return ""
	}
	target, _ := g.target(session.ID)
	if target == nil {
		return ""
	}
	return target.SessionFactoryPersistRoot(session)
}

func (g definitionActivationGatewayRouter) NamedFactoryActivationPaths(session *factorydefinitions.DefinitionSession) (string, string) {
	if session == nil {
		return "", ""
	}
	target, _ := g.target(session.ID)
	if target == nil {
		return "", ""
	}
	return target.NamedFactoryActivationPaths(session)
}

func (g definitionActivationGatewayRouter) SaveNow() time.Time {
	target, _ := g.target("")
	if target == nil {
		return time.Time{}
	}
	return target.SaveNow()
}

func (g definitionActivationGatewayRouter) WithActivationLock(fn func() error) error {
	target, err := g.target("")
	if err != nil {
		return err
	}
	return target.WithActivationLock(fn)
}

func (g definitionActivationGatewayRouter) RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error {
	target, err := g.target(sessionID)
	if err != nil {
		return err
	}
	return target.RequireIdleRuntimeForSession(ctx, sessionID)
}

func (g definitionActivationGatewayRouter) RequireIdleBeforeNamedFactoryActivation(
	ctx context.Context,
	sessionID string,
	session *factorydefinitions.DefinitionSession,
) error {
	target, err := g.target(sessionID)
	if err != nil {
		return err
	}
	return target.RequireIdleBeforeNamedFactoryActivation(ctx, sessionID, session)
}

func (g definitionActivationGatewayRouter) ActivateSessionEditableFactory(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) error {
	target, err := g.target(sessionID)
	if err != nil {
		return err
	}
	return target.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName)
}

func (g definitionActivationGatewayRouter) SwapPersistedNamedFactoryRuntime(
	ctx context.Context,
	sessionID string,
	session *factorydefinitions.DefinitionSession,
	persistRoot string,
	folderPath string,
	factoryDir string,
	name string,
) error {
	target, err := g.target(sessionID)
	if err != nil {
		return err
	}
	return target.SwapPersistedNamedFactoryRuntime(ctx, sessionID, session, persistRoot, folderPath, factoryDir, name)
}

var _ DefinitionHost = definitionHostRouter{}
var _ DefinitionActivationGateway = definitionActivationGatewayRouter{}
