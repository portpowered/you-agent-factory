package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runnerinference "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/inference"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
	"github.com/santhosh-tekuri/jsonschema/v6"
	jsonschemakind "github.com/santhosh-tekuri/jsonschema/v6/kind"
)

type CommandRunner = workerexecution.CommandRunner
type CommandRequest = workerexecution.CommandRequest
type CommandResult = workerexecution.CommandResult
type ExecCommandRunner = workerprocess.ExecCommandRunner
type LoggingCommandRunner = workerprocess.LoggingCommandRunner

func resolveExecutionTimeout(
	executionPolicy interfaces.WorkstationExecutionPolicyService,
	workerDef *interfaces.FactoryWorkerConfig,
	workstationDef *interfaces.FactoryWorkstationConfig,
) (time.Duration, error) {
	if executionPolicy != nil && workstationDef != nil {
		timeout, err := executionPolicy.ExecutionTimeout(workstationDef)
		if err != nil {
			return 0, err
		}
		if timeout > 0 {
			return timeout, nil
		}
	}

	if workerDef != nil && workerDef.Timeout != "" {
		timeout, err := time.ParseDuration(workerDef.Timeout)
		if err != nil {
			return 0, fmt.Errorf("invalid worker timeout %q: %w", workerDef.Timeout, err)
		}
		if timeout > 0 {
			return timeout, nil
		}
	}

	if workerDef != nil && workerDef.Type != "" {
		return defaultSubprocessExecutionTimeout, nil
	}

	return 0, nil
}

func timeoutWorkResult(dispatch work.WorkDispatch, duration time.Duration) workerexecution.WorkResult {
	failureMetadata := &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyRetryable,
		Type:   workerexecution.WorkFailureTypeTimeout,
	}
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         workerexecution.OutcomeFailed,
		Error:           "execution timeout",
		FailureMetadata: workerexecution.CloneWorkFailureMetadata(failureMetadata),
		Metrics:         workerexecution.WorkMetrics{Duration: duration},
	}
}

type ProviderError = workerexecution.ProviderError

const (
	providerSessionKindSessionID       = "session_id"
	codexWindowsProcessFailureExitCode = 4294967295
)

type DefaultPromptRenderer = workerprompting.DefaultPromptRenderer

// PrintTimeoutFromWorkerTimeout parses the authored worker timeout for native
// providers that expose their own print-mode deadline. Invalid values return
// zero; workstation execution remains responsible for reporting the authored
// timeout error before dispatch.
func PrintTimeoutFromWorkerTimeout(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 0
	}
	return timeout
}

func cloneInputTokens(rawTokens []any) []workerexecution.Token {
	if len(rawTokens) == 0 {
		return nil
	}

	out := make([]workerexecution.Token, 0, len(rawTokens))
	for _, raw := range rawTokens {
		token, ok := decodeToken(raw)
		if !ok {
			continue
		}
		out = append(out, token)
	}
	return out
}

func clonePetriInputTokens(inputTokens []workerexecution.Token) []any {
	if len(inputTokens) == 0 {
		return nil
	}

	out := make([]any, 0, len(inputTokens))
	for _, token := range inputTokens {
		out = append(out, token)
	}
	return out
}

func cloneRawInputTokens(inputTokens []any) []any {
	if len(inputTokens) == 0 {
		return nil
	}
	return append([]any(nil), inputTokens...)
}

func decodeToken(raw any) (workerexecution.Token, bool) {
	if token, ok := raw.(workerexecution.Token); ok {
		return token, true
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return workerexecution.Token{}, false
	}
	var token workerexecution.Token
	if err := json.Unmarshal(encoded, &token); err != nil {
		return workerexecution.Token{}, false
	}
	return token, true
}

func InputTokens(tokens ...workerexecution.Token) []any {
	return clonePetriInputTokens(tokens)
}

