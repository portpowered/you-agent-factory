package service

import (
	"context"
	"fmt"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
		factorysessions.ChildExecutorModeFake,
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

func TestNewDurable_DisabledPolicyDoesNotCreateProjectDurableSessions(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	startedOwner, err := NewDurable(
		projectRoot,
		factorysessions.PersistencePolicyDisabled,
		projectPersistenceStoreFactory(),
		factorysessions.ChildExecutorModeFake,
		restartClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		restartSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		factoryruntimefixtures.ScriptedJavaScriptWorkflows{},
		nil,
		factoryruntime.JavaScriptWorkerSettings{},
		restartRecordingWriter{},
		func() string { return "dddddddddddddddddddddddddddddddd" },
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDurable(disabled policy): %v", err)
	}

	_, err = startedOwner.StartSync(context.Background(), factorysessions.DurableStartRequest{
		RequestID:   "req-construction-disabled-persistence-001",
		ProjectRoot: projectRoot,
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
	if _, err := os.Stat(runtimepersist.DirForProjectRoot(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("disabled persistence path stat error = %v, want not exist", err)
	}
}

func TestNewDurable_EnabledPolicyPersistsRequestRoots(t *testing.T) {
	t.Parallel()
	roots := []string{t.TempDir(), t.TempDir()}
	var identities atomic.Uint32
	owner := newPersistingDurable(t, roots[0], func() string {
		return fmt.Sprintf("%032x", identities.Add(1))
	})
	for index, root := range roots {
		t.Run(fmt.Sprintf("root-%d", index), func(t *testing.T) {
			t.Parallel()
			started, err := owner.StartSync(context.Background(), factorysessions.DurableStartRequest{
				RequestID:   fmt.Sprintf("req-construction-persistence-%d", index),
				ProjectRoot: root,
				Source: factorysessions.Source{
					Kind: factoryruntime.WorkflowSourceKindInlineWorkflow,
					InlineWorkflow: &factorysessions.InlineWorkflowSource{
						Dialect: "you-workflow-v1", InlineSource: `return { ok: true };`,
					},
				},
			})
			if err != nil {
				t.Fatalf("StartSync: %v", err)
			}
			peerRoot := roots[1-index]
			if _, err := os.Stat(runtimepersist.SnapshotPathForProjectRoot(peerRoot, started.SessionID)); !os.IsNotExist(err) {
				t.Fatalf("peer snapshot stat = %v, want absent", err)
			}
			// A fresh owner reads persisted customer state instead of the live
			// owner's in-memory projection or the snapshot's file shape.
			restarted := newPersistingDurable(t, root, func() string { return "cccccccccccccccccccccccccccccccc" })
			got, err := restarted.GetSession(context.Background(), started.SessionID)
			if err != nil {
				t.Fatalf("GetSession after restart: %v", err)
			}
			if got.SessionID != started.SessionID || got.Status != factorysessions.LifecycleStatusSucceeded {
				t.Fatalf("restarted session = %+v, want completed %s", got, started.SessionID)
			}
		})
	}
}

func newPersistingDurable(t *testing.T, root string, generateID factorysessions.SessionIDGenerator) durableexecution.Service {
	t.Helper()
	owner, err := NewDurable(
		root,
		factorysessions.PersistencePolicyEnabled,
		projectPersistenceStoreFactory(),
		factorysessions.ChildExecutorModeFake,
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
		generateID,
		nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewDurable(enabled policy): %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := owner.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		}
	})
	return owner
}

func projectPersistenceStoreFactory() roles.RuntimePersistenceStoreFactory {
	return countingProjectPersistenceStoreFactory(nil)
}

func countingProjectPersistenceStoreFactory(calls *int) roles.RuntimePersistenceStoreFactory {
	return func(projectRoot string) (roles.RuntimePersistenceStore, error) {
		if calls != nil {
			*calls++
		}
		store, err := runtimepersist.NewLazyProjectStore(
			projectRoot,
			platformfilesystem.Local{},
		)
		if err != nil {
			return nil, err
		}
		return store, nil
	}
}

type restartClock struct {
	now time.Time
}

func (c restartClock) Now() time.Time { return c.now }

type restartSyncWaitScheduler struct{}

func (restartSyncWaitScheduler) Now() time.Time { return time.Now() }

func (restartSyncWaitScheduler) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}

type restartRecordingWriter struct{}

func (restartRecordingWriter) Write(path string, value recordings.PortableRecording) error {
	if err := recordings.ValidatePortableRecording(value); err != nil {
		return err
	}
	_ = path
	return nil
}
