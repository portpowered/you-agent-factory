package execution_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type fscp03StartedSession struct {
	root   string
	result factorysessions.SessionStartResult
	err    error
}

func TestFSCP03OneDurableOwnerAcrossProjectRootsAndRestart(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)
	home := t.TempDir()
	firstDir := scaffoldFSCP03ProbeFactory(t)
	secondDir := scaffoldFSCP03ProbeFactory(t)
	if firstDir == secondDir {
		t.Fatal("test Factory roots are not distinct")
	}
	edges := serviceedges.Edges{
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
		BrowserOpener:                      func(context.Context, string) error { return nil },
		ProviderCommandRunner:              support.NewStaticSuccessCommandRunner("multi-root durable COMPLETE"),
	}
	process, err := root.BuildProcess(t.Context(), edges)
	if err != nil {
		t.Fatalf("root.BuildProcess: %v", err)
	}
	support.CleanupProcess(t, process)
	canonical, ok := process.FactorySessions().FactorySessions().(factorysessions.Service)
	if !ok {
		t.Fatal("process Factory Sessions capability is unavailable")
	}
	outcomes := make([]fscp03StartedSession, 2)
	roots := []string{firstDir, secondDir}
	var started sync.WaitGroup
	for index, projectRoot := range roots {
		started.Add(1)
		go func() {
			defer started.Done()
			requestID := "fscp03-multi-root-first"
			if index == 1 {
				requestID = "fscp03-multi-root-second"
			}
			outcomes[index].root = projectRoot
			outcomes[index].result, outcomes[index].err = canonical.Start(t.Context(), factorysessions.SessionStartRequest{
				Mode: factorysessions.SessionOperationModeDurable, FolderPath: projectRoot,
				Persistence: factorysessions.PersistencePolicyEnabled,
				Correlation: factorysessions.SessionOperationCorrelation{RequestID: requestID},
				Source:      fscp03ChildSource(requestID), Synchronous: true,
				RuntimeSelection: fscp03RuntimeSelections(projectRoot, home),
			})
		}()
	}
	started.Wait()
	assertFSCP03ProjectScopedSnapshots(t, outcomes)
	if outcomes[0].result.SessionID == outcomes[1].result.SessionID {
		t.Fatal("distinct project roots shared durable SessionID")
	}
	if err := process.Close(t.Context()); err != nil {
		t.Fatalf("first process Close: %v", err)
	}
	restarted, err := root.BuildProcess(t.Context(), edges)
	if err != nil {
		t.Fatalf("restart root.BuildProcess: %v", err)
	}
	support.CleanupProcess(t, restarted)
	restartedSessions, ok := restarted.FactorySessions().FactorySessions().(factorysessions.Service)
	if !ok {
		t.Fatal("restarted process Factory Sessions capability is unavailable")
	}
	for _, outcome := range outcomes {
		selected, err := restartedSessions.Start(t.Context(), factorysessions.SessionStartRequest{
			Mode: factorysessions.SessionOperationModeLive, FolderPath: outcome.root,
			ActivationOnly: true, RuntimeSelection: fscp03RuntimeSelections(outcome.root, home),
		})
		if err != nil {
			t.Fatalf("select current Factory %q after restart: %v", outcome.root, err)
		}
		got, err := restartedSessions.Get(t.Context(), factorysessions.SessionGetRequest{
			SessionID: outcome.result.SessionID, Mode: factorysessions.SessionOperationModeDurable,
		})
		if err != nil || got.Session.SessionID != outcome.result.SessionID || got.Session.Status != string(factorysessions.LifecycleStatusSucceeded) {
			t.Fatalf("Get after restart (%q) = %#v, error = %v", outcome.root, got, err)
		}
		if _, err := restartedSessions.Control(t.Context(), factorysessions.SessionControlRequest{
			SessionID: selected.SessionID, Mode: factorysessions.SessionOperationModeLive,
			Operation: factorysessions.SessionControlClose,
		}); err != nil {
			t.Fatalf("close current Factory %q: %v", outcome.root, err)
		}
	}
}

func assertFSCP03ProjectScopedSnapshots(t *testing.T, outcomes []fscp03StartedSession) {
	t.Helper()
	for _, outcome := range outcomes {
		if outcome.err != nil || outcome.result.SessionID == "" || outcome.result.Status != string(factorysessions.LifecycleStatusSucceeded) {
			t.Fatalf("durable Start(%q) = %#v, error = %v", outcome.root, outcome.result, outcome.err)
		}
		snapshotPath := filepath.Join(outcome.root, ".you-agent-factory", "durable-sessions", outcome.result.SessionID+".json")
		encoded, err := os.ReadFile(snapshotPath)
		if err != nil {
			t.Fatalf("read project-scoped snapshot %q: %v", snapshotPath, err)
		}
		var snapshot struct{ ProjectRoot string }
		if err := json.Unmarshal(encoded, &snapshot); err != nil {
			t.Fatalf("decode snapshot %q: %v", snapshotPath, err)
		}
		if snapshot.ProjectRoot != outcome.root {
			t.Fatalf("snapshot %q project root = %q, want %q", snapshotPath, snapshot.ProjectRoot, outcome.root)
		}
	}
}
