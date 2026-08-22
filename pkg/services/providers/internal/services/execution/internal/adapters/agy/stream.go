package agy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	// AGY emits long step_update records for media work. Keep this limit well
	// above the recorded ground-truth trace while still bounding one line held
	// in memory by the adapter.
	maxAgyStreamRecordBytes = 8 * 1024 * 1024
	agyResultEvent          = "result"
	agyOutputFormatStream   = "stream-json"
	agyOutputFormatJSON     = "json"
)

type parsedAgyOutput struct {
	Content          string
	StructuredOutput json.RawMessage
	JSONSchema       json.RawMessage
	SessionRef       *providers.SessionRef
	Diagnostics      providers.ExecuteDiagnostics
	DurationSeen     bool
}

type agyStreamRecord struct {
	Event          string          `json:"event"`
	ConversationID string          `json:"conversation_id"`
	Result         json.RawMessage `json:"result"`
}

type agyResultRecord struct {
	Status           string          `json:"status"`
	Response         *string         `json:"response"`
	DurationSeconds  json.RawMessage `json:"duration_seconds"`
	NumTurns         json.RawMessage `json:"num_turns"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	JSONSchema       json.RawMessage `json:"json_schema"`
	Usage            json.RawMessage `json:"usage"`
}

type agyJSONEnvelope struct {
	ConversationID   string          `json:"conversation_id"`
	Status           string          `json:"status"`
	Response         *string         `json:"response"`
	DurationSeconds  json.RawMessage `json:"duration_seconds"`
	NumTurns         json.RawMessage `json:"num_turns"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	JSONSchema       json.RawMessage `json:"json_schema"`
	Usage            json.RawMessage `json:"usage"`
}

type agyUsageRecord struct {
	InputTokens     *int64 `json:"input_tokens"`
	OutputTokens    *int64 `json:"output_tokens"`
	ThinkingTokens  *int64 `json:"thinking_tokens"`
	CacheReadTokens *int64 `json:"cache_read_tokens"`
	TotalTokens     *int64 `json:"total_tokens"`
}

func parseAgyOutput(
	stdout []byte,
	requireStreamJSON bool,
	expectedSchema string,
) (parsedAgyOutput, *providers.ExecuteFailure) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		if requireStreamJSON {
			return parsedAgyOutput{}, malformedAgyOutputFailure(
				agyOutputFormatStream,
				fmt.Errorf("stream output was empty"),
			)
		}
		return parsedAgyOutput{}, unusableFinalOnlyFailure()
	}
	if !looksLikeJSON(trimmed) {
		if requireStreamJSON {
			return parsedAgyOutput{}, malformedAgyOutputFailure(
				agyOutputFormatStream,
				fmt.Errorf("output did not begin with a JSON record"),
			)
		}
		content, failure := parseFinalOutput(stdout)
		parsed := parsedAgyOutput{Content: content}
		if failure != nil {
			return parsed, failure
		}
		if err := applyStructuredContract(&parsed, expectedSchema); err != nil {
			return parsedAgyOutput{}, malformedAgyOutputFailure(agyOutputFormatJSON, err)
		}
		return parsed, nil
	}

	if envelope, isEnvelope := singleJSONEnvelope(trimmed); isEnvelope {
		parsed, err := parseAgyEnvelope(envelope)
		if err != nil {
			return parsedAgyOutput{}, malformedAgyOutputFailure(agyOutputFormatJSON, err)
		}
		if err := applyStructuredContract(&parsed, expectedSchema); err != nil {
			return parsedAgyOutput{}, malformedAgyOutputFailure(agyOutputFormatJSON, err)
		}
		return parsed, nil
	}

	parsed, err := parseAgyStream(trimmed)
	if err != nil {
		return parsedAgyOutput{}, malformedAgyOutputFailure(agyOutputFormatStream, err)
	}
	if err := applyStructuredContract(&parsed, expectedSchema); err != nil {
		return parsedAgyOutput{}, malformedAgyOutputFailure(agyOutputFormatStream, err)
	}
	return parsed, nil
}

