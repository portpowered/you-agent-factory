package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBlocksTransportBehaviorOutsideRepresentationAndProtocol(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/command.go", `package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	. "time"

	logging "github.com/portpowered/infinite-you/pkg/platform/logging"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

var hiddenBootstrap = func() {}
var hiddenHTTPClient http.Client

func run(ctx context.Context) {
	fmt.Print("process-global stdout")
	_ = context.Background()
	var _ *os.File
	_, _ = os.ReadFile("factory.json")
	_, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()
	done := make(chan struct{})
	go func() { close(done) }()
	_ = logging.BuildLogger
	_ = factorysessions.ProjectRuntimeContract(ctx)
	Sleep(0)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want transport behavior failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited transport mutable-function-seam behavior: hiddenBootstrap",
		"prohibited transport external-effect behavior: fmt.Print",
		"prohibited transport external-effect behavior: os.File",
		"prohibited transport external-effect behavior: os.ReadFile",
		"prohibited transport external-effect behavior: net/http.Client",
		"prohibited transport lifecycle behavior: context.WithCancel",
		"prohibited transport lifecycle behavior: context.Background",
		"prohibited transport concurrency behavior: sync.Mutex",
		"prohibited transport concurrency behavior: make(chan)",
		"prohibited transport concurrency behavior: go",
		"prohibited transport platform-selection behavior: github.com/portpowered/infinite-you/pkg/platform/logging.BuildLogger",
		"prohibited transport domain-policy behavior: github.com/portpowered/infinite-you/pkg/services/factory_sessions.ProjectRuntimeContract",
		"prohibited transport opaque-import behavior: time",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksTransportServiceForwardingEntrypoint(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/preview.go", `package mapping

import factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

func BuildFactoryPreview(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	input factoryruntime.WorkflowPreviewRequest,
) factoryruntime.WorkflowPreview {
	return workflows.BuildPreview(input)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want service-forwarder failure")
	}
	want := "prohibited transport service-forwarder behavior: BuildFactoryPreview->github.com/portpowered/infinite-you/pkg/services/factory_runtime.BuildPreview"
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("run() stderr = %q, want substring %q", got, want)
	}
}

func TestRunBlocksRetiredWorkflowResultCompositionEntrypoints(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/workflow.go", `package mapping
func BuildWorkflowSessionLiveResult() {}
func BuildWorkflowSessionResult() {}
func BuildWorkflowSessionResultUpdatedPayload() {}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired workflow-result entrypoint failure")
	}
	for _, symbol := range []string{
		"BuildWorkflowSessionLiveResult",
		"BuildWorkflowSessionResult",
		"BuildWorkflowSessionResultUpdatedPayload",
	} {
		want := "prohibited transport alternate-service-entrypoint behavior: " + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksWorkerFailurePolicyInMapping(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/failure.go", `package mapping

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

func mapFailure(err error) {
	_ = workers.NormalizeProviderExecutionError(err)
	_ = workers.NewProviderError(workers.WorkFailureTypeTimeout, "timeout", err)
	_ = workers.SafeWorkDiagnosticsFromWorkDiagnostics(nil)
	_ = workers.CanonicalProviderSessionProvider("agent")
	var _ *workers.WorkDiagnostics
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want workers failure-policy boundary failure")
	}
	for _, symbol := range []string{
		"NormalizeProviderExecutionError", "NewProviderError",
		"SafeWorkDiagnosticsFromWorkDiagnostics", "CanonicalProviderSessionProvider", "WorkDiagnostics",
	} {
		want := "prohibited transport workers-failure-policy behavior: github.com/portpowered/infinite-you/pkg/services/workers." + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksModelReadinessPolicyInMapping(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/readiness.go", `package mapping

import models "github.com/portpowered/infinite-you/pkg/services/models"

var blockingErrors = []error{models.ErrMissing, models.ErrLoading, models.ErrFailed, models.ErrUnsupported}
type readinessError = models.InvocationError

func mapReadiness(runtime models.Runtime, err error) {
	_ = models.InvocationErrorForRuntime(runtime)
	_, _ = models.ReadinessStateFromError(err)
	_ = models.IsInvocationBlocked(err)
	_ = models.IsMissing(err)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Models readiness-policy boundary failure")
	}
	for _, symbol := range []string{
		"ErrMissing", "ErrLoading", "ErrFailed", "ErrUnsupported", "InvocationError",
		"InvocationErrorForRuntime", "ReadinessStateFromError", "IsInvocationBlocked", "IsMissing",
	} {
		want := "prohibited transport models-readiness-policy behavior: github.com/portpowered/infinite-you/pkg/services/models." + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksRetiredFactoryRunServiceEntrypoints(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/factoryrun/forwarders.go", `package factoryrun

func LoadFactoryConfigFromConfigFile() {}
func ResolveFactoryRootFromConfigFile() {}
func ValidateFactoryForPromptRun() {}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired entrypoint failure")
	}
	for _, symbol := range []string{"LoadFactoryConfigFromConfigFile", "ResolveFactoryRootFromConfigFile", "ValidateFactoryForPromptRun"} {
		want := "prohibited transport alternate-service-entrypoint behavior: " + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksProductionMCPMockClientEntrypoint(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mcp/factorysession/client.go", `package factorysession

type Client struct{}

func NewClient() *Client { return &Client{} }
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want alternate MCP test entrypoint failure")
	}
	for _, symbol := range []string{"Client", "NewClient"} {
		want := "prohibited transport alternate-test-entrypoint behavior: " + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksTransportFactoryDefinitionValidationOrchestration(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/validation.go", `package mapping

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

func validate(v factorydefinitions.Validator, input *factorydefinitions.FactoryConfig) any {
	_ = v.WorkTypeHandlingBehavior(nil, input, true)
	return v.ValidateBlockingLoad(nil, input)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want validation-orchestration failure")
	}
	for _, symbol := range []string{"ValidateBlockingLoad", "WorkTypeHandlingBehavior"} {
		want := "prohibited transport validation-orchestration behavior: " + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksTransportWorkflowSourceDefaultSelection(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/preview.go", `package http

type workflows interface { DefaultSourceContext(string) (any, error) }

func preview(service workflows, root string) error {
	_, err := service.DefaultSourceContext(root)
	return err
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want source-default-selection failure")
	}
	want := "prohibited transport source-default-selection behavior: DefaultSourceContext"
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("run() stderr = %q, want substring %q", got, want)
	}
}

func TestRunBlocksTransportWorkRequestNormalization(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/run/clean.go", `package run

type normalizer interface { NormalizeWorkRequest(any) (any, error) }

func prepare(service normalizer, request any) (any, error) {
	return service.NormalizeWorkRequest(request)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Work Request normalization failure")
	}
	want := "prohibited transport work-request-normalization behavior: NormalizeWorkRequest"
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("run() stderr = %q, want substring %q", got, want)
	}
}

func TestRunBlocksHTTPFactoryStatusProjectionPolicy(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/status.go", `package http

type topology interface { StateCategoryForPlace(string) string }

func categorizeStatusTokens(net topology, placeID string) string {
	return net.StateCategoryForPlace(placeID)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Factory status projection failure")
	}
	for _, symbol := range []string{"StateCategoryForPlace", "categorizeStatusTokens"} {
		want := "prohibited transport factory-status-projection behavior: " + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksTransportNamedFactoryPathPolicyInProductionAndTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for _, path := range []string{
		"pkg/transports/cli/factoryload/operator_error.go",
		"pkg/transports/cli/factoryload/operator_error_test.go",
	} {
		writeGoSourceFile(t, repoRoot, path, `package factoryload

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

func candidate(root, name string) (string, error) {
	return factorydefinitions.MapDir(root, name)
}
`)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want named Factory path-policy failure")
	}
	wantSymbol := "github.com/portpowered/infinite-you/pkg/services/factory_definitions.MapDir"
	for _, want := range []string{
		"prohibited transport named-factory-path-policy behavior: " + wantSymbol + " (pkg/transports/cli/factoryload/operator_error.go:",
		"prohibited transport named-factory-path-policy behavior: " + wantSymbol + " (pkg/transports/cli/factoryload/operator_error_test.go:",
	} {
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksTransportWorkersConfigLoadingPolicyInProductionAndTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for _, path := range []string{
		"pkg/transports/cli/run/run.go",
		"pkg/transports/cli/run/run_test.go",
	} {
		writeGoSourceFile(t, repoRoot, path, `package run

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

func load(path string) (*workers.MockWorkersConfig, error) {
	return workers.LoadMockWorkersConfig(path)
}

var prohibitedConfigPolicy = []any{
	workers.NewMockWorkersConfigLoader,
	workers.NewEmptyMockWorkersConfig,
	workers.ParseMockWorkersConfig,
}
`)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Workers config-loading failure")
	}
	for _, path := range []string{
		"pkg/transports/cli/run/run.go",
		"pkg/transports/cli/run/run_test.go",
	} {
		for _, symbol := range []string{
			"LoadMockWorkersConfig", "NewMockWorkersConfigLoader",
			"NewEmptyMockWorkersConfig", "ParseMockWorkersConfig",
		} {
			want := "prohibited transport workers-config-loading behavior: " + workersRootImport + "." + symbol + " (" + path + ":"
			if got := stderr.String(); !strings.Contains(got, want) {
				t.Fatalf("run() stderr = %q, want substring %q", got, want)
			}
		}
	}
}

func TestRunAllowsInjectedWorkersConfigLoaderRole(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/run/run.go", `package run

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

func loadSelected(load workers.MockWorkersConfigLoader, path string) (*workers.MockWorkersConfig, error) {
	return load(path)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v, stderr = %q", err, stderr.String())
	}
}

func TestRunBlocksTransportTestWorkRequestNormalization(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/server_test.go", `package http

type normalizer interface { NormalizeWorkRequest(any) (any, error) }

func acceptTestWork(service normalizer, request any) (any, error) {
	return service.NormalizeWorkRequest(request)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want test Work Request normalization failure")
	}
	want := "prohibited transport work-request-normalization behavior: NormalizeWorkRequest (pkg/transports/http/server_test.go:"
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("run() stderr = %q, want substring %q", got, want)
	}
}

func TestRunAllowsInjectedWorkRequestPreparation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/run/clean.go", `package run

type preparation interface { PrepareSingleWorkTarget(any) (any, error) }

func prepare(service preparation, request any) (any, error) {
	return service.PrepareSingleWorkTarget(request)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v, stderr = %q", err, stderr.String())
	}
}

func TestRunAllowsTransportRepresentationAndInjectedProtocolRoles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/session.go", `package mapping

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

func Control(input factorysessions.ControlRequest) factorysessions.ControlRequest {
	return input
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/session.go", `package http

import (
	"context"
	"io"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type Operation interface { Execute(context.Context, factorysessions.ControlRequest) error }
type Preparation interface { PrepareControl(factorysessions.ControlRequest) (factorysessions.ControlRequest, error) }

func PrepareAndExecute(ctx context.Context, output io.Writer, prepare Preparation, operation Operation, input factorysessions.ControlRequest) error {
	request, err := prepare.PrepareControl(input)
	if err != nil { return err }
	_, _ = output.Write([]byte(request.SessionID))
	return operation.Execute(ctx, request)
}

`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "package boundary passed") {
		t.Fatalf("run() stdout = %q, want package boundary success", stdout.String())
	}
}

func TestRunBlocksTransportFactorySessionNormalizationHelpers(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/session.go", `package mapping

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

func mapRequest(input factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	return factorysessions.NormalizeStartRequest(input)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Factory Session normalization failure")
	}
	want := "prohibited transport service-normalization behavior: github.com/portpowered/infinite-you/pkg/services/factory_sessions.NormalizeStartRequest"
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("run() stderr = %q, want substring %q", got, want)
	}
}

func TestRunBlocksFactorySessionPreparationInsideMapping(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/factorysession/request.go", `package factorysession

type preparation interface { PrepareStart(any) (any, error) }

func mapAndPrepare(prepare preparation, request any) (any, error) {
	return prepare.PrepareStart(request)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Factory Session preparation boundary failure")
	}
	want := "prohibited transport factory-session-preparation behavior: PrepareStart"
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("run() stderr = %q, want substring %q", got, want)
	}
}

func TestRunBlocksFactorySessionPreparationInsideMappingTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/factorysession/request_test.go", `package factorysession

type preparation interface {
	PrepareStart(any) (any, error)
	PrepareControl(any) (any, error)
	PrepareApprove(any) (any, error)
	PrepareRetryDispatch(any) (any, error)
	PrepareInterruptDispatch(any) (any, error)
	PrepareListSessions(any) (any, error)
	PrepareResult(any) (any, error)
	PrepareEventReconnect(any) (any, error)
}

func duplicateOwnerPolicy(prepare preparation, request any) {
	_, _ = prepare.PrepareStart(request)
	_, _ = prepare.PrepareControl(request)
	_, _ = prepare.PrepareApprove(request)
	_, _ = prepare.PrepareRetryDispatch(request)
	_, _ = prepare.PrepareInterruptDispatch(request)
	_, _ = prepare.PrepareListSessions(request)
	_, _ = prepare.PrepareResult(request)
	_, _ = prepare.PrepareEventReconnect(request)
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want mapping-test Factory Session preparation boundary failure")
	}
	for method := range transportFactorySessionPreparationMethods {
		want := "prohibited transport factory-session-preparation behavior: " + method + " (pkg/transports/mapping/factorysession/request_test.go:"
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksTransportWorkPreparationPolicyAndConstruction(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/work/list.go", `package work

import workservice "github.com/portpowered/infinite-you/pkg/services/work"

var prohibited = []any{
	workservice.NormalizeList,
	workservice.ParseCanonicalWorkRequestJSON,
	workservice.ValidateCanonicalWorkRequestJSON,
	workservice.NewListRequestPreparation,
	workservice.NewFactoryRequestBatchPreparation,
	workservice.ResolveTextInput,
	workservice.ResolveAPITextInputContent,
	workservice.NormalizeArguments,
	workservice.NamedArgumentInputsFromAnyMap,
	workservice.NewInvocationInputPreparation,
	workservice.NewContentPreparation,
	workservice.ResolveWorkRequestCurrentChainingTraceID,
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Work preparation boundary failure")
	}
	for _, symbol := range []string{
		"NormalizeList", "ParseCanonicalWorkRequestJSON", "ValidateCanonicalWorkRequestJSON",
		"NewListRequestPreparation", "NewFactoryRequestBatchPreparation",
		"ResolveTextInput", "ResolveAPITextInputContent", "NormalizeArguments",
		"NamedArgumentInputsFromAnyMap", "NewInvocationInputPreparation", "NewContentPreparation",
		"ResolveWorkRequestCurrentChainingTraceID",
	} {
		want := "prohibited transport work-request-preparation behavior: github.com/portpowered/infinite-you/pkg/services/work." + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksWorkContentMapperDomainNormalization(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/workcontent/generated.go", `package workcontent

import workservice "github.com/portpowered/infinite-you/pkg/services/work"

func mapPart(part workservice.WorkContentPart) workservice.WorkContentPart {
	part.Type = part.Type.Normalized()
	return part
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Work content mapper normalization failure")
	}
	want := "prohibited transport work-content-normalization behavior: Normalized"
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("run() stderr = %q, want substring %q", got, want)
	}
}

func TestRunBlocksTransportTestsCallingWorkPreparationPolicy(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/submit/batch_test.go", `package submit

import workservice "github.com/portpowered/infinite-you/pkg/services/work"

func prohibited(data []byte) {
	_, _ = workservice.ParseCanonicalWorkRequestJSON(data)
	_ = workservice.NewFactoryRequestBatchPreparation()
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want transport-test Work preparation boundary failure")
	}
	for _, symbol := range []string{"ParseCanonicalWorkRequestJSON", "NewFactoryRequestBatchPreparation"} {
		want := "prohibited transport work-request-preparation behavior: github.com/portpowered/infinite-you/pkg/services/work." + symbol
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksTransportTestFactorySessionNormalizationHelpers(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mcp/factorysession/execution_test.go", `package factorysession_test

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

func prepare(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	return factorysessions.NormalizeStartRequest(request)
}

var prohibitedHelpers = []any{
	factorysessions.NormalizeControlRequest,
	factorysessions.NormalizeApproveRequest,
	factorysessions.NormalizeRetryDispatchRequest,
	factorysessions.NormalizeInterruptDispatchRequest,
	factorysessions.NormalizeListSessionsRequest,
	factorysessions.NormalizeResultRequest,
	factorysessions.NormalizeEventReconnectRequest,
	factorysessions.NewExecutionValidationError,
}
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want test Factory Session normalization failure")
	}
	for _, symbol := range []string{
		"NormalizeStartRequest", "NormalizeControlRequest", "NormalizeApproveRequest",
		"NormalizeRetryDispatchRequest", "NormalizeInterruptDispatchRequest",
		"NormalizeListSessionsRequest", "NormalizeResultRequest", "NormalizeEventReconnectRequest",
		"NewExecutionValidationError",
	} {
		want := "prohibited transport service-normalization behavior: github.com/portpowered/infinite-you/pkg/services/factory_sessions." + symbol + " (pkg/transports/mcp/factorysession/execution_test.go:"
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunBlocksStaleTransportBehaviorBaselineEntry(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/command.go", "package cli\n")
	baseline := transportBehaviorBaseline{
		Version: 1,
		Entries: []transportBehaviorBaselineEntry{{
			Kind: "external-effect", Symbol: "os.ReadFile", FilePath: "pkg/transports/cli/command.go", Count: 1,
			Stage: transportBehaviorBaselineStage, DeletionGate: transportBehaviorDeletionGate,
		}},
	}
	payload, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repoRoot, transportBehaviorBaselinePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, transportBehaviorBaselinePath), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want stale baseline failure")
	}
	if got := stderr.String(); !strings.Contains(got, "stale transport behavior baseline entry: pkg/transports/cli/command.go external-effect os.ReadFile") {
		t.Fatalf("run() stderr = %q, want stale exact entry diagnostic", got)
	}
}

func TestTransportBehaviorBaselineRejectsWildcardAndEmptyCreation(t *testing.T) {
	t.Parallel()

	entry := transportBehaviorBaselineEntry{
		Kind:         "external-effect",
		Symbol:       "os.*",
		FilePath:     "pkg/transports/cli/*.go",
		Count:        1,
		Stage:        transportBehaviorBaselineStage,
		DeletionGate: transportBehaviorDeletionGate,
	}
	if err := validateTransportBehaviorBaselineEntry(entry); err == nil || !strings.Contains(err.Error(), "wildcards") {
		t.Fatalf("validate wildcard error = %v, want wildcard rejection", err)
	}

	repoRoot := t.TempDir()
	if err := createTransportBehaviorBaseline(config{root: repoRoot}); err == nil || !strings.Contains(err.Error(), "refusing to create empty") {
		t.Fatalf("create empty baseline error = %v, want empty creation rejected", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, transportBehaviorBaselinePath)); !os.IsNotExist(err) {
		t.Fatalf("empty baseline artifact stat error = %v, want file absent", err)
	}
}
