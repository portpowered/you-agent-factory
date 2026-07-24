package functionaltestmetadata

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

	records := make([]Record, 0)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name == nil || !isTestName(function.Name.Name) {
			continue
		}

		record := Record{
			File:    relativePath,
			Package: file.Name.Name,
			Name:    function.Name.Name,
			Line:    fileSet.Position(function.Pos()).Line,
		}
		description, undocumented := descriptionFromDoc(function.Doc)
		record.Description = description
		record.Undocumented = undocumented
		records = append(records, record)
	}
	return records, nil
}

func descriptionFromDoc(group *ast.CommentGroup) (string, bool) {
	if group == nil {
		return "", true
	}
	text := strings.TrimSpace(group.Text())
	if text == "" {
		return "", true
	}
	synopsis := strings.TrimSpace(doc.Synopsis(text))
	if synopsis == "" {
		return "", true
	}
	return synopsis, false
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
