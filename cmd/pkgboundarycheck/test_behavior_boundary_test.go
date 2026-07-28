package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestScanTestBehaviorBoundariesRejectsPolicyAndAlternateCompositionLoopholes(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/example/process_helper_test.go", `package example
import "github.com/portpowered/infinite-you/pkg/root"
func startCustomerProcess() { root.BuildProcess() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/example/built_cli_helper_test.go", `package example
import builtcli "github.com/portpowered/infinite-you/internal/builtcliacceptance"
func startChildCLI() { builtcli.NewHarness() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/example/operator_policy_test.go", `package example
import settings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
func expectedConfig() { settings.DefaultConfigPath("home") }
`)
	writeGoSourceFile(t, repoRoot, "internal/testutil/named_factory.go", `package testutil
import definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
func NamedFactory() { definitions.MapDir("root", "name") }
`)
	writeGoSourceFile(t, repoRoot, "tests/functional/internal/support/workers.go", `package support
import workers "github.com/portpowered/infinite-you/pkg/services/workers"
func ReadMockWorkers() { workers.LoadMockWorkersConfig("workers.json") }
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/session/resume_smoke_test.go", `package session
import "github.com/portpowered/infinite-you/pkg/root"
func customerResumeRuntime() { root.BuildProcess() }
`)
	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	if len(findings) != 6 {
		t.Fatalf("finding count = %d, want 6: %#v", len(findings), findings)
	}
	joined := testBehaviorFindingSummary(findings)
	for _, want := range []string{
		"pkg/services/example/process_helper_test.go|alternate-customer-composition|pkg/root|BuildProcess|1",
		"pkg/services/example/built_cli_helper_test.go|alternate-customer-composition|internal/builtcliacceptance|NewHarness|1",
		"pkg/services/example/operator_policy_test.go|cross-owner-service-policy|pkg/services/operator_settings|DefaultConfigPath|1",
		"internal/testutil/named_factory.go|cross-owner-service-policy|pkg/services/factory_definitions|MapDir|1",
		"tests/functional/internal/support/workers.go|cross-owner-service-policy|pkg/services/workers|LoadMockWorkersConfig|1",
		"pkg/transports/cli/session/resume_smoke_test.go|customer-process-under-transport|pkg/root|BuildProcess|1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("findings = %q, want %q", joined, want)
		}
	}
}

func TestScanTestBehaviorBoundariesRejectsFunctionalTransportConstruction(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "tests/functional/runtime_api/direct_submit_test.go", `package runtime_api
import (
  clihttp "github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
  submit "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
)
func bypassRootProcess() { clihttp.NewProtocol(nil, nil); submit.NewSubmit(nil, nil) }
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	joined := testBehaviorFindingSummary(findings)
	for _, want := range []string{
		"tests/functional/runtime_api/direct_submit_test.go|alternate-customer-composition|pkg/transports/cli/clihttp|NewProtocol|1",
		"tests/functional/runtime_api/direct_submit_test.go|alternate-customer-composition|pkg/transports/cli/submit|NewSubmit|1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("findings = %q, want %q", joined, want)
		}
	}
}

func TestScanTestBehaviorBoundariesRejectsUnregisteredHandwrittenTransportConstructor(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "tests/functional/config/mapper_test.go", `package config
import factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
func constructMappingImplementation() { factorymapping.NewFactoryConfigMapper() }
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scan test behavior boundaries: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one handwritten transport constructor", findings)
	}
	got := findings[0]
	if got.Kind != testBehaviorCompositionKind || got.Symbol != "NewFactoryConfigMapper" ||
		got.ImportPath != repositoryImportPrefix+"pkg/transports/mapping/factoryconfig" {
		t.Fatalf("finding = %#v, want structural mapping constructor rejection", got)
	}
}

func TestScanTestBehaviorBoundariesAllowsGeneratedPublicHTTPConstructors(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "tests/functional/runtime_api/client_test.go", `package runtimeapi
import generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
import factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
func usePublicEdges() { generatedclient.NewClient(""); factoryapi.NewPublicValue() }
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scan test behavior boundaries: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want generated public HTTP edges allowed", findings)
	}
}

func TestScanTestBehaviorBoundariesAllowsOwnerTransportConstruction(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/submit/protocol_test.go", `package submit
import (
  clihttp "github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
  submit "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
)
func protocolInvariant() { clihttp.NewProtocol(nil, nil); submit.NewSubmit(nil, nil) }
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want owner-local transport construction allowed", findings)
	}
}

