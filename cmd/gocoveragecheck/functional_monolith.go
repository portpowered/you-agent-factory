package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const functionalMonolithPackage = modulePath + "/pkg/monolithpilot"

type functionalMonolithGroup struct {
	Package string   `json:"package"`
	Group   string   `json:"group"`
	Tests   []string `json:"top_level_tests"`
}

func consolidateFunctionalCoveragePlan(cfg config, plan *coverageInvocationPlan, packages []string, root string, listed []functionalGoListPackage, selection *functionalCoverageSelection) error {
	if cfg.suite != functionalCoverageSuite {
		return errors.New("functional monolith requires the functional suite")
	}
	dir := filepath.Join(root, ".artifacts", "functional-monolith")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	// Go executes the generated test binary from its package directory even
	// when every source file is supplied by an overlay.
	if err := os.MkdirAll(filepath.Join(root, "pkg", "monolithpilot"), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "running.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("lock functional monolith: %w", err)
	}
	_ = lock.Close()
	previousCleanup := plan.cleanup
	plan.cleanup = func() error { return errors.Join(previousCleanup(), os.Remove(lock.Name())) }
	if listed == nil {
		listed, err = listFunctionalTestPackageMetadata(packages, root)
		if err != nil {
			return err
		}
	}
	if err := writeFunctionalMonolithMetadata(listed, packages, selection, dir); err != nil {
		return err
	}
	python := os.Getenv("PYTHON")
	if python == "" {
		python = "python3"
		if _, err := exec.LookPath(python); err != nil {
			python = "python"
		}
	}
	_, detail, err := runCommand(commandInvocation{name: python, dir: root, args: []string{
		filepath.Join("cmd", "unitlane", "monolith_generate.py"), filepath.Join(dir, "packages.json"), dir, "functional",
	}})
	if err != nil {
		return fmt.Errorf("generate functional monolith: %w: %s", err, detail)
	}
	var groups []functionalMonolithGroup
	data, err := os.ReadFile(filepath.Join(dir, "groups.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &groups); err != nil {
		return err
	}
	if err := validateFunctionalMonolithInventory(groups, selection); err != nil {
		return err
	}
	return applyFunctionalMonolithGroups(plan, groups, filepath.Join(dir, "overlay.json"))
}

func validateFunctionalMonolithInventory(groups []functionalMonolithGroup, selection *functionalCoverageSelection) error {
	if selection == nil {
		return nil
	}
	for _, group := range groups {
		expected := append([]string(nil), selection.SelectedTests[group.Package]...)
		actual := append([]string(nil), group.Tests...)
		slices.Sort(expected)
		slices.Sort(actual)
		if !slices.Equal(expected, actual) {
			return fmt.Errorf("functional monolith inventory mismatch for %s: expected %v, registered %v", group.Package, expected, actual)
		}
	}
	return nil
}

func applyFunctionalMonolithGroups(plan *coverageInvocationPlan, groups []functionalMonolithGroup, overlay string) error {
	merged := map[string]string{}
	identities := map[string]string{}
	for _, group := range groups {
		if group.Package == "" || group.Group == "" || len(group.Tests) == 0 || identities[group.Group] != "" || merged[group.Package] != "" {
			return fmt.Errorf("invalid functional monolith registration: %+v", group)
		}
		merged[group.Package], identities[group.Group] = group.Group, group.Package
	}
	if len(merged) == 0 {
		return errors.New("functional monolith has no compatible packages")
	}
	owner := -1
	for index := range plan.invocations {
		invocation := &plan.invocations[index]
		args := make([]string, 0, len(invocation.args))
		selected := false
		for _, arg := range invocation.args {
			if _, ok := merged[arg]; ok {
				selected = true
				continue
			}
			args = append(args, arg)
		}
		if selected {
			if owner >= 0 {
				return errors.New("functional monolith spans incompatible selector batches")
			}
			owner = index
			args = append(args[:1], append([]string{"-overlay=" + overlay}, args[1:]...)...)
			if !slices.Contains(args, "-json") {
				args = append(args, "-json")
			}
			args = append(args, functionalMonolithPackage)
			invocation.monolithGroups = identities
		}
		invocation.args = args
	}
	if owner < 0 {
		return errors.New("functional monolith packages were absent from invocation plan")
	}
	fmt.Fprintf(stdoutWriter, "Functional consolidated build: merged-packages=%d original-tests=%d native-exceptions=preserved\n", len(groups), functionalMonolithTestCount(groups))
	return nil
}

func functionalMonolithTestCount(groups []functionalMonolithGroup) int {
	count := 0
	for _, group := range groups {
		count += len(group.Tests)
	}
	return count
}

func writeFunctionalMonolithMetadata(listed []functionalGoListPackage, packages []string, selection *functionalCoverageSelection, dir string) error {
	file, err := os.Create(filepath.Join(dir, "packages.json"))
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for index, pkg := range listed {
		if !slices.Contains(packages, pkg.ImportPath) || len(pkg.TestGoFiles)+len(pkg.XTestGoFiles) == 0 {
			continue
		}
		registration, name, reason, err := functionalMonolithRegistration(pkg)
		if err != nil {
			return err
		}
		if selection != nil && len(selection.SelectedTests[pkg.ImportPath]) != len(selection.Inventory.Tests[pkg.ImportPath]) {
			reason = "test-level quarantine selector: preserve native selection"
		}
		base := struct {
			ImportPath, Dir, Name, MonolithExcludeReason string
			TestGoFiles, XTestGoFiles                    []string
		}{pkg.ImportPath, pkg.Dir, name, reason, pkg.TestGoFiles, pkg.XTestGoFiles}
		if err := encoder.Encode(base); err != nil {
			return err
		}
		path := filepath.Join(dir, fmt.Sprintf("registrations-%04d.go", index))
		if err := os.WriteFile(path, []byte(registration), 0600); err != nil {
			return err
		}
		if err := encoder.Encode(struct {
			ImportPath, Name string
			GoFiles          []string
		}{pkg.ImportPath + ".test", "main", []string{path}}); err != nil {
			return err
		}
	}
	return nil
}

func functionalMonolithRegistration(pkg functionalGoListPackage) (string, string, string, error) {
	var tests strings.Builder
	name, reason, mainAlias := "", "", ""
	cleanup := ""
	var mainFunction *ast.FuncDecl
	for _, group := range []struct {
		alias string
		files []string
	}{{"_test", pkg.TestGoFiles}, {"_xtest", pkg.XTestGoFiles}} {
		for _, filename := range group.files {
			path := filepath.Join(pkg.Dir, filename)
			source, err := os.ReadFile(path)
			if err != nil {
				return "", "", "", err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ParseComments)
			if err != nil {
				return "", "", "", err
			}
			name = strings.TrimSuffix(file.Name.Name, "_test")
			if dependency := functionalMonolithNativeReason(string(source)); dependency != "" {
				reason = dependency
			}
			for _, declaration := range file.Decls {
				fn, ok := declaration.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				if fn.Name.Name == "TestMain" {
					mainAlias = group.alias
					mainFunction = fn
				}
				if functionalMonolithCleanupHook(fn, file) {
					cleanup = group.alias + "." + fn.Name.Name
				}
				if functionalTestNamePattern.MatchString(fn.Name.Name) && functionalTestSignature(fn, testingImportNames(file)) {
					fmt.Fprintf(&tests, "{%s, %s.%s},\n", strconv.Quote(fn.Name.Name), group.alias, fn.Name.Name)
				}
			}
		}
	}
	registration := "var tests = []testing.InternalTest{\n" + tests.String() + "\n}\nvar benchmarks = []testing.InternalBenchmark{\n}\nvar fuzzTargets = []testing.InternalFuzzTarget{\n}\nvar examples = []testing.InternalExample{\n}\n"
	if mainAlias != "" {
		registration += "func main() { " + mainAlias + ".TestMain(m) }\n"
	}
	registration, reason = functionalMonolithCleanupRegistration(registration, reason, cleanup, mainFunction)
	return registration, name, reason, nil
}

func functionalMonolithNativeReason(source string) string {
	// An external fixture command does not depend on this test binary's
	// identity. Re-execution and process-wide directory/environment state do.
	for _, marker := range []string{".Setenv(", ".Chdir(", "os.Unsetenv(", "SetWorkingDirectory(", "os.Getwd(", "os.Args[0]", "os.Executable("} {
		if strings.Contains(source, marker) {
			return "process-wide state or executable fixture: preserve native binary"
		}
	}
	for _, marker := range []string{"//go:embed", "filepath.Join(\"..\"", "filepath.Join(\"testdata\"", "\"testdata/", "\"../", "func Fuzz", "func Example"} {
		if strings.Contains(source, marker) {
			return "relative fixture, embed, fuzz or example registration: preserve native binary"
		}
	}
	if functionalMonolithNativeCommand(source) {
		return "executable fixture: preserve native binary"
	}
	return ""
}

func functionalMonolithNativeCommand(source string) bool {
	if !strings.Contains(source, "exec.Command(") && !strings.Contains(source, "exec.CommandContext(") {
		return false
	}
	file, err := parser.ParseFile(token.NewFileSet(), "", source, 0)
	if err != nil {
		file, err = parser.ParseFile(token.NewFileSet(), "", "package fixture\nfunc fixture() {\n"+source+"\n}", 0)
	}
	if err != nil {
		return true
	}
	native := false
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			native = native || functionalMonolithNativeExec(call)
		}
		return true
	})
	return native
}

