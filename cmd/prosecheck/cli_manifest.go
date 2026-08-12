package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const authoredCLIManifestSuffix = "/contracts/cli/commands.json"

type cliJSONKind uint8

const (
	cliJSONInvalid cliJSONKind = iota
	cliJSONObject
	cliJSONArray
	cliJSONString
	cliJSONNumber
	cliJSONBoolean
	cliJSONNull
)

type cliJSONField struct {
	key   string
	start int
	value *cliJSONValue
}

type cliJSONValue struct {
	kind              cliJSONKind
	start             int
	end               int
	stringValue       string
	decodedRawOffsets []int
	fields            []cliJSONField
	fieldIndex        map[string]int
	elements          []*cliJSONValue
}

type cliJSONDocument struct {
	sourcePath string
	data       []byte
	root       *cliJSONValue
	positions  []sourcePosition
}

type cliJSONParser struct {
	data   []byte
	offset int
}

type cliJSONError struct {
	offset  int
	message string
}

func (e cliJSONError) Error() string {
	return fmt.Sprintf("%s at byte %d", e.message, e.offset)
}

type cliManifestExtraction struct {
	spans    []Span
	findings []Finding
}

type cliManifestCollector struct {
	doc      *cliJSONDocument
	policy   Policy
	spans    []Span
	findings []Finding
}

// AnalyzeCLIManifest extracts only authored customer-visible CLI prose and
// sends the resulting spans through the same pure evaluator as Markdown.
func AnalyzeCLIManifest(sourcePath string, data []byte, policy Policy) []Finding {
	extraction := extractCLIManifest(sourcePath, data, policy)
	findings := append([]Finding(nil), extraction.findings...)
	findings = append(findings, Analyze(extraction.spans, policy)...)
	return SortFindings(findings)
}

// ExtractCLIManifestSpans exposes the structural adapter for callers that
// need to inspect the extracted, source-located spans directly. AnalyzeCLIManifest
// retains partial spans and all safe parse findings when a field is malformed.
func ExtractCLIManifestSpans(sourcePath string, data []byte) ([]Span, error) {
	extraction := extractCLIManifest(sourcePath, data, Policy{})
	if len(extraction.findings) > 0 {
		return extraction.spans, errors.New(extraction.findings[0].Excerpt)
	}
	return extraction.spans, nil
}

func extractCLIManifest(sourcePath string, data []byte, policy Policy) cliManifestExtraction {
	document, err := parseCLIManifestJSON(sourcePath, data)
	if err != nil {
		return cliManifestExtraction{findings: []Finding{cliManifestParseFinding(sourcePath, data, "", "", "", cliJSONErrorOffset(err), err.Error())}}
	}
	if document.root.kind != cliJSONObject {
		return cliManifestExtraction{findings: []Finding{cliManifestParseFinding(sourcePath, data, "", "", "", document.root.start, "manifest root must be a JSON object")}}
	}
	commands, ok := document.root.field("commands")
	if !ok {
		return cliManifestExtraction{findings: []Finding{cliManifestParseFinding(sourcePath, data, "", "/commands", "", document.root.start, "JSON path /commands is required")}}
	}
	if commands.kind != cliJSONObject {
		return cliManifestExtraction{findings: []Finding{cliManifestParseFinding(sourcePath, data, "", "/commands", "", commands.start, "JSON path /commands must be an object")}}
	}

	collector := cliManifestCollector{doc: document, policy: policy}
	for _, entry := range sortedCLIFields(commands) {
		collector.visitCommand(entry)
	}
	return cliManifestExtraction{spans: collector.spans, findings: collector.findings}
}

func parseCLIManifestJSON(sourcePath string, data []byte) (*cliJSONDocument, error) {
	if !utf8.Valid(data) {
		return nil, cliJSONError{offset: 0, message: "CLI manifest is not valid UTF-8"}
	}
	parser := cliJSONParser{data: data}
	parser.skipSpace()
	root, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	parser.skipSpace()
	if parser.offset != len(data) {
		return nil, cliJSONError{offset: parser.offset, message: "unexpected data after the JSON document"}
	}
	return &cliJSONDocument{
		sourcePath: sourcePath,
		data:       append([]byte(nil), data...),
		root:       root,
		positions:  cliSourcePositions(data),
	}, nil
}

