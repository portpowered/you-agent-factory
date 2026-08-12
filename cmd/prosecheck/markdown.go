package main

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ExtractMarkdownSpans extracts the natural-language spans that can be safely
// classified from one Markdown document. It keeps source line boundaries and
// source columns so the pure analyzer can report findings against authored
// Markdown rather than against a rendered copy.
func ExtractMarkdownSpans(sourcePath string, data []byte) ([]Span, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("Markdown source is not valid UTF-8")
	}
	parser := newMarkdownParser(sourcePath, string(data))
	if err := validateMarkdownInlineCode(parser.lines); err != nil {
		return nil, err
	}
	if err := parser.parse(); err != nil {
		return nil, err
	}
	return parser.spans, nil
}

// AnalyzeMarkdown is the pure adapter-plus-analyzer entry point used by
// callers that already loaded one Markdown document.
func AnalyzeMarkdown(sourcePath string, data []byte, policy Policy) []Finding {
	spans, err := ExtractMarkdownSpans(sourcePath, data)
	if err != nil {
		return []Finding{markdownParseFinding(sourcePath, data, err.Error())}
	}
	return Analyze(spans, policy)
}

func markdownSurfaceSelectors(sourcePath string) []string {
	surfaces := []string{surfaceCustomerDocumentation}
	path := normalizeSourcePath(sourcePath)
	if strings.Contains(path, "/reference/") {
		surfaces = append(surfaces, surfaceCLIReferenceDocumentation)
	}
	return surfaces
}

type markdownLine struct {
	number int
	start  int
	end    int
	next   int
	text   string
}

type markdownParser struct {
	sourcePath string
	source     string
	lines      []markdownLine
	spans      []Span

	pendingClass        ContentClass
	hasPendingClass     bool
	pendingSuppressions []Suppression
}

func newMarkdownParser(sourcePath, source string) *markdownParser {
	return &markdownParser{
		sourcePath: sourcePath,
		source:     source,
		lines:      splitMarkdownLines(source),
	}
}

func splitMarkdownLines(source string) []markdownLine {
	if source == "" {
		return nil
	}
	lines := make([]markdownLine, 0, strings.Count(source, "\n")+1)
	start := 0
	for start < len(source) {
		next := strings.IndexByte(source[start:], '\n')
		lineNext := len(source)
		if next >= 0 {
			lineNext = start + next + 1
		}
		end := lineNext
		if end > start && source[end-1] == '\n' {
			end--
		}
		if end > start && source[end-1] == '\r' {
			end--
		}
		lines = append(lines, markdownLine{
			number: len(lines) + 1,
			start:  start,
			end:    end,
			next:   lineNext,
			text:   source[start:end],
		})
		if next < 0 {
			break
		}
		start = lineNext
	}
	return lines
}

func (p *markdownParser) parse() error {
	index, err := p.skipFrontMatter()
	if err != nil {
		return err
	}
	for index < len(p.lines) {
		if isMarkdownBlank(p.lines[index].text) {
			index++
			continue
		}
		if next, handled, err := p.consumeFence(index); handled {
			if err != nil {
				return err
			}
			index = next
			continue
		}
		if next, handled, err := p.consumeHTMLComment(index); handled {
			if err != nil {
				return err
			}
			index = next
			continue
		}
		if isIndentedCodeLine(p.lines[index].text) {
			index = skipIndentedCode(p.lines, index)
			continue
		}
		if next, ok := p.consumeHeading(index); ok {
			index = next
			continue
		}
		if next, ok, err := p.consumeTable(index); ok {
			if err != nil {
				return err
			}
			index = next
			continue
		}
		if next, ok, err := p.consumeAdmonition(index); ok {
			if err != nil {
				return err
			}
			index = next
			continue
		}
		if next, ok := p.consumeList(index); ok {
			index = next
			continue
		}
		if isMarkdownHorizontalRule(p.lines[index].text) || isMarkdownReferenceDefinition(p.lines[index].text) {
			index++
			continue
		}
		if next, ok := p.consumeParagraph(index); ok {
			index = next
			continue
		}
		return fmt.Errorf("unsupported Markdown structure at line %d", p.lines[index].number)
	}
	return nil
}

