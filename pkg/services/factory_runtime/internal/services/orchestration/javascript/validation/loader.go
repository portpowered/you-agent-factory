package workflowvalidation

import (
	"path/filepath"
	"strings"
)

const (
	FormatJavaScript = "javascript"
	FormatTypeScript = "typescript"
)

// LoadRequest carries file-backed workflow source to load for validation.
type LoadRequest struct {
	SourceRef    string
	Content      string
	FactoryRoot  string
	BundleReader SourceReader
}

// LoadedSource is the resolved workflow source metadata for validation or preview.
type LoadedSource struct {
	SourceRef        string
	SourceHash       string
	Format           string
	AuthoredSource   string
	ExecutableSource string
	remapLine        func(int) int
}

// RemapLine maps executable-source line numbers back to customer-authored lines when practical.
func (s LoadedSource) RemapLine(line int) int {
	if line <= 0 || s.remapLine == nil {
		return line
	}
	return s.remapLine(line)
}

// Load resolves workflow source format, computes source hash, and prepares executable JavaScript.
func Load(req LoadRequest) (LoadedSource, []Issue) {
	sourceRef := strings.TrimSpace(req.SourceRef)
	content := req.Content
	loaded := LoadedSource{
		SourceRef:      sourceRef,
		SourceHash:     SourceHash([]byte(content)),
		AuthoredSource: content,
		remapLine:      func(line int) int { return line },
	}

	format, issue := resolveSourceFormat(sourceRef)
	if issue != nil {
		return loaded, []Issue{*issue}
	}
	loaded.Format = format

	switch format {
	case FormatJavaScript:
		executable := content
		if strings.TrimSpace(req.FactoryRoot) != "" && req.BundleReader != nil && ContainsFactoryRelativeImports(content) {
			bundled, bundleIssues := BundleFactoryRelativeImports(sourceRef, content, req.BundleReader)
			if len(bundleIssues) > 0 {
				return loaded, bundleIssues
			}
			executable = bundled
		}
		loaded.ExecutableSource = executable
	case FormatTypeScript:
		executable, lineMap, stripIssues := stripTypeScript(content)
		if len(stripIssues) > 0 {
			return loaded, stripIssues
		}
		loaded.ExecutableSource = executable
		loaded.remapLine = lineMapRemapper(lineMap)
	default:
		return loaded, []Issue{{
			Code:    CodeUnsupportedLoader,
			Message: "workflow source format " + format + " is not supported in MVP workflows",
			Path:    sourceRef,
		}}
	}

	return loaded, nil
}

func resolveSourceFormat(sourceRef string) (string, *Issue) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(sourceRef)))
	switch ext {
	case ".js", ".mjs", ".cjs", ".workflow.js":
		return FormatJavaScript, nil
	case ".ts", ".mts", ".workflow.ts":
		return FormatTypeScript, nil
	case ".tsx", ".jsx":
		return "", &Issue{
			Code:    CodeUnsupportedLoader,
			Message: ext + " workflow sources are not supported in the MVP TypeScript loader; use .js or supported .ts syntax",
			Path:    sourceRef,
		}
	default:
		return FormatJavaScript, nil
	}
}

func lineMapRemapper(lineMap []int) func(int) int {
	if len(lineMap) == 0 {
		return func(line int) int { return line }
	}
	return func(line int) int {
		if line <= 0 || line > len(lineMap) {
			return line
		}
		if mapped := lineMap[line-1]; mapped > 0 {
			return mapped
		}
		return line
	}
}

func remapIssues(issues []Issue, remap func(int) int) []Issue {
	if remap == nil {
		return issues
	}
	out := make([]Issue, len(issues))
	for i, issue := range issues {
		out[i] = issue
		if issue.Line > 0 {
			out[i].Line = remap(issue.Line)
		}
	}
	return out
}
