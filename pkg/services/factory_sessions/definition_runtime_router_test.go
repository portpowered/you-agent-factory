package factorysessions

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestDefinitionRuntimeRouter_IsolatesBindingsAndRetainsTheOtherSession(t *testing.T) {
	router := &DefinitionRuntimeRouter{}
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
	router := &DefinitionRuntimeRouter{}
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

func TestDefinitionRuntimeRouter_DefaultHostServesExplicitSession(t *testing.T) {
	t.Parallel()

	router := &DefinitionRuntimeRouter{}
	host := definitionRouterHostStub{sessionID: "default-host"}
	gateway := definitionRouterGatewayStub{sessionID: "default-host"}
	if err := router.Bind(DefaultSessionID, host, gateway); err != nil {
		t.Fatalf("Bind(default) error = %v", err)
	}

	resolved, err := router.Host().RequireSession("session-1")
	if err != nil {
		t.Fatalf("RequireSession(explicit) error = %v", err)
	}
	if resolved == nil || resolved.ID != "default-host" {
		t.Fatalf("resolved session = %#v, want default host session", resolved)
	}
}

func TestDefinitionRuntimeRouter_ForwardsEveryBoundCapability(t *testing.T) {
	fixture := newForwardingDefinitionFixture()
	if err := fixture.router.Bind("session", fixture.host, fixture.gateway); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	assertForwardingHostCapabilities(t, fixture)
	assertForwardingGatewayCapabilities(t, fixture)
}

type forwardingDefinitionFixture struct {
	router       *DefinitionRuntimeRouter
	session      *factorydefinitions.DefinitionSession
	host         forwardingDefinitionHostStub
	gateway      forwardingDefinitionGatewayStub
	hostValidate error
	hostReplace  error
	activate     error
	swap         error
	idle         error
}

func newForwardingDefinitionFixture() forwardingDefinitionFixture {
	session := &factorydefinitions.DefinitionSession{ID: "session"}
	hostValidateErr := errors.New("host validation")
	hostReplaceErr := errors.New("host replace")
	activateErr := errors.New("activate")
	swapErr := errors.New("swap")
	idleErr := errors.New("idle")
	return forwardingDefinitionFixture{
		router:  &DefinitionRuntimeRouter{},
		session: session,
		host: forwardingDefinitionHostStub{
			session:       session,
			persistRoot:   "/persist",
			workflowID:    "workflow",
			validateErr:   hostValidateErr,
			replaceErr:    hostReplaceErr,
			snapshot:      &factorydefinitions.FactorySnapshot{},
			replaceResult: &factorydefinitions.FactorySplitLayoutReplaceResult{},
		},
		gateway: forwardingDefinitionGatewayStub{
			session:       session,
			runSessionID:  "session",
			persistRoot:   "/activation-persist",
			activatePaths: [2]string{"/persist", "/folder"},
			saveNow:       time.Unix(42, 0),
			idleErr:       idleErr,
			activateErr:   activateErr,
			swapErr:       swapErr,
		},
		hostValidate: hostValidateErr,
		hostReplace:  hostReplaceErr,
		activate:     activateErr,
		swap:         swapErr,
		idle:         idleErr,
	}
}

func assertForwardingHostCapabilities(t *testing.T, fixture forwardingDefinitionFixture) {
	t.Helper()
	hostRoute := fixture.router.Host()
	assertForwardingHostIdentity(t, hostRoute, fixture.session)
	assertForwardingHostOperations(t, hostRoute, fixture)
}

func assertForwardingHostIdentity(
	t *testing.T,
	hostRoute DefinitionHost,
	session *factorydefinitions.DefinitionSession,
) {
	t.Helper()
	if hostRoute.PersistRootDir() != "/persist" || hostRoute.WorkflowID() != "workflow" {
		t.Fatalf("host identity = %q, %q", hostRoute.PersistRootDir(), hostRoute.WorkflowID())
	}
	if hostRoute.WorkstationLoader() != nil || hostRoute.CurrentRuntimeConfig() != nil {
		t.Fatal("host value forwarding returned unexpected non-nil placeholder")
	}
	if got, err := hostRoute.RequireSession("session"); err != nil || got != session {
		t.Fatalf("RequireSession() = %#v, %v", got, err)
	}
	if got, err := hostRoute.SessionRuntimeConfig("session"); err != nil || got != nil {
		t.Fatalf("SessionRuntimeConfig() = %#v, %v", got, err)
	}
	if got := hostRoute.SessionFactoryPersistRoot(session); got != "/persist/session" {
		t.Fatalf("SessionFactoryPersistRoot() = %q", got)
	}
}

func assertForwardingHostOperations(t *testing.T, hostRoute DefinitionHost, fixture forwardingDefinitionFixture) {
	t.Helper()
	if err := hostRoute.ValidateEditableFactorySnapshot(context.Background(), &factorydefinitions.FactorySnapshot{}); !errors.Is(err, fixture.hostValidate) {
		t.Fatalf("ValidateEditableFactorySnapshot() error = %v", err)
	}
	if got, err := hostRoute.GetCurrentFactorySnapshotForSession(context.Background(), "session"); err != nil || got != fixture.host.snapshot {
		t.Fatalf("GetCurrentFactorySnapshotForSession() = %#v, %v", got, err)
	}
	if got, err := hostRoute.ReplaceFactoryLayoutAtDir("/target", &factorydefinitions.PreparedFactoryLayoutPayload{}); !errors.Is(err, fixture.hostReplace) || got != fixture.host.replaceResult {
		t.Fatalf("ReplaceFactoryLayoutAtDir() = %#v, %v", got, err)
	}
	if hostRoute.SessionFactoryPersistRoot(nil) != "" {
		t.Fatal("SessionFactoryPersistRoot(nil) returned a path")
	}
}

func assertForwardingGatewayCapabilities(t *testing.T, fixture forwardingDefinitionFixture) {
	t.Helper()
	gatewayRoute := fixture.router.ActivationGateway()
	assertForwardingGatewayIdentity(t, gatewayRoute, fixture.session)
	assertForwardingGatewayOperations(t, gatewayRoute, fixture)
}

func assertForwardingGatewayIdentity(
	t *testing.T,
	gatewayRoute DefinitionActivationGateway,
	session *factorydefinitions.DefinitionSession,
) {
	t.Helper()
	if gatewayRoute.RunSessionID() != "session" {
		t.Fatalf("RunSessionID() = %q", gatewayRoute.RunSessionID())
	}
	if got := gatewayRoute.SessionForActivation("session"); got != session {
		t.Fatalf("SessionForActivation() = %#v", got)
	}
	if got, err := gatewayRoute.RequireSession("session"); err != nil || got != session {
		t.Fatalf("activation RequireSession() = %#v, %v", got, err)
	}
	if gatewayRoute.SessionFactoryPersistRoot(session) != "/activation-persist/session" {
		t.Fatalf("activation SessionFactoryPersistRoot() = %q", gatewayRoute.SessionFactoryPersistRoot(session))
	}
	if persist, folder := gatewayRoute.NamedFactoryActivationPaths(session); persist != "/persist" || folder != "/folder" {
		t.Fatalf("NamedFactoryActivationPaths() = %q, %q", persist, folder)
	}
	if got := gatewayRoute.SaveNow(); !got.Equal(time.Unix(42, 0)) {
		t.Fatalf("SaveNow() = %v", got)
	}
}

func assertForwardingGatewayOperations(t *testing.T, gatewayRoute DefinitionActivationGateway, fixture forwardingDefinitionFixture) {
	t.Helper()
	lockCalled := false
	if err := gatewayRoute.WithActivationLock(func() error { lockCalled = true; return nil }); err != nil || !lockCalled {
		t.Fatalf("WithActivationLock() = %v, called=%v", err, lockCalled)
	}
	if err := gatewayRoute.RequireIdleRuntimeForSession(context.Background(), "session"); !errors.Is(err, fixture.idle) {
		t.Fatalf("RequireIdleRuntimeForSession() error = %v", err)
	}
	if err := gatewayRoute.RequireIdleBeforeNamedFactoryActivation(context.Background(), "session", fixture.session); !errors.Is(err, fixture.idle) {
		t.Fatalf("RequireIdleBeforeNamedFactoryActivation() error = %v", err)
	}
	if err := gatewayRoute.ActivateSessionEditableFactory(context.Background(), fixture.session, "session", "/root", "/factory", "name", "runtime"); !errors.Is(err, fixture.activate) {
		t.Fatalf("ActivateSessionEditableFactory() error = %v", err)
	}
	if err := gatewayRoute.SwapPersistedNamedFactoryRuntime(context.Background(), "session", fixture.session, "/persist", "/folder", "/factory", "name"); !errors.Is(err, fixture.swap) {
		t.Fatalf("SwapPersistedNamedFactoryRuntime() error = %v", err)
	}
	if gatewayRoute.SessionFactoryPersistRoot(nil) != "" {
		t.Fatal("activation SessionFactoryPersistRoot(nil) returned a path")
	}
	if persist, folder := gatewayRoute.NamedFactoryActivationPaths(nil); persist != "" || folder != "" {
		t.Fatalf("NamedFactoryActivationPaths(nil) = %q, %q", persist, folder)
	}
}

type forwardingDefinitionHostStub struct {
	session       *factorydefinitions.DefinitionSession
	persistRoot   string
	workflowID    string
	validateErr   error
	replaceErr    error
	snapshot      *factorydefinitions.FactorySnapshot
	replaceResult *factorydefinitions.FactorySplitLayoutReplaceResult
}

func (s forwardingDefinitionHostStub) PersistRootDir() string {
	return s.persistRoot
}

func (forwardingDefinitionHostStub) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}

