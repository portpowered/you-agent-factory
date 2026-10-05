package analyzers

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/analysis"
	"strconv"
)

// CLIManifestAuthority protects the live manifest owner in its reviewed scopes.
// It consumes compiler syntax, including the legacy predicates verbatim.
var CLIManifestAuthority = &analysis.Analyzer{
	Name: "climanifestauthority",
	Doc:  "reject handwritten command shape in manifest-owned CLI boundaries",
	Run:  runCLIManifestAuthority,
}

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

func runCLIManifestAuthority(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	for _, file := range pass.Files {
		path := timingFile(unit, pass.Fset.Position(file.Pos()).Filename)
		for _, policy := range cliManifestAuthorityPaths {
			if policy.relative != path {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if clause, ok := node.(*ast.CaseClause); ok {
					for _, detail := range cliBindingDetails(clause) {
						pass.Reportf(clause.Pos(), "cli-manifest-authority: %s; use contracts/cli/commands.json and stable input IDs", detail)
					}
					return true
				}
				detail := cliAuthorityDetail(node, policy.transportBoundary)
				if detail != "" {
					pass.Reportf(node.Pos(), "cli-manifest-authority: %s; use contracts/cli/commands.json and stable input IDs", detail)
				}
				return true
			})
		}
	}
	return nil, nil
}

func cliAuthorityDetail(node ast.Node, transportBoundary bool) string {
	switch typed := node.(type) {
	case *ast.FuncDecl:
		if _, protected := retiredCLIShapeFunctions[typed.Name.Name]; protected {
			return "remove handwritten CLI-shape function " + typed.Name.Name
		}
	case *ast.TypeSpec:
		if _, protected := retiredCLIShapeTypes[typed.Name.Name]; transportBoundary && protected {
			return "remove CLI-shape mirror " + typed.Name.Name
		}
	case *ast.CompositeLit:
		if transportBoundary && isCobraCommandType(typed.Type) {
			return "remove handwritten cobra.Command metadata"
		}
	case *ast.CallExpr:
		if transportBoundary && isDirectPublicFlagRegistration(typed) {
			return "remove direct Cobra/pflag public input registration"
		}
	}
	return ""
}

func cliBindingDetails(clause *ast.CaseClause) []string {
	var details []string
	for _, expression := range clause.List {
		literal, ok := expression.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		binding, err := strconv.Unquote(literal.Value)
		if err != nil {
			continue
		}
		if _, protected := manifestOwnedRunServerBindings[binding]; protected {
			details = append(details, "production source must not switch on manifest-owned run/server binding "+binding)
		}
	}
	return details
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
