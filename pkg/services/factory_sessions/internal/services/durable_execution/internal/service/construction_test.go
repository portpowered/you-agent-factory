package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

func TestNewDurable_DefaultPolicyDoesNotCreateProjectDurableSessions(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	storeCalls := 0
	_, err := NewDurable(
		projectRoot,
		factorysessions.PersistencePolicy(""),
		countingProjectPersistenceStoreFactory(&storeCalls),
		nil,
		restartClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		restartSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		nil,
		factoryruntime.JavaScriptWorkerSettings{},
		restartRecordingWriter{},
		func() string { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDurable(default policy): %v", err)
	}
	if storeCalls != 0 {
		t.Fatalf("default policy store factory calls = %d, want 0", storeCalls)
	}
	if _, err := os.Stat(runtimepersist.DirForProjectRoot(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("durable persistence path stat error = %v, want not exist", err)
	}
}

func TestNewDurable_EnabledPolicyPersistsProjectDurableSessions(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	owner, err := NewDurable(
		projectRoot,
		factorysessions.PersistencePolicyEnabled,
		projectPersistenceStoreFactory(),
		nil,
		restartClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		restartSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
			LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
		},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		nil,
		factoryruntime.JavaScriptWorkerSettings{},
		restartRecordingWriter{},
		func() string { return "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" },
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDurable(enabled policy): %v", err)
	}

	started, err := owner.StartSync(context.Background(), factorysessions.DurableStartRequest{
		RequestID: "req-construction-enabled-persistence-001",
		Source: factorysessions.Source{
			Kind: factoryruntime.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &factorysessions.InlineWorkflowSource{
				Dialect:      "you-workflow-v1",
				InlineSource: `return { ok: true };`,
			},
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), started.SessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("enabled persistence snapshot stat error = %v, want exist", err)
	}
}

func TestNewStandalone_DoesNotCreateProjectDurableSessions(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	storeCalls := 0
	_, err := NewStandalone(
		factorysessions.ExecutionProviderJavaScriptRuntime,
		projectRoot,
		countingProjectPersistenceStoreFactory(&storeCalls),
		"",
		factorysessionexecution.ChildExecutorModeFake,
		nil,
		restartClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		restartSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		restartRecordingWriter{},
		func() string { return "cccccccccccccccccccccccccccccccc" },
		nil,
	)
	if err != nil {
		t.Fatalf("NewStandalone: %v", err)
	}
	if storeCalls != 0 {
		t.Fatalf("standalone store factory calls = %d, want 0", storeCalls)
	}
	if _, err := os.Stat(runtimepersist.DirForProjectRoot(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("durable persistence path stat error = %v, want not exist", err)
	}
}

func projectPersistenceStoreFactory() roles.RuntimePersistenceStoreFactory {
	return countingProjectPersistenceStoreFactory(nil)
}

func countingProjectPersistenceStoreFactory(calls *int) roles.RuntimePersistenceStoreFactory {
	return func(projectRoot string) (roles.RuntimePersistenceStore, error) {
		if calls != nil {
			*calls++
		}
		store, err := runtimepersist.NewDirectoryStore(
			runtimepersist.DirForProjectRoot(projectRoot),
			platformfilesystem.Local{},
		)
		if err != nil {
			return nil, err
		}
		return store, nil
	}
}
