package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

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

func TestPartitionTestBehaviorFindingsRejectsNewCountChangesAndStaleEntries(t *testing.T) {
	t.Parallel()
	finding := testBehaviorFinding{
		Kind: testBehaviorPolicyKind, Owner: "factory_sessions", ImportPath: factorySessionsImportPath,
		Symbol: "ApplySessionListScope", FilePath: "pkg/transports/cli/session/list_test.go", Count: 2,
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