func parseAgyStream(stdout []byte) (parsedAgyOutput, error) {
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 64*1024), maxAgyStreamRecordBytes)

	var (
		lineNumber       int
		lastNonEmptyLine int
		lastEvent        string
		conversationID   string
		finalResult      json.RawMessage
	)
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		lastNonEmptyLine = lineNumber

		var record agyStreamRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return parsedAgyOutput{}, fmt.Errorf("malformed JSON on line %d: %w", lineNumber, err)
		}
		record.Event = strings.TrimSpace(record.Event)
		if record.Event == "" {
			return parsedAgyOutput{}, fmt.Errorf("line %d is missing its event type", lineNumber)
		}
		lastEvent = record.Event
		if conversationID == "" {
			conversationID = strings.TrimSpace(record.ConversationID)
		}
		if record.Event != agyResultEvent {
			continue
		}
		if missingJSONValue(record.Result) {
			return parsedAgyOutput{}, fmt.Errorf("result event on line %d is missing its result object", lineNumber)
		}
		finalResult = append(finalResult[:0], record.Result...)
	}
	if err := scanner.Err(); err != nil {
		return parsedAgyOutput{}, fmt.Errorf("stream output was truncated or exceeded the record limit: %w", err)
	}
	if lineNumber == 0 || lastNonEmptyLine == 0 {
		return parsedAgyOutput{}, fmt.Errorf("stream output was empty")
	}
	if lastEvent != agyResultEvent {
		return parsedAgyOutput{}, fmt.Errorf("stream output did not end with a terminal result event")
	}
	if missingJSONValue(finalResult) {
		return parsedAgyOutput{}, fmt.Errorf("stream output did not contain a terminal result object")
	}

	return parseAgyResult(finalResult, conversationID, agyOutputFormatStream)
}

func parseAgyEnvelope(raw []byte) (parsedAgyOutput, error) {
	var envelope agyJSONEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return parsedAgyOutput{}, fmt.Errorf("malformed JSON envelope: %w", err)
	}
	if envelope.Response == nil && strings.TrimSpace(envelope.Status) == "" {
		return parsedAgyOutput{}, fmt.Errorf("JSON envelope is missing response and status")
	}
	result, err := json.Marshal(agyResultRecord{
		Status:           envelope.Status,
		Response:         envelope.Response,
		DurationSeconds:  envelope.DurationSeconds,
		NumTurns:         envelope.NumTurns,
		StructuredOutput: envelope.StructuredOutput,
		JSONSchema:       envelope.JSONSchema,
		Usage:            envelope.Usage,
	})
	if err != nil {
		return parsedAgyOutput{}, fmt.Errorf("marshal JSON envelope result: %w", err)
	}
	return parseAgyResult(result, envelope.ConversationID, agyOutputFormatJSON)
}

