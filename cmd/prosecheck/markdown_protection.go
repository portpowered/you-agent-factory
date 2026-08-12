package main

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

func markdownProtectedRanges(text string) []TextRange {
	ranges := append([]TextRange(nil), LexicalProtectedRanges(text)...)
	ranges = append(ranges, markdownLinkRanges(text)...)
	ranges = append(ranges, markdownHTMLRanges(text)...)
	ranges = append(ranges, markdownRouteRanges(text)...)
	ranges = append(ranges, markdownCommandLineRanges(text)...)
	ranges = append(ranges, markdownStructuredLineRanges(text)...)
	ranges = append(ranges, markdownQuotedOutputRanges(text)...)
	ranges = append(ranges, markdownTechnicalTokenRanges(text)...)
	return mergeMarkdownRanges(ranges)
}

func markdownLinkRanges(text string) []TextRange {
	var ranges []TextRange
	for search := 0; search < len(text); {
		start := strings.Index(text[search:], "](")
		if start < 0 {
			break
		}
		start += search + 1
		end := start + 1
		depth := 1
		for end < len(text) && depth > 0 {
			if text[end] == '\\' {
				end += 2
				continue
			}
			switch text[end] {
			case '(':
				depth++
			case ')':
				depth--
			}
			end++
		}
		if depth == 0 {
			ranges = append(ranges, TextRange{Start: start, End: end})
		}
		search = end
	}
	return ranges
}

func markdownHTMLRanges(text string) []TextRange {
	var ranges []TextRange
	for search := 0; search < len(text); {
		start := strings.IndexByte(text[search:], '<')
		if start < 0 {
			break
		}
		start += search
		end := strings.IndexByte(text[start+1:], '>')
		if end < 0 {
			break
		}
		end += start + 2
		raw := text[start:end]
		if !strings.HasPrefix(raw, "<!--") || !strings.Contains(strings.ToLower(raw), "prosecheck:ignore") {
			ranges = append(ranges, TextRange{Start: start, End: end})
		}
		search = end
	}
	return ranges
}

func markdownRouteRanges(text string) []TextRange {
	pattern := regexp.MustCompile(`(?i)\b(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+[^\s]+`)
	var ranges []TextRange
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		end := match[1]
		for end > match[0] && strings.ContainsRune(".,;:!?)]}", rune(text[end-1])) {
			end--
		}
		ranges = append(ranges, TextRange{Start: match[0], End: end})
	}
	return ranges
}

func markdownCommandLineRanges(text string) []TextRange {
	pattern := regexp.MustCompile(`(?im)^\s*(?:\$\s*)?(?:(?:you\s+(?:run|server|docs|factory|submit|session|work|init|serve|config|help|version)\b)|(?:git\s+(?:add|branch|checkout|commit|diff|log|pull|push|status)\b)|(?:go\s+(?:build|fmt|run|test|vet)\b)|(?:make\s+\S+)|(?:npm|pnpm|bun)\s+(?:install|run|test|exec|build)\b|(?:curl|wget|docker|kubectl|gh)\b)[^\r\n]*`)
	var ranges []TextRange
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		start := match[0]
		for start < match[1] && (text[start] == ' ' || text[start] == '\t') {
			start++
		}
		ranges = append(ranges, TextRange{Start: start, End: match[1]})
	}
	return ranges
}

func markdownStructuredLineRanges(text string) []TextRange {
	var ranges []TextRange
	start := 0
	for start < len(text) {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += start
		}
		line := strings.TrimSpace(strings.TrimSuffix(text[start:end], "\r"))
		jsonArray := strings.HasPrefix(line, "[") && (strings.HasSuffix(line, "]") && strings.ContainsAny(line, "{}\"") || strings.Contains(line, `":`))
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "}") || jsonArray || strings.HasPrefix(line, "]") || strings.Contains(line, `":`) {
			ranges = append(ranges, TextRange{Start: start, End: end})
		}
		if end == len(text) {
			break
		}
		start = end + 1
	}
	return ranges
}

func markdownQuotedOutputRanges(text string) []TextRange {
	pattern := regexp.MustCompile(`(?im)^\s*(?:\$\s*|(?:error|fatal|usage|exit status|stdout|stderr)\s*:)\S[^\r\n]*`)
	var ranges []TextRange
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		ranges = append(ranges, TextRange{Start: match[0], End: match[1]})
	}
	return ranges
}

func markdownTechnicalTokenRanges(text string) []TextRange {
	var ranges []TextRange
	for start := 0; start < len(text); {
		for start < len(text) && unicode.IsSpace(rune(text[start])) {
			start++
		}
		if start >= len(text) {
			break
		}
		end := start
		for end < len(text) && !unicode.IsSpace(rune(text[end])) {
			end++
		}
		if strings.Contains(text[start:end], "](") {
			start = end
			continue
		}
		trimmedStart, trimmedEnd := trimTechnicalPunctuation(text, start, end)
		if trimmedStart < trimmedEnd && (isMarkdownTechnicalToken(text[trimmedStart:trimmedEnd]) || isMarkdownMachineValue(text, trimmedStart, trimmedEnd)) {
			ranges = append(ranges, TextRange{Start: trimmedStart, End: trimmedEnd})
		}
		start = end
	}
	return ranges
}

func isMarkdownMachineValue(text string, start, end int) bool {
	context := start - 1
	for context >= 0 && (text[context] == ' ' || text[context] == '\t') {
		context--
	}
	if context < 0 || (text[context] != '=' && text[context] != ':') {
		return false
	}
	value := text[start:end]
	if value == "true" || value == "false" || value == "null" {
		return true
	}
	for _, runeValue := range value {
		if !unicode.IsDigit(runeValue) && runeValue != '.' && runeValue != '-' {
			return false
		}
	}
	return value != ""
}

func trimTechnicalPunctuation(text string, start, end int) (int, int) {
	for start < end && strings.ContainsRune("\"'([{<", rune(text[start])) {
		start++
	}
	for end > start && strings.ContainsRune("\"'.,;:!?)]}>", rune(text[end-1])) {
		end--
	}
	return start, end
}

func isMarkdownTechnicalToken(value string) bool {
	if value == "" || strings.Contains(value, "https://") || strings.Contains(value, "http://") {
		return false
	}
	if strings.HasPrefix(value, "--") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~\\") {
		return true
	}
	if strings.HasPrefix(value, "B-") && strings.Contains(value, "-") {
		return true
	}
	if strings.ContainsAny(value, "/\\") {
		return true
	}
	upper, lower := 0, 0
	mixedCase := false
	for index, value := range value {
		switch {
		case unicode.IsUpper(value):
			upper++
			if index > 0 {
				mixedCase = true
			}
		case unicode.IsLower(value):
			lower++
		}
	}
	if strings.Contains(value, "_") {
		return true
	}
	return mixedCase && upper > 0 && lower > 0
}

func mergeMarkdownRanges(ranges []TextRange) []TextRange {
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
