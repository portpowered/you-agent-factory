package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	canonicalScriptPollerWorkstationName = "linear-ingress"
	canonicalScriptPollerWorkerName      = "poller-script"
	canonicalScriptPollerCommand         = "factory/scripts/poller.sh"
)

func TestRunScriptPoller_SubmitsCanonicalWorkRequestStdout(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-123","workTypeName":"task","payload":{"id":"ISSUE-123"}}]
	}`)
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersService(runner)
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)

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
	if submitted.submissions[0].Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("submitted request type = %q, want FACTORY_REQUEST_BATCH", submitted.submissions[0].Type)
	}
}

func TestRunScriptPoller_SubmitsSubmitStyleRecordsStdout(t *testing.T) {
	t.Parallel()

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
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: envelopeJSON}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersService(runner)
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)

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
	if len(workRequest.Works) != 1 || workRequest.Works[0].WorkID != "linear-issue-124" {
		t.Fatalf("submitted works = %#v, want one canonical work item", workRequest.Works)
	}
}

func TestRunScriptPoller_RejectsMalformedStdoutWithoutSubmit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		stdout           []byte
		wantErrSubstring string
	}{
		{
			name:             "non-json stdout",
			stdout:           []byte("submitted work\n"),
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "unsupported work request type",
			stdout:           []byte(`{"requestId":"x","type":"UNSUPPORTED","works":[]}`),
			wantErrSubstring: "unsupported work request type",
		},
		{
			name:             "mixed request and submissions",
			stdout:           []byte(`{"request":{"requestId":"a","type":"FACTORY_REQUEST_BATCH","works":[]},"submissions":[]}`),
			wantErrSubstring: "either request or submissions",
		},
		{
			name:             "unsupported raw factory events",
			stdout:           mustMarshalJSON(t, map[string]any{"events": []map[string]any{{"type": "WORK_REQUEST"}}}),
			wantErrSubstring: "unsupported raw factory events",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &sequenceCommandRunner{
				outcomes: []runOutcome{{result: workers.CommandResult{Stdout: tc.stdout}}},
			}
			submitted := &recordingSubmitter{}
			svc := newScriptPollersService(runner)
			poller := newCanonicalScriptPollerWorkstation()
			worker := newCanonicalScriptPollerWorker()
			runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

			err := svc.RunScriptPoller(
				context.Background(),
				runner,
				runtimeCfg,
				poller,
				worker,
				submitted.submit,
			)
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstring) {
				t.Fatalf("RunScriptPoller error = %v, want %q", err, tc.wantErrSubstring)
			}
			if submitted.calls != 0 {
				t.Fatalf("submit calls = %d, want 0 for malformed stdout", submitted.calls)
			}
		})
	}
}

func TestRunScriptPoller_EmptyStdoutDoesNotSubmit(t *testing.T) {
	t.Parallel()

	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersService(runner)
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v, want unexpected exit for empty stdout", err)
	}
	if submitted.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 for empty stdout", submitted.calls)
	}
}

func TestRunScriptPoller_SubmitFailureReturnsTypedSubmitError(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-submit-fail",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-999","workTypeName":"task","payload":{"id":"ISSUE-999"}}]
	}`)
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitErr := errors.New("ingress unavailable")
	submitted := &recordingSubmitter{
		submitOverride: func(_ context.Context, _ work.WorkRequest) error {
			return submitErr
		},
	}
	svc := newScriptPollersService(runner)
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("RunScriptPoller error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeFailed {
		t.Fatalf("submit failure code = %q, want %q", typed.Code, automations.ErrorCodeFailed)
	}
	if !strings.Contains(err.Error(), "submit failed") {
		t.Fatalf("RunScriptPoller error = %v, want submit failed", err)
	}
	if !errors.Is(err, submitErr) {
		t.Fatalf("RunScriptPoller error = %v, want wrapped submit error %v", err, submitErr)
	}
	if submitted.calls != 1 {
		t.Fatalf("submit calls = %d, want 1 before submit failure", submitted.calls)
	}
}