func (p *cliJSONParser) parseValue() (*cliJSONValue, error) {
	if p.offset >= len(p.data) {
		return nil, p.failure("expected a JSON value")
	}
	switch p.data[p.offset] {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		return p.parseString()
	case 't':
		return p.parseLiteral("true", cliJSONBoolean)
	case 'f':
		return p.parseLiteral("false", cliJSONBoolean)
	case 'n':
		return p.parseLiteral("null", cliJSONNull)
	default:
		if p.data[p.offset] == '-' || (p.data[p.offset] >= '0' && p.data[p.offset] <= '9') {
			return p.parseNumber()
		}
		return nil, p.failure("expected a JSON value")
	}
}

func (p *cliJSONParser) parseObject() (*cliJSONValue, error) {
	start := p.offset
	p.offset++
	value := &cliJSONValue{kind: cliJSONObject, start: start, fieldIndex: make(map[string]int)}
	p.skipSpace()
	if p.consume('}') {
		value.end = p.offset
		return value, nil
	}
	for {
		if p.offset >= len(p.data) || p.data[p.offset] != '"' {
			return nil, p.failure("object key must be a JSON string")
		}
		keyNode, err := p.parseString()
		if err != nil {
			return nil, err
		}
		if _, exists := value.fieldIndex[keyNode.stringValue]; exists {
			return nil, cliJSONError{offset: keyNode.start, message: fmt.Sprintf("duplicate object key %q", keyNode.stringValue)}
		}
		p.skipSpace()
		if !p.consume(':') {
			return nil, p.failure("object key must be followed by ':'")
		}
		p.skipSpace()
		child, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		value.fieldIndex[keyNode.stringValue] = len(value.fields)
		value.fields = append(value.fields, cliJSONField{key: keyNode.stringValue, start: keyNode.start, value: child})
		p.skipSpace()
		if p.consume('}') {
			value.end = p.offset
			return value, nil
		}
		if !p.consume(',') {
			return nil, p.failure("object member must be followed by ',' or '}'")
		}
		p.skipSpace()
	}
}

func (p *cliJSONParser) parseArray() (*cliJSONValue, error) {
	start := p.offset
	p.offset++
	value := &cliJSONValue{kind: cliJSONArray, start: start}
	p.skipSpace()
	if p.consume(']') {
		value.end = p.offset
		return value, nil
	}
	for {
		child, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		value.elements = append(value.elements, child)
		p.skipSpace()
		if p.consume(']') {
			value.end = p.offset
			return value, nil
		}
		if !p.consume(',') {
			return nil, p.failure("array value must be followed by ',' or ']'")
		}
		p.skipSpace()
	}
}

func (p *cliJSONParser) parseString() (*cliJSONValue, error) {
	start := p.offset
	p.offset++
	for p.offset < len(p.data) {
		switch p.data[p.offset] {
		case '"':
			p.offset++
			var decoded string
			if err := json.Unmarshal(p.data[start:p.offset], &decoded); err != nil {
				return nil, cliJSONError{offset: start, message: fmt.Sprintf("invalid JSON string: %v", err)}
			}
			offsets, err := cliDecodedRawOffsets(p.data, start, p.offset, decoded)
			if err != nil {
				return nil, err
			}
			return &cliJSONValue{
				kind:              cliJSONString,
				start:             start,
				end:               p.offset,
				stringValue:       decoded,
				decodedRawOffsets: offsets,
			}, nil
		case '\\':
			if p.offset+1 >= len(p.data) {
				return nil, p.failure("unterminated JSON escape")
			}
			if p.data[p.offset+1] == 'u' {
				if p.offset+6 > len(p.data) {
					return nil, p.failure("incomplete Unicode escape")
				}
				p.offset += 6
				continue
			}
			p.offset += 2
		case '\r', '\n':
			return nil, p.failure("JSON strings cannot contain literal line breaks")
		default:
			if p.data[p.offset] < 0x20 {
				return nil, p.failure("JSON strings cannot contain control characters")
			}
			_, size := utf8.DecodeRune(p.data[p.offset:])
			if size == 1 && p.data[p.offset] >= 0x80 {
				return nil, p.failure("JSON string contains invalid UTF-8")
			}
			p.offset += size
		}
	}
	return nil, cliJSONError{offset: start, message: "unterminated JSON string"}
}

