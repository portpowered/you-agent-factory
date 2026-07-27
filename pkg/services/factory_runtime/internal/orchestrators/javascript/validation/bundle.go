package workflowvalidation

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

const (
	CodeImportNotFound = "workflow.source.notFound"
)

type importBinding struct {
	localName  string
	importedAs string
}

type bundledModule struct {
	sourceRef string
	body      string
	imports   []resolvedImport
}

type resolvedImport struct {
	spec       string
	resolved   string
	bindings   []importBinding
	isDefault  bool
	defaultAs  string
}

// ContainsFactoryRelativeImports reports whether source includes factory-relative
// ES module import statements that require bundling before validation or execution.
func ContainsFactoryRelativeImports(source string) bool {
	ast, err := js.Parse(parse.NewInputString(source), js.Options{})
	if err != nil {
		return false
	}
	for _, stmt := range ast.BlockStmt.List {
		importStmt, ok := stmt.(*js.ImportStmt)
		if !ok {
			continue
		}
		if isFactoryRelativeImportSpecifier(unquoteJSModuleSpecifier(string(importStmt.Module))) {
			return true
		}
	}
	return false
}

// BundleFactoryRelativeImports resolves factory-relative ES module imports and
// returns one executable JavaScript source for the workflow runtime.
func BundleFactoryRelativeImports(
	entrySourceRef string,
	content string,
	reader SourceReader,
) (string, []Issue) {
	entrySourceRef = normalizeFactorySourceRef(entrySourceRef)
	if reader == nil {
		return "", []Issue{{
			Code:    CodeSourceUnreadable,
			Message: "workflow source reader is required to resolve factory-relative imports",
			Path:    entrySourceRef,
		}}
	}

	modules := make(map[string]bundledModule)
	visiting := make(map[string]struct{})
	var walk func(sourceRef, source string) []Issue
	walk = func(sourceRef, source string) []Issue {
		sourceRef = normalizeFactorySourceRef(sourceRef)
		if _, seen := modules[sourceRef]; seen {
			return nil
		}
		if _, inProgress := visiting[sourceRef]; inProgress {
			return []Issue{{
				Code: CodeUnsupportedLoader,
				Message: fmt.Sprintf(
					"circular factory-relative import detected involving %q",
					sourceRef,
				),
				Path: sourceRef,
			}}
		}
		visiting[sourceRef] = struct{}{}
		defer delete(visiting, sourceRef)

		ast, err := js.Parse(parse.NewInputString(source), js.Options{})
		if err != nil {
			issue := syntaxIssue(err, Request{SourceRef: sourceRef})
			return []Issue{issue}
		}

		module := bundledModule{sourceRef: sourceRef}
		var bodyLines []string

		for _, stmt := range ast.BlockStmt.List {
			switch node := stmt.(type) {
			case *js.ImportStmt:
				spec := unquoteJSModuleSpecifier(string(node.Module))
				if !isFactoryRelativeImportSpecifier(spec) {
					return []Issue{{
						Code:    CodeForbiddenHostAccess,
						Message: "ES module import is not supported in MVP workflows",
						Path:    sourceRef,
					}}
				}
				resolved, issue := resolveFactoryRelativeImport(sourceRef, spec)
				if issue != nil {
					return []Issue{*issue}
				}
				imported, err := reader.ReadWorkflowSource(resolved)
				if err != nil {
					return []Issue{{
						Code:    CodeImportNotFound,
						Message: fmt.Sprintf("workflow import %q could not be resolved under the factory root: %v", spec, err),
						Path:    sourceRef,
					}}
				}
				if childIssues := walk(resolved, imported); len(childIssues) > 0 {
					return childIssues
				}
				module.imports = append(module.imports, resolvedImport{
					spec:     spec,
					resolved: resolved,
					bindings: importBindingsFromStmt(node),
					isDefault: len(node.Default) > 0,
					defaultAs: string(node.Default),
				})
			case *js.ExportStmt:
				transformed, issue := transformExportStatement(node)
				if issue != nil {
					issue.Path = sourceRef
					return []Issue{*issue}
				}
				bodyLines = append(bodyLines, transformed)
			default:
				bodyLines = append(bodyLines, stmtString(stmt))
			}
		}

		module.body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		modules[sourceRef] = module
		return nil
	}

	if issues := walk(entrySourceRef, content); len(issues) > 0 {
		return "", issues
	}
	return emitBundledExecutable(modules, entrySourceRef), nil
}

