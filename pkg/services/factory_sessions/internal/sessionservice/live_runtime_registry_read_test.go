package service_test

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestLiveControlCapability_OpenListReadPreservesCanonicalIdentity(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-live-control-discovery"
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		FactoryDir: "/tmp/factory",
		FolderPath: "/tmp",
		Project:    "demo",
	}
	session := &livesession.LiveSession{
		ID: sessionID,
		SessionState: livesession.SessionState{
			FactoryDir: target.FactoryDir,
			FolderPath: target.FolderPath,
		},
		Project: target.Project,
		Target:  target.Ref,
	}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			targets:        []factorysessions.Target{target},
			openSessionID:  sessionID,
			requireSession: session,
			sessionIDs:     []string{sessionID},
			sessions:       map[string]*livesession.LiveSession{sessionID: session},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	// The client receives only the owner-published capability. All lifecycle
	// interactions below must stay inside that narrow boundary.
	var client factorysessions.LiveControlService = gateway
	ctx := context.Background()

	opened, err := client.OpenFactorySession(ctx, factorysessions.LiveControlOpenRequest{
		FolderPath: target.FolderPath,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if opened == nil || opened.SessionID != sessionID {
		t.Fatalf("open result = %#v, want session id %q", opened, sessionID)
	}
	if opened.Session == nil || opened.Session.ID != sessionID || opened.Session.Target != target.Ref {
		t.Fatalf("open session summary = %#v, want canonical identity and target %#v", opened.Session, target.Ref)
	}

	listed, err := client.ListFactorySessions(ctx)
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListFactorySessions count = %d, want 1", len(listed))
	}
	if listed[0].Context.FactorySessionID != opened.SessionID ||
		listed[0].Context.Session == nil || listed[0].Context.Session.ID != opened.SessionID {
		t.Fatalf("listed projection = %#v, want canonical identity %q", listed[0], opened.SessionID)
	}

	read, err := client.GetFactorySession(ctx, opened.SessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if read.Context.FactorySessionID != opened.SessionID ||
		read.Context.Session == nil || read.Context.Session.ID != opened.SessionID {
		t.Fatalf("read projection = %#v, want canonical identity %q", read, opened.SessionID)
	}

	_, err = client.GetFactorySession(ctx, "missing-live-session")
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("GetFactorySession missing = %v, want errors.Is ErrSessionNotFound", err)
	}
}

func TestService_LiveOpenRecordsCanonicalIdentityThroughLiveRuntimeOwner(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-open-owner"
	session := &livesession.LiveSession{
		ID: sessionID,
		SessionState: livesession.SessionState{
			FactoryDir: "/tmp/factory",
			FolderPath: "/tmp",
		},
		Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			targets: []factorysessions.Target{{
				Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
				FactoryDir: "/tmp/factory",
				FolderPath: "/tmp",
			}},
			openSessionID:  sessionID,
			requireSession: session,
			sessions:       map[string]*livesession.LiveSession{sessionID: session},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	result, err := gateway.OpenFactorySessionFromFolder(context.Background(), "/tmp", nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	if result == nil || result.SessionID != sessionID {
		t.Fatalf("open result = %#v, want %q", result, sessionID)
	}
	if result.Session == nil || result.Session.ID != sessionID {
		t.Fatalf("open session summary = %#v, want %q", result.Session, sessionID)
	}
	if host.openCalls != 1 {
		t.Fatalf("open calls = %d, want 1", host.openCalls)
	}
}

func TestService_LiveListGetReturnOrderedDetachedProjectionsWithoutRegistryMutation(t *testing.T) {
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
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessionIDs: []string{"beta", factorysessions.DefaultSessionID},
			sessions: map[string]*livesession.LiveSession{
				factorysessions.DefaultSessionID: defaultSession,
				"beta":                           namedSession,
			},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	ctx := context.Background()

	listed, err := gateway.ListFactorySessions(ctx)
	if err != nil || len(listed) != 2 {
		t.Fatalf("ListFactorySessions = (%#v, %v), want two sessions", listed, err)
	}
	if session := listed[0].Context.Session; !session.IsDefault || session.ID != factorysessions.DefaultSessionID {
		t.Fatalf("first listed session = %#v, want default first", session)
	}
	listed[0].Context.Session.FactoryDir = "/mutated"
	if defaultSession.FactoryDir != "/tmp/default" {
		t.Fatal("list projection mutation changed registry session state")
	}

	got, err := gateway.GetFactorySession(ctx, "beta")
	if err != nil || got.Context.Session.ID != "beta" {
		t.Fatalf("GetFactorySession = (%#v, %v), want beta", got, err)
	}
	got.Context.Session.FactoryDir = "/mutated-beta"
	if namedSession.FactoryDir != "/tmp/beta" {
		t.Fatal("get projection mutation changed registry session state")
	}
	if host.openCalls != 0 || host.stopCalls != 0 {
		t.Fatalf("read paths mutated registry: open=%d stop=%d", host.openCalls, host.stopCalls)
	}
}

func TestService_ResolveFactorySession_IdempotentSelectionReturnsSameIdentity(t *testing.T) {
	t.Parallel()

	session := &livesession.LiveSession{ID: "sess-idempotent"}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions: map[string]*livesession.LiveSession{session.ID: session},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	first := gateway.ResolveFactorySession(session.ID)
	second := gateway.ResolveFactorySession(session.ID)
	if first == nil || second == nil || first.ID != session.ID || second.ID != session.ID {
		t.Fatalf("ResolveFactorySession = (%#v, %#v), want stable %q", first, second, session.ID)
	}
	if first != second {
		t.Fatal("ResolveFactorySession returned different handles for the same active session")
	}
	if host.openCalls != 0 {
		t.Fatalf("resolve invoked open %d times, want none", host.openCalls)
	}
}

func TestService_GetFactorySession_ReturnsTypedNotFoundWithoutMutatingRegistry(t *testing.T) {
	t.Parallel()

	remaining := &livesession.LiveSession{ID: "sess-remaining"}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions: map[string]*livesession.LiveSession{remaining.ID: remaining},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	ctx := context.Background()

	_, err := gateway.GetFactorySession(ctx, "missing")
	if err == nil || !errors.Is(err, factorysessions.ErrNotFound) {
		t.Fatalf("GetFactorySession error = %v, want ErrNotFound", err)
	}
	if host.stopCalls != 0 {
		t.Fatalf("not-found get stopped %d sessions, want none", host.stopCalls)
	}
	if got, getErr := gateway.GetFactorySession(ctx, remaining.ID); getErr != nil || got.Context.Session.ID != remaining.ID {
		t.Fatalf("remaining session Get = (%#v, %v), want sess-remaining", got, getErr)
	}
}