func parseAgyResult(
	raw []byte,
	conversationID string,
	outputFormat string,
) (parsedAgyOutput, error) {
	var result agyResultRecord
	if err := json.Unmarshal(raw, &result); err != nil {
		return parsedAgyOutput{}, fmt.Errorf("malformed result object: %w", err)
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		return parsedAgyOutput{}, fmt.Errorf("result is missing status")
	}
	if result.Response == nil || strings.TrimSpace(*result.Response) == "" {
		return parsedAgyOutput{}, fmt.Errorf("result is missing a non-empty response")
	}
	durationSeconds, durationText, err := requiredNonNegativeFloat(
		result.DurationSeconds,
		"duration_seconds",
	)
	if err != nil {
		return parsedAgyOutput{}, err
	}
	numTurns, err := requiredNonNegativeInt(result.NumTurns, "num_turns")
	if err != nil {
		return parsedAgyOutput{}, err
	}
	usage, err := parseUsage(result.Usage)
	if err != nil {
		return parsedAgyOutput{}, err
	}

	content := strings.TrimSpace(*result.Response)
	metadata := map[string]string{
		"output_format":     outputFormat,
		"status":            status,
		"duration_seconds":  durationText,
		"num_turns":         strconv.FormatInt(numTurns, 10),
		"input_tokens":      strconv.FormatInt(*usage.InputTokens, 10),
		"output_tokens":     strconv.FormatInt(*usage.OutputTokens, 10),
		"thinking_tokens":   strconv.FormatInt(*usage.ThinkingTokens, 10),
		"cache_read_tokens": strconv.FormatInt(*usage.CacheReadTokens, 10),
		"total_tokens":      strconv.FormatInt(*usage.TotalTokens, 10),
	}
	usageMetadata := map[string]string{
		"input_tokens":      metadata["input_tokens"],
		"output_tokens":     metadata["output_tokens"],
		"thinking_tokens":   metadata["thinking_tokens"],
		"cache_read_tokens": metadata["cache_read_tokens"],
		"total_tokens":      metadata["total_tokens"],
	}
	progress := []providers.ExecuteProgress{
		{
			Phase:  "run.completed",
			Detail: fmt.Sprintf("Agy result status %s after %d turn(s).", status, numTurns),
			Metadata: map[string]string{
				"status":           status,
				"duration_seconds": durationText,
				"num_turns":        strconv.FormatInt(numTurns, 10),
			},
		},
		{
			Phase:    "usage.updated",
			Detail:   "Agy usage counters recorded.",
			Metadata: usageMetadata,
		},
		{
			Phase:    "message.completed",
			Detail:   boundedText(content),
			Metadata: map[string]string{"status": status},
		},
	}

	parsed := parsedAgyOutput{
		Content:          content,
		StructuredOutput: append(json.RawMessage(nil), result.StructuredOutput...),
		JSONSchema:       append(json.RawMessage(nil), result.JSONSchema...),
		Diagnostics: providers.ExecuteDiagnostics{
			DurationMillis: durationSecondsToMillis(durationSeconds),
			Progress:       progress,
			Metadata:       metadata,
		},
		DurationSeen: true,
	}
	if sessionID := strings.TrimSpace(conversationID); sessionID != "" {
		parsed.SessionRef = &providers.SessionRef{
			Provider: providers.IDAntigravity,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return parsed, nil
}

func applyStructuredContract(parsed *parsedAgyOutput, expectedSchema string) error {
	expected := strings.TrimSpace(expectedSchema)
	if expected == "" {
		return nil
	}
	if !json.Valid([]byte(expected)) {
		return fmt.Errorf("requested JSON schema is malformed")
	}
	if missingJSONValue(parsed.StructuredOutput) {
		return fmt.Errorf("result is missing structured_output for the requested JSON schema")
	}
	if !json.Valid(parsed.StructuredOutput) {
		return fmt.Errorf("result structured_output is malformed")
	}
	if missingJSONValue(parsed.JSONSchema) {
		return fmt.Errorf("result is missing the echoed json_schema")
	}
	if !json.Valid(parsed.JSONSchema) {
		return fmt.Errorf("result json_schema is malformed")
	}
	if !equivalentJSON([]byte(expected), parsed.JSONSchema) {
		return fmt.Errorf("result json_schema does not match the requested JSON schema")
	}
	parsed.Content = strings.TrimSpace(string(parsed.StructuredOutput))
	return nil
}

func equivalentJSON(left, right []byte) bool {
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func parseUsage(raw json.RawMessage) (agyUsageRecord, error) {
	if missingJSONValue(raw) {
		return agyUsageRecord{}, fmt.Errorf("result is missing usage")
	}
	var usage agyUsageRecord
	if err := json.Unmarshal(raw, &usage); err != nil {
		return agyUsageRecord{}, fmt.Errorf("result usage is malformed: %w", err)
	}
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{name: "input_tokens", value: usage.InputTokens},
		{name: "output_tokens", value: usage.OutputTokens},
		{name: "thinking_tokens", value: usage.ThinkingTokens},
		{name: "cache_read_tokens", value: usage.CacheReadTokens},
		{name: "total_tokens", value: usage.TotalTokens},
	} {
		if field.value == nil {
			return agyUsageRecord{}, fmt.Errorf("result usage is missing %s", field.name)
		}
		if *field.value < 0 {
			return agyUsageRecord{}, fmt.Errorf("result usage has negative %s", field.name)
		}
	}
	return usage, nil
}

func requiredNonNegativeInt(raw json.RawMessage, name string) (int64, error) {
	if missingJSONValue(raw) {
		return 0, fmt.Errorf("result is missing %s", name)
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("result %s is not an integer: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("result %s must not be negative", name)
	}
	return value, nil
}

func requiredNonNegativeFloat(raw json.RawMessage, name string) (float64, string, error) {
	if missingJSONValue(raw) {
		return 0, "", fmt.Errorf("result is missing %s", name)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, "", fmt.Errorf("result %s is not a number: %w", name, err)
	}
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, "", fmt.Errorf("result %s must be a finite non-negative number", name)
	}
	return value, strings.TrimSpace(string(raw)), nil
}

func durationSecondsToMillis(seconds float64) int64 {
	return time.Duration(seconds * float64(time.Second)).Milliseconds()
}

func missingJSONValue(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func looksLikeJSON(value []byte) bool {
	return len(value) > 0 && (value[0] == '{' || value[0] == '[')
}

func singleJSONEnvelope(raw []byte) ([]byte, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	if _, hasEvent := fields["event"]; hasEvent {
		return nil, false
	}
	if _, hasResponse := fields["response"]; hasResponse {
		return raw, true
	}
	if _, hasStructuredOutput := fields["structured_output"]; hasStructuredOutput {
		return raw, true
	}
	if _, hasJSONSchema := fields["json_schema"]; hasJSONSchema {
		return raw, true
	}
	return nil, false
}

func malformedAgyOutputFailure(format string, err error) *providers.ExecuteFailure {
	return &providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: fmt.Sprintf("Agy %s output could not be parsed safely: %v", format, err),
	}
}
