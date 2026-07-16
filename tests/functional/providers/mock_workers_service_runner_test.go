package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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

			harness := testutil.NewServiceTestHarness(t, dir,
				testutil.WithMockWorkersConfig(config.NewEmptyMockWorkersConfig()),
				testutil.WithRuntimeLogDir(logDir),
				testutil.WithRuntimeInstanceID(runtimeID),
				testutil.WithRuntimeFileLoggingEnabled(true),
			)
			harness.RunUntilComplete(t, 5*time.Second)
			harness.Assert().PlaceTokenCount(tt.donePlace, 1)

			record := findRuntimeLogRecord(t, requireRuntimeLogPath(t, logDir, runtimeID), workers.WorkLogEventCommandRunnerCompleted)
			if _, ok := record["stdout"]; ok {
				t.Fatalf("command runner completion should omit stdout on success")
			}
			if _, ok := record["stderr"]; ok {
				t.Fatalf("command runner completion should omit stderr on success")
			}
		})
	}
}
