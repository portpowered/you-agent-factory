package main

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var (
	cliFlagPattern        = regexp.MustCompile(`--[A-Za-z0-9][A-Za-z0-9-]*`)
	cliShortFlagPattern   = regexp.MustCompile(`(?:^|[\s([<])-[A-Za-z][A-Za-z0-9-]*`)
	cliPlaceholderPattern = regexp.MustCompile(`<[^>\r\n]{1,80}>`)
	cliPathPattern        = regexp.MustCompile(`(?:\./|\.\./|~/|/|[A-Za-z]:[\\/])[^\s,;!?)]*`)
	cliIdentifierPattern  = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*(?:[._][A-Za-z0-9_-]+)+\b`)
	cliCamelCasePattern   = regexp.MustCompile(`\b[A-Za-z]+[a-z][A-Z][A-Za-z0-9]*\b`)
	cliSnakePattern       = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*(?:_[A-Za-z0-9]+)+\b`)
	cliErrorCodePattern   = regexp.MustCompile(`\b(?:[A-Z][A-Z0-9]*[-_]){1,}[A-Z0-9]+\b`)
	cliProviderPattern    = regexp.MustCompile(`(?i)\b(?:codex|cursor(?:-acp)?|openai|anthropic|ollama|gpt(?:-[A-Za-z0-9.]+)?)\b`)
	cliStructuredPattern  = regexp.MustCompile(`^[- ]*[A-Za-z0-9_.-]+\s*:\s*(?:[\[{\"']|true\b|false\b|null\b|-?\d)`)
	cliSchemePattern      = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>()]*`)
	cliQuotedTechnical    = regexp.MustCompile(`(?i)"(?:error|fatal|usage|stdout|stderr):[^"\r\n]*"`)
)

func cliProtectedRanges(text string, literals []string) []TextRange {
	ranges := append([]TextRange(nil), LexicalProtectedRanges(text)...)
	ranges = append(ranges, cliRegexRanges(text, cliFlagPattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliShortFlagPattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliPlaceholderPattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliPathPattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliIdentifierPattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliCamelCasePattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliSnakePattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliErrorCodePattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliProviderPattern)...)
	ranges = append(ranges, cliRegexRanges(text, cliSchemePattern)...)
	ranges = append(ranges, markdownRouteRanges(text)...)
	ranges = append(ranges, markdownCommandLineRanges(text)...)
	ranges = append(ranges, cliCommandLiteralLineRanges(text, literals)...)
	ranges = append(ranges, cliStructuredLineRanges(text)...)
	ranges = append(ranges, cliQuotedOutputRanges(text)...)
	ranges = append(ranges, cliRegexRanges(text, cliQuotedTechnical)...)
	return mergeCLIRanges(ranges)
}

func cliRegexRanges(text string, pattern *regexp.Regexp) []TextRange {
	matches := pattern.FindAllStringIndex(text, -1)
	ranges := make([]TextRange, 0, len(matches))
	for _, match := range matches {
		ranges = append(ranges, TextRange{Start: match[0], End: match[1]})
	}
	return ranges
}

func cliLiteralRanges(text, literal string) []TextRange {
	if literal == "" || len(literal) > len(text) {
		return nil
	}
	var ranges []TextRange
	for search := 0; search <= len(text)-len(literal); {
		index := strings.Index(text[search:], literal)
		if index < 0 {
			break
		}
		index += search
		end := index + len(literal)
		if cliLiteralBoundary(text, index, end, literal) {
			ranges = append(ranges, TextRange{Start: index, End: end})
		}
		search = index + 1
	}
	return ranges
}

func cliLiteralBoundary(text string, start, end int, literal string) bool {
	if len(literal) == 0 {
		return false
	}
	if isIdentifierRune(rune(literal[0])) && start > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:start])
		if isIdentifierRune(previous) {
			return false
		}
	}
	if isIdentifierRune(rune(literal[len(literal)-1])) && end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[end:])
		if isIdentifierRune(next) {
			return false
		}
	}
	return true
}

func cliStructuredLineRanges(text string) []TextRange {
	var ranges []TextRange
	for start := 0; start < len(text); {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += start
		}
		line := strings.TrimSpace(strings.TrimSuffix(text[start:end], "\r"))
		if cliLooksLikeStructuredLine(line) {
			ranges = append(ranges, TextRange{Start: start, End: end})
		}
		if end == len(text) {
			break
		}
		start = end + 1
	}
	return ranges
}

func cliLooksLikeStructuredLine(line string) bool {
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "}") || strings.HasPrefix(line, "[") || strings.HasPrefix(line, "]") {
		return true
	}
	if strings.Contains(line, `":`) || strings.Contains(line, "':") {
		return true
	}
	return cliStructuredPattern.MatchString(line)
}

func cliQuotedOutputRanges(text string) []TextRange {
	pattern := regexp.MustCompile(`(?im)^\s*(?:error|fatal|usage|exit status|stdout|stderr)\s*:\s*\S[^\r\n]*`)
	return cliRegexRanges(text, pattern)
}

func cliCommandLiteralLineRanges(text string, literals []string) []TextRange {
	var ranges []TextRange
	for lineStart := 0; lineStart < len(text); {
		lineEnd := strings.IndexByte(text[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += lineStart
		}
		line := strings.TrimSuffix(text[lineStart:lineEnd], "\r")
		leading := len(line) - len(strings.TrimLeft(line, " \t"))
		trimmed := line[leading:]
		for _, literal := range literals {
			if !strings.Contains(literal, " ") || !strings.HasPrefix(literal, "you ") || !strings.HasPrefix(trimmed, literal) {
				continue
			}
			if cliLiteralBoundary(trimmed, 0, len(literal), literal) {
				ranges = append(ranges, TextRange{Start: lineStart + leading, End: lineEnd})
				break
			}
		}
		if lineEnd == len(text) {
			break
		}
		lineStart = lineEnd + 1
	}
	return ranges
}

func mergeCLIRanges(ranges []TextRange) []TextRange {
	ordered := append([]TextRange(nil), ranges...)
	slices.SortStableFunc(ordered, func(left, right TextRange) int {
		if left.Start != right.Start {
			return left.Start - right.Start
		}
		return left.End - right.End
	})
	merged := make([]TextRange, 0, len(ordered))
	for _, item := range ordered {
		if item.Start < 0 || item.End <= item.Start {
			continue
		}
		if len(merged) == 0 || item.Start > merged[len(merged)-1].End {
			merged = append(merged, item)
			continue
		}
		if item.End > merged[len(merged)-1].End {
			merged[len(merged)-1].End = item.End
		}
	}
	return merged
}