func (p *markdownParser) skipFrontMatter() (int, error) {
	if len(p.lines) == 0 || strings.TrimSpace(strings.TrimPrefix(p.lines[0].text, "\ufeff")) != "---" {
		return 0, nil
	}
	if len(p.lines) == 1 || !looksLikeFrontMatter(p.lines[1].text) {
		return 0, nil
	}
	for index := 1; index < len(p.lines); index++ {
		if strings.TrimSpace(p.lines[index].text) != "---" && strings.TrimSpace(p.lines[index].text) != "..." {
			continue
		}
		return index + 1, nil
	}
	return 0, fmt.Errorf("unclosed YAML front matter")
}

func looksLikeFrontMatter(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Contains(trimmed, ":") || strings.HasPrefix(trimmed, "{")
}

type markdownFence struct {
	marker byte
	length int
	info   string
}

func (p *markdownParser) consumeFence(index int) (int, bool, error) {
	fence, ok := markdownFenceStart(p.lines[index].text)
	if !ok {
		return index, false, nil
	}
	for cursor := index + 1; cursor < len(p.lines); cursor++ {
		if !markdownFenceClose(p.lines[cursor].text, fence) {
			if comment, commentStart, ok := codeComment(p.lines[cursor].text); ok {
				p.addSpan([]markdownLine{p.lines[cursor]}, commentStart, ContentClassProcedural, comment)
			}
			continue
		}
		return cursor + 1, true, nil
	}
	return index, true, fmt.Errorf("unclosed %s code fence at line %d", strings.Repeat(string(fence.marker), fence.length), p.lines[index].number)
}

func markdownFenceStart(line string) (markdownFence, bool) {
	indent := leadingSpaces(line)
	if indent > 3 || len(line) < indent+3 {
		return markdownFence{}, false
	}
	marker := line[indent]
	if marker != '`' && marker != '~' {
		return markdownFence{}, false
	}
	length := 0
	for indent+length < len(line) && line[indent+length] == marker {
		length++
	}
	if length < 3 {
		return markdownFence{}, false
	}
	return markdownFence{marker: marker, length: length, info: strings.TrimSpace(line[indent+length:])}, true
}

func markdownFenceClose(line string, fence markdownFence) bool {
	indent := leadingSpaces(line)
	if indent > 3 || len(line) < indent+fence.length {
		return false
	}
	for offset := 0; offset < fence.length; offset++ {
		if line[indent+offset] != fence.marker {
			return false
		}
	}
	return strings.TrimSpace(line[indent+fence.length:]) == ""
}

func validateMarkdownInlineCode(lines []markdownLine) error {
	var fence *markdownFence
	openLength := 0
	for _, line := range lines {
		if fence != nil {
			if markdownFenceClose(line.text, *fence) {
				fence = nil
			}
			continue
		}
		if nextFence, ok := markdownFenceStart(line.text); ok {
			fence = &nextFence
			continue
		}
		for index := 0; index < len(line.text); {
			if line.text[index] == '\\' && index+1 < len(line.text) {
				index += 2
				continue
			}
			if line.text[index] != '`' {
				index++
				continue
			}
			end := index
			for end < len(line.text) && line.text[end] == '`' {
				end++
			}
			runLength := end - index
			if openLength == 0 {
				openLength = runLength
			} else if openLength == runLength {
				openLength = 0
			}
			index = end
		}
	}
	if fence != nil {
		return fmt.Errorf("unclosed %s code fence at line %d", strings.Repeat(string(fence.marker), fence.length), lines[len(lines)-1].number)
	}
	if openLength != 0 {
		return fmt.Errorf("unclosed inline code span")
	}
	return nil
}

func codeComment(line string) (string, int, bool) {
	indent := leadingSpaces(line)
	text := line[indent:]
	markerLength := 0
	switch {
	case strings.HasPrefix(text, "#"):
		markerLength = 1
	case strings.HasPrefix(text, "//"):
		markerLength = 2
	default:
		return "", 0, false
	}
	start := indent + markerLength
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	comment := strings.TrimSpace(line[start:])
	if !looksLikeNaturalComment(comment) {
		return "", 0, false
	}
	return comment, start, true
}

