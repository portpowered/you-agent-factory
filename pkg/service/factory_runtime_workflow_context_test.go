package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestRuntimeWorkflowContext_SetsSessionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{name: "default session", sessionID: factorysessions.DefaultSessionID, want: factorysessions.DefaultSessionID},
		{name: "named session", sessionID: "session-beta", want: "session-beta"},
		{name: "blank session falls back to default", sessionID: "   ", want: factorysessions.DefaultSessionID},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wfCtx := runtimeWorkflowContext(&interfaces.FactoryConfig{}, tc.sessionID)
			if wfCtx == nil {
				t.Fatal("workflow context = nil")
			}
			if wfCtx.SessionID != tc.want {
				t.Fatalf("SessionID = %q, want %q", wfCtx.SessionID, tc.want)
			}
		})
	}
}

func TestBuildReplacementFactoryRuntime_WiresWorkflowContextSessionID(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	defaultBundle, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime(default): %v", err)
	}
	defaultCtx := runtime.WorkflowContext(defaultBundle.factory)
	if defaultCtx == nil || defaultCtx.SessionID != factorysessions.DefaultSessionID {
		t.Fatalf("default workflow context = %#v, want SessionID %q", defaultCtx, factorysessions.DefaultSessionID)
	}

	namedBundle, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, "session-beta")
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime(named): %v", err)
	}
	namedCtx := runtime.WorkflowContext(namedBundle.factory)
	if namedCtx == nil || namedCtx.SessionID != "session-beta" {
		t.Fatalf("named workflow context = %#v, want SessionID %q", namedCtx, "session-beta")
	}
}
