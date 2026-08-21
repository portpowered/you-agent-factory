package factory_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"reflect"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const externalConsumerProofFactoryImportPath = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// DEL-RUN-CKPT proof: CaptureCheckpoint, LoadCheckpoint, and RestoreCheckpoint
// are gone from the published Factory Runtime root, and every external
// consumer implementing Service compiles and behaves without them.

func TestServiceDoesNotExposeDeletedCheckpointMethods(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf((*factory.Service)(nil)).Elem()
	for _, forbidden := range []string{"CaptureCheckpoint", "LoadCheckpoint", "RestoreCheckpoint"} {
		if _, ok := serviceType.MethodByName(forbidden); ok {
			t.Fatalf("Service must not expose deleted checkpoint method %s", forbidden)
		}
	}
}

// TestExternalConsumerCannotCallDeletedCheckpointMethods is the required
// external-consumer negative-compilation proof. It type-checks the fixture in
// testdata/checkpointdeletionproof against the Service declaration parsed from
// the published package source and requires an undefined-selector diagnostic
// for every removed method. The fixture stays under testdata so normal package
// discovery never compiles it; this test is its intentional type-checking
// entrypoint.
func TestExternalConsumerCannotCallDeletedCheckpointMethods(t *testing.T) {
	factoryProof := checkpointProofFactoryPackage(t)
	fixtureDiagnostics := checkpointProofTypeCheckFixture(t, factoryProof, "main.go")
	deletedMethods := []string{"CaptureCheckpoint", "LoadCheckpoint", "RestoreCheckpoint"}
	for _, forbidden := range deletedMethods {
		if !checkpointProofHasUndefinedDiagnostic(fixtureDiagnostics, forbidden) {
			t.Errorf("expected compiler diagnostic naming removed method %s as undefined, got diagnostics:\n%s", forbidden, checkpointProofDiagnosticsText(fixtureDiagnostics))
		}
	}
	if len(fixtureDiagnostics) != len(deletedMethods) {
		t.Errorf("expected exactly one diagnostic per deleted method, got %d:\n%s", len(fixtureDiagnostics), checkpointProofDiagnosticsText(fixtureDiagnostics))
	}

	positiveDiagnostics := checkpointProofTypeCheckFixture(t, factoryProof, "positive.go")
	if len(positiveDiagnostics) != 0 {
		t.Fatalf("expected valid external-consumer ControlPause call to type-check, got diagnostics:\n%s", checkpointProofDiagnosticsText(positiveDiagnostics))
	}
}

type checkpointProofFactory struct {
	pkg            *types.Package
	contextPackage *types.Package
}

func checkpointProofFactoryPackage(t *testing.T) checkpointProofFactory {
	t.Helper()

	source, err := os.ReadFile("interfaces.go")
	if err != nil {
		t.Fatalf("read Factory Runtime Service source: %v", err)
	}
	return checkpointProofFactoryFromSource(t, source)
}

func checkpointProofFactoryFromSource(t *testing.T, source []byte) checkpointProofFactory {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "interfaces.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse Factory Runtime Service source: %v", err)
	}
	serviceSpec := checkpointProofServiceSpec(file)
	if serviceSpec == nil {
		t.Fatal("Factory Runtime Service source does not declare Service")
	}

	serviceFile := &ast.File{
		Name: ast.NewIdent("factory"),
		Decls: []ast.Decl{
			&ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{checkpointProofContextImport()}},
			&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{serviceSpec}},
		},
	}
	serviceFile.Imports = []*ast.ImportSpec{serviceFile.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ImportSpec)}
	serviceFile.Decls = append(serviceFile.Decls, checkpointProofServiceStubTypes(serviceSpec)...)

	contextPackage := checkpointProofContextPackage(t)
	config := types.Config{Importer: checkpointProofImporter{contextPackage: contextPackage}}
	factoryPackage, err := config.Check(externalConsumerProofFactoryImportPath, fileSet, []*ast.File{serviceFile}, nil)
	if err != nil {
		t.Fatalf("type-check Factory Runtime Service source: %v", err)
	}
	service := factoryPackage.Scope().Lookup("Service")
	if service == nil {
		t.Fatal("type-checker did not publish Factory Runtime Service")
	}
	serviceType, ok := service.Type().(*types.Named)
	if !ok {
		t.Fatalf("type-checker published Service as %T, want named interface", service.Type())
	}
	serviceInterface, ok := serviceType.Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("type-checker published Service underlying type as %T, want interface", serviceType.Underlying())
	}
	serviceInterface.Complete()
	if types.NewMethodSet(serviceType).Lookup(nil, "ControlPause") == nil {
		t.Fatal("type-checker did not retain surviving ControlPause method")
	}
	return checkpointProofFactory{pkg: factoryPackage, contextPackage: contextPackage}
}

func checkpointProofServiceSpec(file *ast.File) *ast.TypeSpec {
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.TYPE {
			continue
		}
		for _, specification := range group.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if ok && typeSpec.Name.Name == "Service" {
				return typeSpec
			}
		}
	}
	return nil
}

func checkpointProofContextImport() *ast.ImportSpec {
	return &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: `"context"`}}
}

func checkpointProofContextPackage(t *testing.T) *types.Package {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "context.go", []byte("package context\ntype Context interface{}\n"), 0)
	if err != nil {
		t.Fatalf("parse checkpoint proof context package: %v", err)
	}
	contextPackage, err := (&types.Config{}).Check("context", fileSet, []*ast.File{file}, nil)
	if err != nil {
		t.Fatalf("type-check checkpoint proof context package: %v", err)
	}
	return contextPackage
}

