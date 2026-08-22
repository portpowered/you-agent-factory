package workflowvalidation

import (
	"regexp"
	"strings"
)

var (
	tsUnsupportedPatterns = []struct {
		pattern *regexp.Regexp
		message string
	}{
		{regexp.MustCompile(`\bimport\s`), "ES module import is not supported in MVP TypeScript workflows"},
		{regexp.MustCompile(`\bexport\s`), "ES module export is not supported in MVP TypeScript workflows"},
		{regexp.MustCompile(`\benum\s+[A-Za-z_$]`), "TypeScript enum declarations are not supported in the MVP TypeScript loader"},
		{regexp.MustCompile(`\bnamespace\s+[A-Za-z_$]`), "TypeScript namespace declarations are not supported in the MVP TypeScript loader"},
		{regexp.MustCompile(`\bdeclare\s+`), "TypeScript declare blocks are not supported in the MVP TypeScript loader"},
		{regexp.MustCompile(`\bmodule\s+["']`), "ambient module declarations are not supported in the MVP TypeScript loader"},
	}

	tsInterfaceBlock = regexp.MustCompile(`(?ms)^\s*interface\s+[A-Za-z_$][\w$]*\s*\{.*?\}\s*`)
	tsTypeAlias      = regexp.MustCompile(`(?ms)^\s*type\s+[A-Za-z_$][\w$]*\s*=.*?;\s*`)
	tsAsAssertion    = regexp.MustCompile(`\s+as\s+[A-Za-z_$][\w$<>[\],\s|&?]*`)
	tsTypeAnnotation = regexp.MustCompile(`:\s*[A-Za-z_$][\w$<>[\],\s|&?]*`)
)

func stripTypeScript(source string) (string, []int, []Issue) {
	for _, pattern := range tsUnsupportedPatterns {
		if loc := pattern.pattern.FindStringIndex(source); loc != nil {
			line := strings.Count(source[:loc[0]], "\n") + 1
			return "", nil, []Issue{{
				Code:    CodeUnsupportedLoader,
				Message: pattern.message,
				Line:    line,
			}}
		}
	}

	lines := strings.Split(source, "\n")
	executableLines := make([]string, 0, len(lines))
	lineMap := make([]int, 0, len(lines))

	remaining := source
	for len(remaining) > 0 {
		if loc := tsInterfaceBlock.FindStringIndex(remaining); loc != nil && loc[0] == 0 {
			remaining = remaining[loc[1]:]
			continue
		}
		if loc := tsTypeAlias.FindStringIndex(remaining); loc != nil && loc[0] == 0 {
			remaining = remaining[loc[1]:]
			continue
		}
		break
	}

	bodyLines := strings.Split(strings.TrimLeft(remaining, "\n"), "\n")
	bodyOffset := len(lines) - len(bodyLines)
	if bodyOffset < 0 {
		bodyOffset = 0
	}

	for i, line := range bodyLines {
		stripped := stripTypeScriptLine(line)
		if strings.TrimSpace(stripped) == "" && strings.TrimSpace(line) == "" {
			continue
		}
		executableLines = append(executableLines, stripped)
		lineMap = append(lineMap, bodyOffset+i+1)
	}

	return strings.Join(executableLines, "\n"), lineMap, nil
}

func stripTypeScriptLine(line string) string {
	line = tsAsAssertion.ReplaceAllString(line, "")
	line = tsTypeAnnotation.ReplaceAllString(line, "")
	return line
}