func WorkDispatchInputTokens(dispatch work.WorkDispatch) []workerexecution.Token {
	return cloneInputTokens(dispatch.InputTokens)
}

func workDispatchNonResourceTokensForWorkstation(dispatch work.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []workerexecution.Token {
	var tokens []workerexecution.Token
	for _, token := range orderedWorkDispatchTokensForWorkstation(dispatch, workstationDef) {
		if token.Color.DataType != workerexecution.DataTypeResource {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func orderedWorkDispatchTokensForWorkstation(dispatch work.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []workerexecution.Token {
	tokens := WorkDispatchInputTokens(dispatch)
	if workstationDef == nil || len(tokens) < 2 {
		return tokens
	}

	byPlace := make(map[string][]int)
	for i, token := range tokens {
		byPlace[token.PlaceID] = append(byPlace[token.PlaceID], i)
	}

	ordered := make([]workerexecution.Token, 0, len(tokens))
	used := make([]bool, len(tokens))
	appendPlaceTokens := func(placeID string) {
		for _, index := range byPlace[placeID] {
			used[index] = true
			ordered = append(ordered, tokens[index])
		}
	}

	for _, input := range workstationDef.Inputs {
		appendPlaceTokens(fmt.Sprintf("%s:%s", input.WorkTypeName, input.StateName))
	}
	for _, resource := range workstationDef.Resources {
		appendPlaceTokens(fmt.Sprintf("%s:%s", resource.Name, interfaces.ResourceStateAvailable))
	}
	for i, token := range tokens {
		if used[i] {
			continue
		}
		ordered = append(ordered, token)
	}

	return ordered
}

func cloneEnvVars(envVars map[string]string) map[string]string {
	if len(envVars) == 0 {
		return nil
	}
	clone := make(map[string]string, len(envVars))
	for key, value := range envVars {
		clone[key] = value
	}
	return clone
}

func parseOutputAgainstSchema(content string, schemaPayload []byte) (any, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("response is empty")
	}
	validate, err := compileOutputSchema(schemaPayload)
	if err != nil {
		return nil, err
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(content)))
	if err != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", err)
	}
	if err := validate(document); err != nil {
		return nil, fmt.Errorf("response does not satisfy output schema: %s", structuredOutputValidationSummary(err))
	}
	return document, nil
}

const (
	structuredOutputValidationDetailLimit = 512
	structuredOutputValidationCauseLimit  = 3
)

// structuredOutputValidationSummary deliberately builds the diagnostic from
// jsonschema's locations and keyword metadata instead of Error(). Several
// JSON Schema keywords include the rejected value in their localized error
// text, which must never cross the worker failure boundary.
func structuredOutputValidationSummary(err error) string {
	if err == nil {
		return "schema validation failed"
	}

	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) || validationErr == nil {
		return boundedStructuredOutputDiagnostic("schema validation failed")
	}

	details := make([]string, 0, structuredOutputValidationCauseLimit)
	appendStructuredOutputValidationDetails(validationErr, &details)
	if len(details) == 0 {
		return "schema validation failed"
	}
	return boundedStructuredOutputDiagnostic(strings.Join(details, "; "))
}

func appendStructuredOutputValidationDetails(err *jsonschema.ValidationError, details *[]string) {
	if err == nil || len(*details) >= structuredOutputValidationCauseLimit {
		return
	}
	if len(err.Causes) > 0 {
		for _, cause := range err.Causes {
			appendStructuredOutputValidationDetails(cause, details)
			if len(*details) >= structuredOutputValidationCauseLimit {
				return
			}
		}
		return
	}

	*details = append(*details, fmt.Sprintf(
		"instance %s; schema %s; keyword %q: %s",
		structuredOutputInstanceLocation(err.InstanceLocation),
		structuredOutputSchemaLocation(err),
		structuredOutputKeyword(err.ErrorKind),
		structuredOutputValidationReason(err.ErrorKind),
	))
}

func structuredOutputInstanceLocation(location []string) string {
	if len(location) == 0 {
		return "$"
	}
	return structuredOutputJSONPointer(location)
}

func structuredOutputSchemaLocation(err *jsonschema.ValidationError) string {
	const location = "output schema"
	keywordPath := structuredOutputKeywordPath(err.ErrorKind)
	if len(keywordPath) > 0 {
		return location + "#" + structuredOutputJSONPointer(keywordPath)
	}
	return location
}

func structuredOutputKeyword(errorKind jsonschema.ErrorKind) string {
	path := structuredOutputKeywordPath(errorKind)
	if len(path) == 0 {
		return "schema"
	}
	return strings.Join(path, ".")
}

func structuredOutputKeywordPath(errorKind jsonschema.ErrorKind) []string {
	if errorKind == nil {
		return nil
	}
	return errorKind.KeywordPath()
}

func structuredOutputJSONPointer(parts []string) string {
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteByte('/')
		builder.WriteString(strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1"))
	}
	return builder.String()
}

func structuredOutputValidationReason(errorKind jsonschema.ErrorKind) string {
	keywordPath := structuredOutputKeywordPath(errorKind)
	if len(keywordPath) == 0 {
		return "schema validation failed"
	}
	keyword := keywordPath[len(keywordPath)-1]
	switch keyword {
	case "required":
		if required, ok := errorKind.(*jsonschemakind.Required); ok && len(required.Missing) > 0 {
			missing := make([]string, 0, len(required.Missing))
			for _, name := range required.Missing {
				missing = append(missing, fmt.Sprintf("%q", name))
			}
			return "missing required property " + strings.Join(missing, ", ")
		}
		return "missing required property"
	case "pattern":
		return "string does not match the required pattern"
	case "format":
		return "value does not match the required format"
	case "type":
		if typeError, ok := errorKind.(*jsonschemakind.Type); ok {
			return fmt.Sprintf("value has type %q; expected %s", typeError.Got, strings.Join(typeError.Want, " or "))
		}
		return "value has an invalid JSON type"
	default:
		return fmt.Sprintf("validation failed for JSON Schema keyword %q", keyword)
	}
}

func boundedStructuredOutputDiagnostic(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "schema validation failed"
	}
	const suffix = "..."
	runes := []rune(message)
	if len(runes) <= structuredOutputValidationDetailLimit {
		return message
	}
	return string(runes[:structuredOutputValidationDetailLimit-len([]rune(suffix))]) + suffix
}