func (p *cliJSONParser) parseLiteral(literal string, kind cliJSONKind) (*cliJSONValue, error) {
	start := p.offset
	if !strings.HasPrefix(string(p.data[p.offset:]), literal) {
		return nil, p.failure("invalid JSON literal")
	}
	p.offset += len(literal)
	return &cliJSONValue{kind: kind, start: start, end: p.offset}, nil
}

func (p *cliJSONParser) parseNumber() (*cliJSONValue, error) {
	start := p.offset
	if p.data[p.offset] == '-' {
		p.offset++
	}
	if p.offset >= len(p.data) {
		return nil, p.failure("incomplete JSON number")
	}
	if p.data[p.offset] == '0' {
		p.offset++
	} else if p.data[p.offset] >= '1' && p.data[p.offset] <= '9' {
		for p.offset < len(p.data) && p.data[p.offset] >= '0' && p.data[p.offset] <= '9' {
			p.offset++
		}
	} else {
		return nil, p.failure("invalid JSON number")
	}
	if p.consume('.') {
		if p.offset >= len(p.data) || p.data[p.offset] < '0' || p.data[p.offset] > '9' {
			return nil, p.failure("JSON number fraction requires digits")
		}
		for p.offset < len(p.data) && p.data[p.offset] >= '0' && p.data[p.offset] <= '9' {
			p.offset++
		}
	}
	if p.offset < len(p.data) && (p.data[p.offset] == 'e' || p.data[p.offset] == 'E') {
		p.offset++
		if p.offset < len(p.data) && (p.data[p.offset] == '+' || p.data[p.offset] == '-') {
			p.offset++
		}
		if p.offset >= len(p.data) || p.data[p.offset] < '0' || p.data[p.offset] > '9' {
			return nil, p.failure("JSON number exponent requires digits")
		}
		for p.offset < len(p.data) && p.data[p.offset] >= '0' && p.data[p.offset] <= '9' {
			p.offset++
		}
	}
	if !json.Valid(p.data[start:p.offset]) {
		return nil, cliJSONError{offset: start, message: "invalid JSON number"}
	}
	return &cliJSONValue{kind: cliJSONNumber, start: start, end: p.offset}, nil
}

func (p *cliJSONParser) skipSpace() {
	for p.offset < len(p.data) {
		switch p.data[p.offset] {
		case ' ', '\t', '\r', '\n':
			p.offset++
		default:
			return
		}
	}
}

func (p *cliJSONParser) consume(value byte) bool {
	if p.offset >= len(p.data) || p.data[p.offset] != value {
		return false
	}
	p.offset++
	return true
}

func (p *cliJSONParser) failure(message string) error {
	return cliJSONError{offset: p.offset, message: message}
}

func cliJSONErrorOffset(err error) int {
	var value cliJSONError
	if errors.As(err, &value) {
		return value.offset
	}
	return 0
}