func looksLikeNaturalComment(comment string) bool {
	if comment == "" || strings.HasPrefix(comment, "!") || strings.HasPrefix(comment, "#include") {
		return false
	}
	if strings.HasPrefix(strings.ToLower(comment), "todo:") || strings.HasPrefix(strings.ToLower(comment), "nolint") {
		return false
	}
	for _, value := range comment {
		if unicode.IsLetter(value) {
			return true
		}
	}
	return false
}

func (p *markdownParser) consumeHTMLComment(index int) (int, bool, error) {
	line := p.lines[index]
	open := strings.Index(line.text, "<!--")
	if open < 0 || strings.TrimSpace(line.text[:open]) != "" {
		return index, false, nil
	}
	closeLine, close := findHTMLCommentClose(p.lines, index, open)
	if !close {
		return index, true, fmt.Errorf("unclosed HTML comment at line %d", line.number)
	}
	raw := p.source[line.start+open : p.lines[closeLine].end]
	body := htmlCommentBody(raw)
	if class, ok, err := parseMarkdownClassDirective(body); ok {
		if err != nil {
			return closeLine + 1, true, err
		}
		p.pendingClass = class
		p.hasPendingClass = true
		return closeLine + 1, true, nil
	}
	if strings.Contains(strings.ToLower(body), "prosecheck:ignore") {
		suppression, err := ParseSuppressionDirective(raw)
		if err != nil {
			p.addMalformedComment(raw, line, open)
		} else {
			p.pendingSuppressions = append(p.pendingSuppressions, suppression)
		}
		return closeLine + 1, true, nil
	}
	if strings.Contains(strings.ToLower(body), "prosecheck:") {
		return closeLine + 1, true, fmt.Errorf("unsupported prosecheck directive at line %d", line.number)
	}
	comment := strings.TrimSpace(body)
	if comment != "" {
		start := open + len("<!--")
		for start < len(line.text) && (line.text[start] == ' ' || line.text[start] == '\t') {
			start++
		}
		p.addTextSpan(line, start, ContentClassDescriptive, comment)
	}
	return closeLine + 1, true, nil
}

func findHTMLCommentClose(lines []markdownLine, start, open int) (int, bool) {
	for index := start; index < len(lines); index++ {
		from := 0
		if index == start {
			from = open + len("<!--")
		}
		if strings.Index(lines[index].text[from:], "-->") >= 0 {
			return index, true
		}
	}
	return 0, false
}

func htmlCommentBody(raw string) string {
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "<!--"), "-->"))
	return body
}

func parseMarkdownClassDirective(body string) (ContentClass, bool, error) {
	trimmed := strings.TrimSpace(body)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "prosecheck:") {
		return "", false, nil
	}
	rest := strings.TrimSpace(trimmed[len("prosecheck:"):])
	if strings.HasPrefix(strings.ToLower(rest), "ignore") {
		return "", false, nil
	}
	if strings.HasPrefix(strings.ToLower(rest), "class") {
		rest = strings.TrimSpace(rest[len("class"):])
		if strings.HasPrefix(rest, "=") {
			rest = strings.TrimSpace(rest[1:])
		}
		class := ContentClass(strings.ToLower(rest))
		switch class {
		case ContentClassLabel, ContentClassProcedural, ContentClassDescriptive, ContentClassTechnical, ContentClassTerm:
			return class, true, nil
		default:
			return "", true, fmt.Errorf("unsupported prosecheck content class %q", rest)
		}
	}
	class := ContentClass(strings.ToLower(rest))
	switch class {
	case ContentClassLabel, ContentClassProcedural, ContentClassDescriptive, ContentClassTechnical, ContentClassTerm:
		return class, true, nil
	default:
		return "", false, nil
	}
}

func (p *markdownParser) addMalformedComment(raw string, line markdownLine, open int) {
	start := open
	span := Span{
		SourcePath:  p.sourcePath,
		StartLine:   line.number,
		StartColumn: runeColumn(line.text[:start]) + 1,
		Class:       p.pendingContentClass(ContentClassDescriptive),
		Text:        raw,
		Surfaces:    markdownSurfaceSelectors(p.sourcePath),
		Protected:   markdownProtectedRanges(raw),
	}
	p.spans = append(p.spans, span)
	p.clearPending()
}

