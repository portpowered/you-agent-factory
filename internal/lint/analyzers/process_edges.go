package analyzers

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// ProcessEdges enforces the exact, zero-debt Process Edges and Models
// contract boundary using compiler-supplied files. Documentation phrases are
// intentionally not a rule; architecture prose remains a review authority.
var ProcessEdges = &analysis.Analyzer{
	Name: "processedges",
	Doc:  "enforce exact Process Edges types, Models imports, root interface and functional process ports",
	Run:  runProcessEdges,
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

func runProcessEdges(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	owner := strings.TrimSuffix(unit, "_test")
	for _, file := range pass.Files {
		checkFunctionalProcessPorts(pass, file, unit)
	}
	if !under(owner, "pkg/services/edges") && owner != "pkg/services/models" {
		return nil, nil
	}
	var interfaces []string
	var first token.Pos
	for _, file := range pass.Files {
		name := filepath.Base(pass.Fset.Position(file.Pos()).Filename)
		if under(owner, "pkg/services/edges") {
			if err := checkProcessEdgeImports(pass, file, unit); err != nil {
				return nil, err
			}
		}
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(unit, "_test") {
			continue
		}
		if first == token.NoPos {
			first = file.Package
		}
		interfaces = append(interfaces, processEdgeDeclarations(pass, file, owner, name)...)

	}
	if owner == "pkg/services/models" && first != token.NoPos {
		slices.Sort(interfaces)
		if !slices.Equal(interfaces, []string{"service_contract.go:Service"}) {
			pass.Reportf(first, "models-root-service-interface: interfaces=%v want=[service_contract.go:Service]", interfaces)
		}
	}
	return nil, nil
}

// Functional scenarios and their exact shared command fake consume the
// policy-free Platform port, rather than Workers-owned process contracts.
// Generated files remain in scope, as in the retired checker.
func checkFunctionalProcessPorts(pass *analysis.Pass, file *ast.File, unit string) {
	path := serviceSource(pass, unit, file)
	if !functionalSource(path) && path != "internal/testutil/provider_command_runner.go" {
		return
	}
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() ||
			obj.Pkg().Path() != modulePrefix+"pkg/services/workers" {
			return true
		}
		switch obj.Name() {
		case "CommandRunner", "CommandRequest", "CommandResult":
			pass.Reportf(id.Pos(), "functional-workers-process-port: %s -> pkg/services/workers.%s; inject pkg/platform/process.CommandRunner at edges.Edges and observe public Factory Events, Work or Factory Session projections", path, obj.Name())
		}
		return true
	})
}

func allowedProcessEdgeType(file, name string) bool {
	if name == "Edges" {
		return true
	}
	_, allowed := allowedProcessEdgeModelEffectTypes[name]
	return file == "models_effects.go" && allowed
}

func checkProcessEdgeImports(pass *analysis.Pass, file *ast.File, unit string) error {
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return err
		}
		if imported == modulePrefix+"pkg/services/models/wire" {
			pass.Reportf(spec.Path.Pos(), "process-edge-models-wire-import: %s must not import %s", unit, imported)
		}
	}
	return nil
}

func processEdgeDeclarations(pass *analysis.Pass, file *ast.File, owner, name string) []string {
	var interfaces []string
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}
		for _, specification := range generic.Specs {
			typed := specification.(*ast.TypeSpec)
			if owner == "pkg/services/edges" && !allowedProcessEdgeType(name, typed.Name.Name) {
				pass.Reportf(typed.Pos(), "process-edge-contract-ownership: %s#%s; declare only Edges and reviewed Models effect contracts in models_effects.go", name, typed.Name.Name)
			}
			if owner == "pkg/services/models" {
				if _, ok := typed.Type.(*ast.InterfaceType); ok {
					interfaces = append(interfaces, name+":"+typed.Name.Name)
				}
			}
		}
	}
	return interfaces
}