// compileOutputSchema validates workstation configuration without attempting
// to parse a worker response. The returned function validates one already
// decoded JSON value, allowing the worker boundary to decode a response once.
func compileOutputSchema(schemaPayload []byte) (func(any) error, error) {
	if len(bytes.TrimSpace(schemaPayload)) == 0 {
		return nil, fmt.Errorf("output schema is empty")
	}

	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaPayload))
	if err != nil {
		return nil, fmt.Errorf("output schema is malformed: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const schemaID = "worker-output-schema.json"
	if err := compiler.AddResource(schemaID, schemaDocument); err != nil {
		return nil, fmt.Errorf("output schema is invalid: %w", err)
	}
	compiled, err := compiler.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("output schema is invalid: %w", err)
	}
	return compiled.Validate, nil
}

func validateOutputSchema(schemaPayload []byte) error {
	_, err := compileOutputSchema(schemaPayload)
	return err
}

func structuredOutputMisconfigurationMetadata() *workerexecution.WorkFailureMetadata {
	return &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyTerminal,
		Type:   workerexecution.WorkFailureTypeMisconfigured,
	}
}

func outputSchemaConfigurationFailure(
	request workerexecution.WorkstationExecutionRequest,
	err error,
) workerexecution.WorkResult {
	message := "workstation outputSchema is misconfigured"
	if err != nil {
		message += ": " + err.Error()
	}
	return workerexecution.WorkResult{
		DispatchID:      request.Dispatch.DispatchID,
		TransitionID:    request.Dispatch.TransitionID,
		Outcome:         workerexecution.OutcomeFailed,
		Error:           message,
		FailureMetadata: structuredOutputMisconfigurationMetadata(),
	}
}

