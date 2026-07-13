package apiserver_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func drainAPILifecycleRuntimeSessions(t *testing.T, service factorysessionexecution.Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		list, err := service.ListSessions(context.Background(), factorysessionexecution.ListSessionsRequest{
			Scope: factorysessionexecution.SessionListScopeAll,
		})
		if err != nil {
			return
		}
		pending := false
		for _, session := range list.DurableSessions {
			if factorysessionexecution.IsTerminalLifecycleStatus(session.Status) {
				continue
			}
			pending = true
			_, _ = service.Terminate(context.Background(), session.SessionID, factorysessionexecution.ControlRequest{
				Reason: "test cleanup",
			})
		}
		if !pending {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func removeAPILifecycleProjectState(t *testing.T, projectRoot string) {
	t.Helper()
	runtimeStateRoot := filepath.Join(projectRoot, ".you-agent-factory")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := os.RemoveAll(runtimeStateRoot); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}
