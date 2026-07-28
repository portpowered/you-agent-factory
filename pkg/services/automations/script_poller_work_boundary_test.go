package automations_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	scriptPollerBoundaryWorkstationName = "script-poller-boundary"
	scriptPollerBoundaryWorkerName      = "poller-script-boundary"
	scriptPollerBoundaryCommand         = "factory/scripts/poller.sh"
)

func scriptPollerBoundaryExecutionPolicy() interfaces.WorkstationExecutionPolicyService {
	return factorydefinitioncomposition.WorkstationExecutionPolicy{
		Resolve: func(workstation *interfaces.FactoryWorkstationConfig) (time.Duration, error) {
			if workstation == nil {
				return 0, nil
			}
			switch workstation.Limits.MaxExecutionTime {
			case "", "0s":
				return 0, nil
			default:
				return 0, nil
			}
		},
	}
}

func newScriptPollerBoundaryAutomationService(runner workers.CommandRunner) *automationservice.Service {
	return automationservice.New(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		runner,
		"factory/main",
		"",
		nil,
		nil,
		scriptPollerBoundaryExecutionPolicy(),
	)
}

func scriptPollerBoundaryWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           scriptPollerBoundaryWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: scriptPollerBoundaryWorkerName,
	}
}

func scriptPollerBoundaryWorker() *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:    scriptPollerBoundaryWorkerName,
		Type:    interfaces.WorkerTypeScript,
		Command: scriptPollerBoundaryCommand,
	}
}

func scriptPollerBoundaryRuntimeConfig(
	t *testing.T,
	factoryDir string,
) interfaces.MutableLoadedFactorySource {
	t.Helper()

	poller := scriptPollerBoundaryWorkstation()
	worker := scriptPollerBoundaryWorker()
	factoryCfg := &interfaces.FactoryConfig{
		Workers:      []interfaces.FactoryWorkerConfig{{Name: worker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				worker.Name: worker,
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				poller.Name: &poller,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

type scriptPollerBoundaryCommandRunner struct {
	mu       sync.Mutex
	outcomes []workers.CommandResult
}

func (r *scriptPollerBoundaryCommandRunner) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.outcomes) == 0 {
		return workers.CommandResult{}, nil
	}
	return r.outcomes[0], nil
}

// TestParseScriptPollerOutput_ProducesWorkRootRequestFromCanonicalStdout proves
// script poller canonical stdout parsing uses Work root helpers and returns a
// work.WorkRequest before submitter handoff.
func TestParseScriptPollerOutput_ProducesWorkRootRequestFromCanonicalStdout(t *testing.T) {
	t.Parallel()

	stdout := []byte(`{
		"requestId":"script-poller-boundary-batch",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{
				"name":"issue-123",
				"workTypeName":"task",
				"traceId":"trace-boundary",
				"payload":{"id":"ISSUE-123"}
			}
		]
	}`)

	request, hasOutput, err := automationservice.ParseScriptPollerOutput(stdout)
	if err != nil {
		t.Fatalf("ParseScriptPollerOutput: %v", err)
	}
	if !hasOutput {
		t.Fatal("expected canonical stdout to count as poller output")
	}
	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", request.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if request.RequestID != "script-poller-boundary-batch" {
		t.Fatalf("request ID = %q, want script-poller-boundary-batch", request.RequestID)
	}
	if len(request.Works) != 1 {
		t.Fatalf("works count = %d, want 1", len(request.Works))
	}
	workItem := request.Works[0]
	if workItem.Name != "issue-123" {
		t.Fatalf("work name = %q, want issue-123", workItem.Name)
	}
	if workItem.WorkTypeID != "task" {
		t.Fatalf("work type = %q, want task", workItem.WorkTypeID)
	}
	if workItem.TraceID != "trace-boundary" {
		t.Fatalf("trace ID = %q, want trace-boundary", workItem.TraceID)
	}
}

// TestParseScriptPollerOutput_ProducesWorkRootRequestFromSubmissionsEnvelope proves
// script poller submissions stdout uses Work root SubmitRequest and
// WorkRequestFromSubmitRequests helpers before submitter handoff.
func TestParseScriptPollerOutput_ProducesWorkRootRequestFromSubmissionsEnvelope(t *testing.T) {
	t.Parallel()

	stdout := []byte(`{
		"submissions":[
			{
				"requestId":"script-poller-boundary-submit",
				"workId":"work-boundary-124",
				"name":"issue-124",
				"workTypeName":"task",
				"traceId":"trace-124"
			}
		]
	}`)

	request, hasOutput, err := automationservice.ParseScriptPollerOutput(stdout)
	if err != nil {
		t.Fatalf("ParseScriptPollerOutput: %v", err)
	}
	if !hasOutput {
		t.Fatal("expected submissions stdout to count as poller output")
	}
	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", request.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if request.RequestID != "script-poller-boundary-submit" {
		t.Fatalf("request ID = %q, want script-poller-boundary-submit", request.RequestID)
	}
	if len(request.Works) != 1 {
		t.Fatalf("works count = %d, want 1", len(request.Works))
	}
	workItem := request.Works[0]
	if workItem.WorkID != "work-boundary-124" {
		t.Fatalf("work ID = %q, want work-boundary-124", workItem.WorkID)
	}
	if workItem.Name != "issue-124" {
		t.Fatalf("work name = %q, want issue-124", workItem.Name)
	}
	if workItem.WorkTypeID != "task" {
		t.Fatalf("work type = %q, want task", workItem.WorkTypeID)
	}
	if workItem.TraceID != "trace-124" {
		t.Fatalf("trace ID = %q, want trace-124", workItem.TraceID)
	}

	var submissions []work.SubmitRequest
	if err := json.Unmarshal([]byte(`[
		{
			"requestId":"script-poller-boundary-submit",
			"workId":"work-boundary-124",
			"name":"issue-124",
			"workTypeName":"task",
			"traceId":"trace-124"
		}
	]`), &submissions); err != nil {
		t.Fatalf("decode submissions fixture: %v", err)
	}
	expected := work.WorkRequestFromSubmitRequests(submissions)
	if request.RequestID != expected.RequestID {
		t.Fatalf("request ID = %q, want WorkRequestFromSubmitRequests %q", request.RequestID, expected.RequestID)
	}
	if len(request.Works) != len(expected.Works) || request.Works[0].WorkID != expected.Works[0].WorkID {
		t.Fatalf("parsed works = %#v, want %#v from Work root helper", request.Works, expected.Works)
	}
}