// attachStructuredResult validates and attaches a native result for executor
// implementations that return raw output without doing schema handling. Agent
// execution already performs this work and marks the result present, so the
// shared workstation boundary does not decode it a second time.
func attachStructuredResult(
	request workerexecution.WorkstationExecutionRequest,
	result workerexecution.WorkResult,
) workerexecution.WorkResult {
	if request.OutputSchema == "" ||
		result.Outcome != workerexecution.OutcomeAccepted ||
		jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent) {
		return result
	}
	structured, err := parseOutputAgainstSchema(result.Output, []byte(request.OutputSchema))
	if err != nil {
		result.Outcome = workerexecution.OutcomeFailed
		result.StructuredResult = nil
		result.StructuredResultPresent = false
		result.Error = "structured output schema violation: " + err.Error()
		result.FailureMetadata = structuredOutputSchemaViolationMetadata()
		return result
	}
	result.StructuredResult = jsonvalue.Clone(structured)
	result.StructuredResultPresent = true
	return result
}

func structuredOutputFailure(content, contract string) string {
	var object map[string]any
	if err := json.Unmarshal([]byte(content), &object); err != nil || object == nil {
		return ""
	}

	for _, key := range []string{"verdict", "status", "outcome"} {
		value, ok := object[key].(string)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "fail", "failed", "failure", "error", "reject", "rejected":
			return fmt.Sprintf("%s=%s", key, strings.TrimSpace(value))
		}
	}
	for _, key := range []string{"success", "completed"} {
		value, ok := object[key].(bool)
		if ok && !value {
			return fmt.Sprintf("%s=false", key)
		}
	}
	if value, ok := object["action_completed"].(bool); ok && !value &&
		strings.TrimSpace(contract) != outputContractStructuredClipQAVerdictV1 {
		return "action_completed=false"
	}
	if value, ok := object["verdict"].(string); ok &&
		strings.EqualFold(strings.TrimSpace(value), "reroll") &&
		strings.TrimSpace(contract) != outputContractStructuredClipQAVerdictV1 {
		return "verdict=reroll without an explicit structured QA output contract"
	}
	return ""
}

func structuredOutputFailureMetadata() *workerexecution.WorkFailureMetadata {
	return &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyTerminal,
		Type:   workerexecution.WorkFailureTypePermanentBadRequest,
	}
}

func structuredOutputSchemaViolationMetadata() *workerexecution.WorkFailureMetadata {
	return &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyTerminal,
		Type:   workerexecution.WorkFailureTypeStructuredOutputSchemaViolation,
	}
}

const (
	outputContractMarkdownObservationReportV1 = "markdown-observation-report/v1"
	outputContractStructuredClipQAVerdictV1   = "structured-clip-qa/v1"
)

var (
	observationReportTimestampPattern      = regexp.MustCompile(`(?i)(?:\b\d{1,2}:\d{2}(?:\.\d{1,3})?\b|\b\d+(?:\.\d+)?\s*(?:s|sec|secs|seconds)\b)`)
	observationReportStatusPattern         = regexp.MustCompile(`(?im)^\s*inspected\s*:\s*yes\s*$`)
	observationReportAudioPattern          = regexp.MustCompile(`(?im)^\s*audio\s+content\s*:\s*(speech|music|noise|silence|mixed)\s*$`)
	observationReportRecommendationPattern = regexp.MustCompile(`(?im)^\s*(?:overall\s+)?recommendation\s*:\s*(pass|reroll)\s*$`)
)

