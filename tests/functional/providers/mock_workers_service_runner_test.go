package providers

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker service-runner sweep")
	tests := []struct {
		name      string
		fixture   string
		workType  string
		donePlace string
	}{
		{
			name:      "model worker",
			fixture:   "executor_success",
			workType:  "task",
			donePlace: "task:done",
		},
		{
			name:      "script worker",
			fixture:   "script_executor_dir",
			workType:  "task",
			donePlace: "task:done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, tt.fixture))
			testutil.WriteSeedFile(t, dir, tt.workType, []byte("mock-worker service payload"))
			logDir := t.TempDir()
			runtimeID := strings.ReplaceAll(tt.name, " ", "-")

			svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
				Dir:               dir,
				MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
				Logger:            zap.NewNop(),
				RuntimeLogDir:     logDir,
				RuntimeInstanceID: runtimeID,
			})
			if err != nil {
				t.Fatalf("BuildFactoryService: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := svc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("Run: %v", err)
			}

			snapshot, err := svc.GetEngineStateSnapshot(context.Background())
			if err != nil {
				t.Fatalf("GetEngineStateSnapshot: %v", err)
			}
			if got := len(snapshot.Marking.TokensInPlace(tt.donePlace)); got != 1 {
				t.Fatalf("%s token count = %d, want 1", tt.donePlace, got)
			}
			if len(snapshot.DispatchHistory) != 1 {
				t.Fatalf("DispatchHistory count = %d, want 1", len(snapshot.DispatchHistory))
			}
			if snapshot.DispatchHistory[0].Outcome != interfaces.OutcomeAccepted {
				t.Fatalf("dispatch outcome = %s, want %s", snapshot.DispatchHistory[0].Outcome, interfaces.OutcomeAccepted)
			}

			record := findRuntimeLogRecord(t, filepath.Join(logDir, runtimeID+".log"), workers.WorkLogEventCommandRunnerCompleted)
			if _, ok := record["stdout"]; ok {
				t.Fatalf("command runner completion should omit stdout on success")
			}
			if _, ok := record["stderr"]; ok {
				t.Fatalf("command runner completion should omit stderr on success")
			}
		})
	}
}
