package contractguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const compatibilityAliasBoundaryMarker = "compatibility-alias-boundary:"

// DefaultCompatibilityAliasBoundaryPrefixes are repository paths where retained
// compatibility alias surfaces may be referenced while external callability
// remains unchanged.
var DefaultCompatibilityAliasBoundaryPrefixes = []string{
	"api/",
	"contracts/",
	"packages/",
	"pkg/transports/",
	"pkg/factory/",
	"pkg/service/",
	"internal/testutil/",
	"ui/src/api/workflow-preview/",
	"ui/src/api/generated/",
	"ui/src/api/session-routing.ts",
	"ui/src/api/session-factory/",
	"ui/src/features/workflow-preview/",
}

var compatibilityAliasScanRoots = []string{"cmd", "internal", "pkg", "ui/src"}

// CompatibilityAliasViolation records one deliberate compatibility alias
// adoption outside an approved boundary.
type CompatibilityAliasViolation struct {
	FilePath   string
	Line       int
	Column     int
	Term       string
	ItemID     string
	PublicName string
}

// ScanCompatibilityAliasViolations scans handwritten first-party sources for new
// internal adoption of inventoried compatibility alias names.
func ScanCompatibilityAliasViolations(repoRoot string, terms []CompatibilityAliasTerm, boundaryPrefixes []string) ([]CompatibilityAliasViolation, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	if len(boundaryPrefixes) == 0 {
		boundaryPrefixes = DefaultCompatibilityAliasBoundaryPrefixes
	}

	matchIndex := buildCompatibilityAliasMatchIndex(terms)
	var violations []CompatibilityAliasViolation
	for _, sourceRoot := range compatibilityAliasScanRoots {
		rootPath := filepath.Join(repoRoot, filepath.FromSlash(sourceRoot))
		if _, statErr := os.Stat(rootPath); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			return nil, fmt.Errorf("inspect %s: %w", sourceRoot, statErr)
		}
		err = filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if shouldSkipCompatibilityAliasDir(repoRoot, path) {
					return filepath.SkipDir
				}
				return nil
			}
			relative, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			if shouldSkipCompatibilityAliasFile(relative) {
				return nil
			}
			if isCompatibilityAliasBoundary(relative, boundaryPrefixes) {
				return nil
			}
			fileViolations, scanErr := scanCompatibilityAliasFile(path, relative, matchIndex)
			if scanErr != nil {
				return scanErr
			}
			violations = append(violations, fileViolations...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", sourceRoot, err)
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].FilePath == violations[j].FilePath {
			if violations[i].Line == violations[j].Line {
				if violations[i].Column == violations[j].Column {
					return violations[i].Term < violations[j].Term
				}
				return violations[i].Column < violations[j].Column
			}
			return violations[i].Line < violations[j].Line
		}
		return violations[i].FilePath < violations[j].FilePath
	})
	return violations, nil
}

type compatibilityAliasMatch struct {
	term       string
	itemID     string
	publicName string
}

func buildCompatibilityAliasMatchIndex(terms []CompatibilityAliasTerm) []compatibilityAliasMatch {
	seen := make(map[string]compatibilityAliasMatch)
	for _, term := range terms {
		for _, matchValue := range term.MatchValues {
			if _, exists := seen[matchValue]; exists {
				continue
			}
			seen[matchValue] = compatibilityAliasMatch{
				term:       matchValue,
				itemID:     term.ItemID,
				publicName: term.PublicName,
			}
		}
	}
	matches := make([]compatibilityAliasMatch, 0, len(seen))
	for _, match := range seen {
		matches = append(matches, match)
	}
	sort.Slice(matches, func(i, j int) bool {
		if len(matches[i].term) == len(matches[j].term) {
			return matches[i].term < matches[j].term
		}
		return len(matches[i].term) > len(matches[j].term)
	})
	return matches
}

func shouldSkipCompatibilityAliasDir(repoRoot, path string) bool {
	return ShouldSkipDir(repoRoot, path, "vendor", "node_modules", "testdata", "generated", ".git")
}

func shouldSkipCompatibilityAliasFile(relative string) bool {
	if strings.Contains(relative, "/testdata/") {
		return true
	}
	if strings.HasSuffix(relative, "_test.go") {
		return true
	}
	switch filepath.Ext(relative) {
	case ".go":
		return false
	case ".ts", ".tsx":
		return strings.HasSuffix(relative, ".test.ts") ||
			strings.HasSuffix(relative, ".test.tsx") ||
			strings.HasSuffix(relative, ".stories.tsx")
	default:
		return true
	}
}

