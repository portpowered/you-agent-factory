package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	pathpkg "path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TextRange is a half-open byte range in Span.Text. Adapters use it to mark
// machine and technical text before rule evaluation.
type TextRange struct {
	Start int
	End   int
}

// Suppression is a bounded, owner-approved exception for one rule. The
// comment form understood by ParseSuppressionDirective is documented here:
//
//	<!-- prosecheck:ignore B-SEMICOLON reason="..." owner="..." review="..." -->
//
// An adapter should replace the parsed range with the exact source span that
// the directive owns. A zero range is only valid for a parsed inline directive
// while Analyze derives the remainder of its containing structural span.
type Suppression struct {
	RuleID      RuleID
	StartOffset int
	EndOffset   int
	Reason      string
	Owner       string
	Review      string
}

// Span is one structurally classified natural-language block. It is the
// boundary between source adapters and pure rule evaluation.
type Span struct {
	SourcePath   string
	StartLine    int
	StartColumn  int
	Class        ContentClass
	Text         string
	Identity     string
	Protected    []TextRange
	Suppressions []Suppression
	positions    []sourcePosition
}

// sourcePosition is an absolute source location for one byte boundary in a
// span. Structured adapters use it when the authored representation encodes
// text (for example, JSON string escapes) instead of storing it literally.
type sourcePosition struct {
	line   int
	column int
}

// Finding is the stable, actionable result emitted by the analyzer.
type Finding struct {
	RuleID       RuleID       `json:"ruleId"`
	Severity     Severity     `json:"severity"`
	SourcePath   string       `json:"sourcePath"`
	StartLine    int          `json:"startLine"`
	StartColumn  int          `json:"startColumn"`
	EndLine      int          `json:"endLine"`
	EndColumn    int          `json:"endColumn"`
	ContentClass ContentClass `json:"contentClass"`
	Excerpt      string       `json:"excerpt"`
	Guidance     string       `json:"guidance"`
	Fingerprint  string       `json:"fingerprint"`
	Identity     string       `json:"identity,omitempty"`

	startOffset int
	endOffset   int
}

type candidate struct {
	ruleID   RuleID
	class    ContentClass
	start    int
	end      int
	excerpt  string
	guidance string
}

type directiveOccurrence struct {
	start       int
	end         int
	suppression Suppression
}

// Analyze applies all supported deterministic rules to explicitly classified
// spans. It copies no input state, performs no I/O, and returns one total-order
// sorted result for every input enumeration order.
func Analyze(spans []Span, policy Policy) []Finding {
	findings := make([]Finding, 0)
	for _, span := range spans {
		findings = append(findings, analyzeSpan(span, policy)...)
	}
	return SortFindings(findings)
}

// AnalyzeText is a small pure convenience entry point for callers that
// already have one natural-language span. Structural adapters should provide
// their own protected ranges instead of relying on lexical guesses.
func AnalyzeText(sourcePath string, line, column int, class ContentClass, text string, policy Policy) []Finding {
	return Analyze([]Span{{
		SourcePath:  sourcePath,
		StartLine:   line,
		StartColumn: column,
		Class:       class,
		Text:        text,
		Protected:   LexicalProtectedRanges(text),
	}}, policy)
}

func analyzeSpan(span Span, policy Policy) []Finding {
	if !utf8.ValidString(span.Text) {
		return []Finding{parseFinding(span, "source span is not valid UTF-8", 0, 1)}
	}
	if strings.TrimSpace(span.SourcePath) == "" {
		return []Finding{parseFinding(span, "source path is empty", 0, 1)}
	}
	if !policy.knownClass(span.Class) {
		return []Finding{parseFinding(span, fmt.Sprintf("unknown content class %q", span.Class), 0, 1)}
	}

	ranges, err := normalizedRanges(span.Protected, span.Text)
	if err != nil {
		return []Finding{parseFinding(span, err.Error(), 0, 1)}
	}
	masked := maskRanges(span.Text, ranges)
	directives, directiveFindings := parseInlineDirectives(span, policy)
	ranges = append(ranges, directiveRanges(directives)...)
	masked = maskRanges(span.Text, ranges)

	validSuppressions, suppressionFindings := validateSuppressions(span, policy, directives)
	findings := append(directiveFindings, suppressionFindings...)
	if span.Class == ContentClassTechnical || span.Class == ContentClassTerm {
		return SortFindings(findings)
	}

	candidates := analyzeSentences(span, policy, masked)
	candidates = append(candidates, analyzeParagraphs(span, policy, masked)...)
	candidates = append(candidates, analyzePunctuation(span, policy, masked)...)
	candidates = append(candidates, analyzeContractions(span, policy, masked)...)
	candidates = append(candidates, analyzeTerms(span, policy, masked)...)

	for _, item := range candidates {
		if suppressed(item, validSuppressions) {
			continue
		}
		findings = append(findings, findingFromCandidate(span, item))
	}
	return SortFindings(findings)
}