// validateOutputContract validates provider-neutral semantic contracts after
// a worker returns. Provider adapters still own transport/schema parsing; this
// layer owns contracts that cannot be expressed by a provider output mode.
func validateOutputContract(content, contract string) error {
	switch strings.TrimSpace(contract) {
	case "":
		return nil
	case outputContractMarkdownObservationReportV1:
		return validateMarkdownObservationReport(content)
	case outputContractStructuredClipQAVerdictV1:
		return validateStructuredClipQAVerdict(content)
	default:
		return fmt.Errorf("unsupported output contract %q", contract)
	}
}

type structuredClipQAVerdict struct {
	ActionCompleted   *bool     `json:"action_completed"`
	SpecDeviations    *[]string `json:"spec_deviations"`
	TemporalArtifacts *[]string `json:"temporal_artifacts"`
	AudioContent      *string   `json:"audio_content"`
	UnexpectedSpeech  *bool     `json:"unexpected_speech"`
	Verdict           *string   `json:"verdict"`
	Confidence        *float64  `json:"confidence"`
}

var structuredClipQAVerdictRequiredFields = []string{
	"action_completed",
	"spec_deviations",
	"temporal_artifacts",
	"audio_content",
	"unexpected_speech",
	"verdict",
	"confidence",
}

func validateStructuredClipQAVerdict(content string) error {
	if failure := structuredOutputFailure(content, outputContractStructuredClipQAVerdictV1); failure != "" {
		return fmt.Errorf("structured output failure: %s", failure)
	}

	verdict, err := decodeStructuredClipQAVerdict(content)
	if err != nil {
		return err
	}
	return validateStructuredClipQAVerdictFields(verdict)
}

func decodeStructuredClipQAVerdict(content string) (*structuredClipQAVerdict, error) {
	var object map[string]any
	if err := json.Unmarshal([]byte(content), &object); err != nil || object == nil {
		return nil, fmt.Errorf("clip-QA verdict must be a JSON object")
	}
	for _, key := range structuredClipQAVerdictRequiredFields {
		if _, ok := object[key]; !ok {
			return nil, fmt.Errorf("clip-QA verdict is missing required field %q", key)
		}
	}

	var verdict structuredClipQAVerdict
	if err := json.Unmarshal([]byte(content), &verdict); err != nil {
		return nil, fmt.Errorf("clip-QA verdict has invalid field types: %w", err)
	}
	return &verdict, nil
}

func validateStructuredClipQAVerdictFields(verdict *structuredClipQAVerdict) error {
	if verdict.ActionCompleted == nil || verdict.SpecDeviations == nil ||
		verdict.TemporalArtifacts == nil || verdict.AudioContent == nil ||
		verdict.UnexpectedSpeech == nil || verdict.Verdict == nil || verdict.Confidence == nil {
		return fmt.Errorf("clip-QA verdict contains a null required field")
	}
	if err := validateStructuredClipQAConfidence(*verdict.Confidence); err != nil {
		return err
	}
	return validateStructuredClipQAVerdictSemantics(verdict)
}

func validateStructuredClipQAConfidence(confidence float64) error {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	return nil
}

func validateStructuredClipQAVerdictSemantics(verdict *structuredClipQAVerdict) error {
	switch *verdict.Verdict {
	case "pass":
		return validateStructuredClipQAPass(verdict)
	case "reroll":
		return validateStructuredClipQAReroll(verdict)
	default:
		return fmt.Errorf("verdict must be pass or reroll")
	}
}

func validateStructuredClipQAPass(verdict *structuredClipQAVerdict) error {
	if !*verdict.ActionCompleted {
		return fmt.Errorf("pass requires action_completed=true")
	}
	if len(*verdict.SpecDeviations) > 0 {
		return fmt.Errorf("pass requires spec_deviations to be empty")
	}
	if len(*verdict.TemporalArtifacts) > 0 {
		return fmt.Errorf("pass requires temporal_artifacts to be empty")
	}
	if *verdict.UnexpectedSpeech {
		return fmt.Errorf("pass requires unexpected_speech=false")
	}
	return nil
}

