package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	root string
}

type methodContract struct {
	name string
	args []string
}

type facadeTarget struct {
	packagePath string
	receiver    string
}

type locatedMethod struct {
	declaration *ast.FuncDecl
	filePath    string
}

var methodContracts = []methodContract{
	{name: "ListModels", args: []string{"ctx"}},
	{name: "GetModel", args: []string{"ctx", "modelName"}},
	{name: "PullModel", args: []string{"ctx", "modelName"}},
	{name: "InvokeModel", args: []string{"ctx", "modelName", "request"}},
}

var facadeTargets = []facadeTarget{
	{packagePath: "pkg/service", receiver: "FactoryService"},
	{packagePath: "pkg/runtimehost", receiver: "Host"},
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, stdoutWriter, stderrWriter); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	findings, err := scanFacades(cfg.root)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Fprintln(stdout, "[agent-factory:model-facade] model facade methods are thin delegates with explicit construction dependencies")
		return nil
	}
	for _, finding := range findings {
		fmt.Fprintln(stderr, finding)
	}
	return fmt.Errorf("[agent-factory:model-facade] found %d facade contract violation(s)", len(findings))
}

func scanFacades(root string) ([]string, error) {
	repoRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	var findings []string
	for _, target := range facadeTargets {
		methods, err := findTargetMethods(repoRoot, target)
		if err != nil {
			return nil, err
		}
		for _, contract := range methodContracts {
			finding := validateLocatedMethod(target, contract, methods[contract.name])
			if finding != "" {
				findings = append(findings, finding)
			}
		}
		constructionFindings, err := scanModelServiceConstruction(repoRoot, target)
		if err != nil {
			return nil, err
		}
		findings = append(findings, constructionFindings...)
	}
	return findings, nil
}

func scanModelServiceConstruction(repoRoot string, target facadeTarget) ([]string, error) {
	packageDir := filepath.Join(repoRoot, filepath.FromSlash(target.packagePath))
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return nil, fmt.Errorf("read facade package %s: %w", target.packagePath, err)
	}

	broadAdapters := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || !isProductionGoFile(entry.Name()) {
			continue
		}
		filePath := filepath.Join(packageDir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(filePath), err)
		}
		for typeName := range broadAdapterTypes(file, target.receiver) {
			broadAdapters[typeName] = struct{}{}
		}
	}

	var findings []string
	for _, entry := range entries {
		if entry.IsDir() || !isProductionGoFile(entry.Name()) {
			continue
		}
		filePath := filepath.Join(packageDir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(filePath), err)
		}
		aliases := modelServiceImportAliases(file)
		if len(aliases) == 0 {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			constructor := modelServiceConstructor(call.Fun, aliases)
			if constructor == "NewFromHost" {
				findings = append(findings, fmt.Sprintf("%s | %s calls retired models service NewFromHost; use New(Dependencies)", filepath.ToSlash(filePath), target.packagePath))
				return true
			}
			if constructor == "New" && callCarriesBroadAdapter(call, broadAdapters) {
				findings = append(findings, fmt.Sprintf("%s | %s passes a broad %s carrier into models service construction; use explicit Dependencies", filepath.ToSlash(filePath), target.packagePath, target.receiver))
			}
			return true
		})
	}
	return findings, nil
}

func modelServiceImportAliases(file *ast.File) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, imported := range file.Imports {
		if strings.Trim(imported.Path.Value, `"`) != "github.com/portpowered/infinite-you/pkg/models/service" {
			continue
		}
		alias := "service"
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		if alias != "." && alias != "_" {
			aliases[alias] = struct{}{}
		}
	}
	return aliases
}

func broadAdapterTypes(file *ast.File, receiverType string) map[string]struct{} {
	types := map[string]struct{}{}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structure.Fields.List {
				if receiverBaseType(field.Type) == receiverType {
					types[typeSpec.Name.Name] = struct{}{}
					break
				}
			}
		}
	}
	return types
}

func modelServiceConstructor(expression ast.Expr, aliases map[string]struct{}) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	if _, ok := aliases[identifier.Name]; !ok {
		return ""
	}
	return selector.Sel.Name
}