func analyzeSentences(span Span, policy Policy, masked string) []candidate {
	var ruleID RuleID
	limit := 0
	switch span.Class {
	case ContentClassProcedural:
		ruleID = RuleProceduralSentenceLength
		limit = 20
	case ContentClassDescriptive:
		ruleID = RuleDescriptiveSentenceLength
		limit = 25
	default:
		return nil
	}
	rule, ok := policy.rule(ruleID)
	if !ok || rule.Limit != limit {
		return nil
	}
	var findings []candidate
	for _, sentence := range sentenceRanges(masked) {
		if words := naturalWordCount(masked[sentence.Start:sentence.End]); words > limit {
			findings = append(findings, candidate{
				ruleID:   ruleID,
				class:    span.Class,
				start:    sentence.Start,
				end:      sentence.End,
				excerpt:  span.Text[sentence.Start:sentence.End],
				guidance: sentenceGuidance(ruleID, limit),
			})
		}
	}
	return findings
}

func analyzeParagraphs(span Span, policy Policy, masked string) []candidate {
	if span.Class != ContentClassDescriptive {
		return nil
	}
	if rule, ok := policy.rule(RuleDescriptiveParagraphLength); !ok || rule.Limit != 6 {
		return nil
	}
	var findings []candidate
	for _, paragraph := range paragraphRanges(masked) {
		paragraphText := masked[paragraph.Start:paragraph.End]
		sentences := sentenceRanges(paragraphText)
		if len(sentences) <= 6 {
			continue
		}
		findings = append(findings, candidate{
			ruleID:   RuleDescriptiveParagraphLength,
			class:    span.Class,
			start:    paragraph.Start,
			end:      paragraph.End,
			excerpt:  span.Text[paragraph.Start:paragraph.End],
			guidance: "Split the descriptive paragraph into no more than six sentences.",
		})
	}
	return findings
}

func analyzePunctuation(span Span, policy Policy, masked string) []candidate {
	if _, ok := policy.rule(RuleSemicolon); !ok {
		return nil
	}
	var findings []candidate
	for offset := 0; offset < len(masked); offset++ {
		if masked[offset] != ';' {
			continue
		}
		findings = append(findings, candidate{
			ruleID:   RuleSemicolon,
			class:    span.Class,
			start:    offset,
			end:      offset + 1,
			excerpt:  span.Text[offset : offset+1],
			guidance: "Replace the semicolon with a full stop or separate the two ideas.",
		})
	}
	return findings
}

func analyzeContractions(span Span, policy Policy, masked string) []candidate {
	if _, ok := policy.rule(RuleContraction); !ok {
		return nil
	}
	var findings []candidate
	for _, occurrence := range contractionOccurrences(masked) {
		findings = append(findings, candidate{
			ruleID:   RuleContraction,
			class:    span.Class,
			start:    occurrence.Start,
			end:      occurrence.End,
			excerpt:  span.Text[occurrence.Start:occurrence.End],
			guidance: "Use the complete form instead of the contraction.",
		})
	}
	return findings
}

type termMatch struct {
	start       int
	end         int
	ruleID      RuleID
	replacement string
}