func checkpointProofServiceStubTypes(serviceSpec *ast.TypeSpec) []ast.Decl {
	selectorIdentifiers := make(map[*ast.Ident]struct{})
	methodIdentifiers := make(map[*ast.Ident]struct{})
	ast.Inspect(serviceSpec.Type, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.SelectorExpr:
			if qualifier, ok := current.X.(*ast.Ident); ok {
				selectorIdentifiers[qualifier] = struct{}{}
			}
			selectorIdentifiers[current.Sel] = struct{}{}
		case *ast.Field:
			for _, name := range current.Names {
				methodIdentifiers[name] = struct{}{}
			}
		}
		return true
	})

	stubNames := make(map[string]struct{})
	ast.Inspect(serviceSpec.Type, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Name == "Service" {
			return true
		}
		if _, ok := selectorIdentifiers[identifier]; ok {
			return true
		}
		if _, ok := methodIdentifiers[identifier]; ok {
			return true
		}
		if types.Universe.Lookup(identifier.Name) != nil {
			return true
		}
		stubNames[identifier.Name] = struct{}{}
		return true
	})

	declarations := make([]ast.Decl, 0, len(stubNames))
	for name := range stubNames {
		declarations = append(declarations, &ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{
				Name: ast.NewIdent(name),
				Type: &ast.StructType{Fields: &ast.FieldList{}},
			}},
		})
	}
	return declarations
}

func checkpointProofTypeCheckFixture(t *testing.T, factoryProof checkpointProofFactory, filename string) []error {
	t.Helper()

	fileSet := token.NewFileSet()
	source, err := os.ReadFile("testdata/checkpointdeletionproof/" + filename)
	if err != nil {
		t.Fatalf("read checkpoint deletion proof fixture %s: %v", filename, err)
	}
	file, err := parser.ParseFile(fileSet, filename, source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse checkpoint deletion proof fixture %s: %v", filename, err)
	}

	diagnostics := make([]error, 0)
	config := types.Config{
		Importer: checkpointProofImporter{
			factoryPackage: factoryProof.pkg,
			contextPackage: factoryProof.contextPackage,
		},
		Error: func(err error) { diagnostics = append(diagnostics, err) },
	}
	_, _ = config.Check("checkpointdeletionproof", fileSet, []*ast.File{file}, nil)
	return diagnostics
}

type checkpointProofImporter struct {
	factoryPackage *types.Package
	contextPackage *types.Package
}

func (i checkpointProofImporter) Import(path string) (*types.Package, error) {
	switch path {
	case externalConsumerProofFactoryImportPath:
		return i.factoryPackage, nil
	case "context":
		return i.contextPackage, nil
	default:
		return nil, fmt.Errorf("checkpoint proof importer cannot load %q", path)
	}
}

func checkpointProofHasUndefinedDiagnostic(diagnostics []error, method string) bool {
	for _, diagnostic := range diagnostics {
		message := diagnostic.Error()
		if strings.Contains(message, "svc."+method+" undefined") && strings.Contains(message, "factory.Service") {
			return true
		}
	}
	return false
}

func checkpointProofDiagnosticsText(diagnostics []error) string {
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Error())
	}
	return strings.Join(messages, "\n")
}

// externalConsumerPeer implements factory.Service using only the surviving
// root vocabulary, proving an external consumer can satisfy the interface
// without any checkpoint request/result/error vocabulary in scope.
type externalConsumerPeer struct{}

var _ factory.Service = (*externalConsumerPeer)(nil)

func (externalConsumerPeer) ControlPause(context.Context, factory.PauseRequest) (factory.PauseResult, error) {
	return factory.PauseResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (externalConsumerPeer) ControlResume(context.Context, factory.ResumeRequest) (factory.ResumeResult, error) {
	return factory.ResumeResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (externalConsumerPeer) ControlTerminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (externalConsumerPeer) ControlWaitToComplete(factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}
func (externalConsumerPeer) ControlMoveWork(_ context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{WorkID: req.WorkID, ToState: req.StateName}, nil
}
func (externalConsumerPeer) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{Observation: factory.Observation{Status: factory.ObservationStatusActive}}, nil
}
func (externalConsumerPeer) CleanInvocationSnapshot(context.Context) (factory.CleanInvocationSnapshot, error) {
	return factory.CleanInvocationSnapshot{}, nil
}
func (externalConsumerPeer) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{Outcome: factory.DispatchPlanOutcomeAccepted, DispatchID: req.DispatchID}, nil
}
func (externalConsumerPeer) InvokeWorker(_ context.Context, _ factory.InvokeWorkerRequest) (factory.InvokeWorkerResult, error) {
	return factory.InvokeWorkerResult{}, nil
}

func (externalConsumerPeer) AcceptDispatchResult(_ context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{Outcome: factory.DispatchPlanOutcomeRetired, DispatchID: req.DispatchID}, nil
}

func TestExternalConsumerSatisfiesServiceWithoutCheckpointVocabulary(t *testing.T) {
	t.Parallel()

	var runtime factory.Service = externalConsumerPeer{}
	ctx := context.Background()

	if _, err := runtime.ControlPause(ctx, factory.PauseRequest{}); err != nil {
		t.Fatalf("ControlPause() error = %v", err)
	}
	if _, err := runtime.Observe(ctx, factory.ObserveRequest{}); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if _, err := runtime.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "del-run-ckpt-proof"}); err != nil {
		t.Fatalf("PlanDispatch() error = %v", err)
	}
	if _, err := runtime.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "del-run-ckpt-proof"}); err != nil {
		t.Fatalf("AcceptDispatchResult() error = %v", err)
	}
}
