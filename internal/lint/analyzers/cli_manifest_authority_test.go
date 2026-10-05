package analyzers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestCLIManifestAuthorityDiagnostics(t *testing.T) {
	// The component owns compiler AST diagnostics; no production-tree inventory.
	for _, test := range []struct {
		name, path, source, want string
	}{
		{"permitted", "pkg/transports/cli/root_work.go", "func executeWork(inputID string) string { return inputID }", ""},
		{"function", "pkg/transports/cli/root_work.go", "func newRunCommand() {}", "CLI-shape function newRunCommand"},
		{"type", "pkg/transports/cli/root_factory.go", "type SessionFamilyBindings struct{}", "CLI-shape mirror SessionFamilyBindings"},
		{"metadata", "pkg/transports/cli/root_workflow.go", "var command = cobra.Command{}", "cobra.Command metadata"},
		{"flags", "pkg/transports/cli/root_submit_batch.go", "func execute() { command.Flags().String(\"name\", \"\", \"help\") }", "public input registration"},
		{"persistent", "pkg/transports/cli/root_work.go", "func execute() { command.PersistentFlags().Bool(\"server\", false, \"help\") }", "public input registration"},
		{"binding", "pkg/transports/cli/climanifestcobra/run_submit_constructor.go", "func execute(key string) { switch key { case \"with-server\": } }", "binding with-server"},
		{"registry-function", "pkg/transports/cli/commandregistry/representative_handlers.go", "func NewSessionRegistry() {}", "CLI-shape function NewSessionRegistry"},
		{"registry-storage", "pkg/transports/cli/commandregistry/representative_handlers.go", "type SessionHandlers struct{}", ""},
		{"unprotected-path", "pkg/transports/cli/other.go", "func newRunCommand() {}", ""},
		{"test-file", "pkg/transports/cli/root_work_test.go", "func newRunCommand() {}", ""},
		{"ordinary-binding", "pkg/transports/cli/root_work.go", "func execute(key string) { switch key { case \"factory-id\": } }", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, test.path, "package cli\n"+test.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			unit := test.path[:strings.LastIndex(test.path, "/")]
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{
				Analyzer: CLIManifestAuthority, Fset: fset, Files: []*ast.File{file},
				Pkg:    types.NewPackage(modulePrefix+unit, "cli"),
				Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) },
			}
			if _, err := CLIManifestAuthority.Run(pass); err != nil {
				t.Fatal(err)
			}
			if test.want == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("unexpected findings: %v", diagnostics)
				}
				return
			}
			if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, test.want) {
				t.Fatalf("findings %v, want %q", diagnostics, test.want)
			}
			if pos := fset.Position(diagnostics[0].Pos); pos.Filename != test.path || pos.Line != 2 {
				t.Fatalf("diagnostic location = %v", pos)
			}
		})
	}
}

func TestCLIManifestBindingClauseReportsEachOwnedInput(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "binding.go", `package cli
func execute(key string) {
 switch key { case "with-server", "with-site", "continuously", "factory-id": }
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	var details []string
	ast.Inspect(file, func(node ast.Node) bool {
		if clause, ok := node.(*ast.CaseClause); ok {
			details = cliBindingDetails(clause)
		}
		return true
	})
	if len(details) != 3 {
		t.Fatalf("findings %v, want all three owned inputs", details)
	}
}