func emitBundledExecutable(modules map[string]bundledModule, entrySourceRef string) string {
	var out strings.Builder
	out.WriteString("const __modules = {};\n")
	out.WriteString("function __factoryRequire(ref){\n")
	out.WriteString("if(__modules[ref]===undefined){\n")
	out.WriteString("throw new Error(\"missing workflow module \"+ref);\n")
	out.WriteString("}\nreturn __modules[ref];\n}\n")

	visited := make(map[string]struct{})
	var emitDependencies func(sourceRef string)
	emitDependencies = func(sourceRef string) {
		sourceRef = normalizeFactorySourceRef(sourceRef)
		if _, ok := visited[sourceRef]; ok {
			return
		}
		visited[sourceRef] = struct{}{}
		module := modules[sourceRef]
		for _, imp := range module.imports {
			emitDependencies(imp.resolved)
		}
		if sourceRef == normalizeFactorySourceRef(entrySourceRef) {
			return
		}
		out.WriteString("__modules[")
		out.WriteString(jsStringLiteral(sourceRef))
		out.WriteString("]=(function(exports, __factoryRequire){\n")
		for _, imp := range module.imports {
			out.WriteString(importBindingLine(imp))
			out.WriteString("\n")
		}
		out.WriteString(module.body)
		if module.body != "" {
			out.WriteString("\n")
		}
		out.WriteString("return exports;\n})({}, __factoryRequire);\n")
	}
	emitDependencies(entrySourceRef)

	entry := modules[entrySourceRef]
	for _, imp := range entry.imports {
		out.WriteString(importBindingLine(imp))
		out.WriteString("\n")
	}
	out.WriteString(entry.body)
	if entry.body != "" {
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String())
}

func importBindingLine(imp resolvedImport) string {
	moduleRef := jsStringLiteral(imp.resolved)
	var lines []string
	if imp.isDefault {
		name := strings.TrimSpace(imp.defaultAs)
		if name == "" {
			name = "default"
		}
		lines = append(lines, fmt.Sprintf("const %s = __factoryRequire(%s).default;", name, moduleRef))
	}
	if len(imp.bindings) == 1 && imp.bindings[0].importedAs == "*" {
		lines = append(lines, fmt.Sprintf("const %s = __factoryRequire(%s);", imp.bindings[0].localName, moduleRef))
	} else if len(imp.bindings) > 0 {
		parts := make([]string, 0, len(imp.bindings))
		for _, binding := range imp.bindings {
			if binding.localName == binding.importedAs {
				parts = append(parts, binding.localName)
			} else {
				parts = append(parts, binding.importedAs+": "+binding.localName)
			}
		}
		lines = append(lines, fmt.Sprintf("const {%s} = __factoryRequire(%s);", strings.Join(parts, ", "), moduleRef))
	}
	if len(lines) == 0 {
		return fmt.Sprintf("__factoryRequire(%s);", moduleRef)
	}
	return strings.Join(lines, "\n")
}

func importBindingsFromStmt(stmt *js.ImportStmt) []importBinding {
	bindings := make([]importBinding, 0, len(stmt.List))
	for _, alias := range stmt.List {
		localName := string(alias.Binding)
		importedAs := string(alias.Name)
		if importedAs == "" {
			importedAs = localName
		}
		if len(importedAs) == 1 && importedAs[0] == '*' {
			if localName == "" {
				localName = "namespace"
			}
		}
		bindings = append(bindings, importBinding{
			localName:  localName,
			importedAs: importedAs,
		})
	}
	return bindings
}

