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
	writeTestFile(t, repoRoot, "pkg/services/orders/services/history/service.go", "package history\ntype Service interface { List() []Result }\ntype Result struct { ID string }\n")
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
		"factory", "provider_sessions", "events", "models", "guards", "resources",
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

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{
		ruleFunctionalShallowFile + "|tests/functional/workers/shallow_test.go|workers",
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
	writeTestFile(t, repoRoot, "pkg/services/orders/services/helpers.go", "package services\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	want := []string{
		ruleServiceContainerGoFile + "|pkg/services/orders/services/helpers.go|pkg/services/orders",
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
	writeTestFile(t, repoRoot, "pkg/services/orders/services/history/result.go", "package history\ntype Result struct { ID string }\n")

	findings, err := scan(repoRoot)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	assertFindingKeys(t, findings, []string{ruleServiceInterfaceCount + "|pkg/services/orders/services/history|<none>"})
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
