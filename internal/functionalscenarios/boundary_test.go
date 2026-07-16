package functionalscenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFunctionalTestBoundariesRejectsDirectImplementations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		importPath string
		alias      string
		symbol     string
		boundary   string
	}{
		{name: "service", importPath: "pkg/service", alias: "service", symbol: "BuildFactoryService", boundary: "service"},
		{name: "runtime", importPath: "pkg/factory/runtime", alias: "runtime", symbol: "New", boundary: "runtime"},
		{name: "handler", importPath: "pkg/transports/http", alias: "transport", symbol: "NewServer", boundary: "handler"},
		{name: "repository", importPath: "pkg/factory/sessions/execution/runtimepersist", alias: "runtimepersist", symbol: "NewDirectoryStore", boundary: "repository"},
		{name: "recorder", importPath: "pkg/replay", alias: "replay", symbol: "NewRecorder", boundary: "recorder"},
		{name: "projection", importPath: "pkg/factory/projections", alias: "projections", symbol: "ProjectInitialStructure", boundary: "projection"},
		{name: "poller", importPath: "pkg/service/poller", alias: "poller", symbol: "New", boundary: "poller"},
		{name: "supervisor", importPath: "pkg/factory/supervisor", alias: "supervisor", symbol: "Start", boundary: "supervisor"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeBoundaryFixture(t, root, `package fixture
import `+test.alias+` "github.com/portpowered/infinite-you/`+test.importPath+`"
func direct() { `+test.alias+`.`+test.symbol+`() }
`)
			err := CheckFunctionalTestBoundaries(root)
			want := `functional test boundary [direct-product-boundary]: tests/functional/fixture_test.go:3 directly uses ` + test.boundary + ` implementation`
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), test.importPath+"."+test.symbol) || !strings.Contains(err.Error(), "REST, CLI, MCP, or SSE") {
				t.Fatalf("CheckFunctionalTestBoundaries() error = %v, want stable %s diagnostic and remediation", err, test.boundary)
			}
		})
	}
}

func TestCheckFunctionalTestBoundariesAllowsApprovedSeams(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeBoundaryFixture(t, root, `package fixture
import (
    restclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
    contract "github.com/portpowered/infinite-you/pkg/transports/http/generated"
    clidriver "github.com/portpowered/infinite-you/pkg/transports/cli/run"
    mcpclient "github.com/portpowered/infinite-you/pkg/transports/mcp/client"
    wire "github.com/portpowered/infinite-you/pkg/wire"
    provider "github.com/portpowered/infinite-you/pkg/workers/provider"
    service "github.com/portpowered/infinite-you/pkg/service"
)
var _ = contract.SubmitWorkJSONRequestBody{}
var _ = service.FactoryServiceConfig{}
func allowed() {
    _, _ = restclient.NewClient("http://example.invalid")
    _ = clidriver.Options{}
    _ = mcpclient.Options{}
    _ = wire.FactoryServiceSet
    _ = provider.NewFakeProvider()
    _ = service.NewHostedWorkersConfig()
}
`)
	if err := CheckFunctionalTestBoundaries(root); err != nil {
		t.Fatalf("CheckFunctionalTestBoundaries() error = %v", err)
	}
}

func TestCheckFunctionalTestBoundariesRejectsInvocationOnImplementationValue(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeBoundaryFixture(t, root, `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
func direct(svc *service.FactoryService) { svc.Run(nil) }
`)
	err := CheckFunctionalTestBoundaries(root)
	want := `directly uses service implementation "github.com/portpowered/infinite-you/pkg/service.FactoryService.Run"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckFunctionalTestBoundaries() error = %v, want %q", err, want)
	}
}

func TestCheckFunctionalTestBoundariesRejectsImplementationFunctionValue(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeBoundaryFixture(t, root, `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
func direct() { build := service.BuildFactoryService; build() }
`)
	err := CheckFunctionalTestBoundaries(root)
	want := `directly uses service implementation "github.com/portpowered/infinite-you/pkg/service.BuildFactoryService"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckFunctionalTestBoundaries() error = %v, want %q", err, want)
	}
}

