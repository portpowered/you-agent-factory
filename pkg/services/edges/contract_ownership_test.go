package edges

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPackageDocumentsProcessEdgeArchitectureException(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "definition.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse definition.go: %v", err)
	}
	if file.Doc == nil {
		t.Fatal("package edges is missing package documentation")
	}
	doc := file.Doc.Text()
	requiredPhrases := []string{
		"process-edge aggregator",
		"root.BuildProcess",
		"pkg/wire",
		"functional",
		"not a service locator",
		"Initializer",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(doc, phrase) {
			t.Errorf("package documentation must state the process-edge architecture exception; missing %q", phrase)
		}
	}
	if !strings.Contains(doc, "exact") || !strings.Contains(strings.ToLower(doc), "port") {
		t.Error("package documentation must tell constructed services to take exact ports rather than the broad Edges bag")
	}
}

func TestEdgesDoNotImportModelsWire(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, importSpec := range file.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote %s import path: %w", path, err)
			}
			if importPath == "github.com/portpowered/infinite-you/pkg/services/models/"+"wire" {
				return fmt.Errorf("%s imports Models construction package %q", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestModelsRootDeclaresOnlyServiceInterface(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("../models")
	if err != nil {
		t.Fatalf("read Models root: %v", err)
	}
	fileSet := token.NewFileSet()
	var interfaces []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, "../models/"+entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse Models root file %s: %v", entry.Name(), err)
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typed, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, ok := typed.Type.(*ast.InterfaceType); ok {
					interfaces = append(interfaces, entry.Name()+":"+typed.Name.Name)
				}
			}
		}
	}
	if len(interfaces) != 1 || interfaces[0] != "service_contract.go:Service" {
		t.Fatalf("Models root interfaces = %v, want [service_contract.go:Service]", interfaces)
	}
}

func TestPackageOwnsOnlyTheEdgeAggregator(t *testing.T) {
	t.Parallel()

	localModelEffectTypes := map[string]struct{}{
		"PullMetric":           {},
		"AssetMakeDirectories": {}, "AssetInspectPath": {},
		"AssetResolveHomeDirectory": {}, "AssetWriteFile": {}, "AssetRenamePath": {},
		"AssetRemovePath": {}, "AssetReadFile": {}, "AssetReadDirectory": {},
		"AssetCreateFile": {}, "AssetOpenFile": {},
		"AssetStagingCoordination": {}, "AssetStagingCoordinationFactory": {},
		"ModelCLIInputReadFile":        {},
		"ModelCLIOutputCreateTempFile": {}, "ModelCLIOutputInspectPath": {},
		"HostProcessStartSpec":                 {},
		"ModelBackendArtifactSelectionRequest": {}, "ModelBackendArtifactSelection": {},
		"ModelResolveBackendArtifact":         {},
		"ModelInvocationBackend":              {},
		"ModelASRBackend":                     {},
		"ModelEmbeddingBackend":               {},
		"ModelHostProtocolNegotiationRequest": {}, "ModelHostProtocolNegotiationResult": {},
		"ModelHostProtocolNegotiator": {}, "ModelHostGRPCDialer": {}, "ModelHostGRPCConnection": {},
		"ModelHostCompatibilityRequest": {}, "ModelHostCompatibilityChecker": {},
		"RuntimeInspectFile":   {},
		"RuntimeTempDirectory": {}, "RuntimeCreateTempFile": {},
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read edges package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typed, ok := specification.(*ast.TypeSpec)
				if !ok || typed.Name.Name == "Edges" {
					continue
				}
				if entry.Name() == "models_effects.go" {
					if _, allowed := localModelEffectTypes[typed.Name.Name]; allowed {
						continue
					}
				}
				t.Errorf(
					"%s declares %s; edges may only declare the Edges aggregate and its exact Models process-edge contracts",
					entry.Name(),
					typed.Name.Name,
				)
			}
		}
	}
}
