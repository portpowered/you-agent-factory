package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

func TestConstructionReportsPreserveExistingCommandStatus(t *testing.T) {
	t.Parallel()
	for _, blocking := range []bool{false, true} {
		name := "report-only"
		if blocking {
			name = "existing-blocker"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeGoSourceFile(t, root, "go.mod", "module example.test/factory\n\ngo 1.25.0\n")
			writeGoSourceFile(t, root, "pkg/services/example/service.go", `package example
type Service struct{}
func New() *Service { return &Service{} }
func (s *Service) Run() { New() }
`)
			const owner = "example.test/factory/pkg/services/example"
			registry := contractguard.ConstructionRegistry{
				CapabilitySets: []contractguard.ConstructionCapabilitySet{{Name: "example", OwnerTask: "T01", Mode: contractguard.ConstructionReport}},
				Constructors:   []contractguard.ConstructionConstructor{{Symbol: contractguard.ConstructionSymbol{ImportPath: owner, Name: "New"}, CapabilitySet: "example", Results: []contractguard.ConstructionSymbol{{ImportPath: owner, Name: "Service"}}}},
				Types:          []contractguard.ConstructionType{{Symbol: contractguard.ConstructionSymbol{ImportPath: owner, Name: "Service"}, CapabilitySet: "example", Kind: contractguard.ConstructionBehavior}},
			}
			if blocking {
				makeDir(t, root, "pkg/unapproved")
			}
			var stdout, stderr bytes.Buffer
			err := run(config{root: root, packageRoot: defaultScanRoot, constructionRegistry: &registry}, &stdout, &stderr)
			if (err != nil) != blocking {
				t.Fatalf("run status = %v; blocking=%v", err, blocking)
			}
			output := stdout.String()
			if blocking {
				output = stderr.String()
				if !strings.Contains(output, "unapproved root package family") {
					t.Fatalf("existing diagnostic lost: %q", output)
				}
			}
			for _, want := range []string{"pkg/services/example/service.go:4", owner + ".(Service).Run", "callee=" + owner + ".New", "mode=report"} {
				if !strings.Contains(output, want) {
					t.Fatalf("output %q missing %q", output, want)
				}
			}
		})
	}
}

func TestConstructionReportIsNotAdmittedToRecordedBaseline(t *testing.T) {
	t.Parallel()
	finding := contractguard.ConstructionFinding{Mode: contractguard.ConstructionEnforce, Rule: "registered-construction"}
	visible, _ := filterRecordedScanResult(scanResult{constructionFindings: []contractguard.ConstructionFinding{finding}}, recordedBoundaryBaseline{})
	if len(visible.constructionFindings) != 1 || countBlockingViolations(visible) != 1 {
		t.Fatal("construction finding lost to existing baseline partition")
	}
}

func TestHistoricalBoundaryDebtSurvivesNewConstructionMetadata(t *testing.T) {
	t.Parallel()
	for _, oldConstructor := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent-owner", true: "older-signature"}[oldConstructor], func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeGoSourceFile(t, root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
			writeGoImportFile(t, root, "pkg/services/work/recorded.go", "work", repositoryImportPrefix+"pkg/transports/http")
			const servicePath = "pkg/services/chat_sessions/internal/service/service.go"
			if oldConstructor {
				writeGoSourceFile(t, root, servicePath, "package service\ntype Service struct{}\nfunc New() *Service { return &Service{} }\n")
			}
			commitRecordedBoundaryFixture(t, root)
			writeGoSourceFile(t, root, "pkg/services/operator_settings/service.go", "package operator_settings\ntype Service interface{}\n")
			writeGoSourceFile(t, root, "pkg/services/factory_definitions/service.go", "package factory_definitions\ntype CatalogPathsService interface{}\n")
			writeGoSourceFile(t, root, "pkg/platform/logging/logger.go", "package logging\ntype Logger interface{}\n")
			writeGoSourceFile(t, root, servicePath, `package service
import (
 "github.com/portpowered/infinite-you/pkg/services/operator_settings"
 "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
 "github.com/portpowered/infinite-you/pkg/platform/logging"
)
type Service struct{}
func New(a operator_settings.Service, b factory_definitions.CatalogPathsService, c logging.Logger) *Service {
 if a == nil { panic("required") }
 return &Service{}
}
`)
			writeGoSourceFile(t, root, "pkg/services/chat_sessions/wire/wire.go", `package wire
import (
 "github.com/portpowered/infinite-you/pkg/services/operator_settings"
 "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
 "github.com/portpowered/infinite-you/pkg/platform/logging"
 "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
)
func NewFactoryTargetCatalogService(a operator_settings.Service, b factory_definitions.CatalogPathsService, c logging.Logger) *service.Service { return service.New(a,b,c) }
`)
			for _, all := range []bool{false, true} {
				var stdout, stderr bytes.Buffer
				if err := run(config{root: root, packageRoot: defaultScanRoot, baseRef: "HEAD", all: all}, &stdout, &stderr); err != nil {
					t.Fatalf("all=%v: historical debt became blocking: %v; stderr=%s", all, err, &stderr)
				}
				if strings.Contains(stdout.String(), "recorded.go") != all || stderr.Len() != 0 {
					t.Fatalf("all=%v: unexpected historical diagnostics: stdout=%s stderr=%s", all, &stdout, &stderr)
				}
				if !strings.Contains(stdout.String(), "rule=required-dependency-guard") {
					t.Fatalf("current construction observation lost: %s", &stdout)
				}
			}
			writeGoImportFile(t, root, "pkg/services/work/new.go", "work", repositoryImportPrefix+"pkg/transports/http")
			var stdout, stderr bytes.Buffer
			err := run(config{root: root, packageRoot: defaultScanRoot, baseRef: "HEAD"}, &stdout, &stderr)
			if err == nil || err.Error() != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" ||
				!strings.Contains(stderr.String(), "new.go") || strings.Contains(stderr.String(), "recorded.go") {
				t.Fatalf("new debt must block independently: err=%v stderr=%s", err, &stderr)
			}
			writeGoSourceFile(t, root, servicePath, "package service\ntype Service struct{}\nfunc New() *Service { return &Service{} }\n")
			if err := run(config{root: root, packageRoot: defaultScanRoot, baseRef: "HEAD"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "construction registry:") || !strings.Contains(err.Error(), "invalid or duplicate parameter index") {
				t.Fatalf("current registry mismatch must remain actionable: %v", err)
			}
		})
	}
}