func analyzeTerms(span Span, policy Policy, masked string) []candidate {
	_, publicOK := policy.rule(RulePublicTerm)
	_, caseOK := policy.rule(RuleTermCase)
	if !publicOK && !caseOK {
		return nil
	}
	matches := make([]termMatch, 0)
	lowerMasked := strings.ToLower(masked)
	for _, term := range policy.Terms {
		if publicOK {
			for _, alternative := range term.DiscouragedAlternatives {
				if alternative.Status != "prohibited" {
					continue
				}
				for _, start := range foldedOccurrences(lowerMasked, strings.ToLower(alternative.Text)) {
					end := start + len(alternative.Text)
					if isTermBoundary(masked, start, end) {
						matches = append(matches, termMatch{
							start: start, end: end, ruleID: RulePublicTerm,
							replacement: alternative.Replacement,
						})
					}
				}
			}
		}
		if !caseOK || term.Category != "product-term" {
			continue
		}
		for _, spelling := range termSpellings(term) {
			for _, start := range foldedOccurrences(lowerMasked, strings.ToLower(spelling)) {
				end := start + len(spelling)
				if !isTermBoundary(masked, start, end) || masked[start:end] == spelling {
					continue
				}
				matches = append(matches, termMatch{
					start: start, end: end, ruleID: RuleTermCase,
					replacement: spelling,
				})
			}
		}
	}

	slices.SortStableFunc(matches, func(left, right termMatch) int {
		if left.start != right.start {
			return left.start - right.start
		}
		if left.end != right.end {
			return right.end - left.end
		}
		return strings.Compare(string(left.ruleID), string(right.ruleID))
	})
	selected := make([]termMatch, 0, len(matches))
	for _, match := range matches {
		if len(selected) > 0 && match.start < selected[len(selected)-1].end {
			continue
		}
		selected = append(selected, match)
	}

	findings := make([]candidate, 0, len(selected))
	for _, match := range selected {
		guidance := fmt.Sprintf("Use the approved customer term `%s`.", match.replacement)
		if match.ruleID == RuleTermCase {
			guidance = fmt.Sprintf("Use the canonical product-term spelling and capitalization `%s`.", match.replacement)
		}
		findings = append(findings, candidate{
			ruleID: match.ruleID, class: span.Class,
			start: match.start, end: match.end,
			excerpt: span.Text[match.start:match.end], guidance: guidance,
		})
	}
	return findings
}

func termSpellings(term Term) []string {
	spellings := []string{term.Canonical}
	for _, form := range []string{"singular", "plural"} {
		if value := term.ApprovedForms[form]; value != "" && value != term.Canonical {
			spellings = append(spellings, value)
		}
	}
	slices.SortStableFunc(spellings, func(left, right string) int {
		if len(left) != len(right) {
			return len(right) - len(left)
		}
		return strings.Compare(strings.ToLower(left), strings.ToLower(right))
	})
	return spellings
}

func sentenceGuidance(ruleID RuleID, limit int) string {
	if ruleID == RuleProceduralSentenceLength {
		return fmt.Sprintf("Shorten the procedural sentence to %d natural-language words or fewer.", limit)
	}
	return fmt.Sprintf("Shorten the descriptive sentence to %d natural-language words or fewer.", limit)
}

func findingFromCandidate(span Span, item candidate) Finding {
	path := normalizeSourcePath(span.SourcePath)
	startLine, startColumn := spanPosition(span, item.start)
	endLine, endColumn := spanPosition(span, item.end)
	excerpt := boundedExcerpt(item.excerpt)
	finding := Finding{
		RuleID:       item.ruleID,
		Severity:     SeverityBlocking,
		SourcePath:   path,
		StartLine:    startLine,
		StartColumn:  startColumn,
		EndLine:      endLine,
		EndColumn:    endColumn,
		ContentClass: item.class,
		Excerpt:      excerpt,
		Guidance:     item.guidance,
		Identity:     strings.TrimSpace(span.Identity),
		startOffset:  item.start,
		endOffset:    item.end,
	}
	finding.Fingerprint = fingerprint(finding)
	return finding
}

func parseFinding(span Span, message string, start, end int) Finding {
	if end <= start {
		end = start + 1
	}
	if end > len(span.Text) {
		end = len(span.Text)
	}
	if end <= start {
		end = start
	}
	item := candidate{
		ruleID: RuleParse, class: span.Class, start: start, end: end,
		excerpt: message, guidance: "Fix the source so the analyzer can safely extract every prose span.",
	}
	return findingFromCandidate(span, item)
}

func fingerprint(finding Finding) string {
	parts := []string{
		string(finding.RuleID), string(finding.Severity), finding.SourcePath,
		finding.Identity, string(finding.ContentClass),
		fmt.Sprint(finding.StartLine), fmt.Sprint(finding.StartColumn),
		normalizeFingerprintText(finding.Excerpt),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func normalizeSourcePath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" {
		return "<unknown>"
	}
	return pathpkg.Clean(value)
}

func normalizeFingerprintText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Join(strings.Fields(value), " ")
}