func functionalMonolithNativeExec(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	owner, ok := selector.X.(*ast.Ident)
	if !ok || owner.Name != "exec" || (selector.Sel.Name != "Command" && selector.Sel.Name != "CommandContext") {
		return false
	}
	index := 0
	if selector.Sel.Name == "CommandContext" {
		index = 1
	}
	if len(call.Args) <= index {
		return true
	}
	literal, ok := call.Args[index].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return true
	}
	command, err := strconv.Unquote(literal.Value)
	return err != nil || command != "git"
}

// Explicit cleanup hooks preserve the native lifecycle only when TestMain has
// no setup before m.Run. Packages with setup, relative fixtures or global effects
// continue to use their native executable.
func functionalMonolithCleanupHook(fn *ast.FuncDecl, file *ast.File) bool {
	return fn.Name.Name == "FunctionalMonolithCleanup" && functionalTestSignature(fn, testingImportNames(file))
}

func functionalMonolithCleanupRegistration(registration, reason, cleanup string, mainFunction *ast.FuncDecl) (string, string) {
	if mainFunction == nil {
		return registration, reason
	}
	if cleanup == "" || !functionalMonolithMainRunsFirst(mainFunction) {
		return registration, "custom TestMain: preserve native binary"
	}
	return registration + "var monolithCleanup = " + cleanup + "\n", reason
}

func functionalMonolithMainRunsFirst(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 || len(fn.Type.Params.List[0].Names) != 1 {
		return false
	}
	statement, ok := fn.Body.List[0].(*ast.AssignStmt)
	if !ok || len(statement.Rhs) != 1 {
		return false
	}
	call, ok := statement.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Run" {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == fn.Type.Params.List[0].Names[0].Name && functionalMonolithRunCallCount(fn.Body, receiver.Name) == 1
}

func functionalMonolithRunCallCount(body *ast.BlockStmt, receiver string) int {
	count := 0
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Run" {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		if ok && owner.Name == receiver {
			count++
		}
		return true
	})
	return count
}