func (forwardingDefinitionHostStub) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return nil
}

func (s forwardingDefinitionHostStub) WorkflowID() string {
	return s.workflowID
}

func (s forwardingDefinitionHostStub) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return s.session, nil
}

func (forwardingDefinitionHostStub) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	return nil, nil
}

func (s forwardingDefinitionHostStub) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	if session == nil {
		return ""
	}
	return s.persistRoot + "/" + session.ID
}

func (s forwardingDefinitionHostStub) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	return s.validateErr
}

func (s forwardingDefinitionHostStub) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return s.snapshot, nil
}

func (s forwardingDefinitionHostStub) ReplaceFactoryLayoutAtDir(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return s.replaceResult, s.replaceErr
}

type forwardingDefinitionGatewayStub struct {
	session       *factorydefinitions.DefinitionSession
	runSessionID  string
	persistRoot   string
	activatePaths [2]string
	saveNow       time.Time
	idleErr       error
	activateErr   error
	swapErr       error
}

func (s forwardingDefinitionGatewayStub) RunSessionID() string { return s.runSessionID }

func (s forwardingDefinitionGatewayStub) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return s.session
}

func (s forwardingDefinitionGatewayStub) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return s.session, nil
}

func (s forwardingDefinitionGatewayStub) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	if session == nil {
		return ""
	}
	return s.persistRoot + "/" + session.ID
}

func (s forwardingDefinitionGatewayStub) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return s.activatePaths[0], s.activatePaths[1]
}

func (s forwardingDefinitionGatewayStub) SaveNow() time.Time { return s.saveNow }

func (forwardingDefinitionGatewayStub) WithActivationLock(fn func() error) error { return fn() }

func (s forwardingDefinitionGatewayStub) RequireIdleRuntimeForSession(context.Context, string) error {
	return s.idleErr
}

func (s forwardingDefinitionGatewayStub) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error {
	return s.idleErr
}

func (s forwardingDefinitionGatewayStub) ActivateSessionEditableFactory(context.Context, *factorydefinitions.DefinitionSession, string, string, string, string, string) error {
	return s.activateErr
}

func (s forwardingDefinitionGatewayStub) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorydefinitions.DefinitionSession, string, string, string, string) error {
	return s.swapErr
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