func cliDecodedRawOffsets(data []byte, start, end int, decoded string) ([]int, error) {
	offsets := make([]int, 0, len(decoded)+1)
	for offset := start + 1; offset < end-1; {
		rawStart := offset
		if data[offset] != '\\' {
			_, size := utf8.DecodeRune(data[offset : end-1])
			if size == 0 || size == 1 && data[offset] >= 0x80 {
				return nil, cliJSONError{offset: offset, message: "invalid UTF-8 in JSON string"}
			}
			for index := 0; index < size; index++ {
				offsets = append(offsets, offset+index)
			}
			offset += size
			continue
		}
		if offset+1 >= end-1 {
			return nil, cliJSONError{offset: offset, message: "unterminated JSON escape"}
		}
		switch data[offset+1] {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			offsets = append(offsets, rawStart)
			offset += 2
		case 'u':
			value, consumed, err := cliUnicodeEscape(data, offset, end-1)
			if err != nil {
				return nil, err
			}
			for range []byte(string(value)) {
				offsets = append(offsets, rawStart)
			}
			offset += consumed
		default:
			return nil, cliJSONError{offset: offset, message: "invalid JSON escape"}
		}
	}
	if len(offsets) != len(decoded) {
		return nil, cliJSONError{offset: start, message: "JSON string decoding changed its byte length unexpectedly"}
	}
	offsets = append(offsets, end)
	return offsets, nil
}

func cliUnicodeEscape(data []byte, offset, end int) (rune, int, error) {
	if offset+6 > end || data[offset] != '\\' || data[offset+1] != 'u' {
		return 0, 0, cliJSONError{offset: offset, message: "incomplete Unicode escape"}
	}
	value, ok := cliHexRune(data[offset+2 : offset+6])
	if !ok {
		return 0, 0, cliJSONError{offset: offset, message: "invalid Unicode escape"}
	}
	consumed := 6
	if value >= 0xd800 && value <= 0xdbff {
		if offset+consumed+6 > end || data[offset+consumed] != '\\' || data[offset+consumed+1] != 'u' {
			return 0, 0, cliJSONError{offset: offset, message: "high surrogate is missing its low surrogate"}
		}
		low, ok := cliHexRune(data[offset+consumed+2 : offset+consumed+6])
		if !ok || low < 0xdc00 || low > 0xdfff {
			return 0, 0, cliJSONError{offset: offset, message: "invalid low surrogate"}
		}
		value = 0x10000 + ((value-0xd800)<<10 | (low - 0xdc00))
		consumed += 6
	} else if value >= 0xdc00 && value <= 0xdfff {
		return 0, 0, cliJSONError{offset: offset, message: "low surrogate has no high surrogate"}
	}
	return value, consumed, nil
}

func cliHexRune(value []byte) (rune, bool) {
	if len(value) != 4 {
		return 0, false
	}
	var result rune
	for _, item := range value {
		result <<= 4
		switch {
		case item >= '0' && item <= '9':
			result += rune(item - '0')
		case item >= 'a' && item <= 'f':
			result += rune(item-'a') + 10
		case item >= 'A' && item <= 'F':
			result += rune(item-'A') + 10
		default:
			return 0, false
		}
	}
	return result, true
}

func cliSourcePositions(data []byte) []sourcePosition {
	positions := make([]sourcePosition, len(data)+1)
	line, column := 1, 1
	for offset := 0; offset < len(data); {
		positions[offset] = sourcePosition{line: line, column: column}
		runeValue, size := utf8.DecodeRune(data[offset:])
		if size == 0 {
			size = 1
		}
		for index := 1; index < size && offset+index < len(data); index++ {
			positions[offset+index] = sourcePosition{line: line, column: column}
		}
		switch runeValue {
		case '\r':
			line++
			column = 1
		case '\n':
			if offset == 0 || data[offset-1] != '\r' {
				line++
			}
			column = 1
		default:
			column++
		}
		offset += size
	}
	positions[len(data)] = sourcePosition{line: line, column: column}
	return positions
}

func (value *cliJSONValue) field(key string) (*cliJSONValue, bool) {
	if value == nil || value.kind != cliJSONObject {
		return nil, false
	}
	index, ok := value.fieldIndex[key]
	if !ok {
		return nil, false
	}
	return value.fields[index].value, true
}

func sortedCLIFields(value *cliJSONValue) []cliJSONField {
	if value == nil || value.kind != cliJSONObject {
		return nil
	}
	fields := append([]cliJSONField(nil), value.fields...)
	slices.SortStableFunc(fields, func(left, right cliJSONField) int {
		return strings.Compare(left.key, right.key)
	})
	return fields
}