func (p *markdownParser) consumeHeading(index int) (int, bool) {
	line := p.lines[index]
	if heading, start, ok := markdownATXHeading(line.text); ok {
		p.addTextSpan(line, start, ContentClassLabel, heading)
		return index + 1, true
	}
	if index+1 < len(p.lines) && !isMarkdownBlank(line.text) && isSetextUnderline(p.lines[index+1].text) {
		start := leadingSpaces(line.text)
		p.addTextSpan(line, start, ContentClassLabel, strings.TrimSpace(line.text[start:]))
		return index + 2, true
	}
	return index, false
}

func markdownATXHeading(line string) (string, int, bool) {
	indent := leadingSpaces(line)
	if indent > 3 || len(line) <= indent || line[indent] != '#' {
		return "", 0, false
	}
	count := 0
	for indent+count < len(line) && line[indent+count] == '#' {
		count++
	}
	if count == 0 || count > 6 || (indent+count < len(line) && line[indent+count] != ' ' && line[indent+count] != '\t') {
		return "", 0, false
	}
	start := indent + count
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	end := len(line)
	for end > start && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	if end > start && line[end-1] == '#' {
		trimmed := end
		for trimmed > start && line[trimmed-1] == '#' {
			trimmed--
		}
		if trimmed == start || (trimmed > start && (line[trimmed-1] == ' ' || line[trimmed-1] == '\t')) {
			end = trimmed
			for end > start && (line[end-1] == ' ' || line[end-1] == '\t') {
				end--
			}
		}
	}
	if end <= start {
		return "", 0, false
	}
	return line[start:end], start, true
}

func isSetextUnderline(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}
	return strings.Trim(trimmed, "=") == "" || strings.Trim(trimmed, "-") == ""
}

func (p *markdownParser) consumeTable(index int) (int, bool, error) {
	if index+1 >= len(p.lines) {
		return index, false, nil
	}
	header := splitMarkdownTableCells(p.lines[index].text)
	separator := splitMarkdownTableCells(p.lines[index+1].text)
	if len(header) < 2 || len(header) != len(separator) || !tableSeparatorCells(p.lines[index+1].text, separator) {
		if looksLikeMarkdownTable(p.lines, index) {
			return index, true, fmt.Errorf("malformed Markdown table at line %d", p.lines[index].number)
		}
		return index, false, nil
	}
	end := index + 2
	for end < len(p.lines) && !isMarkdownBlank(p.lines[end].text) && strings.Contains(p.lines[end].text, "|") {
		cells := splitMarkdownTableCells(p.lines[end].text)
		if len(cells) != len(header) {
			return index, true, fmt.Errorf("Markdown table row at line %d has %d cells; expected %d", p.lines[end].number, len(cells), len(header))
		}
		end++
	}
	p.addTableRow(p.lines[index], header, ContentClassLabel)
	for row := index + 2; row < end; row++ {
		p.addTableRow(p.lines[row], splitMarkdownTableCells(p.lines[row].text), ContentClassDescriptive)
	}
	return end, true, nil
}

type markdownCell struct {
	start int
	end   int
}

func splitMarkdownTableCells(line string) []markdownCell {
	var cells []markdownCell
	start := 0
	inCode := false
	escaped := false
	for index := 0; index < len(line); index++ {
		if escaped {
			escaped = false
			continue
		}
		if line[index] == '\\' {
			escaped = true
			continue
		}
		if line[index] == '`' {
			inCode = !inCode
			continue
		}
		if line[index] == '|' && !inCode {
			cells = append(cells, markdownCell{start: start, end: index})
			start = index + 1
		}
	}
	cells = append(cells, markdownCell{start: start, end: len(line)})
	if len(cells) > 0 && strings.TrimSpace(line[cells[0].start:cells[0].end]) == "" && strings.HasPrefix(strings.TrimSpace(line), "|") {
		cells = cells[1:]
	}
	if len(cells) > 0 && strings.TrimSpace(line[cells[len(cells)-1].start:cells[len(cells)-1].end]) == "" && strings.HasSuffix(strings.TrimSpace(line), "|") {
		cells = cells[:len(cells)-1]
	}
	return cells
}

