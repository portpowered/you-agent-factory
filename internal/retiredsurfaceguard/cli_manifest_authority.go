package retiredsurfaceguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
)

type cliManifestAuthorityPath struct {
	relative          string
	transportBoundary bool
}

var cliManifestAuthorityPaths = []cliManifestAuthorityPath{
	{relative: "pkg/transports/cli/root_work.go", transportBoundary: true},
	{relative: "pkg/transports/cli/root_workflow.go", transportBoundary: true},
	{relative: "pkg/transports/cli/root_submit_batch.go", transportBoundary: true},
	{relative: "pkg/transports/cli/root_factory.go", transportBoundary: true},
	{relative: "pkg/transports/cli/commandregistry/representative_handlers.go"},
	{relative: "pkg/transports/cli/climanifestcobra/run_submit_constructor.go"},
}

var retiredCLIShapeFunctions = map[string]struct{}{
	"newRunCommand":                    {},
	"registerRunCommandFlags":          {},
	"runExecutionLocalBindingTarget":   {},
	"runRuntimeLocalBindingTarget":     {},
	"runInvocationLocalBindingTarget":  {},
	"newSubmitCommand":                 {},
	"newSubmitCommandWithHandlers":     {},
	"newSubmitBatchCommandWithHandler": {},
	"sessionInputBindings":             {},
	"applySessionGenericFlagUsages":    {},
	"sessionFamilyFlagUsages":          {},
	"submitFlagUsages":                 {},
	"workFamilyFlagUsages":             {},
	"newWorkCommand":                   {},
	"newWorkListCommand":               {},
	"newWorkShowCommand":               {},
	"newWorkMoveCommand":               {},
	"newWorkVisualizeCommand":          {},
	"RunnableSessionCommandIDs":        {},
	"VerifySessionRunnableCoverage":    {},
	"NewSessionRegistry":               {},
}

var manifestOwnedRunServerBindings = map[string]struct{}{
	"continuously": {},
	"with-server":  {},
	"with-site":    {},
}

var retiredCLIShapeTypes = map[string]struct{}{
	"SessionFamilyBindings": {},
	"SessionHandlers":       {},
	"RunFamilyBindings":     {},
	"SubmitFamilyBindings":  {},
}

var publicFlagRegistrationMethods = map[string]struct{}{
	"Bool": {}, "BoolP": {}, "BoolVar": {}, "BoolVarP": {},
	"Duration": {}, "DurationP": {}, "DurationVar": {}, "DurationVarP": {},
	"Int": {}, "IntP": {}, "IntVar": {}, "IntVarP": {},
	"String": {}, "StringP": {}, "StringVar": {}, "StringVarP": {},
	"StringSlice": {}, "StringSliceP": {}, "StringSliceVar": {}, "StringSliceVarP": {},
	"Var": {}, "VarP": {},
}

// ScanCLIManifestAuthoritySourceViolations reports handwritten public command
// metadata, flag registration, CLI-shaped mirrors, and public binding switches
// in transport paths whose public shape is owned by the CLI manifest.
func ScanCLIManifestAuthoritySourceViolations(repoRoot string) ([]Violation, error) {
	var violations []Violation
	for _, policy := range cliManifestAuthorityPaths {
		relative := policy.relative
		path := filepath.Join(repoRoot, filepath.FromSlash(relative))
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", relative, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.FuncDecl:
				if _, retired := retiredCLIShapeFunctions[typed.Name.Name]; retired {
					violations = append(violations, cliManifestAuthorityViolation(
						relative,
						"remove handwritten CLI-shape function "+typed.Name.Name+
							"; project public names, usage, defaults, and flags from contracts/cli/commands.json",
					))
				}
			case *ast.TypeSpec:
				if !policy.transportBoundary {
					break
				}
				if _, retired := retiredCLIShapeTypes[typed.Name.Name]; retired {
					violations = append(violations, cliManifestAuthorityViolation(
						relative,
						"remove CLI-shape mirror "+typed.Name.Name+
							"; keep parser storage keyed by stable manifest input ID and map into domain requests at invocation time",
					))
				}
			case *ast.CompositeLit:
				if policy.transportBoundary && isCobraCommandType(typed.Type) {
					violations = append(violations, cliManifestAuthorityViolation(
						relative,
						"remove handwritten cobra.Command metadata; project command usage and help from contracts/cli/commands.json",
					))
				}
			case *ast.CallExpr:
				if policy.transportBoundary && isDirectPublicFlagRegistration(typed) {
					violations = append(violations, cliManifestAuthorityViolation(
						relative,
						"remove direct Cobra/pflag public input registration; project flags from contracts/cli/commands.json and bind by stable input ID",
					))
				}
			case *ast.CaseClause:
				for _, expression := range typed.List {
					literal, ok := expression.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					binding, err := strconv.Unquote(literal.Value)
					if err != nil {
						continue
					}
					if _, manifestOwned := manifestOwnedRunServerBindings[binding]; manifestOwned {
						violations = append(violations, cliManifestAuthorityViolation(
							relative,
							"production source must not switch on manifest-owned run/server binding "+binding,
						))
					}
				}
			}
			return true
		})
	}
	return violations, nil
}

func isCobraCommandType(expression ast.Expr) bool {
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Command"
}

func isDirectPublicFlagRegistration(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if _, registration := publicFlagRegistrationMethods[selector.Sel.Name]; !registration {
		return false
	}
	flagSetCall, ok := selector.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	flagSetSelector, ok := flagSetCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return flagSetSelector.Sel.Name == "Flags" || flagSetSelector.Sel.Name == "PersistentFlags"
}

func cliManifestAuthorityViolation(relative, detail string) Violation {
	return Violation{
		Family:  "cli-manifest-authority",
		Surface: filepath.ToSlash(relative),
		Detail:  detail,
	}
}
