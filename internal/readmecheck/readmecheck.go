package readmecheck

import (
	"regexp"
	"slices"
	"strings"
)

// RequiredSectionTitles are top-level README headings that must remain present.
var RequiredSectionTitles = []string{
	"Installation",
	"Quick start",
	"Features",
	"Comparison",
	"References",
	"License",
}

var (
	headingPattern     = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
	markdownRefPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
	htmlImgSrcPattern  = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)
)

// TopLevelSections returns ## heading titles in document order.
func TopLevelSections(content string) []string {
	matches := headingPattern.FindAllStringSubmatch(content, -1)
	sections := make([]string, 0, len(matches))
	for _, match := range matches {
		sections = append(sections, strings.TrimSpace(match[1]))
	}
	return sections
}

// MissingRequiredSections reports required section titles that are absent.
func MissingRequiredSections(content string) []string {
	present := TopLevelSections(content)
	var missing []string
	for _, required := range RequiredSectionTitles {
		if !slices.Contains(present, required) {
			missing = append(missing, required)
		}
	}
	return missing
}

// LocalReferencePaths returns repo-relative paths referenced by the README.
func LocalReferencePaths(content string) []string {
	seen := map[string]struct{}{}
	var paths []string

	addPath := func(raw string) {
		path := normalizeLocalPath(raw)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	for _, match := range markdownRefPattern.FindAllStringSubmatch(content, -1) {
		addPath(match[1])
	}
	for _, match := range htmlImgSrcPattern.FindAllStringSubmatch(content, -1) {
		addPath(match[1])
	}

	slices.Sort(paths)
	return paths
}

func normalizeLocalPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "#") {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return ""
	}
	if strings.HasPrefix(trimmed, "mailto:") {
		return ""
	}

	withoutFragment, _, _ := strings.Cut(trimmed, "#")
	withoutQuery, _, _ := strings.Cut(withoutFragment, "?")
	path := strings.TrimPrefix(withoutQuery, "./")
	path = strings.TrimPrefix(path, "/")
	return path
}