func tableSeparatorCells(line string, cells []markdownCell) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !isTableSeparatorCell(line[cell.start:cell.end]) {
			return false
		}
	}
	return true
}

func looksLikeMarkdownTable(lines []markdownLine, index int) bool {
	if index+1 >= len(lines) || !strings.Contains(lines[index].text, "|") || !strings.Contains(lines[index+1].text, "|") {
		return false
	}
	return strings.Contains(lines[index+1].text, "-")
}

func isTableSeparatorCell(value string) bool {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, ":")
	trimmed = strings.TrimSuffix(trimmed, ":")
	return len(trimmed) >= 3 && strings.Trim(trimmed, "-") == ""
}

func (p *markdownParser) addTableRow(line markdownLine, cells []markdownCell, class ContentClass) {
	for _, cell := range cells {
		start, end := trimMarkdownCell(line.text, cell.start, cell.end)
		if start >= end {
			continue
		}
		p.addTextSpan(line, start, class, line.text[start:end])
	}
}

func trimMarkdownCell(line string, start, end int) (int, int) {
	for start < end && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	for end > start && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	return start, end
}

func (p *markdownParser) consumeAdmonition(index int) (int, bool, error) {
	if isQuoteLine(p.lines[index].text) {
		return p.consumeQuote(index)
	}
	if isContainerAdmonition(p.lines[index].text) {
		return p.consumeContainerAdmonition(index)
	}
	if !isDirectiveAdmonition(p.lines[index].text) {
		return index, false, nil
	}
	start := index + 1
	for start < len(p.lines) && isMarkdownBlank(p.lines[start].text) {
		start++
	}
	if start >= len(p.lines) || leadingSpaces(p.lines[start].text) < 3 {
		return index, true, fmt.Errorf("admonition at line %d has no indented body", p.lines[index].number)
	}
	end := start
	for end < len(p.lines) {
		if isMarkdownBlank(p.lines[end].text) {
			if end+1 < len(p.lines) && leadingSpaces(p.lines[end+1].text) >= 3 {
				end++
				continue
			}
			break
		}
		if leadingSpaces(p.lines[end].text) >= 3 {
			end++
			continue
		}
		break
	}
	firstStart := leadingSpaces(p.lines[start].text)
	p.addSpan(p.lines[start:end], firstStart, ContentClassDescriptive, p.lines[start].text[firstStart:])
	return end, true, nil
}

func (p *markdownParser) consumeContainerAdmonition(index int) (int, bool, error) {
	for end := index + 1; end < len(p.lines); end++ {
		if strings.TrimSpace(p.lines[end].text) != ":::" {
			continue
		}
		start := index + 1
		for start < end && isMarkdownBlank(p.lines[start].text) {
			start++
		}
		if start < end {
			firstStart := leadingSpaces(p.lines[start].text)
			p.addSpan(p.lines[start:end], firstStart, ContentClassDescriptive, p.lines[start].text[firstStart:])
		}
		return end + 1, true, nil
	}
	return index, true, fmt.Errorf("unclosed container admonition at line %d", p.lines[index].number)
}

func (p *markdownParser) consumeQuote(index int) (int, bool, error) {
	end := index
	for end < len(p.lines) && isQuoteLine(p.lines[end].text) {
		end++
	}
	first := index
	for first < end {
		contentStart, content := quoteContent(p.lines[first].text)
		if markerText, markerOffset, ok := inlineAdmonitionContent(content); ok {
			p.addTextSpan(p.lines[first], contentStart+markerOffset, ContentClassDescriptive, markerText)
			first++
			continue
		}
		if strings.TrimSpace(content) != "" && !isAdmonitionMarker(content) {
			p.addSpan(p.lines[first:end], contentStart, ContentClassDescriptive, content)
			return end, true, nil
		}
		first++
	}
	return end, true, nil
}

func isQuoteLine(line string) bool {
	indent := leadingSpaces(line)
	return indent <= 3 && indent < len(line) && line[indent] == '>'
}

func quoteContent(line string) (int, string) {
	indent := leadingSpaces(line)
	start := indent + 1
	if start < len(line) && line[start] == ' ' {
		start++
	}
	return start, line[start:]
}