func transformExportStatement(stmt *js.ExportStmt) (string, *Issue) {
	if stmt.Decl == nil {
		return "", &Issue{
			Code:    CodeUnsupportedLoader,
			Message: "re-export statements are not supported in MVP factory-relative imports",
		}
	}
	if stmt.Default {
		return fmt.Sprintf("exports.default = %s;", declExpressionString(stmt.Decl)), nil
	}
	switch decl := stmt.Decl.(type) {
	case *js.VarDecl:
		lines := make([]string, 0, len(decl.List))
		for _, binding := range decl.List {
			name := bindingName(binding.Binding)
			if name == "" {
				continue
			}
			if binding.Default != nil {
				lines = append(lines, fmt.Sprintf("exports.%s = %s;", name, exprString(binding.Default)))
				continue
			}
			lines = append(lines, fmt.Sprintf("var %s; exports.%s = %s;", name, name, name))
		}
		return strings.Join(lines, "\n"), nil
	case *js.FuncDecl:
		name := ""
		if decl.Name != nil {
			name = string(decl.Name.Name())
		}
		if name == "" {
			return "", &Issue{
				Code:    CodeUnsupportedLoader,
				Message: "anonymous export functions are not supported in factory-relative workflow modules",
			}
		}
		return fmt.Sprintf("exports.%s = %s;", name, declString(decl)), nil
	default:
		return "", &Issue{
			Code:    CodeUnsupportedLoader,
			Message: "unsupported export declaration in factory-relative workflow module",
		}
	}
}

func resolveFactoryRelativeImport(importerRef, spec string) (string, *Issue) {
	spec = strings.TrimSpace(spec)
	if !isFactoryRelativeImportSpecifier(spec) {
		return "", &Issue{
			Code:    CodeForbiddenHostAccess,
			Message: "ES module import is not supported in MVP workflows",
			Path:    importerRef,
		}
	}
	importerDir := filepath.Dir(filepath.FromSlash(importerRef))
	joined := filepath.ToSlash(filepath.Clean(filepath.Join(importerDir, spec)))
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", &Issue{
			Code:    CodeImportNotFound,
			Message: fmt.Sprintf("workflow import %q escapes the factory root", spec),
			Path:    importerRef,
		}
	}
	return normalizeFactorySourceRef(joined), nil
}

func isFactoryRelativeImportSpecifier(spec string) bool {
	spec = strings.TrimSpace(spec)
	return strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")
}

func normalizeFactorySourceRef(sourceRef string) string {
	ref := filepath.ToSlash(strings.TrimSpace(sourceRef))
	ref = strings.TrimPrefix(ref, "./")
	return ref
}

func unquoteJSModuleSpecifier(spec string) string {
	spec = strings.TrimSpace(spec)
	if len(spec) >= 2 {
		if (spec[0] == '"' && spec[len(spec)-1] == '"') || (spec[0] == '\'' && spec[len(spec)-1] == '\'') {
			return spec[1 : len(spec)-1]
		}
	}
	return spec
}

func jsStringLiteral(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func stmtString(stmt js.IStmt) string {
	var buf strings.Builder
	stmt.JS(&buf)
	return strings.TrimSpace(buf.String())
}

func declString(decl *js.FuncDecl) string {
	var buf strings.Builder
	decl.JS(&buf)
	return strings.TrimSpace(buf.String())
}

func exprString(expr js.IExpr) string {
	var buf strings.Builder
	expr.JS(&buf)
	return strings.TrimSpace(buf.String())
}

func declExpressionString(decl js.IExpr) string {
	switch node := decl.(type) {
	case *js.FuncDecl:
		return declString(node)
	default:
		return exprString(decl)
	}
}

func bindingName(binding js.IBinding) string {
	if binding == nil {
		return ""
	}
	if variable, ok := binding.(*js.Var); ok {
		return string(variable.Name())
	}
	var buf strings.Builder
	binding.JS(&buf)
	return strings.TrimSpace(buf.String())
}