func cliManifestParseFinding(sourcePath string, data []byte, commandID, jsonPath, inputID string, offset int, message string) Finding {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(data) && len(data) > 0 {
		offset = len(data) - 1
	}
	span := Span{
		SourcePath:  sourcePath,
		StartLine:   1,
		StartColumn: 1,
		Class:       ContentClassTechnical,
		Text:        string(data),
		Identity:    cliManifestIdentity(commandID, inputID, jsonPath),
	}
	return parseFinding(span, message, offset, offset+1)
}

func cliManifestIdentity(commandID, inputID, jsonPath string) string {
	parts := make([]string, 0, 3)
	if commandID != "" {
		parts = append(parts, "command="+commandID)
	}
	if inputID != "" {
		parts = append(parts, "input="+inputID)
	}
	if jsonPath != "" {
		parts = append(parts, "json="+jsonPath)
	}
	return strings.Join(parts, ";")
}

func cliJSONPointer(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ReplaceAll(part, "~", "~0")
		part = strings.ReplaceAll(part, "/", "~1")
		result = append(result, part)
	}
	return "/" + strings.Join(result, "/")
}

func (c *cliManifestCollector) issue(node *cliJSONValue, context cliManifestContext, message string) {
	offset := 0
	if node != nil {
		offset = node.start
	}
	c.findings = append(c.findings, cliManifestParseFinding(
		c.doc.sourcePath,
		c.doc.data,
		context.commandID,
		context.jsonPath,
		context.inputID,
		offset,
		message,
	))
}

func (c *cliManifestCollector) addStringSpan(node *cliJSONValue, start, end int, class ContentClass, context cliManifestContext) {
	if node == nil || node.kind != cliJSONString || start < 0 || end > len(node.stringValue) || start >= end {
		return
	}
	text := node.stringValue[start:end]
	if strings.TrimSpace(text) == "" {
		return
	}
	positions := make([]sourcePosition, len(text)+1)
	for index := range positions {
		rawOffset := node.decodedRawOffsets[start+index]
		if rawOffset >= 0 && rawOffset < len(c.doc.positions) {
			positions[index] = c.doc.positions[rawOffset]
		}
	}
	span := Span{
		SourcePath:  c.doc.sourcePath,
		StartLine:   positions[0].line,
		StartColumn: positions[0].column,
		Class:       class,
		Text:        text,
		Identity:    cliManifestIdentity(context.commandID, context.inputID, context.jsonPath),
		Surfaces:    append([]string(nil), context.surfaces...),
		Protected:   cliProtectedRanges(text, context.literals),
		positions:   positions,
	}
	c.spans = append(c.spans, span)
}

func (c *cliManifestCollector) stringField(parent *cliJSONValue, key, path string, context cliManifestContext, required bool) (string, bool, *cliJSONValue) {
	field, ok := parent.field(key)
	if !ok {
		if required {
			c.issue(parent, contextWithPath(context, path), fmt.Sprintf("JSON path %s is required and must be a string", path))
		}
		return "", false, nil
	}
	if field.kind != cliJSONString {
		c.issue(field, contextWithPath(context, path), fmt.Sprintf("JSON path %s must be a string", path))
		return "", false, field
	}
	if required && strings.TrimSpace(field.stringValue) == "" {
		c.issue(field, contextWithPath(context, path), fmt.Sprintf("JSON path %s must not be empty", path))
		return "", false, field
	}
	return field.stringValue, true, field
}

func contextWithPath(context cliManifestContext, path string) cliManifestContext {
	context.jsonPath = path
	return context
}