func isAdmonitionMarker(content string) bool {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "[!") {
		return strings.Contains(trimmed, "]")
	}
	return strings.HasPrefix(trimmed, "**Note:**") || strings.HasPrefix(trimmed, "**Warning:**") || strings.HasPrefix(trimmed, "**Tip:**")
}

func isDirectiveAdmonition(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "!!! ") || strings.HasPrefix(trimmed, "!!!\t") || isContainerAdmonition(line)
}

func isContainerAdmonition(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, ":::") && trimmed != ":::"
}

func inlineAdmonitionContent(content string) (string, int, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "[!") {
		return "", 0, false
	}
	close := strings.IndexByte(trimmed, ']')
	if close < 0 {
		return "", 0, false
	}
	start := strings.Index(content, trimmed) + close + 1
	for start < len(content) && (content[start] == ' ' || content[start] == '\t') {
		start++
	}
	if start >= len(content) {
		return "", 0, false
	}
	return content[start:], start, true
}

func (p *markdownParser) consumeList(index int) (int, bool) {
	indent, ordered, start, ok := markdownListMarker(p.lines[index].text)
	if !ok {
		return index, false
	}
	end := index + 1
	for end < len(p.lines) {
		if _, _, _, nextItem := markdownListMarker(p.lines[end].text); nextItem {
			nextIndent, _, _, _ := markdownListMarker(p.lines[end].text)
			if nextIndent <= indent {
				break
			}
			break
		}
		if isMarkdownBlank(p.lines[end].text) {
			if end+1 < len(p.lines) && leadingSpaces(p.lines[end+1].text) > indent {
				end++
				continue
			}
			break
		}
		if leadingSpaces(p.lines[end].text) <= indent {
			break
		}
		end++
	}
	class := ContentClassDescriptive
	if ordered {
		class = ContentClassProcedural
	}
	p.addSpan(p.lines[index:end], start, class, p.lines[index].text[start:])
	return end, true
}

func markdownListMarker(line string) (int, bool, int, bool) {
	indent := leadingSpaces(line)
	if indent > 3 || indent >= len(line) {
		return 0, false, 0, false
	}
	if line[indent] == '-' || line[indent] == '*' || line[indent] == '+' {
		start := indent + 1
		if start < len(line) && (line[start] == ' ' || line[start] == '\t') {
			for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
				start++
			}
			return indent, false, start, true
		}
		return 0, false, 0, false
	}
	digitStart := indent
	for digitStart < len(line) && line[digitStart] >= '0' && line[digitStart] <= '9' {
		digitStart++
	}
	if digitStart == indent || digitStart >= len(line) || (line[digitStart] != '.' && line[digitStart] != ')') {
		return 0, false, 0, false
	}
	start := digitStart + 1
	if start >= len(line) || (line[start] != ' ' && line[start] != '\t') {
		return 0, false, 0, false
	}
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	return indent, true, start, true
}

func (p *markdownParser) consumeParagraph(index int) (int, bool) {
	end := index + 1
	for end < len(p.lines) {
		if isMarkdownBlank(p.lines[end].text) || markdownStructuralStart(p.lines, end) {
			break
		}
		if isIndentedCodeLine(p.lines[end].text) {
			break
		}
		end++
	}
	line := p.lines[index]
	start := leadingSpaces(line.text)
	if start >= len(line.text) {
		return end, false
	}
	p.addSpan(p.lines[index:end], start, ContentClassDescriptive, line.text[start:])
	return end, true
}

func markdownStructuralStart(lines []markdownLine, index int) bool {
	if _, ok := markdownFenceStart(lines[index].text); ok {
		return true
	}
	if strings.TrimSpace(lines[index].text) == "<!--" || strings.HasPrefix(strings.TrimSpace(lines[index].text), "<!--") {
		return true
	}
	if _, _, ok := markdownATXHeading(lines[index].text); ok {
		return true
	}
	if _, _, _, ok := markdownListMarker(lines[index].text); ok {
		return true
	}
	if isQuoteLine(lines[index].text) || isDirectiveAdmonition(lines[index].text) || isMarkdownHorizontalRule(lines[index].text) {
		return true
	}
	if index+1 < len(lines) {
		header := splitMarkdownTableCells(lines[index].text)
		separator := splitMarkdownTableCells(lines[index+1].text)
		if (len(header) >= 2 && len(header) == len(separator) && tableSeparatorCells(lines[index+1].text, separator)) || looksLikeMarkdownTable(lines, index) {
			return true
		}
	}
	if index+1 < len(lines) && isSetextUnderline(lines[index+1].text) {
		return true
	}
	return false
}

