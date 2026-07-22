package providers

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker service-runner sweep")
	support.SetWorkingDirectory(t, t.TempDir())
	for _, test := range []struct {
		name     string
		fixture  string
		workType string
	}{
		{name: "model worker", fixture: "executor_success", workType: "task"},
		{name: "script worker", fixture: "script_executor_dir", workType: "task"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, test.fixture))
			testutil.WriteSeedFile(t, dir, test.workType, []byte("mock-worker service payload"))
			logDir := t.TempDir()

			server := support.NewProcessAPIServer()
			process := support.BuildProcess(t, serviceedges.Edges{
				APIServerStarter: server.Start,
			})
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "run",
				"--dir", dir,
				"--continuously",
				"--server", "http://127.0.0.1:1",
				"--with-mock-workers",
				"--runtime-log-dir", logDir,
				"--quiet",
				"--no-record",
			})
			daemon := support.StartProcessCommand(t, process, inputs.Input)
			defer daemon.Stop(t)
			baseURL := server.WaitForURL(t)

			status := support.WaitForStatus(t, baseURL, 5*time.Second, func(status factoryapi.StatusResponse) bool {
				return status.TotalTokens == 1 && status.Categories.Terminal == 1
			})
			if status.Categories.Failed != 0 {
				t.Fatalf("GET /status failed count = %d, want 0", status.Categories.Failed)
			}
			work := support.ListDefaultSessionWork(t, baseURL)
			if len(work.Results) != 1 || work.Results[0].State == nil ||
				work.Results[0].State.Type != factoryapi.WorkStateTypeTERMINAL {
				t.Fatalf("GET /work results = %#v, want one terminal work", work.Results)
			}

			record := findRuntimeLogRecord(
				t,
				requireOnlyRuntimeLogPath(t, logDir),
				commandRunnerCompletedLogEvent,
			)
			if _, ok := record["stdout"]; ok {
				t.Fatal("command runner completion should omit stdout on success")
			}
			if _, ok := record["stderr"]; ok {
				t.Fatal("command runner completion should omit stderr on success")
			}
		})
	}
}

func requireOnlyRuntimeLogPath(t *testing.T, logDir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*", "*-runtime-log-*.log"))
	if err != nil {
		t.Fatalf("glob runtime log path: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("runtime log paths under %s = %v, want exactly one", logDir, matches)
	}
	return matches[0]
}