func TestCheckFunctionalTestBoundariesRejectsNestedImplementationReceiver(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeBoundaryFixture(t, root, `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
type holder struct { service *service.FactoryService }
func direct(value holder) { value.service.Run(nil) }
`)
	err := CheckFunctionalTestBoundaries(root)
	want := `directly uses service implementation "github.com/portpowered/infinite-you/pkg/service.FactoryService.Run"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckFunctionalTestBoundaries() error = %v, want %q", err, want)
	}
}

func TestCheckFunctionalTestBoundariesRejectsTypeResolvedImplementationForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "type alias receiver",
			content: `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
type serviceAlias = service.FactoryService
func direct(svc *serviceAlias) { svc.Run(nil) }
`,
		},
		{
			name: "indexed receiver",
			content: `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
func direct(services map[string]*service.FactoryService) { services["primary"].Run(nil) }
`,
		},
		{
			name: "method expression",
			content: `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
func direct(svc *service.FactoryService) { run := service.FactoryService.Run; run(svc, nil) }
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeBoundaryFixture(t, root, test.content)
			err := CheckFunctionalTestBoundaries(root)
			want := `directly uses service implementation "github.com/portpowered/infinite-you/pkg/service.FactoryService.Run"`
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("CheckFunctionalTestBoundaries() error = %v, want %q", err, want)
			}
		})
	}
}

func TestCheckFunctionalTestBoundariesIgnoresFilesOutsideFunctionalTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "pkg", "service", "service_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create package test fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(`package service_test
import "github.com/portpowered/infinite-you/pkg/service"
func direct() { service.BuildFactoryService() }
`), 0o644); err != nil {
		t.Fatalf("write package test fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tests", "functional"), 0o755); err != nil {
		t.Fatalf("create functional fixture directory: %v", err)
	}
	if err := CheckFunctionalTestBoundaries(root); err != nil {
		t.Fatalf("CheckFunctionalTestBoundaries() error = %v", err)
	}
}

func TestCheckFunctionalTestBoundariesQuarantinesExactLegacyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
func direct() { service.BuildFactoryService() }
`
	writeBoundaryFixture(t, root, content)
	writeBoundaryBaselineFixture(t, root, content)

	report, err := CheckFunctionalTestBoundariesReport(root)
	if err != nil {
		t.Fatalf("CheckFunctionalTestBoundariesReport() error = %v", err)
	}
	if report.BaselinedLegacyFiles != 1 {
		t.Fatalf("BaselinedLegacyFiles = %d, want 1", report.BaselinedLegacyFiles)
	}
}

func TestCheckFunctionalTestBoundariesQuarantinesWindowsCheckoutOfReviewedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	reviewedContent := "package fixture\n" +
		"import service \"github.com/portpowered/infinite-you/pkg/service\"\n" +
		"func direct() { service.BuildFactoryService() }\n"
	windowsCheckoutContent := strings.ReplaceAll(reviewedContent, "\n", "\r\n")
	writeBoundaryFixture(t, root, windowsCheckoutContent)
	writeBoundaryBaselineFixture(t, root, reviewedContent)

	report, err := CheckFunctionalTestBoundariesReport(root)
	if err != nil {
		t.Fatalf("CheckFunctionalTestBoundariesReport() error = %v", err)
	}
	if report.BaselinedLegacyFiles != 1 {
		t.Fatalf("BaselinedLegacyFiles = %d, want 1", report.BaselinedLegacyFiles)
	}
}

func TestCheckFunctionalTestBoundariesRejectsChangedLegacyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := `package fixture
import service "github.com/portpowered/infinite-you/pkg/service"
func direct() { service.BuildFactoryService() }
`
	writeBoundaryFixture(t, root, content)
	writeBoundaryBaselineFixture(t, root, content+"// reviewed before this change\n")

	_, err := CheckFunctionalTestBoundariesReport(root)
	want := `functional test boundary [invalid-boundary-baseline]: baseline path "tests/functional/fixture_test.go" changed`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckFunctionalTestBoundariesReport() error = %v, want %q", err, want)
	}
}

func TestCheckFunctionalTestBoundariesRejectsStaleLegacyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := "package fixture\nfunc allowed() {}\n"
	writeBoundaryFixture(t, root, content)
	writeBoundaryBaselineFixture(t, root, content)

	_, err := CheckFunctionalTestBoundariesReport(root)
	want := `functional test boundary [invalid-boundary-baseline]: baseline path "tests/functional/fixture_test.go" has no direct product-boundary violation; remove its stale entry`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckFunctionalTestBoundariesReport() error = %v, want %q", err, want)
	}
}

func writeBoundaryFixture(t *testing.T, root, content string) {
	t.Helper()
	path := filepath.Join(root, "tests", "functional", "fixture_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create functional fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write functional fixture: %v", err)
	}
}

func writeBoundaryBaselineFixture(t *testing.T, root, hashedContent string) {
	t.Helper()
	hash := functionalBoundaryContentHash([]byte(hashedContent))
	content := fmt.Sprintf(`{"formatVersion":"functional-boundary-baseline/v1","migrationTask":"task.md","files":[{"path":"tests/functional/fixture_test.go","sha256":"%s"}]}`, hash)
	path := filepath.Join(root, filepath.FromSlash(functionalBoundaryBaselinePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create boundary baseline directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write boundary baseline: %v", err)
	}
}
