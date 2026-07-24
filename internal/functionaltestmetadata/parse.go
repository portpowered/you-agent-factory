package functionaltestmetadata

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Parse walks root for *_test.go files and inventories every top-level Test*
// declaration. Paths in returned records are relative to root, slash-normalized,
// and ordered stably by file then name. Malformed source fails closed with a
// file-scoped error rather than returning a partial inventory.
func Parse(root string) ([]Record, error) {
	absRoot, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}

	var records []Record
	err = filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", normalizePath(path), walkErr)
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		relative, err := filepath.Rel(absRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", normalizePath(path), err)
		}
		fileRecords, err := parseTestFile(path, normalizePath(relative))
		if err != nil {
			return err
		}
		records = append(records, fileRecords...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(records, compareRecords)
	return records, nil
}

func resolveRoot(root string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(root, "\\", "/")))
	absRoot, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("resolve functional test root %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("stat functional test root %s: %w", normalizePath(absRoot), err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("functional test root %s is not a directory", normalizePath(absRoot))
	}
	return absRoot, nil
}

func parseTestFile(absolutePath, relativePath string) ([]Record, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, absolutePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse functional test file %s: %w", relativePath, err)
	}

	buildTags, err := fileBuildTags(file, relativePath)
	if err != nil {
		return nil, err
	}

	classification := classifyPath(relativePath)
	records := make([]Record, 0)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name == nil || !isTestName(function.Name.Name) {
			continue
		}

		record := Record{
			File:           relativePath,
			Package:        file.Name.Name,
			Name:           function.Name.Name,
			Line:           fileSet.Position(function.Pos()).Line,
			BuildTags:      cloneStrings(buildTags),
			Classification: classification,
		}
		description, undocumented := descriptionFromDoc(function.Doc)
		record.Description = description
		record.Undocumented = undocumented
		record.Golden = goldenReference(function)
		records = append(records, record)
	}
	return records, nil
}

// classifyPath labels a repository-relative *_test.go path as customer or
// harness verification. Paths under internal/** and helper-only filenames
// (basename contains "helpers") are harness; everything else is customer.
func classifyPath(relativePath string) Classification {
	normalized := normalizePath(relativePath)
	if isInternalSupportPath(normalized) || isHelperOnlyFile(normalized) {
		return ClassificationHarness
	}
	return ClassificationCustomer
}

func isInternalSupportPath(relativePath string) bool {
	return relativePath == "internal" || strings.HasPrefix(relativePath, "internal/")
}

func isHelperOnlyFile(relativePath string) bool {
	base := filepath.Base(relativePath)
	name := strings.TrimSuffix(base, "_test.go")
	if name == base {
		return false
	}
	return strings.Contains(name, "helpers")
}

func fileBuildTags(file *ast.File, relativePath string) ([]string, error) {
	var goBuild []string
	var plusBuild []string
	for _, group := range file.Comments {
		if group.Pos() >= file.Package {
			break
		}
		for _, comment := range group.List {
			text := comment.Text
			switch {
			case constraint.IsGoBuild(text):
				expr, err := constraint.Parse(text)
				if err != nil {
					return nil, fmt.Errorf("parse build constraint in %s: %w", relativePath, err)
				}
				goBuild = append(goBuild, expr.String())
			case constraint.IsPlusBuild(text):
				expr, err := constraint.Parse(text)
				if err != nil {
					return nil, fmt.Errorf("parse build constraint in %s: %w", relativePath, err)
				}
				plusBuild = append(plusBuild, expr.String())
			}
		}
	}
	// Prefer //go:build when present, matching the go command.
	tags := plusBuild
	if len(goBuild) > 0 {
		tags = goBuild
	}
	if len(tags) == 0 {
		return nil, nil
	}
	slices.Sort(tags)
	return slices.Clip(slices.Compact(tags)), nil
}

func descriptionFromDoc(group *ast.CommentGroup) (string, bool) {
	if group == nil {
		return "", true
	}
	text := strings.TrimSpace(docTextWithoutGolden(group))
	if text == "" {
		return "", true
	}
	synopsis := strings.TrimSpace(doc.Synopsis(text))
	if synopsis == "" {
		return "", true
	}
	return synopsis, false
}

// docTextWithoutGolden returns ordinary Go-doc text with //golden: directive
// lines removed so catalog descriptions stay free of machine labels.
func docTextWithoutGolden(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	var lines []string
	for _, comment := range group.List {
		if _, ok := parseGoldenDirective(comment.Text); ok {
			continue
		}
		body := strings.TrimSpace(comment.Text)
		switch {
		case strings.HasPrefix(body, "//"):
			lines = append(lines, strings.TrimSpace(body[2:]))
		case strings.HasPrefix(body, "/*"):
			lines = append(lines, strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(body, "/*"), "*/")))
		}
	}
	return strings.Join(lines, "\n")
}

func goldenReference(function *ast.FuncDecl) string {
	if path := goldenFromComments(function.Doc); path != "" {
		return path
	}
	return goldenFromBody(function.Body)
}

func goldenFromComments(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	for _, comment := range group.List {
		if path, ok := parseGoldenDirective(comment.Text); ok {
			return path
		}
	}
	return ""
}

func parseGoldenDirective(commentText string) (string, bool) {
	body := strings.TrimSpace(commentText)
	switch {
	case strings.HasPrefix(body, "//"):
		body = strings.TrimSpace(body[2:])
	case strings.HasPrefix(body, "/*"):
		body = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(body, "/*"), "*/"))
	default:
		return "", false
	}
	if len(body) < len("golden:") || !strings.EqualFold(body[:len("golden:")], "golden:") {
		return "", false
	}
	path := strings.TrimSpace(body[len("golden:"):])
	if path == "" {
		return "", false
	}
	return normalizePath(path), true
}

func goldenFromBody(body *ast.BlockStmt) string {
	if body == nil {
		return ""
	}
	var found string
	ast.Inspect(body, func(node ast.Node) bool {
		if found != "" {
			return false
		}
		decl, ok := node.(*ast.GenDecl)
		if !ok || (decl.Tok != token.CONST && decl.Tok != token.VAR) {
			return true
		}
		for _, spec := range decl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if name == nil || !isGoldenDeclName(name.Name) || i >= len(valueSpec.Values) {
					continue
				}
				if path, ok := stringLiteralPath(valueSpec.Values[i]); ok {
					found = path
					return false
				}
			}
		}
		return true
	})
	return found
}

func isGoldenDeclName(name string) bool {
	switch name {
	case "golden", "goldenManifest", "goldenFixture", "Golden", "GoldenManifest", "GoldenFixture":
		return true
	default:
		return false
	}
}

func stringLiteralPath(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil || strings.TrimSpace(unquoted) == "" {
		return "", false
	}
	return normalizePath(unquoted), true
}

// isTestName mirrors go test discovery: "Test" or "Test" followed by an
// uppercase letter.
func isTestName(name string) bool {
	const prefix = "Test"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return unicode.IsUpper(r)
}

func compareRecords(a, b Record) int {
	if a.File != b.File {
		return strings.Compare(a.File, b.File)
	}
	if a.Name != b.Name {
		return strings.Compare(a.Name, b.Name)
	}
	return a.Line - b.Line
}

// normalizePath converts OS-specific or mixed separators into a stable
// slash-separated path identity.
func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(path, "\\", "/"))))
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return slices.Clone(values)
}
