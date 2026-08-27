package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanAcceptsRecursiveServiceShapeAndDomainSubsectionTests(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", `package orders
type Service interface { Get() (Response, error) }
type Request struct { ID string }
type Response struct { ID string }
`)
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/service.go", "package internal\nfunc run() {}\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/wire/providers.go", "package wire\nfunc provide() {}\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/transports/http/handler.go", "package http\ntype Handler struct{}\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() []Result }\ntype Result struct { ID string }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/service.go", "package internal\nfunc run() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/workers/script/execution_test.go", "package script_test\nfunc TestExecution() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/models/model_invoke/ready_test.go", "package model_invoke_test\nfunc TestReady() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/internal/support/process.go", "package support\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("scan() findings = %#v, want none", findings)
	}
}

func TestAllowedFunctionalDomainsMatchExpansionPlan(t *testing.T) {
	t.Parallel()
	want := []string{
		"transport", "workers", "orchestration", "workstations", "work", "sessions",
		"factory", "providers", "provider_sessions", "events", "recordings", "models", "guards", "resources",
		"observability", "product", "resilience",
	}
	if len(allowedFunctionalDomains) != len(want) {
		t.Fatalf("allowedFunctionalDomains len = %d, want %d", len(allowedFunctionalDomains), len(want))
	}
	for _, domain := range want {
		if !isAllowedFunctionalDomain(domain) {
			t.Fatalf("domain %q missing from allowlist", domain)
		}
	}
	if isAllowedFunctionalDomain("orders") || isAllowedFunctionalDomain("smoke") || isAllowedFunctionalDomain("workflow") {
		t.Fatal("catch-all or unclassified roots must not be approved domains")
	}
}

func TestScanAcceptsApprovedDomainSubsectionWithoutDebt(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/workers/script/create_test.go", "package script_test\nfunc TestCreate() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/providers/contract/custom_integration_test.go", "package contract_test\nfunc TestCustomIntegration() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/providers/gemini/invoke_test.go", "package gemini_test\nfunc TestInvoke() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/recordings/process/flush_test.go", "package process_test\nfunc TestFlush() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/transport/cli/flag_parsing_test.go", "package cli_test\nfunc TestFlagParsing() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/work/visualization/graph_test.go", "package visualization_test\nfunc TestGraph() {}\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("approved domain/subsection paths must not be structure debt; findings = %#v", findings)
	}
}

func TestScanStillRequiresSubsectionDepthForApprovedDomains(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/workers/shallow_test.go", "package workers_test\nfunc TestShallow() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/providers/root_test.go", "package providers_test\nfunc TestRoot() {}\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleFunctionalShallowFile + "|tests/functional/providers/root_test.go|providers",
		ruleFunctionalShallowFile + "|tests/functional/workers/shallow_test.go|workers",
	})
}

func TestScanRejectsNewShallowCatchAllAndUnclassifiedPackages(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/smoke/new_smoke_test.go", "package smoke_test\nfunc TestNewSmoke() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/workflow/new_workflow_test.go", "package workflow_test\nfunc TestNewWorkflow() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/cli/session/new_cli_test.go", "package session_test\nfunc TestNewCLI() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/legacy_bucket/orphan_test.go", "package legacy_bucket_test\nfunc TestOrphan() {}\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleFunctionalShallowFile + "|tests/functional/legacy_bucket/orphan_test.go|legacy_bucket",
		ruleFunctionalShallowFile + "|tests/functional/smoke/new_smoke_test.go|smoke",
		ruleFunctionalShallowFile + "|tests/functional/workflow/new_workflow_test.go|workflow",
		ruleFunctionalUnclassifiedDomain + "|tests/functional/cli/session/new_cli_test.go|cli",
	})
	for _, item := range findings {
		remediation := deletionGates[item.Rule]
		if !strings.Contains(remediation, "tests/functional/<domain>/<subsection>/...") {
			t.Fatalf("remediation for %s = %q, want domain/subsection guidance", item.Rule, remediation)
		}
	}
}

func TestRunRejectsUnrecordedUnclassifiedDebtAndStaleBaseline(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/cli/session/existing_test.go", "package session_test\nfunc TestExisting() {}\n")
	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	writeBaseline(t, repoRoot, findings)

	if err := run(config{root: repoRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() with baselined unclassified debt error = %v", err)
	}

	writeTestFile(t, repoRoot, "tests/functional/cli/session/extra_test.go", "package session_test\nfunc TestExtra() {}\n")
	stderr := &bytes.Buffer{}
	err = run(config{root: repoRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "new violation") || !strings.Contains(stderr.String(), ruleFunctionalUnclassifiedDomain) {
		t.Fatalf("run() with new unclassified debt error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "tests/functional/<domain>/<subsection>/...") {
		t.Fatalf("new violation remediation missing domain guidance: %q", stderr.String())
	}

	if err := os.Remove(filepath.Join(repoRoot, "tests", "functional", "cli", "session", "extra_test.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repoRoot, "tests", "functional", "cli", "session", "existing_test.go")); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	err = run(config{root: repoRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "stale baseline") {
		t.Fatalf("run() with stale unclassified debt error = %v, stderr = %q", err, stderr.String())
	}
}

func TestScanRejectsPublicSiblingServicesContainer(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/services/history/service.go", "package history\ntype Service interface { List() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/services/history/internal/store.go", "package internal\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleServiceUnexpectedDir + "|pkg/services/orders/services|pkg/services/orders",
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run(config{root: repoRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want public sibling services/ violation")
	}
	if !strings.Contains(stderr.String(), ruleServiceUnexpectedDir) ||
		!strings.Contains(stderr.String(), "pkg/services/orders/services") {
		t.Fatalf("run() stderr = %q, want reviewer-visible rule and path for public sibling services/", stderr.String())
	}
}

func TestScanAcceptsParentPrivateInternalServicesContainer(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/store.go", "package internal\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("scan() findings = %#v, want none for parent-private internal/services nesting", findings)
	}
}

func TestScanAcceptsDoubleNestedInternalServices(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/store.go", "package internal\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/services/archive/service.go", "package archive\ntype Service interface { Compact() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/services/archive/internal/compact.go", "package internal\nfunc compact() {}\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("scan() findings = %#v, want none for double-nested internal/services/<child>/internal", findings)
	}
}

func TestScanRejectsUnexpectedDirectoryAtNestedSubservice(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/store.go", "package internal\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/legacy/note.go", "package legacy\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleServiceUnexpectedDir + "|pkg/services/orders/internal/services/history/legacy|pkg/services/orders/internal/services/history",
	})
}

func TestScanRejectsGoFileInNestedInternalServicesContainer(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/store.go", "package internal\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/services/helpers.go", "package services\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleServiceContainerGoFile + "|pkg/services/orders/internal/services/history/internal/services/helpers.go|pkg/services/orders/internal/services/history",
	})
}

func TestScanFindsServiceRootContractAndDirectoryViolations(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/api.go", `package orders
type Reader interface { Read() }
type Writer interface { Write() }
func New() Reader { return nil }
`)
	writeTestFile(t, repoRoot, "pkg/services/orders/legacy/implementation.go", "package legacy\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/helpers.go", "package services\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	want := []string{
		ruleServiceContainerGoFile + "|pkg/services/orders/internal/services/helpers.go|pkg/services/orders",
		ruleServiceExportedFunction + "|pkg/services/orders/api.go|New",
		ruleServiceInterfaceCount + "|pkg/services/orders|pkg/services/orders/api.go:Reader,pkg/services/orders/api.go:Writer",
		ruleServiceUnexpectedDir + "|pkg/services/orders/legacy|pkg/services/orders",
	}
	assertFindingKeys(t, findings, want)
}

func TestScanRequiresOneInterfaceForNestedSubservice(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/result.go", "package history\ntype Result struct { ID string }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/store.go", "package internal\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{ruleServiceInterfaceCount + "|pkg/services/orders/internal/services/history|<none>"})
}

func TestScanRejectsMissingSubserviceInternal(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() }\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleServiceMissingInternal + "|pkg/services/orders/internal/services/history/internal|pkg/services/orders/internal/services/history",
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run(config{root: repoRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want missing subservice internal violation")
	}
	if !strings.Contains(stderr.String(), ruleServiceMissingInternal) ||
		!strings.Contains(stderr.String(), "pkg/services/orders/internal/services/history/internal") {
		t.Fatalf("run() stderr = %q, want reviewer-visible rule and path for missing subservice internal", stderr.String())
	}
}

func TestScanAcceptsSubserviceWithInternalDoesNotEmitMissingInternal(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/store.go", "package internal\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	for _, item := range findings {
		if item.Rule == ruleServiceMissingInternal {
			t.Fatalf("scan() emitted unexpected missing-internal finding: %#v", item)
		}
	}
	if len(findings) != 0 {
		t.Fatalf("scan() findings = %#v, want none when subservice owns internal/", findings)
	}
}

func TestScanRejectsMissingInternalAtDeeperNesting(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/service.go", "package history\ntype Service interface { List() }\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/store.go", "package internal\n")
	writeTestFile(t, repoRoot, "pkg/services/orders/internal/services/history/internal/services/archive/service.go", "package archive\ntype Service interface { Compact() }\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleServiceMissingInternal + "|pkg/services/orders/internal/services/history/internal/services/archive/internal|pkg/services/orders/internal/services/history/internal/services/archive",
	})
}

func TestScanFindsShallowFunctionalSourcesAndRuntimeAPIDebt(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	writeTestFile(t, repoRoot, "tests/functional/orders/create_test.go", "package orders_test\nfunc TestCreate() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/runtime_api/session_test.go", "package runtime_api\nfunc TestStart() {}\nfunc helper() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/runtime_api/factory/status_test.go", "package factory\nfunc TestStatus() {}\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	want := []string{
		ruleRuntimeAPIFile + "|tests/functional/runtime_api/factory/status_test.go|tests/functional/runtime_api",
		ruleRuntimeAPIFile + "|tests/functional/runtime_api/session_test.go|tests/functional/runtime_api",
		ruleRuntimeAPITest + "|tests/functional/runtime_api/factory/status_test.go|TestStatus",
		ruleRuntimeAPITest + "|tests/functional/runtime_api/session_test.go|TestStart",
		ruleFunctionalShallowFile + "|tests/functional/orders/create_test.go|orders",
	}
	assertFindingKeys(t, findings, want)
}

func TestScanExemptsGenuineRuntimeAPITestMain(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/runtime_api/shared_test.go", `package runtime_api

import testpkg "testing"

func TestMain(m *testpkg.M) {}
func TestScenario() {}
`)

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleRuntimeAPIFile + "|tests/functional/runtime_api/shared_test.go|tests/functional/runtime_api",
		ruleRuntimeAPITest + "|tests/functional/runtime_api/shared_test.go|TestScenario",
	})
}

func TestScanRejectsNonLifecycleRuntimeAPITestMain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
	}{
		{name: "no parameter", source: "func TestMain() {}"},
		{name: "testing test parameter", source: "func TestMain(t *testing.T) {}"},
		{name: "wrong package", source: "func TestMain(m *other.M) {}"},
		{name: "result", source: "func TestMain(m *testing.M) int { return 0 }"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeTestFile(t, repoRoot, "tests/functional/runtime_api/scenario_test.go", "package runtime_api\n\nimport \"testing\"\n\n"+test.source+"\n")

			findings, err := scan(repoRoot)
			if err != nil {
				t.Fatalf("scan() error = %v", err)
			}
			assertFindingKeys(t, findings, []string{
				ruleRuntimeAPIFile + "|tests/functional/runtime_api/scenario_test.go|tests/functional/runtime_api",
				ruleRuntimeAPITest + "|tests/functional/runtime_api/scenario_test.go|TestMain",
			})
		})
	}
}

func TestScanPreservesInternalSupportExceptionOnly(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/internal/support/process.go", "package support\n")
	writeTestFile(t, repoRoot, "tests/functional/internal/support/cmd/harness/main.go", "package main\nfunc main() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/internal/restclient/adapter.go", "package restclient\n")
	writeTestFile(t, repoRoot, "tests/functional/internal/restclient/adapter_test.go", "package restclient\nfunc TestAdapter() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/internal/orphan.go", "package internal\n")
	writeTestFile(t, repoRoot, "tests/functional/shared/helpers/util.go", "package helpers\n")
	writeTestFile(t, repoRoot, "tests/functional/support/helpers/util.go", "package helpers\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleFunctionalShallowFile + "|tests/functional/internal/orphan.go|internal",
		ruleFunctionalUnclassifiedDomain + "|tests/functional/internal/restclient/adapter.go|internal",
		ruleFunctionalUnclassifiedDomain + "|tests/functional/internal/restclient/adapter_test.go|internal",
		ruleFunctionalUnclassifiedDomain + "|tests/functional/shared/helpers/util.go|shared",
		ruleFunctionalUnclassifiedDomain + "|tests/functional/support/helpers/util.go|support",
	})
}

func TestRunRejectsNewRuntimeAPIDebtAndStaleBaseline(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/runtime_api/session_test.go", "package runtime_api\nfunc TestStart() {}\n")
	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	writeBaseline(t, repoRoot, findings)

	if err := run(config{root: repoRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() with baselined runtime_api debt error = %v", err)
	}

	writeTestFile(t, repoRoot, "tests/functional/runtime_api/extra_file_test.go", "package runtime_api\nfunc TestExtraFile() {}\n")
	stderr := &bytes.Buffer{}
	err = run(config{root: repoRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "new violation") || !strings.Contains(stderr.String(), ruleRuntimeAPIFile) {
		t.Fatalf("run() with new runtime_api file error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), ruleRuntimeAPITest) || !strings.Contains(stderr.String(), "TestExtraFile") {
		t.Fatalf("run() with new runtime_api file missing scenario debt: %q", stderr.String())
	}

	if err := os.Remove(filepath.Join(repoRoot, "tests", "functional", "runtime_api", "extra_file_test.go")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repoRoot, "tests/functional/runtime_api/session_test.go", "package runtime_api\nfunc TestStart() {}\nfunc TestExtraScenario() {}\n")
	stderr.Reset()
	err = run(config{root: repoRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "new violation") || !strings.Contains(stderr.String(), "TestExtraScenario") {
		t.Fatalf("run() with new runtime_api Test* error = %v, stderr = %q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), ruleRuntimeAPITest) {
		t.Fatalf("run() with new runtime_api Test* missing rule: %q", stderr.String())
	}

	writeTestFile(t, repoRoot, "tests/functional/runtime_api/session_test.go", "package runtime_api\nfunc TestStart() {}\n")
	if err := os.Remove(filepath.Join(repoRoot, "tests", "functional", "runtime_api", "session_test.go")); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	err = run(config{root: repoRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "stale baseline") {
		t.Fatalf("run() with stale runtime_api debt error = %v, stderr = %q", err, stderr.String())
	}
}

func TestRunUsesDeletionOnlyBaseline(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\nfunc New() Service { return nil }\n")
	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	writeBaseline(t, repoRoot, findings)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "1 deletion-only baseline entries remain") {
		t.Fatalf("run() stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q", stderr.String())
	}

	writeTestFile(t, repoRoot, "pkg/services/orders/other.go", "package orders\nfunc Open() Service { return nil }\n")
	stderr.Reset()
	err = run(config{root: repoRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "new violation") {
		t.Fatalf("run() with new debt error = %v, stderr = %q", err, stderr.String())
	}

	if err := os.Remove(filepath.Join(repoRoot, "pkg", "services", "orders", "other.go")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\ntype Service interface { Get() }\n")
	stderr.Reset()
	err = run(config{root: repoRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "stale baseline") {
		t.Fatalf("run() with stale debt error = %v, stderr = %q", err, stderr.String())
	}
}

func TestCreateBaselineRefusesOverwrite(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "pkg/services/orders/service.go", "package orders\nfunc New() {}\n")
	if err := run(config{root: repoRoot, createBaseline: true}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("create baseline: %v", err)
	}
	if err := run(config{root: repoRoot, createBaseline: true}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second create baseline error = %v", err)
	}
}

// TestDomainLayoutEnforcementProof consolidates the accept/reject outcomes required
// by the domain-mirrored functional layout gate: conforming domain/subsection paths
// and internal/support are allowed; new shallow, unclassified/catch-all, and
// runtime_api scenarios are rejected.
func TestDomainLayoutEnforcementProof(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "tests/functional/workers/script/execution_test.go", "package script_test\nfunc TestExecution() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/internal/support/process.go", "package support\n")
	writeTestFile(t, repoRoot, "tests/functional/smoke/new_smoke_test.go", "package smoke_test\nfunc TestNewSmoke() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/cli/session/new_cli_test.go", "package session_test\nfunc TestNewCLI() {}\n")
	writeTestFile(t, repoRoot, "tests/functional/runtime_api/new_scenario_test.go", "package runtime_api\nfunc TestNewScenario() {}\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleRuntimeAPIFile + "|tests/functional/runtime_api/new_scenario_test.go|tests/functional/runtime_api",
		ruleRuntimeAPITest + "|tests/functional/runtime_api/new_scenario_test.go|TestNewScenario",
		ruleFunctionalShallowFile + "|tests/functional/smoke/new_smoke_test.go|smoke",
		ruleFunctionalUnclassifiedDomain + "|tests/functional/cli/session/new_cli_test.go|cli",
	})
}

func assertFindingKeys(t *testing.T, findings []finding, want []string) {
	t.Helper()
	got := make([]string, 0, len(findings))
	for _, item := range findings {
		got = append(got, item.Rule+"|"+item.FilePath+"|"+item.Target)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("finding keys:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func writeBaseline(t *testing.T, repoRoot string, findings []finding) {
	t.Helper()
	if err := createBaseline(repoRoot, findings, &bytes.Buffer{}); err != nil {
		t.Fatalf("createBaseline: %v", err)
	}
}

func writeTestFile(t *testing.T, repoRoot, relative, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