func (c *cliManifestCollector) visitCommand(entry cliJSONField) {
	commandPath := cliJSONPointer("commands", entry.key)
	if entry.value.kind != cliJSONObject {
		c.issue(entry.value, cliManifestContext{jsonPath: commandPath}, fmt.Sprintf("JSON path %s must be a command object", commandPath))
		return
	}
	visibility := c.recordVisibility(entry.value, commandPath, cliManifestContext{jsonPath: commandPath})
	commandID, hasID, _ := c.stringField(entry.value, "id", cliJSONPointer("commands", entry.key, "id"), cliManifestContext{jsonPath: commandPath}, !visibility.hidden)
	if !hasID || strings.TrimSpace(commandID) == "" {
		commandID = entry.key
	}
	context := cliManifestContext{
		commandID: commandID,
		inputID:   commandID,
		jsonPath:  commandPath,
		surfaces:  cliManifestSurfaceSelectors(c.policy, commandID),
		literals:  c.commandLiterals(entry.value),
	}
	c.visitGuidanceFields(entry.value, context)
	c.visitLifecycle(entry.value, context)
	if visibility.hidden {
		return
	}

	c.visitDocumentation(entry.value, context)
	c.visitUsage(entry.value, context)
	c.visitInputRecords(entry.value, context, "flags")
	c.visitInputRecords(entry.value, context, "arguments")
}

type cliVisibility struct {
	hidden bool
}

func (c *cliManifestCollector) recordVisibility(record *cliJSONValue, path string, context cliManifestContext) cliVisibility {
	value, ok, field := c.stringField(record, "visibility", cliJSONPointerFromPath(path, "visibility"), context, false)
	if !ok {
		if field != nil {
			return cliVisibility{}
		}
		return cliVisibility{}
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hidden", "private", "internal":
		return cliVisibility{hidden: true}
	default:
		return cliVisibility{}
	}
}

func (c *cliManifestCollector) visitDocumentation(command *cliJSONValue, context cliManifestContext) {
	documentationPath := cliJSONPointerFromPath(context.jsonPath, "documentation")
	documentation, ok := command.field("documentation")
	if !ok {
		c.issue(command, contextWithPath(context, documentationPath), fmt.Sprintf("JSON path %s is required for a visible command", documentationPath))
		return
	}
	if documentation.kind != cliJSONObject {
		c.issue(documentation, contextWithPath(context, documentationPath), fmt.Sprintf("JSON path %s must be an object", documentationPath))
		return
	}
	copyPath := cliJSONPointerFromPath(documentationPath, "documentation")
	copy, ok := documentation.field("documentation")
	if !ok {
		c.issue(documentation, contextWithPath(context, copyPath), fmt.Sprintf("JSON path %s is required", copyPath))
		return
	}
	if copy.kind != cliJSONObject {
		c.issue(copy, contextWithPath(context, copyPath), fmt.Sprintf("JSON path %s must be an object", copyPath))
		return
	}
	copyVisibility := c.recordVisibility(copy, copyPath, context)
	if !copyVisibility.hidden {
		c.visitDocumentationField(copy, context, "title", ContentClassLabel)
		c.visitDocumentationField(copy, context, "description", ContentClassDescriptive)
		c.visitDocumentationExamples(documentation, context, documentationPath)
	}
}

func (c *cliManifestCollector) visitDocumentationField(copy *cliJSONValue, context cliManifestContext, key string, class ContentClass) {
	fieldPath := cliJSONPointerFromPath(context.jsonPath, "documentation", "documentation", key)
	field, ok := copy.field(key)
	if !ok {
		c.issue(copy, contextWithPath(context, fieldPath), fmt.Sprintf("JSON path %s is required", fieldPath))
		return
	}
	if field.kind != cliJSONObject {
		c.issue(field, contextWithPath(context, fieldPath), fmt.Sprintf("JSON path %s must be an object", fieldPath))
		return
	}
	inputID, hasID, _ := c.stringField(field, "id", cliJSONPointerFromPath(fieldPath, "id"), context, false)
	if !hasID || strings.TrimSpace(inputID) == "" {
		inputID = context.commandID + "." + key
	}
	valuePath := cliJSONPointerFromPath(fieldPath, "canonicalEnglish")
	value, ok, valueNode := c.stringField(field, "canonicalEnglish", valuePath, contextWithPath(context, valuePath), true)
	if ok {
		c.addStringSpan(valueNode, 0, len(value), class, cliManifestContext{
			commandID: context.commandID,
			inputID:   inputID,
			jsonPath:  valuePath,
			surfaces:  context.surfaces,
			literals:  context.literals,
		})
	}
}

