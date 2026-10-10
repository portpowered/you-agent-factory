package analyzers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

// Reuse tiny controlled fixtures at the real canonical composition identity.
// Each invocation owns its GOPATH; production source is never copied or loaded.
func registeredProviderTestData(t *testing.T, unit string) string {
	t.Helper()
	files := map[string]string{}
	for source, target := range map[string]string{unit: "pkg/wire", "pkg/registeredowner": "pkg/registeredowner"} {
		entries, err := os.ReadDir(filepath.Join(analysistest.TestData(), "src/m", source))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			contents, err := os.ReadFile(filepath.Join(analysistest.TestData(), "src/m", source, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			files["m/"+target+"/"+entry.Name()] = string(contents)
		}
	}
	// Historical callable cases used labels rather than constructor names.
	// Keep their bodies and expectations while giving providers legal names.
	names := regexp.MustCompile(`func ([A-Z][A-Za-z0-9_]*)\(`)
	replacements := []string{}
	for path, source := range files {
		if strings.HasPrefix(path, "m/pkg/wire/") {
			for _, match := range names.FindAllStringSubmatch(source, -1) {
				if !serviceConstructorName(match[1]) {
					replacements = append(replacements, match[1], "New"+match[1])
				}
			}
		}
	}
	for path, source := range files {
		if strings.HasPrefix(path, "m/pkg/wire/") {
			for i := 0; i < len(replacements); i += 2 {
				source = regexp.MustCompile(`\b`+replacements[i]+`\b`).ReplaceAllString(source, replacements[i+1])
			}
			files[path] = source
		}
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return dir
}

// Historical source-only examples that are invalid Go belong to the compiler
// boundary, rather than successful construction-analysis observations.
func TestConstructionCompilerRejectsInvalidHistoricalCases(t *testing.T) {
	for _, test := range []struct{ unit, message string }{
		{"registeredinvalidcycle", "initialization cycle"},
		{"registeredinvalidselector", "ambiguous selector"},
		{"registeredinvalidalias", "invalid recursive type"},
		{"registeredinvalidunknown", "undefined: Unknown"},
	} {
		t.Run(test.unit, func(t *testing.T) {
			loaded, err := packages.Load(&packages.Config{
				Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
				Dir:  analysistest.TestData(),
				Env:  append(os.Environ(), "GO111MODULE=off", "GOPATH="+analysistest.TestData(), "GOWORK=off"),
			}, "m/pkg/"+test.unit)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, pkg := range loaded {
				for _, diagnostic := range pkg.Errors {
					if diagnostic.Kind == packages.TypeError && strings.Contains(diagnostic.Msg, test.message) {
						found = true
						if diagnostic.Pos == "" {
							t.Error("compiler error lost its position")
						}
					}
				}
			}
			if !found {
				t.Fatalf("compiler did not reject %s with %q", test.unit, test.message)
			}
		})
	}
}

// Compiler-loaded fixtures exercise only construction lint. Serial execution
// preserves the shared baseline fixture state; no application is assembled.
func TestConstructionRegisteredProviders(t *testing.T) {
	useFixtures(t)
	registry := registeredFixtureRegistry(ConstructionEnforce)
	analyzer := registeredConstructionAnalyzer(registry)
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		findings, err := run(pass)
		if err != nil || pass.Pkg.Path() != "m/pkg/wire" {
			return findings, err
		}
		reportRegistry := registry
		reportRegistry.CapabilitySets = []ConstructionCapabilitySet{{Name: "fixture", OwnerTask: "T20", Mode: ConstructionReport}}
		report, err := runRegisteredConstruction(pass, reportRegistry)
		if err != nil {
			return nil, err
		}
		actual, observations := findings.([]ConstructionFinding), report.([]ConstructionFinding)
		if len(actual) != 41 || len(observations) != len(actual) {
			t.Errorf("enforce/report=%d/%d, want 41/41", len(actual), len(observations))
		}
		for i, finding := range actual {
			if finding.Callee != registry.Constructors[0].Symbol || finding.Mode != ConstructionEnforce || finding.Line == 0 {
				t.Errorf("invalid finding: %#v", finding)
			}
			finding.Mode = ConstructionReport
			if i < len(observations) && finding != observations[i] {
				t.Errorf("report changed finding: %#v / %#v", finding, observations[i])
			}
		}
		return findings, nil
	}
	analysistest.Run(t, registeredProviderTestData(t, "pkg/registeredproviders"), analyzer, "m/pkg/wire")
}

func TestConstructionRegisteredProviderBaseline(t *testing.T) {
	const unit = "pkg/wire"
	const edge = "m/" + unit + "."
	const target = "->m/pkg/registeredowner.New"
	useFixtures(t,
		"registered-construction|"+unit+"|"+edge+"NewRecursive"+target,
		"unresolved-focused-provider-dispatch|"+unit+"|"+edge+"NewCallback"+target,
		"unresolved-focused-provider-dispatch|"+unit+"|"+edge+"NewRemoved"+target)
	registry := registeredFixtureRegistry(ConstructionEnforce)
	analysistest.Run(t, registeredProviderTestData(t, "pkg/registeredproviderlisted"), registeredConstructionAnalyzer(registry), "m/"+unit)
}

// Compile-valid retained provider cases live at the owning Wire boundary and diagnostic
// controls. Callable returns use the operator-authorized typed rule. Shared
// baseline fixture state requires serial execution.
func TestConstructionRegisteredProviderParity(t *testing.T) {
	useFixtures(t)
	cases := registeredProviderParityCases()
	for _, unit := range []string{"pkg/registeredprovidercycles", "pkg/registeredproviderdebt", "pkg/registeredproviderexecution"} {
		t.Run(unit, func(t *testing.T) {
			registry := registeredFixtureRegistry(ConstructionEnforce)
			expected := map[ConstructionSymbol]int{}
			for _, test := range cases {
				if test.unit != unit {
					continue
				}
				caller := ConstructionSymbol{ImportPath: "m/pkg/wire", Name: "New" + test.name}
				expected[caller] = test.count
			}
			analyzer := registeredConstructionAnalyzer(registry)
			run := analyzer.Run
			analyzer.Run = func(pass *analysis.Pass) (any, error) {
				result, err := run(pass)
				if err != nil {
					return result, err
				}
				reportRegistry := registry
				reportRegistry.CapabilitySets = []ConstructionCapabilitySet{{Name: "fixture", OwnerTask: "T20", Mode: ConstructionReport}}
				report, err := runRegisteredConstruction(pass, reportRegistry)
				if err != nil {
					return result, err
				}
				findings, observations := result.([]ConstructionFinding), report.([]ConstructionFinding)
				if len(findings) != len(observations) {
					t.Errorf("enforce/report count mismatch: %d/%d", len(findings), len(observations))
				}
				counts := map[ConstructionSymbol]int{}
				for i, finding := range findings {
					counts[finding.Caller]++
					if finding.Callee != registry.Constructors[0].Symbol || finding.Line == 0 || finding.Mode != ConstructionEnforce {
						t.Errorf("invalid finding: %#v", finding)
					}
					finding.Mode = ConstructionReport
					if i < len(observations) && finding != observations[i] {
						t.Errorf("report changed finding: %#v / %#v", finding, observations[i])
					}
				}
				for caller, count := range expected {
					if caller.ImportPath == pass.Pkg.Path() && counts[caller] != count {
						t.Errorf("%s count=%d, want %d", caller, counts[caller], count)
					}
				}
				return result, nil
			}
			analysistest.Run(t, registeredProviderTestData(t, unit), analyzer, "m/pkg/wire")
		})
	}
}

type registeredProviderParityCase struct {
	unit, name string
	count      int
}

func registeredProviderCycleCases() []registeredProviderParityCase {
	return []registeredProviderParityCase{
		{"pkg/registeredprovidercycles", "Case01", 1},
		{"pkg/registeredprovidercycles", "Case02", 1},
		{"pkg/registeredprovidercycles", "Case03", 1},
		{"pkg/registeredprovidercycles", "Case04", 1},
		{"pkg/registeredprovidercycles", "Case05", 1},
		{"pkg/registeredprovidercycles", "Case06", 1},
		{"pkg/registeredprovidercycles", "Case07", 1},
		{"pkg/registeredprovidercycles", "Case08", 1},
		{"pkg/registeredprovidercycles", "Case09", 1},
		{"pkg/registeredprovidercycles", "Case10", 1},
		{"pkg/registeredprovidercycles", "Case11", 1},
		{"pkg/registeredprovidercycles", "Case12", 1},
		{"pkg/registeredprovidercycles", "Case13", 1},
		{"pkg/registeredprovidercycles", "Case14", 1},
		{"pkg/registeredprovidercycles", "Case15", 1},
		{"pkg/registeredprovidercycles", "Case16", 1},
		{"pkg/registeredprovidercycles", "Case17", 1},
		{"pkg/registeredprovidercycles", "Case18", 1},
		{"pkg/registeredprovidercycles", "Case19", 1},
		{"pkg/registeredprovidercycles", "Case20", 1},
		{"pkg/registeredprovidercycles", "Case21", 1},
		{"pkg/registeredprovidercycles", "Case22", 0},
		{"pkg/registeredprovidercycles", "Case23", 0},
		{"pkg/registeredprovidercycles", "Case24", 0},
		{"pkg/registeredprovidercycles", "Case25", 0},
		{"pkg/registeredprovidercycles", "Case32", 0},
		{"pkg/registeredprovidercycles", "Case33", 0},
		{"pkg/registeredprovidercycles", "Case34", 0},
		{"pkg/registeredprovidercycles", "Case38", 0},
		{"pkg/registeredprovidercycles", "Case39", 0},
		{"pkg/registeredprovidercycles", "Case40", 0},
		{"pkg/registeredprovidercycles", "Case41", 0},
		{"pkg/registeredprovidercycles", "Case42", 0},
		{"pkg/registeredprovidercycles", "Case43", 0},
	}
}

func registeredProviderDispatchCases() []registeredProviderParityCase {
	return []registeredProviderParityCase{
		{"pkg/registeredproviderdebt", "Case06", 1},
		{"pkg/registeredproviderdebt", "Case07", 1},
		{"pkg/registeredproviderdebt", "Case08", 1},
		{"pkg/registeredproviderdebt", "Case09", 1},
		{"pkg/registeredproviderdebt", "Case10", 1},
		{"pkg/registeredproviderdebt", "Case11", 1},
		{"pkg/registeredproviderdebt", "Case12", 1},
		{"pkg/registeredproviderdebt", "Case13", 1},
		{"pkg/registeredproviderdebt", "Case14", 1},
		{"pkg/registeredproviderdebt", "Case15", 1},
		{"pkg/registeredproviderdebt", "Case16", 1},
		{"pkg/registeredproviderdebt", "Case17", 1},
		{"pkg/registeredproviderdebt", "Case18", 0},
		{"pkg/registeredproviderdebt", "Case19", 1},
		{"pkg/registeredproviderdebt", "Case20", 1},
		{"pkg/registeredproviderdebt", "Case21", 1},
		{"pkg/registeredproviderdebt", "Case22", 1},
		{"pkg/registeredproviderdebt", "Case23", 1},
		{"pkg/registeredproviderdebt", "Case24", 1},
		{"pkg/registeredproviderdebt", "Case25", 1},
		{"pkg/registeredproviderdebt", "Case26", 1},
		{"pkg/registeredproviderdebt", "Case27", 1},
		{"pkg/registeredproviderdebt", "Case28", 1},
		{"pkg/registeredproviderdebt", "Case29", 1},
		{"pkg/registeredproviderdebt", "Case30", 1},
		{"pkg/registeredproviderdebt", "Case31", 0},
		{"pkg/registeredproviderdebt", "Case32", 0},
		{"pkg/registeredproviderdebt", "Case33", 0},
		{"pkg/registeredproviderdebt", "Case34", 0},
		{"pkg/registeredproviderdebt", "Case35", 0},
		{"pkg/registeredproviderdebt", "Case36", 0},
		{"pkg/registeredproviderdebt", "Case37", 0},
		{"pkg/registeredproviderdebt", "Case38", 0},
		{"pkg/registeredproviderdebt", "Case39", 0},
		{"pkg/registeredproviderdebt", "Case40", 0},
		{"pkg/registeredproviderdebt", "Case41", 0},
		{"pkg/registeredproviderexecution", "Case01", 0},
		{"pkg/registeredproviderexecution", "Case02", 0},
		{"pkg/registeredproviderexecution", "Case03", 0},
		{"pkg/registeredproviderexecution", "Case04", 1},
		{"pkg/registeredproviderexecution", "Case05", 1},
		{"pkg/registeredproviderexecution", "Case06", 1},
		{"pkg/registeredproviderexecution", "Case07", 1},
		{"pkg/registeredproviderexecution", "Case08", 1},
		{"pkg/registeredproviderexecution", "Case09", 1},
		{"pkg/registeredproviderexecution", "Case10", 1},
		{"pkg/registeredproviderexecution", "Case11", 0},
		{"pkg/registeredproviderexecution", "Case12", 0},
	}
}

func registeredProviderParityCases() []registeredProviderParityCase {
	return append(registeredProviderCycleCases(), registeredProviderDispatchCases()...)
}