func TestScanTestBehaviorBoundariesAllowsOwnerPolicyPublicValuesAndCanonicalFunctionalProcess(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_definitions/named_test.go", `package factory_definitions
import definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
func ownerPolicy() { definitions.MapDir("root", "name") }
`)
	writeGoSourceFile(t, repoRoot, "tests/functional/process_test.go", `package functional
import (
  "github.com/portpowered/infinite-you/pkg/root"
  definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)
func customerProcess() { root.BuildProcess(); definitions.NewFactorySnapshot(nil) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/server_test.go", `package http
import sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
func strictRole() { _ = sessions.RequestPreparationFunc(nil) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/run/direct_test.go", `package run
import sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
func invokeFocusedTransportOperation() { _ = sessions.RequestPreparationFunc(nil); Run(nil, RunConfig{}) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/commandidentity/root_process_test.go", `package commandidentity
import "github.com/portpowered/infinite-you/pkg/root"
func commandInventoryParity() { root.BuildProcess() }
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScanTestBehaviorBoundariesRejectsTransportTestsThatInvokeOwnerPolicy(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/workflow_source_test.go", `package mapping_test
import "github.com/portpowered/infinite-you/pkg/transports/mapping"
func projectWorkflowResults() {
  apisurface.BuildWorkflowSessionLiveResult()
  apisurface.BuildWorkflowSessionResult()
  apisurface.BuildWorkflowSessionResultUpdatedPayload()
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/session/list_test.go", `package session
import sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
func listSessions() { sessions.ApplySessionListScope() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/run/response_test.go", `package run
import . "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
func validateFixture() { ValidateFactoryResponseEvent() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/contracttests/models_test.go", `package contracttests
import (
  definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
  models "github.com/portpowered/infinite-you/pkg/services/models"
)
func deriveTransportContract() {
  models.SupportedProviders()
  definitions.PublicWorkerModelProviderFromInternal()
  definitions.InternalModelProviderFromPublicWorkerModelProvider()
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/work_read_role_test.go", `package http
import (
  runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
  sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
  work "github.com/portpowered/infinite-you/pkg/services/work"
)
func hiddenQueryImplementation() {
  work.PrepareInvocationInput()
  work.NormalizeList()
  runtime.CollectPublicWorkTokens()
  runtime.SplitPlaceID()
  runtime.CategoryForState()
  sessions.ProjectFactorySessionStopSummary()
  sessions.ProjectWorkStopSummary()
}
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	if len(findings) != 15 {
		t.Fatalf("finding count = %d, want 15: %#v", len(findings), findings)
	}
	joined := testBehaviorFindingSummary(findings)
	for _, want := range []string{
		"pkg/transports/mapping/workflow_source_test.go|cross-owner-service-policy|pkg/transports/mapping|BuildWorkflowSessionLiveResult|1",
		"pkg/transports/mapping/workflow_source_test.go|cross-owner-service-policy|pkg/transports/mapping|BuildWorkflowSessionResult|1",
		"pkg/transports/mapping/workflow_source_test.go|cross-owner-service-policy|pkg/transports/mapping|BuildWorkflowSessionResultUpdatedPayload|1",
		"pkg/transports/cli/session/list_test.go|cross-owner-service-policy|pkg/services/factory_sessions|ApplySessionListScope|1",
		"pkg/transports/cli/run/response_test.go|cross-owner-service-policy|pkg/services/factory_sessions|ValidateFactoryResponseEvent|1",
		"pkg/transports/http/contracttests/models_test.go|cross-owner-service-policy|pkg/services/models|SupportedProviders|1",
		"pkg/transports/http/contracttests/models_test.go|cross-owner-service-policy|pkg/services/factory_definitions|PublicWorkerModelProviderFromInternal|1",
		"pkg/transports/http/work_read_role_test.go|cross-owner-service-policy|pkg/services/work|PrepareInvocationInput|1",
		"pkg/transports/http/work_read_role_test.go|cross-owner-service-policy|pkg/services/work|NormalizeList|1",
		"pkg/transports/http/work_read_role_test.go|cross-owner-service-policy|pkg/services/factory_runtime|CollectPublicWorkTokens|1",
		"pkg/transports/http/work_read_role_test.go|cross-owner-service-policy|pkg/services/factory_runtime|SplitPlaceID|1",
		"pkg/transports/http/work_read_role_test.go|cross-owner-service-policy|pkg/services/factory_runtime|CategoryForState|1",
		"pkg/transports/http/work_read_role_test.go|cross-owner-service-policy|pkg/services/factory_sessions|ProjectFactorySessionStopSummary|1",
		"pkg/transports/http/work_read_role_test.go|cross-owner-service-policy|pkg/services/factory_sessions|ProjectWorkStopSummary|1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("findings = %q, want %q", joined, want)
		}
	}
	if !strings.Contains(joined, "InternalModelProviderFromPublicWorkerModelProvider|1") {
		t.Fatalf("findings = %q, want reverse model-provider mapping", joined)
	}
}

func TestScanTestBehaviorBoundariesRejectsProviderSessionPolicyInTransportSupport(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/server_test_helpers_test.go", `package http
import (
  sessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
  cursor "github.com/portpowered/infinite-you/pkg/services/provider_sessions/cursor"
  service "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"
  workers "github.com/portpowered/infinite-you/pkg/services/workers"
)
type testProviderSessionService struct{}
func newTestProviderSessionService() { sessions.CanonicalProvider("agent"); service.NewForRoots(); cursor.LoadDetails(); workers.CanonicalProviderSessionProvider("agent") }
func scriptedProviderSessionDetail() {}
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries: %v", err)
	}
	joined := testBehaviorFindingSummary(findings)
	for _, symbol := range []string{
		"CanonicalProvider", "CanonicalProviderSessionProvider", "NewForRoots", "LoadDetails",
		"newTestProviderSessionService", "scriptedProviderSessionDetail", "testProviderSessionService",
	} {
		if !strings.Contains(joined, "|"+symbol+"|") {
			t.Fatalf("findings = %q, want Provider Sessions policy symbol %q", joined, symbol)
		}
	}
}

func TestScanTestBehaviorBoundariesAllowsDetachedOwnerValuesAndStrictRoles(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/server_test.go", `package http
import (
  runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
  sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
  models "github.com/portpowered/infinite-you/pkg/services/models"
)
func detachedContracts() {
  _ = runtime.WorkflowPreview{}
  _ = sessions.ListSessionsResult{}
  _ = sessions.ResponseEventValidator(nil)
  _ = models.Provider("CODEX")
}
`)
	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScanTestBehaviorBoundariesRejectsEngineImplementationsInHTTPTransportTests(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/servertests/server_move_work_test.go", `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func hiddenEngineFixtures() {
  _ = runtime.RuntimeToken{}
  _ = runtime.PetriMarkingSnapshot{}
  _ = runtime.Net{}
}
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	joined := testBehaviorFindingSummary(findings)
	for _, symbol := range []string{"RuntimeToken", "PetriMarkingSnapshot", "Net"} {
		want := "pkg/transports/http/servertests/server_move_work_test.go|cross-owner-service-policy|pkg/services/factory_runtime|" + symbol + "|1"
		if !strings.Contains(joined, want) {
			t.Fatalf("findings = %q, want %q", joined, want)
		}
	}
}

func TestScanTestBehaviorBoundariesRejectsWorkInvocationInputPolicyInTransportFakes(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/root_run_test.go", `package cli
import (
  "context"
  "strings"
  work "github.com/portpowered/infinite-you/pkg/services/work"
)
type invocationInputFake struct{}
func (invocationInputFake) PrepareInvocationInput(_ context.Context, request work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
  text := strings.TrimSpace(strings.Join(request.Arguments, " "))
  return work.PreparedInvocationInput{ResolvedInput: work.ResolvedInput{Text: text}}, nil
}
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	want := "pkg/transports/cli/root_run_test.go|cross-owner-service-policy|pkg/services/work|PrepareInvocationInput|1"
	if joined := testBehaviorFindingSummary(findings); !strings.Contains(joined, want) {
		t.Fatalf("findings = %q, want %q", joined, want)
	}
}

func TestScanTestBehaviorBoundariesAllowsStrictWorkInvocationInputCallback(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/root_run_test.go", `package cli
import (
  "context"
  work "github.com/portpowered/infinite-you/pkg/services/work"
)
type invocationInputFake struct {
  prepare func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error)
}
func (fake invocationInputFake) PrepareInvocationInput(ctx context.Context, request work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
  return fake.prepare(ctx, request)
}
`)

	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		t.Fatalf("scanTestBehaviorBoundaries() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestPartitionTestBehaviorFindingsRejectsNewCountChangesAndStaleEntries(t *testing.T) {
	t.Parallel()
	finding := testBehaviorFinding{
		Kind: testBehaviorPolicyKind, Owner: "workers", ImportPath: workersImportPath,
		Symbol: "LoadMockWorkersConfig", FilePath: "tests/functional/workers_test.go", Count: 2,
	}
	entry := testBehaviorBaselineEntry{
		Kind: finding.Kind, Owner: finding.Owner, ImportPath: finding.ImportPath,
		Symbol: finding.Symbol, FilePath: finding.FilePath, Count: finding.Count,
		Stage: testBehaviorBaselineStage, DeletionGate: testBehaviorBaselineDeletionGate,
	}

	blocking, stale, err := partitionTestBehaviorFindings([]testBehaviorFinding{finding}, testBehaviorBaseline{Version: 1, Entries: []testBehaviorBaselineEntry{entry}})
	if err != nil || len(blocking) != 0 || len(stale) != 0 {
		t.Fatalf("matching baseline = blocking %#v stale %#v err %v", blocking, stale, err)
	}

	increased := finding
	increased.Count++
	blocking, stale, err = partitionTestBehaviorFindings([]testBehaviorFinding{increased}, testBehaviorBaseline{Version: 1, Entries: []testBehaviorBaselineEntry{entry}})
	if err != nil || len(blocking) != 1 || len(stale) != 0 {
		t.Fatalf("increased count = blocking %#v stale %#v err %v", blocking, stale, err)
	}

	blocking, stale, err = partitionTestBehaviorFindings(nil, testBehaviorBaseline{Version: 1, Entries: []testBehaviorBaselineEntry{entry}})
	if err != nil || len(blocking) != 0 || len(stale) != 1 {
		t.Fatalf("removed finding = blocking %#v stale %#v err %v", blocking, stale, err)
	}

	blocking, stale, err = partitionTestBehaviorFindings([]testBehaviorFinding{finding}, testBehaviorBaseline{})
	if err != nil || len(blocking) != 1 || len(stale) != 0 {
		t.Fatalf("new finding = blocking %#v stale %#v err %v", blocking, stale, err)
	}
}

func TestTestBehaviorBaselineRejectsWildcardAndEmptyInventories(t *testing.T) {
	t.Parallel()
	wildcard := testBehaviorBaselineEntry{
		Kind: testBehaviorPolicyKind, Owner: "workers", ImportPath: workersImportPath,
		Symbol: "Load*", FilePath: "tests/functional/*.go", Count: 1,
		Stage: testBehaviorBaselineStage, DeletionGate: testBehaviorBaselineDeletionGate,
	}
	if err := validateTestBehaviorBaselineEntry(wildcard); err == nil || !strings.Contains(err.Error(), "wildcards") {
		t.Fatalf("validate wildcard error = %v, want wildcard rejection", err)
	}
	wrongScope := testBehaviorBaselineEntry{
		Kind: testBehaviorPolicyKind, Owner: "factory_sessions", ImportPath: factorySessionsImportPath,
		Symbol: "ApplySessionListScope", FilePath: "tests/functional/session_list_test.go", Count: 1,
		Stage: testBehaviorBaselineStage, DeletionGate: testBehaviorBaselineDeletionGate,
	}
	if err := validateTestBehaviorBaselineEntry(wrongScope); err == nil || !strings.Contains(err.Error(), "outside pkg/transports") {
		t.Fatalf("validate transport-only scope error = %v, want scope rejection", err)
	}

	repoRoot := t.TempDir()
	payload, err := json.Marshal(testBehaviorBaseline{Version: 1, Entries: []testBehaviorBaselineEntry{}})
	if err != nil {
		t.Fatalf("marshal empty baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, testBehaviorBaselinePath), payload, 0o644); err != nil {
		t.Fatalf("write empty baseline: %v", err)
	}
	if _, err := loadTestBehaviorBaseline(repoRoot); err == nil || !strings.Contains(err.Error(), "delete the file to record zero debt") {
		t.Fatalf("load empty baseline error = %v, want empty-baseline rejection", err)
	}
}

func testBehaviorFindingSummary(findings []testBehaviorFinding) string {
	var rows []string
	for _, finding := range findings {
		rows = append(rows, strings.Join([]string{
			finding.FilePath,
			finding.Kind,
			strings.TrimPrefix(finding.ImportPath, repositoryImportPrefix),
			finding.Symbol,
			strconv.Itoa(finding.Count),
		}, "|"))
	}
	return strings.Join(rows, "\n")
}