func (c *cliManifestCollector) visitDocumentationExamples(documentation *cliJSONValue, context cliManifestContext, documentationPath string) {
	examplesPath := cliJSONPointerFromPath(documentationPath, "examples")
	examples, ok := documentation.field("examples")
	if !ok {
		return
	}
	if examples.kind != cliJSONArray {
		c.issue(examples, contextWithPath(context, examplesPath), fmt.Sprintf("JSON path %s must be an array of strings", examplesPath))
		return
	}
	for index, example := range examples.elements {
		path := cliJSONPointerFromPath(examplesPath, strconv.Itoa(index))
		if example.kind != cliJSONString {
			c.issue(example, contextWithPath(context, path), fmt.Sprintf("JSON path %s must be a string", path))
			continue
		}
		c.addExampleComments(example, context, context.commandID+".documentation.example."+strconv.Itoa(index), path)
	}
}

func (c *cliManifestCollector) visitUsage(command *cliJSONValue, context cliManifestContext) {
	usagePath := cliJSONPointerFromPath(context.jsonPath, "usage")
	usage, ok := command.field("usage")
	if !ok {
		return
	}
	if usage.kind != cliJSONObject {
		c.issue(usage, contextWithPath(context, usagePath), fmt.Sprintf("JSON path %s must be an object", usagePath))
		return
	}
	if line, ok, _ := c.stringField(usage, "line", cliJSONPointerFromPath(usagePath, "line"), context, false); ok {
		_ = line
	}
	examplePath := cliJSONPointerFromPath(usagePath, "example")
	example, ok := usage.field("example")
	if !ok {
		return
	}
	if example.kind != cliJSONString {
		c.issue(example, contextWithPath(context, examplePath), fmt.Sprintf("JSON path %s must be a string", examplePath))
		return
	}
	c.addExampleComments(example, context, context.commandID+".usage.example", examplePath)
}