func (p *markdownParser) addSpan(lines []markdownLine, firstStart int, fallbackClass ContentClass, firstText string) {
	if len(lines) == 0 {
		return
	}
	text := joinMarkdownLines(p.source, lines, firstStart)
	if strings.TrimSpace(firstText) == "" || strings.TrimSpace(text) == "" {
		return
	}
	class := p.pendingContentClass(fallbackClass)
	suppressions := append([]Suppression(nil), p.pendingSuppressions...)
	for index := range suppressions {
		suppressions[index].StartOffset = 0
		suppressions[index].EndOffset = len(text)
	}
	span := Span{
		SourcePath:   p.sourcePath,
		StartLine:    lines[0].number,
		StartColumn:  runeColumn(lines[0].text[:firstStart]) + 1,
		Class:        class,
		Text:         text,
		Surfaces:     markdownSurfaceSelectors(p.sourcePath),
		Protected:    markdownProtectedRanges(text),
		Suppressions: suppressions,
	}
	p.spans = append(p.spans, span)
	p.clearPending()
}

func (p *markdownParser) addTextSpan(line markdownLine, start int, fallbackClass ContentClass, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	class := p.pendingContentClass(fallbackClass)
	suppressions := append([]Suppression(nil), p.pendingSuppressions...)
	for index := range suppressions {
		suppressions[index].StartOffset = 0
		suppressions[index].EndOffset = len(text)
	}
	p.spans = append(p.spans, Span{
		SourcePath:   p.sourcePath,
		StartLine:    line.number,
		StartColumn:  runeColumn(line.text[:start]) + 1,
		Class:        class,
		Text:         text,
		Surfaces:     markdownSurfaceSelectors(p.sourcePath),
		Protected:    markdownProtectedRanges(text),
		Suppressions: suppressions,
	})
	p.clearPending()
}

func (p *markdownParser) pendingContentClass(fallback ContentClass) ContentClass {
	if p.hasPendingClass {
		return p.pendingClass
	}
	return fallback
}

func (p *markdownParser) clearPending() {
	p.pendingClass = ""
	p.hasPendingClass = false
	p.pendingSuppressions = nil
}

func joinMarkdownLines(source string, lines []markdownLine, firstStart int) string {
	var builder strings.Builder
	for index, line := range lines {
		start := 0
		if index == 0 {
			start = firstStart
		}
		if start < 0 || start > len(line.text) {
			start = 0
		}
		builder.WriteString(line.text[start:])
		if index+1 < len(lines) {
			builder.WriteString(source[line.end:line.next])
		}
	}
	return builder.String()
}

func markdownParseFinding(sourcePath string, data []byte, message string) Finding {
	return parseFinding(Span{
		SourcePath:  sourcePath,
		StartLine:   1,
		StartColumn: 1,
		Class:       ContentClassTechnical,
		Text:        string(data),
	}, message, 0, 1)
}

func leadingSpaces(value string) int {
	count := 0
	for count < len(value) && (value[count] == ' ' || value[count] == '\t') {
		count++
	}
	return count
}

func runeColumn(value string) int {
	return utf8.RuneCountInString(value)
}

func isMarkdownBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}

func isIndentedCodeLine(value string) bool {
	return strings.HasPrefix(value, "    ") || strings.HasPrefix(value, "\t")
}

func skipIndentedCode(lines []markdownLine, index int) int {
	for index < len(lines) && (isMarkdownBlank(lines[index].text) || isIndentedCodeLine(lines[index].text)) {
		index++
	}
	return index
}

func isMarkdownHorizontalRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}
	for _, marker := range []string{"-", "*", "_"} {
		if strings.Trim(trimmed, marker) == "" {
			return true
		}
	}
	return false
}

func isMarkdownReferenceDefinition(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]:")
}