func validateStructuredClipQAReroll(verdict *structuredClipQAVerdict) error {
	if *verdict.ActionCompleted && len(*verdict.SpecDeviations) == 0 &&
		len(*verdict.TemporalArtifacts) == 0 && !*verdict.UnexpectedSpeech {
		return fmt.Errorf("reroll requires an observed failure reason")
	}
	return nil
}

func validateMarkdownObservationReport(content string) error {
	sections := markdownReportSections(content)
	for _, name := range []string{
		"inspection status",
		"chronological events",
		"temporal or transient defects",
		"audio content and defects",
		"observed speech",
		"overall recommendation",
	} {
		if _, ok := sections[name]; !ok {
			return fmt.Errorf("missing required section %q", name)
		}
		if strings.TrimSpace(sections[name]) == "" {
			return fmt.Errorf("section %q is empty; state none observed when applicable", name)
		}
	}

	if !observationReportStatusPattern.MatchString(sections["inspection status"]) {
		return fmt.Errorf("inspection status must contain exactly Inspected: yes")
	}
	if !observationReportTimestampPattern.MatchString(sections["chronological events"]) {
		return fmt.Errorf("chronological events must include timestamps")
	}
	for _, name := range []string{"temporal or transient defects", "audio content and defects", "observed speech"} {
		body := strings.ToLower(sections[name])
		if !strings.Contains(body, "none observed") && !observationReportTimestampPattern.MatchString(body) {
			return fmt.Errorf("section %q must include timestamps or an explicit none observed statement", name)
		}
	}
	if !observationReportAudioPattern.MatchString(sections["audio content and defects"]) {
		return fmt.Errorf("audio content and defects must name speech, music, noise, silence, or mixed")
	}
	recommendations := observationReportRecommendationPattern.FindAllStringSubmatch(content, -1)
	if len(recommendations) != 1 {
		return fmt.Errorf("report must contain exactly one pass or reroll recommendation")
	}
	if len(observationReportRecommendationPattern.FindAllStringSubmatch(sections["overall recommendation"], -1)) != 1 {
		return fmt.Errorf("overall recommendation must contain exactly one pass or reroll recommendation")
	}
	return nil
}

func markdownReportSections(content string) map[string]string {
	sections := make(map[string]string)
	current := ""
	for _, rawLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "## ") {
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			if _, exists := sections[current]; !exists {
				sections[current] = ""
			}
			continue
		}
		if current == "" || line == "" {
			continue
		}
		if sections[current] != "" {
			sections[current] += "\n"
		}
		sections[current] += line
	}
	return sections
}

const (
	WorkLogEventCommandRunnerRequested      = "command_runner.requested"
	WorkLogEventCommandRunnerCompleted      = "command_runner.completed"
	WorkLogEventCommandRunnerRequestDetails = "command_runner.request_details"
	WorkLogEventCommandRunnerOutputDetails  = "command_runner.output_details"
)

// WorkLogFields returns stable structured log fields for work-scoped runtime
// records. Empty strings are intentional so unavailable IDs remain explicit.
func WorkLogFields(metadata work.ExecutionMetadata, keysAndValues ...any) []any {
	fields := []any{
		"request_id", metadata.RequestID,
		"trace_id", metadata.TraceID,
		"work_id", primaryWorkID(metadata.WorkIDs),
		"work_ids", cloneWorkIDs(metadata.WorkIDs),
	}
	return append(fields, keysAndValues...)
}

func primaryWorkID(workIDs []string) string {
	for _, workID := range workIDs {
		if workID != "" {
			return workID
		}
	}
	return ""
}

func cloneWorkIDs(workIDs []string) []string {
	if workIDs == nil {
		return []string{}
	}
	return append([]string(nil), workIDs...)
}