func callCarriesBroadAdapter(call *ast.CallExpr, broadAdapters map[string]struct{}) bool {
	for _, argument := range call.Args {
		if unary, ok := argument.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			argument = unary.X
		}
		literal, ok := argument.(*ast.CompositeLit)
		if !ok {
			continue
		}
		identifier, ok := literal.Type.(*ast.Ident)
		if !ok {
			continue
		}
		if _, ok := broadAdapters[identifier.Name]; ok {
			return true
		}
	}
	return false
}

func findTargetMethods(repoRoot string, target facadeTarget) (map[string][]locatedMethod, error) {
	packageDir := filepath.Join(repoRoot, filepath.FromSlash(target.packagePath))
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return nil, fmt.Errorf("read facade package %s: %w", target.packagePath, err)
	}

	methods := make(map[string][]locatedMethod, len(methodContracts))
	for _, entry := range entries {
		if entry.IsDir() || !isProductionGoFile(entry.Name()) {
			continue
		}
		filePath := filepath.Join(packageDir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(filePath), err)
		}
		collectTargetMethods(methods, file, target.receiver, filepath.ToSlash(filePath))
	}
	return methods, nil
}

func isProductionGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func collectTargetMethods(methods map[string][]locatedMethod, file *ast.File, receiverType string, filePath string) {
	for _, declaration := range file.Decls {
		method, ok := declaration.(*ast.FuncDecl)
		if !ok || method.Recv == nil || method.Name == nil || receiverBaseType(method.Recv.List[0].Type) != receiverType {
			continue
		}
		for _, contract := range methodContracts {
			if method.Name.Name == contract.name {
				methods[contract.name] = append(methods[contract.name], locatedMethod{declaration: method, filePath: filePath})
			}
		}
	}
}

func validateLocatedMethod(target facadeTarget, contract methodContract, methods []locatedMethod) string {
	qualifiedName := target.receiver + "." + contract.name
	if len(methods) == 0 {
		return fmt.Sprintf("%s | missing production method %s", target.packagePath, qualifiedName)
	}
	if len(methods) > 1 {
		return fmt.Sprintf("%s | method %s has %d production declarations", target.packagePath, qualifiedName, len(methods))
	}
	if err := validateThinDelegate(methods[0].declaration, contract); err != nil {
		return fmt.Sprintf("%s | method %s in %s %v", target.packagePath, qualifiedName, methods[0].filePath, err)
	}
	return ""
}

func validateThinDelegate(method *ast.FuncDecl, contract methodContract) error {
	if method.Body == nil || len(method.Body.List) != 1 {
		return fmt.Errorf("must contain a single return delegation")
	}
	returnStatement, ok := method.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returnStatement.Results) != 1 {
		return fmt.Errorf("must contain a single return delegation")
	}
	call, ok := returnStatement.Results[0].(*ast.CallExpr)
	if !ok || !isModelServiceMethodCall(call.Fun, methodReceiverName(method), contract.name) {
		return fmt.Errorf("must delegate through requireModelService().%s", contract.name)
	}
	if !callArgumentsMatch(call.Args, contract.args) {
		return fmt.Errorf("must forward arguments unchanged")
	}
	return nil
}

func receiverBaseType(expression ast.Expr) string {
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func methodReceiverName(method *ast.FuncDecl) string {
	if method.Recv == nil || len(method.Recv.List) != 1 || len(method.Recv.List[0].Names) != 1 {
		return ""
	}
	return method.Recv.List[0].Names[0].Name
}

func isModelServiceMethodCall(expression ast.Expr, receiverName string, methodName string) bool {
	methodSelector, ok := expression.(*ast.SelectorExpr)
	if !ok || methodSelector.Sel.Name != methodName {
		return false
	}
	requireCall, ok := methodSelector.X.(*ast.CallExpr)
	if !ok || len(requireCall.Args) != 0 {
		return false
	}
	requireSelector, ok := requireCall.Fun.(*ast.SelectorExpr)
	if !ok || requireSelector.Sel.Name != "requireModelService" {
		return false
	}
	receiver, ok := requireSelector.X.(*ast.Ident)
	return ok && receiver.Name == receiverName
}

func callArgumentsMatch(arguments []ast.Expr, names []string) bool {
	if len(arguments) != len(names) {
		return false
	}
	for index, argument := range arguments {
		identifier, ok := argument.(*ast.Ident)
		if !ok || identifier.Name != names[index] {
			return false
		}
	}
	return true
}
