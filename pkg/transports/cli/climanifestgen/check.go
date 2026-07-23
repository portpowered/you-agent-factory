package climanifestgen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
)

const rootGlobalRegistrationAuthority = "pkg/transports/cli/climanifestcobra/constructor.go"

// RootGlobalAuthorityViolation identifies handwritten root persistent-flag
// registration outside the manifest-to-Cobra projection boundary.
type RootGlobalAuthorityViolation struct {
	Path   string
	Line   int
	Method string
}

// CheckRootGlobalAuthority rejects a second writable definition path for
// public root/global inputs while permitting read-only Cobra flag mechanics.
func CheckRootGlobalAuthority(source fs.FS) ([]RootGlobalAuthorityViolation, error) {
	if source == nil {
		return nil, fmt.Errorf("check root/global CLI authority: source filesystem is required")
	}
	if _, err := fs.Stat(source, "pkg/transports/cli"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("check root/global CLI authority: %w", err)
	}
	var violations []RootGlobalAuthorityViolation
	err := fs.WalkDir(source, "pkg/transports/cli", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Ext(filePath) != ".go" ||
			strings.HasSuffix(filePath, "_test.go") ||
			filePath == rootGlobalRegistrationAuthority {
			return nil
		}
		payload, err := fs.ReadFile(source, filePath)
		if err != nil {
			return err
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, filePath, payload, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filePath, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := persistentFlagRegistrationMethod(call)
			if !ok {
				return true
			}
			violations = append(violations, RootGlobalAuthorityViolation{
				Path: filePath, Line: fileSet.Position(call.Pos()).Line, Method: method,
			})
			return true
		})
		return nil
	})
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Path == violations[j].Path {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].Path < violations[j].Path
	})
	return violations, err
}

func persistentFlagRegistrationMethod(call *ast.CallExpr) (string, bool) {
	registration, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isFlagRegistrationMethod(registration.Sel.Name) {
		return "", false
	}
	flagSetCall, ok := registration.X.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	flagSetSelector, ok := flagSetCall.Fun.(*ast.SelectorExpr)
	if !ok || flagSetSelector.Sel.Name != "PersistentFlags" {
		return "", false
	}
	return registration.Sel.Name, true
}

func isFlagRegistrationMethod(method string) bool {
	if method == "Var" || method == "VarP" ||
		method == "AddFlag" || method == "AddFlagSet" {
		return true
	}
	for _, prefix := range []string{
		"Bool", "Bytes", "Count", "Duration", "Float", "IP", "Int",
		"String", "Uint",
	} {
		if strings.HasPrefix(method, prefix) {
			return true
		}
	}
	return false
}

// Drift describes byte-level differences between generated artifacts and the
// current generator output.
type Drift struct {
	Stale      []string
	Missing    []string
	Unexpected []string
	// CommandIDs maps a generated artifact path to the stable IDs affected by drift.
	CommandIDs map[string][]string
}

// Empty reports whether generated artifacts match the generator output.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 && len(drift.Missing) == 0 && len(drift.Unexpected) == 0
}

// AnnotateDrift adds CLI command identity context to policy-free artifact
// drift computed by the command-selected Platform store.
func AnnotateDrift(base generatedartifacts.Drift) Drift {
	drift := Drift{
		Stale: append([]string(nil), base.Stale...), Missing: append([]string(nil), base.Missing...),
		Unexpected: append([]string(nil), base.Unexpected...), CommandIDs: map[string][]string{},
	}
	artifactIDs := map[string][]string{
		RunSubmitFamilyJSONPath:       RunSubmitFamilyCommandIDs,
		RunSubmitFamilyCommandIDsPath: RunSubmitFamilyCommandIDs,
		MCPFamilyJSONPath:             MCPFamilyCommandIDs,
		MCPFamilyCommandIDsPath:       MCPFamilyCommandIDs,
	}
	for _, paths := range [][]string{drift.Missing, drift.Stale} {
		for _, path := range paths {
			if ids := artifactIDs[path]; len(ids) > 0 {
				drift.CommandIDs[path] = append([]string(nil), ids...)
			}
		}
	}
	return drift
}