// NoopExecutor is a WorkerExecutor that always returns OutcomeAccepted
// without calling any LLM or script. It is used as a fallback when no
// AGENTS.md is configured for a worker, allowing tests to exercise the
// petri-net topology without providing real worker configuration.
//
// Hosted/poller Worker shapes must not use this type: Automations owns those
// ingress sources and they are omitted from Workers executor construction.
type NoopExecutor struct{}

// Execute implements WorkerExecutor. It propagates the first input token's
// color and returns OutcomeAccepted immediately.
func (n *NoopExecutor) Execute(_ context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

// Compile-time check.
var _ WorkerExecutor = (*NoopExecutor)(nil)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	inputTokens []workerexecution.Token,
) ([]workerexecution.ResolvedModelOperationBinding, error) {
	return runnerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}

func (we *WorkstationExecutor) promptFileReader() interfaces.FileReader {
	if we.FileSystem == nil {
		return nil
	}
	return we.FileSystem.ReadFile
}

func (we *WorkstationExecutor) prepareWorkstationDefinition(
	dispatch work.WorkDispatch,
	workstationName string,
	workstationDef *interfaces.FactoryWorkstationConfig,
	invocationArgs *work.InvocationArguments,
	readFile interfaces.FileReader,
	diagnostics *workerexecution.WorkDiagnostics,
	start time.Time,
) (*interfaces.FactoryWorkstationConfig, *workerexecution.WorkResult) {
	snapshot, promptPath, err := we.workstationPromptSnapshot(workstationName, workstationDef)
	if err != nil {
		result := promptSourceFailureResult(
			dispatch,
			"workstation",
			workstationName,
			promptPath,
			err,
			diagnostics,
			we.Now().Sub(start),
		)
		return nil, &result
	}
	if we.Interpolation == nil {
		return snapshot, nil
	}
	interpolated, err := we.Interpolation.InterpolateWorkstationConfig(*snapshot, invocationArgs, readFile)
	if err != nil {
		return nil, promptPreparationFailureResult(dispatch, err, diagnostics, we.Now().Sub(start))
	}
	return &interpolated, nil
}

func (we *WorkstationExecutor) prepareWorkerDefinition(
	dispatch work.WorkDispatch,
	workerName string,
	workerDef *interfaces.FactoryWorkerConfig,
	invocationArgs *work.InvocationArguments,
	readFile interfaces.FileReader,
	diagnostics *workerexecution.WorkDiagnostics,
	start time.Time,
) (*interfaces.FactoryWorkerConfig, *workerexecution.WorkResult) {
	snapshot, promptPath, err := we.workerPromptSnapshot(workerName, workerDef)
	if err != nil {
		result := promptSourceFailureResult(
			dispatch,
			"worker",
			workerName,
			promptPath,
			err,
			diagnostics,
			we.Now().Sub(start),
		)
		return nil, &result
	}
	if we.Interpolation == nil {
		return snapshot, nil
	}
	interpolated, err := we.Interpolation.InterpolateWorkerConfig(*snapshot, invocationArgs, readFile)
	if err != nil {
		return nil, promptPreparationFailureResult(dispatch, err, diagnostics, we.Now().Sub(start))
	}
	if strings.TrimSpace(interpolated.ModelProvider) == "" {
		interpolated.ModelProvider = interpolated.RuntimeDefaultModelProvider
	}
	if strings.TrimSpace(interpolated.Model) == "" {
		interpolated.Model = interpolated.RuntimeDefaultModel
	}
	if failed := we.resolveInvocationProvider(dispatch, &interpolated, diagnostics, start); failed != nil {
		return nil, failed
	}
	return &interpolated, nil
}

