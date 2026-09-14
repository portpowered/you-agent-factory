package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	processEdgesRootPath        = "pkg/services/edges"
	modelsRootPath              = "pkg/services/models"
	modelsWireImportPath        = repositoryImportPrefix + "pkg/services/models/wire"
	processEdgeModelsEffectFile = "models_effects.go"

	processEdgeDocumentationRule = "process-edge-package-documentation"
	processEdgeImportRule        = "process-edge-models-wire-import"
	modelsRootInterfaceRule      = "models-root-service-interface"
	processEdgeTypeRule          = "process-edge-contract-ownership"
)

// processEdgeContractFinding is an exact, zero-debt static finding. These
// invariants describe package ownership and source topology; they do not
// belong in a runtime package's behavioral test lane.
type processEdgeContractFinding struct {
	rule     string
	filePath string
	target   string
	line     int
}

var allowedProcessEdgeModelEffectTypes = map[string]struct{}{
	"PullMetric":           {},
	"AssetMakeDirectories": {}, "AssetInspectPath": {},
	"AssetResolveHomeDirectory": {}, "AssetWriteFile": {}, "AssetRenamePath": {},
	"AssetRemovePath": {}, "AssetReadFile": {}, "AssetReadDirectory": {},
	"AssetCreateFile": {}, "AssetOpenFile": {},
	"AssetStagingCoordination": {}, "AssetStagingCoordinationFactory": {},
	"ModelCLIInputReadFile":        {},
	"ModelCLIOutputCreateTempFile": {}, "ModelCLIOutputInspectPath": {},
	"HostProcessStartSpec":                 {},
	"HostProcessStreamDiagnostic":          {},
	"HostProcessDiagnosticSnapshot":        {},
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

var requiredProcessEdgeDocumentation = []string{
	"process-edge aggregator",
	"root.BuildProcess",
	"pkg/wire",
	"functional",
	"not a service locator",
	"Initializer",
}

func hasProcessEdgeContractSurface(repoRoot string) bool {
	for _, relativePath := range []string{
		filepath.Join(processEdgesRootPath, "definition.go"),
		filepath.Join(modelsRootPath, "service_contract.go"),
	} {
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

func scanProcessEdgeContracts(repoRoot string) ([]processEdgeContractFinding, error) {
	edgesRoot := filepath.Join(repoRoot, filepath.FromSlash(processEdgesRootPath))
	if _, err := os.Stat(edgesRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat process-edge root: %w", err)
	}

	var findings []processEdgeContractFinding
	documentationFindings, err := scanProcessEdgeDocumentation(repoRoot, edgesRoot)
	if err != nil {
		return nil, err
	}
	findings = append(findings, documentationFindings...)

	importFindings, err := scanProcessEdgeImports(repoRoot, edgesRoot)
	if err != nil {
		return nil, err
	}
	findings = append(findings, importFindings...)

	typeFindings, err := scanProcessEdgeTypes(repoRoot, edgesRoot)
	if err != nil {
		return nil, err
	}
	findings = append(findings, typeFindings...)

	modelsFindings, err := scanModelsRootInterface(repoRoot)
	if err != nil {
		return nil, err
	}
	findings = append(findings, modelsFindings...)

	slices.SortFunc(findings, func(left, right processEdgeContractFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		if comparison := left.line - right.line; comparison != 0 {
			return comparison
		}
		if comparison := strings.Compare(left.rule, right.rule); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.target, right.target)
	})
	return findings, nil
}

func scanProcessEdgeDocumentation(repoRoot, edgesRoot string) ([]processEdgeContractFinding, error) {
	path := filepath.Join(edgesRoot, "definition.go")
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse process-edge definition: %w", err)
	}
	relative, err := relativeProcessEdgePath(repoRoot, path)
	if err != nil {
		return nil, err
	}
	var documentation string
	if file.Doc != nil {
		documentation = file.Doc.Text()
	}
	var findings []processEdgeContractFinding
	for _, phrase := range requiredProcessEdgeDocumentation {
		if !strings.Contains(documentation, phrase) {
			findings = append(findings, processEdgeContractFinding{
				rule: processEdgeDocumentationRule, filePath: relative, target: phrase,
				line: fileSet.Position(file.Pos()).Line,
			})
		}
	}
	if !strings.Contains(documentation, "exact") || !strings.Contains(strings.ToLower(documentation), "port") {
		findings = append(findings, processEdgeContractFinding{
			rule: processEdgeDocumentationRule, filePath: relative, target: "exact ports",
			line: fileSet.Position(file.Pos()).Line,
		})
	}
	return findings, nil
}

func scanProcessEdgeImports(repoRoot, edgesRoot string) ([]processEdgeContractFinding, error) {
	var findings []processEdgeContractFinding
	err := filepath.WalkDir(edgesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse process-edge imports %s: %w", filepath.ToSlash(path), err)
		}
		relative, err := relativeProcessEdgePath(repoRoot, path)
		if err != nil {
			return err
		}
		for _, importSpec := range file.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote process-edge import %s: %w", relative, err)
			}
			if importPath != modelsWireImportPath {
				continue
			}
			findings = append(findings, processEdgeContractFinding{
				rule: processEdgeImportRule, filePath: relative, target: importPath,
				line: fileSet.Position(importSpec.Path.Pos()).Line,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan process-edge imports: %w", err)
	}
	return findings, nil
}

func scanProcessEdgeTypes(repoRoot, edgesRoot string) ([]processEdgeContractFinding, error) {
	var findings []processEdgeContractFinding
	entries, err := os.ReadDir(edgesRoot)
	if err != nil {
		return nil, fmt.Errorf("read process-edge root: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !isProductionGoFile(entry.Name()) {
			continue
		}
		path := filepath.Join(edgesRoot, entry.Name())
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse process-edge type file %s: %w", filepath.ToSlash(path), err)
		}
		relative, err := relativeProcessEdgePath(repoRoot, path)
		if err != nil {
			return nil, err
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
				if entry.Name() == processEdgeModelsEffectFile {
					if _, allowed := allowedProcessEdgeModelEffectTypes[typed.Name.Name]; allowed {
						continue
					}
				}
				findings = append(findings, processEdgeContractFinding{
					rule: processEdgeTypeRule, filePath: relative, target: typed.Name.Name,
					line: fileSet.Position(typed.Pos()).Line,
				})
			}
		}
	}
	return findings, nil
}

