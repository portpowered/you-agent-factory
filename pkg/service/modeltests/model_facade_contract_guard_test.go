package modeltests

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

type modelFacadeMethodContract struct {
	name string
	args []string
}

var modelFacadeMethodContracts = []modelFacadeMethodContract{
	{name: "ListModels", args: []string{"ctx"}},
	{name: "GetModel", args: []string{"ctx", "modelName"}},
	{name: "PullModel", args: []string{"ctx", "modelName"}},
	{name: "InvokeModel", args: []string{"ctx", "modelName", "request"}},
}

func TestModelFacadeContractGuard_ProductionMethodsStayThinDelegates(t *testing.T) {
	t.Parallel()

	targets := []struct {
		path         string
		receiverType string
	}{
		{path: filepath.Join("..", "model_catalog.go"), receiverType: "FactoryService"},
		{path: filepath.Join("..", "..", "runtimehost", "model_catalog.go"), receiverType: "Host"},
	}

	for _, target := range targets {
		file, err := parser.ParseFile(token.NewFileSet(), target.path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", target.path, err)
		}
		if err := validateModelFacadeMethods(file, target.receiverType); err != nil {
			t.Fatalf("%s: %v", target.path, err)
		}
	}
}

func TestModelFacadeContractGuard_AcceptsThinDelegation(t *testing.T) {
	t.Parallel()

	file := parseModelFacadeGuardFixture(t, `package fixture

func (fs *Host) ListModels(ctx context.Context) (ListResponse, error) {
	return fs.requireModelService().ListModels(ctx)
}
func (fs *Host) GetModel(ctx context.Context, modelName string) (ModelDetail, error) {
	return fs.requireModelService().GetModel(ctx, modelName)
}
func (fs *Host) PullModel(ctx context.Context, modelName string) (PullResult, error) {
	return fs.requireModelService().PullModel(ctx, modelName)
}
func (fs *Host) InvokeModel(ctx context.Context, modelName string, request InvocationRequest) (InvocationResult, error) {
	return fs.requireModelService().InvokeModel(ctx, modelName, request)
}
`)
	if err := validateModelFacadeMethods(file, "Host"); err != nil {
		t.Fatalf("thin delegation rejected: %v", err)
	}
}

func TestModelFacadeContractGuard_RejectsCopiedPullPolicy(t *testing.T) {
	t.Parallel()

	file := parseModelFacadeGuardFixture(t, `package fixture

func (fs *Host) PullModel(ctx context.Context, modelName string) (PullResult, error) {
	puller := fs.modelAssetPuller()
	return puller.PullModel(ctx, fs.currentRuntimeConfig(), modelName)
}
`)
	err := validateModelFacadeMethod(file, "Host", modelFacadeMethodContract{
		name: "PullModel",
		args: []string{"ctx", "modelName"},
	})
	if err == nil || !strings.Contains(err.Error(), "single return delegation") {
		t.Fatalf("copied pull policy error = %v, want single return delegation rejection", err)
	}
}

func parseModelFacadeGuardFixture(t *testing.T, source string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatalf("parse guard fixture: %v", err)
	}
	return file
}

func validateModelFacadeMethods(file *ast.File, receiverType string) error {
	for _, contract := range modelFacadeMethodContracts {
		if err := validateModelFacadeMethod(file, receiverType, contract); err != nil {
			return err
		}
	}
	return nil
}

func validateModelFacadeMethod(file *ast.File, receiverType string, contract modelFacadeMethodContract) error {
	method := findModelFacadeMethod(file, receiverType, contract.name)
	if method == nil {
		return fmt.Errorf("missing %s.%s", receiverType, contract.name)
	}
	if method.Body == nil || len(method.Body.List) != 1 {
		return fmt.Errorf("%s.%s must contain a single return delegation", receiverType, contract.name)
	}
	returnStmt, ok := method.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returnStmt.Results) != 1 {
		return fmt.Errorf("%s.%s must contain a single return delegation", receiverType, contract.name)
	}
	call, ok := returnStmt.Results[0].(*ast.CallExpr)
	if !ok || !isRequiredModelServiceMethodCall(call.Fun, methodReceiverName(method), contract.name) {
		return fmt.Errorf("%s.%s must delegate through requireModelService().%s", receiverType, contract.name, contract.name)
	}
	if !callArgumentsMatch(call.Args, contract.args) {
		return fmt.Errorf("%s.%s must forward arguments unchanged", receiverType, contract.name)
	}
	return nil
}

func findModelFacadeMethod(file *ast.File, receiverType, methodName string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		method, ok := decl.(*ast.FuncDecl)
		if !ok || method.Name == nil || method.Name.Name != methodName || method.Recv == nil || len(method.Recv.List) != 1 {
			continue
		}
		if receiverBaseType(method.Recv.List[0].Type) == receiverType {
			return method
		}
	}
	return nil
}

func receiverBaseType(expr ast.Expr) string {
	if pointer, ok := expr.(*ast.StarExpr); ok {
		expr = pointer.X
	}
	ident, _ := expr.(*ast.Ident)
	if ident == nil {
		return ""
	}
	return ident.Name
}

func methodReceiverName(method *ast.FuncDecl) string {
	if method.Recv == nil || len(method.Recv.List) != 1 || len(method.Recv.List[0].Names) != 1 {
		return ""
	}
	return method.Recv.List[0].Names[0].Name
}

func isRequiredModelServiceMethodCall(expr ast.Expr, receiverName, methodName string) bool {
	methodSelector, ok := expr.(*ast.SelectorExpr)
	if !ok || methodSelector.Sel == nil || methodSelector.Sel.Name != methodName {
		return false
	}
	requireCall, ok := methodSelector.X.(*ast.CallExpr)
	if !ok || len(requireCall.Args) != 0 {
		return false
	}
	requireSelector, ok := requireCall.Fun.(*ast.SelectorExpr)
	if !ok || requireSelector.Sel == nil || requireSelector.Sel.Name != "requireModelService" {
		return false
	}
	receiver, ok := requireSelector.X.(*ast.Ident)
	return ok && receiver.Name == receiverName
}

func callArgumentsMatch(args []ast.Expr, names []string) bool {
	if len(args) != len(names) {
		return false
	}
	for index, arg := range args {
		ident, ok := arg.(*ast.Ident)
		if !ok || ident.Name != names[index] {
			return false
		}
	}
	return true
}