func TestScriptPollerCommandRequest_ResolvesCommandArgsWorkdirAndEnv(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	runtimeBaseDir := t.TempDir()
	poller := interfaces.FactoryWorkstationConfig{
		Name:             canonicalScriptPollerWorkstationName,
		Kind:             interfaces.WorkstationKindPoller,
		WorkerTypeName:   canonicalScriptPollerWorkerName,
		WorkingDirectory: "pollers/linear",
		Env: map[string]string{
			"LINEAR_API_KEY": "test-key",
		},
	}
	worker := newCanonicalScriptPollerWorker("--mode", "watch")
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)
	runtimeCfg.SetRuntimeBaseDir(runtimeBaseDir)

	req, err := scriptpollers.ScriptPollerCommandRequest(runtimeCfg, poller, worker, nil)
	if err != nil {
		t.Fatalf("ScriptPollerCommandRequest: %v", err)
	}
	if req.Command != filepath.Join(factoryDir, "scripts", "poller.sh") {
		t.Fatalf("poller command = %q, want resolved factory script path", req.Command)
	}
	if len(req.Args) != 2 || req.Args[0] != "--mode" || req.Args[1] != "watch" {
		t.Fatalf("poller args = %#v, want [--mode watch]", req.Args)
	}
	wantWorkDir := filepath.Clean(filepath.Join(runtimeBaseDir, "pollers", "linear"))
	if req.WorkDir != wantWorkDir {
		t.Fatalf("poller workdir = %q, want %q", req.WorkDir, wantWorkDir)
	}
	if !containsEnv(req.Env, "LINEAR_API_KEY=test-key") {
		t.Fatalf("poller env = %#v, want LINEAR_API_KEY=test-key", req.Env)
	}
}

type recordingSubmitter struct {
	mu             sync.Mutex
	calls          int
	submissions    []work.WorkRequest
	submitOverride func(context.Context, work.WorkRequest) error
}

func (r *recordingSubmitter) submit(ctx context.Context, request work.WorkRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.submissions = append(r.submissions, request)
	if r.submitOverride != nil {
		return r.submitOverride(ctx, request)
	}
	return nil
}

type runOutcome struct {
	result        workers.CommandResult
	err           error
	waitForCancel bool
}

type sequenceCommandRunner struct {
	mu       sync.Mutex
	calls    int
	outcomes []runOutcome
}

func (r *sequenceCommandRunner) Run(ctx context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	index := r.calls - 1
	var outcome runOutcome
	if index < len(r.outcomes) {
		outcome = r.outcomes[index]
	} else if len(r.outcomes) > 0 {
		outcome = r.outcomes[len(r.outcomes)-1]
	}
	r.mu.Unlock()

	if outcome.waitForCancel {
		<-ctx.Done()
		return outcome.result, ctx.Err()
	}
	return outcome.result, outcome.err
}

func (r *sequenceCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func newScriptPollersService(runner workers.CommandRunner) scriptpollers.Service {
	return newScriptPollersServiceWithOptions(scriptPollersServiceOptions{runner: runner})
}

func newCanonicalScriptPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           canonicalScriptPollerWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
}

func newCanonicalScriptPollerWorker(args ...string) *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:    canonicalScriptPollerWorkerName,
		Type:    interfaces.WorkerTypeScript,
		Command: canonicalScriptPollerCommand,
		Args:    args,
	}
}

func newScriptPollerLoadedRuntimeConfig(
	t *testing.T,
	factoryDir string,
	poller interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
) interfaces.MutableLoadedFactorySource {
	t.Helper()

	factoryCfg := &interfaces.FactoryConfig{
		Workers:      []interfaces.FactoryWorkerConfig{{Name: worker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	workerConfigs := map[string]*interfaces.FactoryWorkerConfig{
		worker.Name: worker,
	}
	workstationConfigs := map[string]*interfaces.FactoryWorkstationConfig{
		poller.Name: &poller,
	}

	loaded, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers:      workerConfigs,
			Workstations: workstationConfigs,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return data
}
