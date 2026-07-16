package functionalscenarios

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
)

// ReplayBoundaryFiles are the replay scenarios that previously required a
// reviewed functional-boundary exception. They must continue to exercise only
// customer-visible recording and replay paths.
var ReplayBoundaryFiles = []string{
	"tests/functional/replay_contracts/replay_event_stream_artifact_smoke_long_test.go",
	"tests/functional/replay_contracts/replay_live_helpers_test.go",
	"tests/functional/replay_contracts/replay_record_end_to_end_long_test.go",
	"tests/functional/replay_contracts/replay_regression_harness_long_test.go",
}

var replayBoundaryForbiddenImports = map[string]bool{
	"github.com/portpowered/infinite-you/pkg/factory/projections": true,
	"github.com/portpowered/infinite-you/pkg/factory/runtime":     true,
	"github.com/portpowered/infinite-you/pkg/replay":              true,
}

var replayBoundaryForbiddenTestutilCalls = map[string]bool{
	"AssertReplaySucceeds":     true,
	"NewReplayHarness":         true,
	"NewServiceTestHarness":    true,
	"NewServiceRuntimeHarness": true,
}

// CheckReplayContractBoundaries rejects direct replay/runtime execution in the
// four migrated scenarios while allowing test-only composition configuration.
func CheckReplayContractBoundaries(repositoryRoot string) error {
	for _, relativePath := range ReplayBoundaryFiles {
		if err := checkReplayContractBoundaryFile(repositoryRoot, relativePath); err != nil {
			return err
		}
	}
	return nil
}

func checkReplayContractBoundaryFile(repositoryRoot, relativePath string) error {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(relativePath))
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return fmt.Errorf("replay functional boundary: parse %s: %w", relativePath, err)
	}

	imports := make(map[string]string, len(file.Imports))
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			return fmt.Errorf("replay functional boundary: read import in %s: %w", relativePath, err)
		}
		name := filepath.Base(importPath)
		if specification.Name != nil {
			name = specification.Name.Name
		}
		imports[name] = importPath
	}

	var violation error
	ast.Inspect(file, func(node ast.Node) bool {
		if violation != nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath := imports[qualifier.Name]
		if replayBoundaryForbiddenImports[importPath] ||
			(importPath == "github.com/portpowered/infinite-you/pkg/service" && selector.Sel.Name != "") ||
			(importPath == "github.com/portpowered/infinite-you/pkg/testutil" && replayBoundaryForbiddenTestutilCalls[selector.Sel.Name]) {
			violation = fmt.Errorf("replay functional boundary: %s:%d directly invokes %s.%s; use CLI, generated REST, or Factory Session SSE instead", relativePath, fileSet.Position(call.Pos()).Line, importPath, selector.Sel.Name)
		}
		return true
	})
	return violation
}
