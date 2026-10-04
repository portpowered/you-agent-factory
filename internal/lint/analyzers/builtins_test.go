package analyzers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/OpenPeeDeeP/depguard/v2"
	"github.com/ashanbrown/forbidigo/v2/forbidigo"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"gopkg.in/yaml.v3"
)

// Test-only conversion of authored golangci settings into upstream component
// APIs. Real runner evidence owns merge-base filtering and exclusion processing.
type builtinConfig struct {
	Linters struct {
		Settings struct {
			Forbidigo struct {
				AnalyzeTypes    bool `yaml:"analyze-types"`
				ExcludeExamples bool `yaml:"exclude-godoc-examples"`
				Forbid          []struct {
					Pattern string
					Pkg     string
					Msg     string
				}
			}
			Depguard struct {
				Rules map[string]struct {
					ListMode string `yaml:"list-mode"`
					Files    []string
					Allow    []string
					Deny     []struct{ Pkg, Desc string }
				}
			}
		}
		Exclusions struct {
			Rules []struct {
				Linters    []string
				Path       string
				PathExcept string `yaml:"path-except"`
				Text       string
			}
		}
	}
}

func readBuiltinConfig(t *testing.T) builtinConfig {
	t.Helper()
	data, err := os.ReadFile("../../../.golangci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg builtinConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func builtinDepguard(t *testing.T, cfg builtinConfig) *analysis.Analyzer {
	t.Helper()
	settings := depguard.LinterSettings{}
	for name, rule := range cfg.Linters.Settings.Depguard.Rules {
		deny := map[string]string{}
		for _, entry := range rule.Deny {
			deny[entry.Pkg] = entry.Desc
		}
		settings[name] = &depguard.List{ListMode: rule.ListMode, Files: rule.Files, Allow: rule.Allow, Deny: deny}
	}
	analyzer, err := depguard.NewAnalyzer(&settings)
	if err != nil {
		t.Fatal(err)
	}
	return analyzer
}

func builtinForbidigo(t *testing.T, cfg builtinConfig) *forbidigo.Linter {
	t.Helper()
	var patterns []string
	for _, rule := range cfg.Linters.Settings.Forbidigo.Forbid {
		// The upstream API takes p, while golangci's authored key is pattern.
		data, err := yaml.Marshal(map[string]string{"p": rule.Pattern, "pkg": rule.Pkg, "msg": rule.Msg})
		if err != nil {
			t.Fatal(err)
		}
		patterns = append(patterns, string(data))
	}
	linter, err := forbidigo.NewLinter(patterns,
		forbidigo.OptionAnalyzeTypes(cfg.Linters.Settings.Forbidigo.AnalyzeTypes),
		forbidigo.OptionExcludeGodocExamples(cfg.Linters.Settings.Forbidigo.ExcludeExamples),
		forbidigo.OptionIgnorePermitDirectives(true))
	if err != nil {
		t.Fatal(err)
	}
	return linter
}

func (cfg builtinConfig) excluded(linter, path, message string) bool {
	for _, rule := range cfg.Linters.Exclusions.Rules {
		applies := false
		for _, name := range rule.Linters {
			applies = applies || name == linter
		}
		if !applies || (rule.Text != "" && !regexp.MustCompile(rule.Text).MatchString(message)) {
			continue
		}
		if rule.Path != "" && !regexp.MustCompile(rule.Path).MatchString(path) {
			continue
		}
		if rule.PathExcept != "" && regexp.MustCompile(rule.PathExcept).MatchString(path) {
			continue
		}
		return true
	}
	return false
}

func TestFunctionalBuiltins(t *testing.T) {
	t.Parallel()
	cfg := readBuiltinConfig(t)
	imports, calls := builtinDepguard(t, cfg), builtinForbidigo(t, cfg)
	analyzer := &analysis.Analyzer{Name: "functionalbuiltins", Doc: "exercise upstream import and identifier rules"}
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		if _, err := imports.Run(pass); err != nil {
			return nil, err
		}
		for _, file := range pass.Files {
			issues, err := calls.RunWithConfig(forbidigo.RunConfig{Fset: pass.Fset, TypesInfo: pass.TypesInfo}, file)
			if err != nil {
				return nil, err
			}
			for _, issue := range issues {
				path := filepath.ToSlash(pass.Fset.Position(issue.Pos()).Filename)
				_, path, _ = strings.Cut(path, "/src/m/")
				if !cfg.excluded("forbidigo", path, issue.Details()) {
					pass.Report(analysis.Diagnostic{Pos: issue.Pos(), Message: issue.Details()})
				}
			}
		}
		return nil, nil
	}
	analysistest.Run(t, builtinFixtures(t), analyzer,
		"m/tests/functional/osboundary", "m/tests/functional/osalias", "m/tests/functional/osdot",
		"m/tests/functional/internal/support/osboundary", "m/tests/functional/testdata/osboundary",
		"m/tests/integration/osboundary", "m/pkg/osboundary", "m/pkg/wire",
		"m/pkg/osboundary/localexec", "m/tests/functional/composition",
		"m/tests/integration/composition", "github.com/portpowered/infinite-you/tests/functional/composition",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/tests/functional/composition")
}

// Keep the outer fixture root outside testdata: the authored testdata exclusion
// must apply to nested controls, without excluding every analysistest input.
func builtinFixtures(t *testing.T) string {
	t.Helper()
	files := map[string]string{}
	for _, path := range []string{
		"m/tests/functional/osboundary/helper.go",
		"m/tests/functional/osalias/alias_test.go",
		"m/tests/functional/osdot/dot_test.go",
		"m/tests/functional/internal/support/osboundary/allowed.go",
		"m/tests/functional/testdata/osboundary/allowed.go",
		"m/tests/integration/osboundary/allowed.go",
		"m/pkg/osboundary/production.go",
		"m/pkg/osboundary/allowed_test.go",
		"m/pkg/osboundary/localexec/local.go",
		"m/pkg/wire/profiles.go",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl/stub.go",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/tests/functional/composition/imports.go",

		"github.com/portpowered/infinite-you/tests/functional/composition/imports.go",
		"github.com/portpowered/infinite-you/pkg/initializer/stub.go",
		"github.com/portpowered/infinite-you/pkg/platform/runtimeinput/stub.go",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/stub.go",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/scaffold/stub.go",
		"github.com/portpowered/infinite-you/pkg/services/recordings/artifacts/stub.go",
		"github.com/portpowered/infinite-you/pkg/services/recordings/events/stub.go",
		"github.com/portpowered/infinite-you/pkg/services/recordings/projections/stub.go",
		"github.com/portpowered/infinite-you/pkg/services/recordings/replay/stub.go",
		"github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection/stub.go",
		"github.com/portpowered/infinite-you/pkg/wire/stub.go",
		"m/tests/functional/composition/calls.go",
		"m/tests/integration/composition/allowed.go",
	} {
		data, err := os.ReadFile(filepath.Join(analysistest.TestData(), "src", path))
		if err != nil {
			t.Fatal(err)
		}
		files[path] = string(data)
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return dir
}