func scanModelsRootInterface(repoRoot string) ([]processEdgeContractFinding, error) {
	modelsRoot := filepath.Join(repoRoot, filepath.FromSlash(modelsRootPath))
	if _, err := os.Stat(modelsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat Models root: %w", err)
	}
	entries, err := os.ReadDir(modelsRoot)
	if err != nil {
		return nil, fmt.Errorf("read Models root: %w", err)
	}
	var interfaces []string
	for _, entry := range entries {
		if entry.IsDir() || !isProductionGoFile(entry.Name()) {
			continue
		}
		path := filepath.Join(modelsRoot, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse Models root file %s: %w", entry.Name(), err)
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
	slices.Sort(interfaces)
	if slices.Equal(interfaces, []string{"service_contract.go:Service"}) {
		return nil, nil
	}
	return []processEdgeContractFinding{{
		rule:     modelsRootInterfaceRule,
		filePath: modelsRootPath,
		target:   fmt.Sprintf("interfaces=%v want=[service_contract.go:Service]", interfaces),
	}}, nil
}

func isProductionGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func relativeProcessEdgePath(repoRoot, path string) (string, error) {
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return "", fmt.Errorf("relativize process-edge path %s: %w", filepath.ToSlash(path), err)
	}
	return filepath.ToSlash(relative), nil
}

func writeProcessEdgeContractFindings(writer io.Writer, findings []processEdgeContractFinding) {
	for _, finding := range findings {
		location := finding.filePath
		if finding.line > 0 {
			location = fmt.Sprintf("%s:%d", location, finding.line)
		}
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] process-edge contract violation: %s (%s)\n", finding.target, location)
		switch finding.rule {
		case processEdgeDocumentationRule:
			fmt.Fprintln(writer, "  reason: the process-edge package documentation must state its exact external-effect aggregation boundary.")
		case processEdgeImportRule:
			fmt.Fprintln(writer, "  reason: Process Edges must not import the Models construction package, and Models must expose only its root Service interface.")
		case modelsRootInterfaceRule:
			fmt.Fprintln(writer, "  reason: Models root source topology is part of the service ownership boundary.")
		case processEdgeTypeRule:
			fmt.Fprintln(writer, "  reason: pkg/services/edges may declare only Edges and the reviewed exact Models process-effect contracts.")
		}
		fmt.Fprintln(writer, "  remediation: keep this repository-shape rule in the pkg-boundary static gate and update the owning contract deliberately.")
	}
}