func TestFocusedProviderDispatchDebtControlsCommandStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, helper, rule string
	}{
		{"callback", "callback();", "", "unresolved-focused-provider-dispatch"},
		{"returned-acyclic", "factory()();", "func factory() func() { return func() {} }", ""},
		{"returned-recursive", "factory()();", "func factory() func() { return func() { Provide(nil) } }", "registered-construction"},
		{"returned-unsupported", "factory()();", "func factory() (next func()) { return func() {} }", "unresolved-focused-provider-dispatch"},
		{"field", "h := Hook{}; h.Run();", "", "unresolved-focused-provider-dispatch"},
	} {
		for _, mode := range []contractguard.ConstructionMode{contractguard.ConstructionReport, contractguard.ConstructionEnforce} {
			t.Run(tc.name+"-"+string(mode), func(t *testing.T) {
				t.Parallel()
				root, registry := focusedProviderReportFixture(t, tc.body, tc.helper, mode)
				var stdout, stderr bytes.Buffer
				err := run(config{root: root, packageRoot: defaultScanRoot, constructionRegistry: &registry}, &stdout, &stderr)
				if (err != nil) != (tc.rule != "" && mode == contractguard.ConstructionEnforce) {
					t.Fatalf("mode %s: unexpected command status %v; stderr=%s", mode, err, &stderr)
				}
				output := stdout.String() + stderr.String()
				if tc.rule == "" {
					if strings.Contains(output, "rule=") || stderr.Len() != 0 {
						t.Fatalf("acyclic result acquired construction debt: %q", output)
					}
					return
				}
				for _, want := range []string{"pkg/wire/provider.go:3", "caller=" + registry.Allowances[0].Caller.String(), "callee=" + registry.Constructors[0].Symbol.String(), "mode=" + string(mode), "rule=" + tc.rule} {
					if !strings.Contains(output, want) {
						t.Fatalf("output %q missing %q", output, want)
					}
				}
				if strings.Contains(output, tc.body) {
					t.Fatalf("diagnostic exposed source text: %q", output)
				}
			})
		}
	}
}

func focusedProviderReportFixture(t *testing.T, body, helper string, mode contractguard.ConstructionMode) (string, contractguard.ConstructionRegistry) {
	t.Helper()
	root := t.TempDir()
	writeGoSourceFile(t, root, "go.mod", "module example.test/factory\n\ngo 1.25.0\n")
	writeGoSourceFile(t, root, "pkg/services/example/service.go", `package example
 type Service struct{}
 func New() *Service { return &Service{} }
 `)
	writeGoSourceFile(t, root, "pkg/wire/provider.go", `package wire
 import "example.test/factory/pkg/services/example"
 func Provide(callback func()) { `+body+` example.New() }
 type Hook struct { Run func() }
 `)
	writeGoSourceFile(t, root, "pkg/wire/helpers.go", "package wire\n"+helper)
	const owner = "example.test/factory/pkg/services/example"
	constructor := contractguard.ConstructionSymbol{ImportPath: owner, Name: "New"}
	provider := contractguard.ConstructionSymbol{ImportPath: "example.test/factory/pkg/wire", Name: "Provide"}
	service := contractguard.ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry := contractguard.ConstructionRegistry{
		CapabilitySets: []contractguard.ConstructionCapabilitySet{{Name: "example", OwnerTask: "T20", Mode: mode}},
		Constructors:   []contractguard.ConstructionConstructor{{Symbol: constructor, CapabilitySet: "example", Results: []contractguard.ConstructionSymbol{service}}},
		Types:          []contractguard.ConstructionType{{Symbol: service, CapabilitySet: "example", Kind: contractguard.ConstructionBehavior}},
		Allowances: []contractguard.ConstructionAllowance{{Caller: provider, Callee: constructor, FilePath: "pkg/wire/provider.go",
			Kind: "focused-provider", OwnerTask: "T20", Reason: "direct focused construction"}},
	}
	return root, registry
}