// TestRunScriptPoller_HandsWorkRootRequestToAutomationsSubmitter proves script
// poller admission constructs work.WorkRequest values and hands them to the
// Automations WorkRequestSubmitter contract.
func TestRunScriptPoller_HandsWorkRootRequestToAutomationsSubmitter(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	stdout := []byte(`{
		"requestId":"script-poller-runtime-boundary",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-runtime","workTypeName":"task","payload":{"id":"ISSUE-RUNTIME"}}]
	}`)
	runner := &scriptPollerBoundaryCommandRunner{
		outcomes: []workers.CommandResult{{Stdout: stdout}},
	}
	svc := newScriptPollerBoundaryAutomationService(runner)
	runtimeCfg := scriptPollerBoundaryRuntimeConfig(t, factoryDir)

	var submitCalls int
	var submitted work.WorkRequest
	submitter := automations.WorkRequestSubmitter(func(_ context.Context, request work.WorkRequest) error {
		submitCalls++
		submitted = request
		return nil
	})

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		scriptPollerBoundaryWorkstation(),
		scriptPollerBoundaryWorker(),
		submitter,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v, want unexpected exit after successful submit", err)
	}
	if submitCalls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls)
	}
	if submitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", submitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if submitted.RequestID != "script-poller-runtime-boundary" {
		t.Fatalf("request ID = %q, want script-poller-runtime-boundary", submitted.RequestID)
	}
	if len(submitted.Works) != 1 {
		t.Fatalf("works count = %d, want 1", len(submitted.Works))
	}
	if submitted.Works[0].Name != "issue-runtime" {
		t.Fatalf("work name = %q, want issue-runtime", submitted.Works[0].Name)
	}
	if submitted.Works[0].WorkTypeID != "task" {
		t.Fatalf("work type = %q, want task", submitted.Works[0].WorkTypeID)
	}
}

// TestParseScriptPollerOutput_RejectsMalformedAndEmptyStdout proves malformed
// or empty script poller stdout still fails with Automations-observable
// rejection behavior before submitter handoff.
func TestParseScriptPollerOutput_RejectsMalformedAndEmptyStdout(t *testing.T) {
	t.Parallel()

	t.Run("empty stdout is not output", func(t *testing.T) {
		t.Parallel()
		_, hasOutput, err := automationservice.ParseScriptPollerOutput(nil)
		if hasOutput || err != nil {
			t.Fatalf("empty stdout = hasOutput %v err %v, want no output", hasOutput, err)
		}
	})

	t.Run("malformed stdout", func(t *testing.T) {
		t.Parallel()
		_, hasOutput, err := automationservice.ParseScriptPollerOutput([]byte("submitted work\n"))
		if !hasOutput {
			t.Fatal("expected non-empty stdout to count as poller output")
		}
		if err == nil || !strings.Contains(err.Error(), "malformed stdout") {
			t.Fatalf("error = %v, want malformed stdout", err)
		}
	})

	t.Run("empty submissions array", func(t *testing.T) {
		t.Parallel()
		_, hasOutput, err := automationservice.ParseScriptPollerOutput([]byte(`{"submissions":[]}`))
		if !hasOutput {
			t.Fatal("expected submissions envelope to count as poller output")
		}
		if err == nil || !strings.Contains(err.Error(), "submissions must contain at least one item") {
			t.Fatalf("error = %v, want empty submissions rejection", err)
		}
	})
}