func promptPreparationFailureResult(
	dispatch work.WorkDispatch,
	err error,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) *workerexecution.WorkResult {
	return &workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        err.Error(),
		Diagnostics:  diagnostics,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func (we *WorkstationExecutor) workerPromptSnapshot(
	workerName string,
	workerDef *interfaces.FactoryWorkerConfig,
) (*interfaces.FactoryWorkerConfig, string, error) {
	snapshot := interfaces.CloneWorkerConfig(*workerDef)
	source := we.workerPromptSource(workerName, workerDef)
	snapshot.PromptSourcePath = source.Path
	if err := we.refreshWorkerPrompt(&snapshot); err != nil {
		return nil, snapshot.PromptSourcePath, err
	}
	return &snapshot, snapshot.PromptSourcePath, nil
}

func (we *WorkstationExecutor) refreshWorkerPrompt(
	workerDef *interfaces.FactoryWorkerConfig,
) error {
	if workerDef == nil || workerDef.PromptSourcePath == "" {
		return nil
	}
	body, err := workerprompting.ResolveAuthoredPromptSource(
		we.FileSystem,
		workerDef.PromptSourcePath,
		true,
	)
	if err != nil {
		return err
	}
	workerDef.Body = body
	return nil
}

func (we *WorkstationExecutor) workerPromptSource(
	workerName string,
	workerDef *interfaces.FactoryWorkerConfig,
) interfaces.PromptSource {
	if lookup, ok := we.RuntimeConfig.(interfaces.RuntimePromptSourceLookup); ok {
		if source, found := lookup.WorkerPromptSource(workerName); found {
			return source
		}
	}
	if workerDef == nil {
		return interfaces.PromptSource{}
	}
	return interfaces.PromptSource{Path: workerDef.PromptSourcePath}
}

func (we *WorkstationExecutor) workstationPromptSnapshot(
	workstationName string,
	workstationDef *interfaces.FactoryWorkstationConfig,
) (*interfaces.FactoryWorkstationConfig, string, error) {
	snapshot := interfaces.CloneWorkstationConfig(*workstationDef)
	source := we.workstationPromptSource(workstationName, workstationDef)
	snapshot.PromptSourcePath = source.Path
	snapshot.PromptSourceIsTemplate = source.IsTemplate
	if err := we.refreshWorkstationPrompt(&snapshot); err != nil {
		return nil, snapshot.PromptSourcePath, err
	}
	return &snapshot, snapshot.PromptSourcePath, nil
}

func (we *WorkstationExecutor) refreshWorkstationPrompt(
	workstationDef *interfaces.FactoryWorkstationConfig,
) error {
	if workstationDef == nil || workstationDef.PromptSourcePath == "" {
		return nil
	}
	prompt, err := workerprompting.ResolveAuthoredPromptSource(
		we.FileSystem,
		workstationDef.PromptSourcePath,
		!workstationDef.PromptSourceIsTemplate,
	)
	if err != nil {
		return err
	}
	if workstationDef.PromptSourceIsTemplate {
		workstationDef.PromptTemplate = prompt
		return nil
	}
	workstationDef.Body = prompt
	workstationDef.PromptTemplate = prompt
	return nil
}

func (we *WorkstationExecutor) workstationPromptSource(
	workstationName string,
	workstationDef *interfaces.FactoryWorkstationConfig,
) interfaces.PromptSource {
	if lookup, ok := we.RuntimeConfig.(interfaces.RuntimePromptSourceLookup); ok {
		if source, found := lookup.WorkstationPromptSource(workstationName); found {
			return source
		}
	}
	if workstationDef == nil {
		return interfaces.PromptSource{}
	}
	return interfaces.PromptSource{
		Path:       workstationDef.PromptSourcePath,
		IsTemplate: workstationDef.PromptSourceIsTemplate,
	}
}

func promptSourceFailureResult(
	dispatch work.WorkDispatch,
	role string,
	name string,
	path string,
	err error,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error: fmt.Sprintf(
			"%s %q prompt source %s: %v",
			role,
			name,
			path,
			err,
		),
		Diagnostics: diagnostics,
		Metrics: workerexecution.WorkMetrics{
			Duration: duration,
		},
	}
}
