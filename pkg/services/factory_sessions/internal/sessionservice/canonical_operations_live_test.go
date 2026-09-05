package service_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

func TestService_CanonicalStartLiveReturnsOpaqueProjection(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "demo"},
			Label:      "Demo",
			FolderPath: "/workspace",
			FactoryDir: "/workspace/demo",
			Project:    "demo",
		}},
		openSessionID: "live-1",
		requireSession: &livesession.LiveSession{
			ID: "live-1",
			SessionState: livesession.SessionState{
				FactoryDir: "/workspace/demo",
				FolderPath: "/workspace",
			},
			Project: "demo",
			Target:  factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "demo"},
		},
	}
	gateway := newServiceTestGateway(host)

	got, err := gateway.Start(context.Background(), factorysessions.SessionStartRequest{
		Mode:       factorysessions.SessionOperationModeLive,
		FolderPath: "  /workspace  ",
		Target:     &factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "demo"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got.SessionID != "live-1" || got.Mode != factorysessions.SessionOperationModeLive || got.Status != "OPENED" {
		t.Fatalf("Start() = %#v, want live opened identity/mode/status", got)
	}
	if got.Live == nil || got.Live.Session == nil {
		t.Fatalf("Start() live projection = %#v, want session projection", got.Live)
	}
	view := got.Live.Session
	if view.SessionID != "live-1" || view.Mode != factorysessions.SessionOperationModeLive || view.Status != "OPENED" || view.FactoryDir != "/workspace/demo" || view.FolderPath != "/workspace" || view.Project != "demo" || view.RuntimeAvailable {
		t.Fatalf("live SessionView = %#v, want runtime-free opened projection", view)
	}
	if len(got.Live.Targets) != 0 {
		t.Fatalf("live start targets = %#v, want no discovery payload after open", got.Live.Targets)
	}
}