func boundedExcerpt(value string) string {
	value = normalizeFingerprintText(value)
	const maxRunes = 160
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes-1]) + "…"
}

func spanPosition(span Span, offset int) (int, int) {
	if len(span.positions) > 0 {
		if offset < 0 {
			offset = 0
		}
		if offset >= len(span.positions) {
			offset = len(span.positions) - 1
		}
		position := span.positions[offset]
		if position.line > 0 && position.column > 0 {
			return position.line, position.column
		}
	}
	line := span.StartLine
	column := span.StartColumn
	if line < 1 {
		line = 1
	}
	if column < 1 {
		column = 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(span.Text) {
		offset = len(span.Text)
	}
	for index := 0; index < offset; {
		runeValue, size := utf8.DecodeRuneInString(span.Text[index:])
		if runeValue == '\r' {
			line++
			column = 1
			if index+size < offset && span.Text[index+size] == '\n' {
				index += size
				size = 1
			}
		} else if runeValue == '\n' {
			if index == 0 || span.Text[index-1] != '\r' {
				line++
			}
			column = 1
		} else {
			column++
		}
		index += size
	}
	return line, column
}

func normalizedRanges(ranges []TextRange, text string) ([]TextRange, error) {
	copyRanges := append([]TextRange(nil), ranges...)
	slices.SortStableFunc(copyRanges, func(left, right TextRange) int {
		if left.Start != right.Start {
			return left.Start - right.Start
		}
		return left.End - right.End
	})
	previousEnd := 0
	for _, item := range copyRanges {
		if item.Start < 0 || item.End <= item.Start || item.End > len(text) {
			return nil, fmt.Errorf("protected range [%d,%d) is outside the source span", item.Start, item.End)
		}
		if !isUTF8Boundary(text, item.Start) || !isUTF8Boundary(text, item.End) {
			return nil, fmt.Errorf("protected range [%d,%d) does not align to UTF-8 boundaries", item.Start, item.End)
		}
		if item.Start < previousEnd {
			return nil, fmt.Errorf("protected ranges overlap at byte %d", item.Start)
		}
		previousEnd = item.End
	}
	return copyRanges, nil
}

func isUTF8Boundary(text string, offset int) bool {
	return offset == 0 || offset == len(text) || utf8.RuneStart(text[offset])
}

func maskRanges(text string, ranges []TextRange) string {
	masked := []byte(text)
	for _, item := range ranges {
		for index := item.Start; index < item.End; index++ {
			switch masked[index] {
			case '\n', '\r', '\t':
			default:
				masked[index] = ' '
			}
		}
	}
	return string(masked)
}

type byteRange struct {
	Start int
	End   int
}

func sentenceRanges(text string) []byteRange {
	var ranges []byteRange
	start := skipWhitespace(text, 0)
	for start < len(text) {
		end := len(text)
		for index := start; index < len(text); {
			runeValue, size := utf8.DecodeRuneInString(text[index:])
			if runeValue == '.' || runeValue == '!' || runeValue == '?' {
				end = index + size
				for end < len(text) {
					closing, closingSize := utf8.DecodeRuneInString(text[end:])
					if !isClosingPunctuation(closing) {
						break
					}
					end += closingSize
				}
				break
			}
			index += size
		}
		if end > start {
			ranges = append(ranges, byteRange{Start: start, End: end})
		}
		start = skipWhitespace(text, end)
	}
	return ranges
}

func paragraphRanges(text string) []byteRange {
	var ranges []byteRange
	start := skipWhitespace(text, 0)
	for start < len(text) {
		end := len(text)
		for index := start; index < len(text); {
			if text[index] == '\n' {
				next := index + 1
				for next < len(text) && (text[next] == ' ' || text[next] == '\t' || text[next] == '\r') {
					next++
				}
				if next < len(text) && text[next] == '\n' {
					end = index
					break
				}
			}
			_, size := utf8.DecodeRuneInString(text[index:])
			index += size
		}
		end = trimTrailingWhitespace(text, start, end)
		if end > start {
			ranges = append(ranges, byteRange{Start: start, End: end})
		}
		start = skipWhitespace(text, end)
	}
	return ranges
}

func naturalWordCount(text string) int {
	count := 0
	inWord := false
	for index := 0; index < len(text); {
		runeValue, size := utf8.DecodeRuneInString(text[index:])
		if (runeValue == '\'' || runeValue == '’') && inWord && index+size < len(text) {
			next, _ := utf8.DecodeRuneInString(text[index+size:])
			if unicode.IsLetter(next) || unicode.IsDigit(next) {
				index += size
				continue
			}
		}
		word := unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue)
		if word && !inWord {
			count++
		}
		inWord = word
		index += size
	}
	return count
}

