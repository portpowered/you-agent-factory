//go:build functionallong

package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceLifecycle_InitInputCompletesThroughFactoryService(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle init-input sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	writeServiceLifecycleSeed(t, dir, "work-1.json", `{"title": "integration test"}`)

	service := startFunctionalService(t, dir)
	assertSingleCompletedTask(t, service)
}

func TestServiceLifecycle_WorkFileSubmissionCompletesTwoStagePipeline(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle work-file sweep")
	dir := support.ScaffoldFactory(t, twoStageServicePipelineConfig())
	workFilePath := filepath.Join(dir, "initial-work.json")
	support.WriteWorkRequestFile(t, workFilePath, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title": "work-file test"}`),
	})

	service := startFunctionalService(t, dir, "--work", workFilePath)
	assertSingleCompletedTask(t, service)
}

func TestServiceLifecycle_PreseededWorkCompletesOnStartup(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle preseeded-startup sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	writeServiceLifecycleSeed(t, dir, "preseed-work.json", `{"title": "preseed test"}`)

	service := startFunctionalService(t, dir)
	assertSingleCompletedTask(t, service)
}

func TestServiceLifecycle_EmptyPreseedDirectoryRemainsIdle(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle empty-preseed sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	if err := os.MkdirAll(
		filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName),
		0o755,
	); err != nil {
		t.Fatalf("create inputs directory: %v", err)
	}

	service := startFunctionalService(t, dir)
	status := support.WaitForRuntimeIdle(t, service.url, 5*time.Second)
	if status.TotalTokens != 0 {
		t.Fatalf("GET /status totalTokens = %d, want 0", status.TotalTokens)
	}
	if work := support.ListDefaultSessionWork(t, service.url); len(work.Results) != 0 {
		t.Fatalf("GET /work result count = %d, want 0", len(work.Results))
	}
}

func TestServiceLifecycle_TerminalStatusSignalsForSeededWatcherInput(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle terminal-status sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	writeServiceLifecycleSeed(t, dir, "seed-work.json", `{"title": "wait-to-complete demo"}`)

	service := startFunctionalService(t, dir)
	assertSingleCompletedTask(t, service)
}

func TestServiceLifecycle_CopyFixtureDirParallelCopiesStayIsolated(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle fixture-copy isolation sweep")
	srcDir := createParallelIsolationSourceFixture(t)

	for _, tc := range []struct {
		name    string
		payload string
	}{
		{name: "subtest-alpha", payload: `{"title":"alpha-work"}`},
		{name: "subtest-beta", payload: `{"title":"beta-work"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, srcDir)
			writeServiceLifecycleSeed(t, dir, "seed.json", tc.payload)
			assertSingleCompletedTask(t, startFunctionalService(t, dir))
		})
	}
}

type functionalServiceProcess struct {
	process support.Process
	server  *support.ProcessAPIServer
	daemon  *support.ProcessCommand
	url     string
}

func startFunctionalService(t *testing.T, dir string, extraArgs ...string) *functionalServiceProcess {
	t.Helper()
	support.SetWorkingDirectory(t, t.TempDir())
	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: server.Start,
	})
	args := []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--with-mock-workers",
		"--quiet",
		"--no-record",
	}
	args = append(args, extraArgs...)
	inputs := support.FakeInputs(t.Context(), args)
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	url := server.WaitForURL(t)
	support.WaitForRuntimeIdle(t, url, 5*time.Second)
	return &functionalServiceProcess{process: process, server: server, daemon: daemon, url: url}
}

func assertSingleCompletedTask(t *testing.T, service *functionalServiceProcess) {
	t.Helper()
	status := support.WaitForStatus(t, service.url, 10*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.TotalTokens == 1 && status.Categories.Terminal == 1
	})
	if status.Categories.Failed != 0 {
		t.Fatalf("GET /status failed count = %d, want 0", status.Categories.Failed)
	}
	work := support.ListDefaultSessionWork(t, service.url)
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	state := work.Results[0].State
	if state == nil || state.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want TERMINAL", state)
	}
}

func writeServiceLifecycleSeed(t *testing.T, dir, name, payload string) {
	t.Helper()
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, name), []byte(payload), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
}

func createParallelIsolationSourceFixture(t *testing.T) string {
	t.Helper()
	srcDir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	support.WriteAgentConfig(t, srcDir, "worker-a", "---\ntype: MODEL_WORKER\nstopToken: COMPLETE\n---\nProcess work.\n")
	return srcDir
}
