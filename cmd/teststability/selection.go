package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var changedTestNamePattern = regexp.MustCompile(`^Test[A-Z0-9_][A-Za-z0-9_]*$`)

type diffFile struct {
	oldPath string
	newPath string
}

type selectedTest struct {
	File string
	Name string
}

type testFunction struct {
	Name string
	Body []byte
}

type sourceReader func(revision, path string) ([]byte, error)

func selectChangedTests(rawDiff, headRevision, baseRevision string, readSource sourceReader) ([]selectedTest, error) {
	files, err := parseGitDiff(rawDiff)
	if err != nil {
		return nil, err
	}
	oldBodies, err := collectOldTestBodies(files, baseRevision, readSource)
	if err != nil {
		return nil, err
	}

	selected := make([]selectedTest, 0)
	for _, file := range files {
		if !isGoTestPath(file.newPath) {
			continue
		}
		source, err := readSource(headRevision, file.newPath)
		if err != nil {
			return nil, fmt.Errorf("read head test source %q: %w", file.newPath, err)
		}
		headTests, err := parseTopLevelTests(file.newPath, source)
		if err != nil {
			return nil, err
		}
		oldTests := map[string]testFunction{}
		if file.oldPath != "" {
			oldSource, err := readSource(baseRevision, file.oldPath)
			if err != nil {
				return nil, fmt.Errorf("read base test source %q: %w", file.oldPath, err)
			}
			parsed, err := parseTopLevelTests(file.oldPath, oldSource)
			if err != nil {
				return nil, err
			}
			for _, test := range parsed {
				oldTests[test.Name] = test
			}
		}

		for _, test := range headTests {
			oldTest, samePath := oldTests[test.Name]
			if samePath {
				if bytes.Equal(oldTest.Body, test.Body) {
					continue
				}
				selected = append(selected, selectedTest{File: file.newPath, Name: test.Name})
				continue
			}
			if oldBody, moved := oldBodies[test.Name]; moved && bytes.Equal(oldBody, test.Body) {
				continue
			}
			selected = append(selected, selectedTest{File: file.newPath, Name: test.Name})
		}
	}

	return dedupeSelectedTests(selected), nil
}

func collectOldTestBodies(files []diffFile, baseRevision string, readSource sourceReader) (map[string][]byte, error) {
	bodies := make(map[string][]byte)
	for _, file := range files {
		if !isGoTestPath(file.oldPath) {
			continue
		}
		source, err := readSource(baseRevision, file.oldPath)
		if err != nil {
			return nil, fmt.Errorf("read base test source %q: %w", file.oldPath, err)
		}
		tests, err := parseTopLevelTests(file.oldPath, source)
		if err != nil {
			return nil, err
		}
		for _, test := range tests {
			if _, exists := bodies[test.Name]; !exists {
				bodies[test.Name] = append([]byte(nil), test.Body...)
			}
		}
	}
	return bodies, nil
}

func parseTopLevelTests(path string, source []byte) ([]testFunction, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, source, 0)
	if err != nil {
		return nil, fmt.Errorf("parse Go test source %q: %w", path, err)
	}
	tests := make([]testFunction, 0)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Body == nil || !isGoTestFunction(function) {
			continue
		}
		name := function.Name.Name
		if name == "TestMain" || !changedTestNamePattern.MatchString(name) {
			continue
		}
		start := fileSet.Position(function.Body.Lbrace).Offset
		end := fileSet.Position(function.Body.Rbrace).Offset + 1
		if start < 0 || end < start || end > len(source) {
			return nil, fmt.Errorf("parse Go test source %q: invalid body range for %s", path, name)
		}
		tests = append(tests, testFunction{Name: name, Body: append([]byte(nil), source[start:end]...)})
	}
	slices.SortFunc(tests, func(left, right testFunction) int {
		return strings.Compare(left.Name, right.Name)
	})
	return tests, nil
}

func isGoTestFunction(function *ast.FuncDecl) bool {
	if function.Type.Params == nil || len(function.Type.Params.List) != 1 {
		return false
	}
	parameter, ok := function.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := parameter.X.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "T" && selectorPackageName(selector) == "testing"
}

func selectorPackageName(selector *ast.SelectorExpr) string {
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func parseGitDiff(raw string) ([]diffFile, error) {
	var files []diffFile
	var current *diffFile
	flush := func() {
		if current == nil {
			return
		}
		if current.newPath != "" || current.oldPath != "" {
			files = append(files, *current)
		}
		current = nil
	}

	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			oldPath, newPath := parseGitHeaderPaths(strings.TrimPrefix(line, "diff --git "))
			current = &diffFile{oldPath: oldPath, newPath: newPath}
		case current == nil:
			continue
		case strings.HasPrefix(line, "--- "):
			current.oldPath = parseDiffPath(strings.TrimPrefix(line, "--- "), "a/")
		case strings.HasPrefix(line, "+++ "):
			current.newPath = parseDiffPath(strings.TrimPrefix(line, "+++ "), "b/")
		case strings.HasPrefix(line, "rename from "):
			current.oldPath = normalizeDiffPath(strings.TrimPrefix(line, "rename from "))
		case strings.HasPrefix(line, "rename to "):
			current.newPath = normalizeDiffPath(strings.TrimPrefix(line, "rename to "))
		}
	}
	flush()
	return files, nil
}

func parseGitHeaderPaths(raw string) (string, string) {
	parts := splitGitHeader(raw)
	if len(parts) != 2 {
		return "", ""
	}
	return parseDiffPath(parts[0], "a/"), parseDiffPath(parts[1], "b/")
}

func splitGitHeader(raw string) []string {
	var parts []string
	for len(raw) > 0 {
		raw = strings.TrimLeft(raw, " \t")
		if raw == "" {
			break
		}
		if raw[0] != '"' {
			end := strings.IndexAny(raw, " \t")
			if end < 0 {
				parts = append(parts, raw)
				break
			}
			parts = append(parts, raw[:end])
			raw = raw[end:]
			continue
		}
		end := 1
		for end < len(raw) {
			if raw[end] == '\\' {
				end += 2
				continue
			}
			if raw[end] == '"' {
				end++
				break
			}
			end++
		}
		if end > len(raw) {
			return nil
		}
		value, err := strconv.Unquote(raw[:end])
		if err != nil {
			return nil
		}
		parts = append(parts, value)
		raw = raw[end:]
	}
	return parts
}

func parseDiffPath(raw, prefix string) string {
	raw = strings.TrimSpace(raw)
	if raw == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(raw, "\"") {
		if value, err := strconv.Unquote(raw); err == nil {
			raw = value
		}
	}
	return normalizeDiffPath(strings.TrimPrefix(raw, prefix))
}

func normalizeDiffPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	if path == "." || path == "" {
		return ""
	}
	return path
}

func isGoTestPath(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

func dedupeSelectedTests(tests []selectedTest) []selectedTest {
	seen := make(map[string]struct{}, len(tests))
	result := make([]selectedTest, 0, len(tests))
	for _, test := range tests {
		key := test.File + "\x00" + test.Name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, test)
	}
	slices.SortFunc(result, func(left, right selectedTest) int {
		if comparison := strings.Compare(left.File, right.File); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.Name, right.Name)
	})
	return result
}