func skipWhitespace(text string, start int) int {
	for start < len(text) {
		runeValue, size := utf8.DecodeRuneInString(text[start:])
		if !unicode.IsSpace(runeValue) {
			break
		}
		start += size
	}
	return start
}

func trimTrailingWhitespace(text string, start, end int) int {
	for end > start {
		runeValue, size := utf8.DecodeLastRuneInString(text[start:end])
		if !unicode.IsSpace(runeValue) {
			break
		}
		end -= size
	}
	return end
}

func isClosingPunctuation(value rune) bool {
	switch value {
	case '\'', '"', ')', ']', '}', '»', '”', '’':
		return true
	default:
		return false
	}
}

type occurrence struct {
	Start int
	End   int
}

func contractionOccurrences(text string) []occurrence {
	var occurrences []occurrence
	for index := 0; index < len(text); {
		runeValue, size := utf8.DecodeRuneInString(text[index:])
		if !unicode.IsLetter(runeValue) {
			index += size
			continue
		}
		start := index
		index += size
		for index < len(text) {
			value, valueSize := utf8.DecodeRuneInString(text[index:])
			if unicode.IsLetter(value) || value == '\'' || value == '’' {
				index += valueSize
				continue
			}
			break
		}
		word := strings.ToLower(strings.NewReplacer("’", "'").Replace(text[start:index]))
		if strings.Contains(word, "'") && isContraction(word) {
			occurrences = append(occurrences, occurrence{Start: start, End: index})
		}
	}
	return occurrences
}

func isContraction(word string) bool {
	switch word {
	case "ain't", "aren't", "can't", "couldn't", "didn't", "doesn't", "don't", "hadn't", "hasn't", "haven't", "he'd", "he'll", "he's", "i'd", "i'll", "i'm", "i've", "isn't", "let's", "mightn't", "mustn't", "shan't", "she'd", "she'll", "she's", "shouldn't", "that's", "there'd", "there'll", "there's", "they'd", "they'll", "they're", "they've", "wasn't", "we'd", "we'll", "we're", "we've", "weren't", "what'll", "what's", "when's", "where'd", "where'll", "where's", "who'd", "who'll", "who's", "won't", "wouldn't", "you'd", "you'll", "you're", "you've":
		return true
	default:
		return false
	}
}

func foldedOccurrences(text, needle string) []int {
	if needle == "" {
		return nil
	}
	var occurrences []int
	for start := 0; start <= len(text)-len(needle); {
		index := strings.Index(text[start:], needle)
		if index < 0 {
			break
		}
		index += start
		occurrences = append(occurrences, index)
		start = index + 1
	}
	return occurrences
}

func isTermBoundary(text string, start, end int) bool {
	if start > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:start])
		if isIdentifierRune(previous) {
			return false
		}
	}
	if end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[end:])
		if isIdentifierRune(next) {
			return false
		}
	}
	return true
}

func isIdentifierRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_'
}

// LexicalProtectedRanges recognizes only unambiguous inline technical forms.
// Structural adapters remain responsible for fenced blocks, JSON/YAML nodes,
// command lines, and source-owned literals.
func LexicalProtectedRanges(text string) []TextRange {
	var ranges []TextRange
	for index := 0; index < len(text); {
		if text[index] != '`' {
			index++
			continue
		}
		end := strings.IndexByte(text[index+1:], '`')
		if end < 0 {
			break
		}
		end += index + 2
		ranges = append(ranges, TextRange{Start: index, End: end})
		index = end
	}
	urlPattern := regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>()]+`)
	for _, match := range urlPattern.FindAllStringIndex(text, -1) {
		end := match[1]
		for end > match[0] && strings.ContainsRune(".,;:!?", rune(text[end-1])) {
			end--
		}
		if end > match[0] {
			ranges = append(ranges, TextRange{Start: match[0], End: end})
		}
	}
	slices.SortStableFunc(ranges, func(left, right TextRange) int {
		if left.Start != right.Start {
			return left.Start - right.Start
		}
		return left.End - right.End
	})
	merged := make([]TextRange, 0, len(ranges))
	for _, item := range ranges {
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