func isCompatibilityAliasBoundary(relative string, boundaryPrefixes []string) bool {
	for _, prefix := range boundaryPrefixes {
		if relative == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(relative, prefix) {
			return true
		}
	}
	return false
}

func scanCompatibilityAliasFile(path, relative string, matches []compatibilityAliasMatch) ([]CompatibilityAliasViolation, error) {
	switch filepath.Ext(relative) {
	case ".go":
		return scanGoCompatibilityAliasFile(path, relative, matches)
	case ".ts", ".tsx":
		return scanTypeScriptCompatibilityAliasFile(path, relative, matches)
	default:
		return nil, nil
	}
}

func scanGoCompatibilityAliasFile(path, relative string, matches []compatibilityAliasMatch) ([]CompatibilityAliasViolation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relative, err)
	}
	if strings.Contains(string(raw), compatibilityAliasBoundaryMarker) {
		return nil, nil
	}

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, raw, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relative, err)
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}

	seen := make(map[string]struct{})
	var violations []CompatibilityAliasViolation
	record := func(position token.Pos, match compatibilityAliasMatch) {
		if position == token.NoPos {
			return
		}
		line, column := fileSet.Position(position).Line, fileSet.Position(position).Column
		key := fmt.Sprintf("%s:%d:%d:%s", relative, line, column, match.term)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		violations = append(violations, CompatibilityAliasViolation{
			FilePath:   relative,
			Line:       line,
			Column:     column,
			Term:       match.term,
			ItemID:     match.itemID,
			PublicName: match.publicName,
		})
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.ImportSpec:
			if typed.Path != nil {
				for _, match := range matches {
					if containsCompatibilityAliasTerm(unquoteGoString(typed.Path.Value), match.term) ||
						containsCompatibilityAliasTerm(typed.Path.Value, match.term) {
						record(typed.Path.ValuePos, match)
					}
				}
			}
		case *ast.BasicLit:
			if typed.Kind != token.STRING {
				return true
			}
			literal := unquoteGoString(typed.Value)
			for _, match := range matches {
				if containsCompatibilityAliasTerm(literal, match.term) {
					record(typed.ValuePos, match)
				}
			}
		case *ast.Ident:
			for _, match := range matches {
				if typed.Name == match.term {
					record(typed.NamePos, match)
				}
			}
		}
		return true
	})
	return violations, nil
}

func scanTypeScriptCompatibilityAliasFile(path, relative string, matches []compatibilityAliasMatch) ([]CompatibilityAliasViolation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relative, err)
	}
	content := string(raw)
	if strings.Contains(content, compatibilityAliasBoundaryMarker) {
		return nil, nil
	}

	seen := make(map[string]struct{})
	var violations []CompatibilityAliasViolation
	lines := strings.Split(content, "\n")
	for lineIndex, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		for _, match := range matches {
			column := strings.Index(line, match.term)
			if column < 0 || !hasCompatibilityAliasTermBoundary(line, column, len(match.term)) {
				continue
			}
			key := fmt.Sprintf("%s:%d:%d:%s", relative, lineIndex+1, column+1, match.term)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			violations = append(violations, CompatibilityAliasViolation{
				FilePath:   relative,
				Line:       lineIndex + 1,
				Column:     column + 1,
				Term:       match.term,
				ItemID:     match.itemID,
				PublicName: match.publicName,
			})
		}
	}
	return violations, nil
}

func unquoteGoString(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '`' && value[len(value)-1] == '`') {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return unquoted
			}
		}
	}
	return value
}

func containsCompatibilityAliasTerm(content, term string) bool {
	if term == "" {
		return false
	}
	start := 0
	for {
		index := strings.Index(content[start:], term)
		if index < 0 {
			return false
		}
		position := start + index
		if hasCompatibilityAliasTermBoundary(content, position, len(term)) {
			return true
		}
		start = position + 1
	}
}

func hasCompatibilityAliasTermBoundary(content string, start, length int) bool {
	if start > 0 {
		if previous := rune(content[start-1]); isCompatibilityAliasTermChar(previous) {
			return false
		}
	}
	end := start + length
	if end < len(content) {
		if next := rune(content[end]); isCompatibilityAliasTermChar(next) {
			return false
		}
	}
	return true
}

func isCompatibilityAliasTermChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}
