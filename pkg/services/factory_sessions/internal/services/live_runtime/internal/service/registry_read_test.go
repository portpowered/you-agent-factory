package service_test

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
)

func TestServiceOpenReturnsCanonicalSessionIdentity(t *testing.T) {
	t.Parallel()

	const canonicalID = "sess-open-canonical"
	dependencies := testDependencies()
	dependencies.OpenForTarget = func(_ context.Context, target factorysessions.Target) (string, error) {
		if target.Ref.Kind != factorysessions.TargetKindNamed || target.Ref.Name != "alpha" {
			t.Fatalf("target = %#v, want named alpha", target.Ref)
		}
		return canonicalID, nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	opened, err := service.OpenForTarget(context.Background(), factorysessions.Target{
		Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "alpha"},
	})
	if err != nil || opened != canonicalID {
		t.Fatalf("OpenForTarget = (%q, %v), want %q", opened, err, canonicalID)
	}
}

func TestServiceListReturnsOrderedDetachedProjectionsWithoutRegistryMutation(t *testing.T) {
	t.Parallel()

	defaultSession := &livesession.LiveSession{
		ID:        factorysessions.DefaultSessionID,
		IsDefault: true,
		SessionState: livesession.SessionState{
			FactoryDir: "/tmp/default",
		},
	}
	namedSession := &livesession.LiveSession{
		ID: "beta",
		SessionState: livesession.SessionState{
			FactoryDir: "/tmp/beta",
		},
	}
	sessions := map[string]*livesession.LiveSession{
		factorysessions.DefaultSessionID: defaultSession,
		"beta":                           namedSession,
	}
	openCalls := 0
	stopCalls := 0
	dependencies := testDependencies()
	dependencies.OpenForTarget = func(context.Context, factorysessions.Target) (string, error) {
		openCalls++
		return "", nil
	}
	dependencies.StopSession = func(string) error {
		stopCalls++
		return nil
	}
	dependencies.ListSessionIDs = func() []string {
		return []string{"beta", factorysessions.DefaultSessionID}
	}
	dependencies.GetSession = func(id string) *livesession.LiveSession { return sessions[id] }
	dependencies.RequireSession = func(id string) (*livesession.LiveSession, error) {
		if session := sessions[id]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrNotFound
	}
	dependencies.BuildProjectionContext = func(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
		return factorysessions.ProjectionContext{
			Session: &factorysessions.ScopedLiveSessionSummary{
				ID: livesession.CanonicalID(session), FactoryDir: session.FactoryDir,
				IsDefault: session.IsDefault,
			},
			FactorySessionID: livesession.CanonicalID(session),
		}, nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	listed, err := service.List(context.Background())
	if err != nil || len(listed) != 2 {
		t.Fatalf("List = (%#v, %v), want two sessions", listed, err)
	}
	if session := listed[0].Context.Session; !session.IsDefault || session.ID != factorysessions.DefaultSessionID {
		t.Fatalf("first listed session = %#v, want default first", session)
	}
	if session := listed[1].Context.Session; session.ID != "beta" {
		t.Fatalf("second listed session = %#v, want beta", session)
	}
	listed[0].Context.Session.FactoryDir = "/mutated"
	if sessions[factorysessions.DefaultSessionID].FactoryDir != "/tmp/default" {
		t.Fatal("list projection mutation changed registry session state")
	}

	got, err := service.Get(context.Background(), "beta")
	if err != nil || got.Context.Session.ID != "beta" {
		t.Fatalf("Get = (%#v, %v), want beta session", got, err)
	}
	got.Context.Session.FactoryDir = "/mutated-beta"
	if sessions["beta"].FactoryDir != "/tmp/beta" {
		t.Fatal("get projection mutation changed registry session state")
	}
	if openCalls != 0 || stopCalls != 0 {
		t.Fatalf("registry mutation effects: open=%d stop=%d, want none", openCalls, stopCalls)
	}
}

func TestServiceResolveReturnsAuthoritativeIdentityWithoutOpening(t *testing.T) {
	t.Parallel()

	session := &livesession.LiveSession{ID: "sess-resolve"}
	openCalls := 0
	dependencies := testDependencies()
	dependencies.OpenForTarget = func(context.Context, factorysessions.Target) (string, error) {
		openCalls++
		return "", nil
	}
	dependencies.GetSession = func(id string) *livesession.LiveSession {
		if id == session.ID {
			return session
		}
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	first := service.Resolve(session.ID)
	second := service.Resolve(session.ID)
	if first == nil || second == nil || first.ID != session.ID || second.ID != session.ID {
		t.Fatalf("Resolve = (%#v, %#v), want stable %q", first, second, session.ID)
	}
	if first != second {
		t.Fatal("Resolve returned different handles for the same active session")
	}
	if openCalls != 0 {
		t.Fatalf("resolve invoked open %d times, want none", openCalls)
	}
}

func TestServiceGetUnknownSessionReturnsTypedNotFoundWithoutMutatingRegistry(t *testing.T) {
	t.Parallel()

	remaining := &livesession.LiveSession{ID: "sess-remaining"}
	sessions := map[string]*livesession.LiveSession{"sess-remaining": remaining}
	stopCalls := 0
	dependencies := testDependencies()
	dependencies.StopSession = func(string) error {
		stopCalls++
		return nil
	}
	dependencies.GetSession = func(id string) *livesession.LiveSession { return sessions[id] }
	dependencies.RequireSession = func(id string) (*livesession.LiveSession, error) {
		if session := sessions[id]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrNotFound
	}
	dependencies.BuildProjectionContext = func(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
		return factorysessions.ProjectionContext{
			Session: &factorysessions.ScopedLiveSessionSummary{
				ID: livesession.CanonicalID(session),
			},
			FactorySessionID: livesession.CanonicalID(session),
		}, nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.Get(context.Background(), "missing")
	if err == nil || !errors.Is(err, factorysessions.ErrNotFound) {
		t.Fatalf("Get error = %v, want ErrNotFound", err)
	}
	if stopCalls != 0 {
		t.Fatalf("not-found get stopped %d sessions, want none", stopCalls)
	}
	if got, getErr := service.Get(context.Background(), remaining.ID); getErr != nil || got.Context.Session.ID != remaining.ID {
		t.Fatalf("remaining session Get = (%#v, %v), want sess-remaining", got, getErr)
	}
}
