package service_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

const (
	canonicalScriptPollerWorkstationName = "linear-ingress"
	canonicalScriptPollerWorkerName      = "poller-script"
	canonicalScriptPollerCommand         = "factory/scripts/poller.sh"
)

type recordingSubmitter struct {
	calls       int
	submissions []interfaces.WorkRequest
}

func (r *recordingSubmitter) submit(_ context.Context, request interfaces.WorkRequest) error {
	r.calls++
	r.submissions = append(r.submissions, request)
	return nil
}

func TestRunScriptPoller_SubmitsCanonicalWorkRequestStdout(t *testing.T) {
	factoryDir := t.TempDir()
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-123","workTypeName":"task","payload":{"id":"ISSUE-123"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &recordingSubmitter{}
	svc := workersservice.New(workersservice.Config{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v, want unexpected exit after successful submit", err)
	}
	if submitted.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitted.calls)
	}
	if len(submitted.submissions) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submitted.submissions))
	}
	if submitted.submissions[0].RequestID != "linear-issue-batch-1" {
		t.Fatalf("submitted request ID = %q, want linear-issue-batch-1", submitted.submissions[0].RequestID)
	}
	if submitted.submissions[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("submitted request type = %q, want FACTORY_REQUEST_BATCH", submitted.submissions[0].Type)
	}
}

func TestRunScriptPoller_SubmitsSubmitStyleRecordsStdout(t *testing.T) {
	factoryDir := t.TempDir()
	envelopeJSON := []byte(`{
		"submissions":[
			{
				"requestId":"linear-issue-batch-2",
				"workId":"linear-issue-124",
				"name":"issue-124",
				"workTypeName":"task",
				"traceId":"trace-124"
			}
		]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: envelopeJSON}}},
	}
	submitted := &recordingSubmitter{}
	svc := workersservice.New(workersservice.Config{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v, want unexpected exit after successful submit", err)
	}
	if len(submitted.submissions) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submitted.submissions))
	}
	workRequest := submitted.submissions[0]
	if workRequest.RequestID != "linear-issue-batch-2" {
		t.Fatalf("submitted request ID = %q, want linear-issue-batch-2", workRequest.RequestID)
	}
	if workRequest.Works == nil || len(workRequest.Works) != 1 {
		t.Fatalf("submitted works = %#v, want one canonical work item", workRequest.Works)
	}
	if workRequest.Works[0].WorkID != "linear-issue-124" {
		t.Fatalf("submitted work ID = %q, want linear-issue-124", workRequest.Works[0].WorkID)
	}
}

func TestScriptPollerCommandRequest_DefaultsEmptyWorkingDirectoryToRuntimeBaseDirectory(t *testing.T) {
	factoryDir := t.TempDir()
	runtimeBaseDir := t.TempDir()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller: newCanonicalScriptPollerWorkstation(),
		},
	)
	runtimeCfg.SetRuntimeBaseDir(runtimeBaseDir)

	req, err := workersservice.ScriptPollerCommandRequest(
		runtimeCfg,
		newCanonicalScriptPollerWorkstation(),
		newCanonicalScriptPollerWorker("--mode", "watch"),
	)
	if err != nil {
		t.Fatalf("ScriptPollerCommandRequest: %v", err)
	}
	if req.WorkDir != runtimeBaseDir {
		t.Fatalf("poller workdir = %q, want %q", req.WorkDir, runtimeBaseDir)
	}
	if req.Command != filepath.Join(factoryDir, "scripts", "poller.sh") {
		t.Fatalf("poller command = %q, want resolved factory script path", req.Command)
	}
}

func TestParseScriptPollerOutput_RejectsUnsupportedRawFactoryEvents(t *testing.T) {
	rawEventJSON, err := json.Marshal(map[string]any{
		"events": []map[string]any{{
			"type": "WORK_REQUEST",
		}},
	})
	if err != nil {
		t.Fatalf("marshal raw event payload: %v", err)
	}

	_, hasOutput, parseErr := workersservice.ParseScriptPollerOutput(rawEventJSON)
	if !hasOutput {
		t.Fatal("expected raw event payload to count as poller output")
	}
	if parseErr == nil || !strings.Contains(parseErr.Error(), "unsupported raw factory events") {
		t.Fatalf("parse error = %v, want unsupported raw factory events", parseErr)
	}
}

func TestParseScriptPollerOutput_RejectsMalformedStdout(t *testing.T) {
	_, hasOutput, err := workersservice.ParseScriptPollerOutput([]byte("submitted work\n"))
	if !hasOutput {
		t.Fatal("expected non-empty stdout to count as poller output")
	}
	if err == nil {
		t.Fatal("expected malformed stdout error")
	}
	if !strings.Contains(err.Error(), "malformed stdout") {
		t.Fatalf("error = %v, want malformed stdout", err)
	}
}

func newCanonicalScriptPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           canonicalScriptPollerWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
}

func newCanonicalScriptPollerWorker(args ...string) *interfaces.WorkerConfig {
	return &interfaces.WorkerConfig{
		Name:    canonicalScriptPollerWorkerName,
		Type:    interfaces.WorkerTypeScript,
		Command: canonicalScriptPollerCommand,
		Args:    args,
	}
}

type scriptPollerRuntimeConfigOptions struct {
	poller       interfaces.FactoryWorkstationConfig
	pollerWorker *interfaces.WorkerConfig
}

func newScriptPollerLoadedRuntimeConfig(
	t *testing.T,
	factoryDir string,
	options scriptPollerRuntimeConfigOptions,
) *config.LoadedFactoryConfig {
	t.Helper()

	poller := options.poller
	if poller.Name == "" {
		poller = newCanonicalScriptPollerWorkstation()
	}
	pollerWorker := options.pollerWorker
	if pollerWorker == nil {
		pollerWorker = newCanonicalScriptPollerWorker()
	}

	factoryCfg := &interfaces.FactoryConfig{
		Workers:      []interfaces.WorkerConfig{{Name: pollerWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	workerConfigs := map[string]*interfaces.WorkerConfig{
		pollerWorker.Name: pollerWorker,
	}
	workstationConfigs := map[string]*interfaces.FactoryWorkstationConfig{
		poller.Name: &poller,
	}

	loaded, err := config.NewLoadedFactoryConfig(factoryDir, factoryCfg, runtimefixtures.RuntimeDefinitionLookupFixture{
		Workers:      workerConfigs,
		Workstations: workstationConfigs,
	})
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

type pollerRunOutcome struct {
	result        workers.CommandResult
	err           error
	waitForCancel bool
}

type pollerSequenceCommandRunner struct {
	calls    int
	reqs     []workers.CommandRequest
	outcomes []pollerRunOutcome
}

func (r *pollerSequenceCommandRunner) Run(ctx context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.calls++
	r.reqs = append(r.reqs, req)
	index := r.calls - 1
	var outcome pollerRunOutcome
	if index < len(r.outcomes) {
		outcome = r.outcomes[index]
	} else if len(r.outcomes) > 0 {
		outcome = r.outcomes[len(r.outcomes)-1]
	}

	if outcome.waitForCancel {
		<-ctx.Done()
		return outcome.result, ctx.Err()
	}
	return outcome.result, outcome.err
}