func (c *cliManifestCollector) addExampleComments(node *cliJSONValue, context cliManifestContext, inputID, path string) {
	value := node.stringValue
	for lineStart := 0; lineStart < len(value); {
		lineEnd := strings.IndexByte(value[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(value)
		} else {
			lineEnd += lineStart
		}
		contentEnd := lineEnd
		if contentEnd > lineStart && value[contentEnd-1] == '\r' {
			contentEnd--
		}
		cursor := lineStart
		for cursor < contentEnd && (value[cursor] == ' ' || value[cursor] == '\t') {
			cursor++
		}
		if cursor < contentEnd && value[cursor] == '#' {
			contentStart := cursor + 1
			if contentStart < contentEnd && value[contentStart] == ' ' {
				contentStart++
			}
			spanContext := context
			spanContext.inputID = inputID
			spanContext.jsonPath = path
			c.addStringSpan(node, contentStart, contentEnd, ContentClassProcedural, spanContext)
		}
		if lineEnd == len(value) {
			break
		}
		lineStart = lineEnd + 1
	}
}

func (c *cliManifestCollector) visitInputRecords(command *cliJSONValue, context cliManifestContext, kind string) {
	basePath := cliJSONPointerFromPath(context.jsonPath, kind)
	records, ok := command.field(kind)
	if !ok {
		return
	}
	if records.kind != cliJSONObject {
		c.issue(records, contextWithPath(context, basePath), fmt.Sprintf("JSON path %s must be an object", basePath))
		return
	}
	for _, entry := range sortedCLIFields(records) {
		path := cliJSONPointerFromPath(basePath, entry.key)
		if entry.value.kind != cliJSONObject {
			c.issue(entry.value, contextWithPath(context, path), fmt.Sprintf("JSON path %s must be an input object", path))
			continue
		}
		inputID, hasID, _ := c.stringField(entry.value, "id", cliJSONPointerFromPath(path, "id"), context, false)
		if !hasID || strings.TrimSpace(inputID) == "" {
			inputID = entry.key
		}
		inputContext := cliManifestContext{
			commandID: context.commandID,
			inputID:   inputID,
			jsonPath:  path,
			surfaces:  context.surfaces,
			literals:  c.inputLiterals(context, entry.value),
		}
		visibility := c.recordVisibility(entry.value, path, inputContext)
		if usage, present, usageNode := c.stringField(entry.value, "usage", cliJSONPointerFromPath(path, "usage"), inputContext, false); present && !visibility.hidden {
			c.addStringSpan(usageNode, 0, len(usage), ContentClassProcedural, cliManifestContext{
				commandID: context.commandID,
				inputID:   inputID,
				jsonPath:  cliJSONPointerFromPath(path, "usage"),
				surfaces:  inputContext.surfaces,
				literals:  inputContext.literals,
			})
		}
		c.visitGuidanceFields(entry.value, inputContext)
		c.visitLifecycle(entry.value, inputContext)
	}
}

func (c *cliManifestCollector) visitGuidanceFields(record *cliJSONValue, context cliManifestContext) {
	for _, key := range []string{"deprecation", "deprecationMessage", "replacement", "replacementGuidance"} {
		path := cliJSONPointerFromPath(context.jsonPath, key)
		value, ok, node := c.stringField(record, key, path, context, false)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		c.addStringSpan(node, 0, len(value), cliGuidanceClass(value), cliManifestContext{
			commandID: context.commandID,
			inputID:   context.inputID + "." + key,
			jsonPath:  path,
			surfaces:  context.surfaces,
			literals:  context.literals,
		})
	}
}

func (c *cliManifestCollector) visitLifecycle(record *cliJSONValue, context cliManifestContext) {
	path := cliJSONPointerFromPath(context.jsonPath, "lifecycle")
	lifecycle, ok := record.field("lifecycle")
	if !ok {
		return
	}
	if lifecycle.kind != cliJSONObject {
		c.issue(lifecycle, contextWithPath(context, path), fmt.Sprintf("JSON path %s must be an object", path))
		return
	}
	for _, key := range []string{"deprecated", "removed", "deprecation", "deprecationMessage", "replacement", "replacementGuidance"} {
		valuePath := cliJSONPointerFromPath(path, key)
		value, present, node := c.stringField(lifecycle, key, valuePath, context, false)
		if present && strings.TrimSpace(value) != "" {
			c.addStringSpan(node, 0, len(value), cliGuidanceClass(value), cliManifestContext{
				commandID: context.commandID,
				inputID:   context.inputID + "." + key,
				jsonPath:  valuePath,
				surfaces:  context.surfaces,
				literals:  context.literals,
			})
		}
	}
	successorPath := cliJSONPointerFromPath(path, "successor")
	successor, ok := lifecycle.field("successor")
	if !ok {
		return
	}
	if successor.kind != cliJSONObject {
		c.issue(successor, contextWithPath(context, successorPath), fmt.Sprintf("JSON path %s must be an object", successorPath))
		return
	}
	if target, present, _ := c.stringField(successor, "targetItemId", cliJSONPointerFromPath(successorPath, "targetItemId"), context, false); present {
		_ = target
	}
	guidancePath := cliJSONPointerFromPath(successorPath, "canonicalEnglish")
	guidance, present, node := c.stringField(successor, "canonicalEnglish", guidancePath, context, false)
	if present && strings.TrimSpace(guidance) != "" {
		c.addStringSpan(node, 0, len(guidance), cliGuidanceClass(guidance), cliManifestContext{
			commandID: context.commandID,
			inputID:   context.inputID + ".successor",
			jsonPath:  guidancePath,
			surfaces:  context.surfaces,
			literals:  context.literals,
		})
	}
}

func cliGuidanceClass(value string) ContentClass {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "use ") {
		return ContentClassProcedural
	}
	return ContentClassDescriptive
}

func cliJSONPointerFromPath(path string, parts ...string) string {
	if path == "" {
		return cliJSONPointer(parts...)
	}
	return path + cliJSONPointer(parts...)
}
