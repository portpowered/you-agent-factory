package retiredsurfaceguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
)

var cliManifestAuthorityPaths = []string{
	"pkg/transports/cli/root_work.go",
	"pkg/transports/cli/climanifestcobra/run_submit_constructor.go",
}

var retiredRunServerRegistrationFunctions = map[string]struct{}{
	"newRunCommand":                   {},
	"registerRunCommandFlags":         {},
	"runExecutionLocalBindingTarget":  {},
	"runRuntimeLocalBindingTarget":    {},
	"runInvocationLocalBindingTarget": {},
}

var manifestOwnedRunServerBindings = map[string]struct{}{
	"continuously": {},
	"with-server":  {},
	"with-site":    {},
}

// ScanCLIManifestAuthoritySourceViolations reports handwritten run/server
// registration and public binding switches in paths owned by the CLI manifest.
func ScanCLIManifestAuthoritySourceViolations(repoRoot string) ([]Violation, error) {
	var violations []Violation
	for _, relative := range cliManifestAuthorityPaths {
		path := filepath.Join(repoRoot, filepath.FromSlash(relative))
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", relative, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.FuncDecl:
				if _, retired := retiredRunServerRegistrationFunctions[typed.Name.Name]; retired {
					violations = append(violations, cliManifestAuthorityViolation(
						relative,
						"production source must not declare handwritten run/server registration "+typed.Name.Name,
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

func cliManifestAuthorityViolation(relative, detail string) Violation {
	return Violation{
		Family:  "cli-manifest-authority",
		Surface: filepath.ToSlash(relative),
		Detail:  detail,
	}
}
