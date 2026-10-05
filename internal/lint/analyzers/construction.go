package analyzers

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Construction rejects references to product constructors outside their owner
// or canonical composition boundary, including references used as values.
var Construction = &analysis.Analyzer{
	Name: "construction",
	Doc:  "restrict product service construction to its owner and pkg/wire",
	Run:  runConstruction,
}

var constructionRules = setOf("service-construction", "service-construction-test", "service-construction-callable", "service-construction-callable-test")
var constructionSiteRules = setOf("construction-recorded-site", "construction-recorded-site-test")

var serviceValueConstructors = map[string]map[string]bool{
	"pkg/services/factory_definitions": setOf("NewBlockingFactoryLoadError", "NewFactoryEvent", "NewFactorySnapshot"),
	"pkg/services/factory_runtime":     setOf("NewEngineStateSnapshot"),
	"pkg/services/factory_sessions":    setOf("BuildProjectionContext", "BuildTargetFromConfig", "NewLogicalTargetValidationError", "NewSessionID"),
	"pkg/services/operator_settings":   setOf("EnsureLocalBackendScope"),
	"pkg/services/recordings":          setOf("BuildFactoryWorldWorkstationRequestProjectionSlice", "BuildPortableRecording"),
	"pkg/services/workers":             setOf("NewCapabilities", "NewEmptyMockWorkersConfig", "NewProviderError"),
	"pkg/services/providers/wire":      setOf("NewCapabilitySet"),
}

func serviceConstructorName(name string) bool {
	for _, prefix := range []string{"New", "Build", "Create", "Ensure", "Open", "Provide"} {
		if name == prefix || (strings.HasPrefix(name, prefix) && len(name) > len(prefix) && name[len(prefix)] >= 'A' && name[len(prefix)] <= 'Z') {
			return true
		}
	}
	return false
}

func constructorAllowed(importer, target string) bool {
	if under(importer, "pkg/wire") {
		return true
	}
	parts := strings.Split(target, "/")
	if len(parts) < 3 || parts[0] != "pkg" || parts[1] != "services" {
		return true
	}
	if under(importer, "pkg/services/"+parts[2]) {
		return true
	}
	return len(parts) >= 5 && parts[3] == "transports" && under(importer, "pkg/transports/"+parts[4])
}

func runConstruction(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	importer := strings.TrimSuffix(unit, "_test")
	var found []violation
	hasTests := false
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}
		test := strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
		hasTests = hasTests || test
		called := testBoundaryCalls(file)
		ast.Inspect(file, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pass.TypesInfo.Uses[id]
			if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() || !strings.HasPrefix(obj.Pkg().Path(), modulePrefix) || !testBoundaryCallable(obj, called[id]) {
				return true
			}
			target := strings.TrimPrefix(obj.Pkg().Path(), modulePrefix)
			if constructorAllowed(importer, target) || !serviceConstructorName(obj.Name()) || serviceValueConstructors[target][obj.Name()] {
				return true
			}
			rule := "service-construction"
			if _, function := obj.(*types.Func); !function {
				rule += "-callable"
			}
			if test {
				rule += "-test"
			}
			found = append(found, violation{rule: rule, importer: unit, importee: target + "." + obj.Name(), pos: id.Pos(), hint: "inject the constructed service role from pkg/wire"})
			return true
		})
	}
	reportConstructionRecordedSites(pass, unit, found, hasTests)
	reportAgainstBaseline(pass, unit, constructionRules, found, hasTests)
	return nil, nil
}

// Supplemental source/count keys prevent a package-level allowance from
// permitting another constructor reference, including in a neighboring file.
// New unrecorded symbols continue to fail the original ownership rule.
func reportConstructionRecordedSites(pass *analysis.Pass, unit string, found []violation, hasTests bool) {
	listed := baseline()
	var sites []violation
	for _, v := range found {
		if _, recorded := listed[v.key()]; !recorded {
			continue
		}
		test := strings.HasSuffix(v.rule, "-test")
		v.importee = timingFile(unit, pass.Fset.Position(v.pos).Filename) + "#" + v.rule + "#" + v.importee
		v.rule = "construction-recorded-site"
		if test {
			v.rule += "-test"
		}
		sites = append(sites, v)
	}
	selected, ignored := map[string]bool{}, map[string]bool{}
	for _, file := range pass.Files {
		selected[serviceSource(pass, unit, file)] = true
	}
	for _, name := range pass.IgnoredFiles {
		ignored[sourceName(unit, name)] = true
	}
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || !constructionSiteRules[parts[0]] || parts[1] != unit {
			continue
		}
		name, _, _ := strings.Cut(parts[2], "#")
		if !selected[name] && ignored[name] {
			delete(listed, key)
		}
	}
	reportWithBaseline(pass, unit, constructionSiteRules, countedTestPolicy(sites), hasTests, listed)
}

func validateConstructionSiteKey(parts []string) error {
	site, count, counted := strings.Cut(parts[2], "::count=")
	fields := strings.Split(site, "#")
	unit := strings.TrimSuffix(parts[1], "_test")
	if len(fields) != 3 || !counted || !positiveDecimal(count) ||
		!strings.HasSuffix(fields[0], ".go") || !strings.HasPrefix(fields[0], unit+"/") ||
		strings.Contains(fields[0], "..") || !constructionRules[fields[1]] || fields[2] == "" ||
		strings.HasSuffix(parts[0], "-test") != strings.HasSuffix(fields[1], "-test") ||
		strings.HasSuffix(parts[0], "-test") != strings.HasSuffix(fields[0], "_test.go") {
		return fmt.Errorf("malformed construction site baseline key: %s", strings.Join(parts, "|"))
	}
	return nil
}
