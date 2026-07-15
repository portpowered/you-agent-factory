package invocation

import (
	"errors"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
)

const (
	InvocationMetricNormalizationAttempts = "invocation.normalization_attempts"
	InvocationMetricNormalizationSuccess  = "invocation.normalization_success"
	InvocationMetricNormalizationFailure  = "invocation.normalization_failure"
	InvocationMetricInterpolationFailure  = "invocation.interpolation_failure"
	InvocationMetricAttempts              = "invocation.attempts"
	InvocationMetricSuccess               = "invocation.success"
	InvocationMetricFailure               = "invocation.failure"
	InvocationMetricUnresolvedPrimary     = "invocation.unresolved_primary"
	InvocationMetricFallbackPolicyUsed    = "invocation.fallback_policy_used"
	InvocationMetricResultType            = "invocation.result_type"
)

const (
	invocationPolicySubmittedWorkTerminal = "SUBMITTED_WORK_TERMINAL"
	invocationPolicyExplicit              = "EXPLICIT"
	invocationPolicyModeAuthored          = "authored"
	invocationPolicyModeFallback          = "fallback"
)

// SessionInvocationMetric is one invocation-owned counter emission.
type SessionInvocationMetric struct {
	Name   string
	Labels map[string]string
}

// SessionInvocationLogRecord is one invocation-owned structured log record.
// Fields intentionally contain diagnostics only; invocation content never
// crosses this boundary.
type SessionInvocationLogRecord struct {
	Level   string
	Message string
	Fields  map[string]any
	Error   error
}

// PackagedInvocationTelemetry describes stable packaged-factory observability
// without coupling the invocation package to a particular packaged factory.
type PackagedInvocationTelemetry struct {
	Active         func(*interfaces.FactoryConfig) bool
	FactoryName    string
	Backend        string
	AttemptsMetric string
	SuccessMetric  string
	FailureMetric  string
	NotReadyMetric string
	LoadingClass   string
	SuccessClass   string
	NotReadyClass  string
}

// SessionInvocationTelemetryDependencies are the output sinks and optional
// packaged-factory descriptor used by the canonical telemetry policy.
type SessionInvocationTelemetryDependencies struct {
	RecordMetric func(SessionInvocationMetric)
	RecordLog    func(SessionInvocationLogRecord)
	Packaged     *PackagedInvocationTelemetry
}

type sessionInvocationTelemetry struct {
	deps SessionInvocationTelemetryDependencies
}

// NewSessionInvocationTelemetry creates the canonical invocation metric and
// safe-log policy. Runtime edges only adapt the emitted records to concrete
// logging and metrics implementations.
func NewSessionInvocationTelemetry(deps SessionInvocationTelemetryDependencies) SessionInvocationTelemetry {
	return &sessionInvocationTelemetry{deps: deps}
}

// SessionInvocationPackagedTelemetry is invoked by SessionOwner at the three
// packaged-only lifecycle points.
type SessionInvocationPackagedTelemetry interface {
	PackagedInvocationActive(string, SessionInvocationWaitInput)
	PackagedInvocationCompleted(string, SessionInvocationWaitInput, workinvocation.PrimaryResultSelection)
	PackagedInvocationFailed(string, SessionInvocationWaitInput, SessionInvocationSpecialFailure)
}

func (t *sessionInvocationTelemetry) NormalizationAttempt(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel) {
	t.metric(InvocationMetricNormalizationAttempts, invocationMetricLabels(cfg, source))
}

func (t *sessionInvocationTelemetry) NormalizationFailure(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel, err error) {
	t.metric(InvocationMetricNormalizationFailure, mergeMetricLabels(invocationMetricLabels(cfg, source), invocationErrorMetricLabels(err)))
}

func (t *sessionInvocationTelemetry) NormalizationSuccess(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel) {
	t.metric(InvocationMetricNormalizationSuccess, invocationMetricLabels(cfg, source))
}

func (t *sessionInvocationTelemetry) InterpolationFailure(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel, err error) {
	t.metric(InvocationMetricInterpolationFailure, mergeMetricLabels(invocationMetricLabels(cfg, source), invocationErrorMetricLabels(err)))
}

func (t *sessionInvocationTelemetry) SubmissionFailure(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel, _ error) {
	t.metric(InvocationMetricFailure, invocationMetricLabels(cfg, source))
}

func (t *sessionInvocationTelemetry) InvocationSubmitted(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel) {
	t.metric(InvocationMetricAttempts, invocationMetricLabels(cfg, source))
	if policyModeForInvocation(cfg.InvocationReturn) == invocationPolicyModeFallback {
		t.metric(InvocationMetricFallbackPolicyUsed, invocationMetricLabels(cfg, source))
	}
	if packaged := t.packaged(cfg); packaged != nil {
		t.packagedMetric(packaged.AttemptsMetric, source, nil)
	}
}

func (t *sessionInvocationTelemetry) InvocationCompleted(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel, result []work.WorkContentPart) {
	t.metric(InvocationMetricSuccess, invocationMetricLabels(cfg, source))
	t.metric(InvocationMetricResultType, mergeMetricLabels(invocationMetricLabels(cfg, source), map[string]string{
		"result_type": primaryResultMetricType(result),
	}))
}

func (t *sessionInvocationTelemetry) InvocationFailed(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel, errorCode string) {
	t.metric(InvocationMetricFailure, invocationMetricLabels(cfg, source))
	if errorCode == string(workinvocation.PrimaryResultErrorCodeUnresolved) {
		t.metric(InvocationMetricUnresolvedPrimary, invocationMetricLabels(cfg, source))
	}
}

func (t *sessionInvocationTelemetry) LogArgumentFailure(
	sessionID string,
	source workinvocation.InputSourceLabel,
	cfg *interfaces.FactoryConfig,
	normalized *workinvocation.NormalizedArguments,
	err error,
	failureClass string,
) {
	fields := invocationLogFields(sessionID, source, invocationReturn(cfg), cfg)
	fields["status"] = string(interfaces.InvocationTerminalStatusFailed)
	fields["failure_class"] = failureClass
	var argumentErr *workinvocation.ArgumentError
	if errors.As(err, &argumentErr) {
		addArgumentErrorFields(fields, normalized, argumentErr)
	}
	t.log("warn", "factory session invocation argument failure", fields, err)
}

func (t *sessionInvocationTelemetry) LogSubmissionFailure(sessionID string, source workinvocation.InputSourceLabel, cfg *interfaces.FactoryConfig, err error) {
	fields := invocationLogFields(sessionID, source, invocationReturn(cfg), nil)
	fields["status"] = string(interfaces.InvocationTerminalStatusFailed)
	fields["error_code"] = string(interfaces.InvocationErrorCodeRuntimeFailure)
	fields["failure_class"] = "runtime_failure"
	t.log("warn", "factory session invocation failed", fields, err)
}

func (t *sessionInvocationTelemetry) LogInvocationSubmitted(
	sessionID string,
	source workinvocation.InputSourceLabel,
	cfg *interfaces.FactoryConfig,
	result work.WorkRequestSubmitResult,
) {
	if packaged := t.packaged(cfg); packaged != nil {
		fields := t.packagedLogFields(sessionID, source, cfg.InvocationReturn, cfg, packaged)
		fields["request_id"], fields["trace_id"] = result.RequestID, result.TraceID
		fields["readiness_outcome"] = packaged.LoadingClass
		t.log("info", "packaged tts invocation submitted", fields, nil)
	}
	fields := invocationLogFields(sessionID, source, cfg.InvocationReturn, cfg)
	fields["request_id"], fields["trace_id"] = result.RequestID, result.TraceID
	t.log("info", "factory session invocation submitted", fields, nil)
}

func (t *sessionInvocationTelemetry) LogInvocationCompleted(sessionID string, input SessionInvocationWaitInput, selection workinvocation.PrimaryResultSelection) {
	fields := invocationLogFields(sessionID, input.InputSource, input.InvocationReturn, input.FactoryConfig)
	fields["request_id"], fields["trace_id"] = input.RequestID, input.TraceID
	fields["status"] = string(interfaces.InvocationTerminalStatusCompleted)
	fields["resolved_work_id"], fields["resolved_work_type"] = selection.WorkID, selection.WorkTypeName
	fields["resolved_work_name"], fields["resolved_terminal_state"] = selection.WorkName, selection.TerminalState
	fields["result_type"] = primaryResultMetricType(selection.PrimaryResult)
	t.log("info", "factory session invocation completed", fields, nil)
}

func (t *sessionInvocationTelemetry) LogInvocationFailed(sessionID string, input SessionInvocationWaitInput, result FactoryInvocationResult, failureClass string) {
	fields := invocationLogFields(sessionID, input.InputSource, input.InvocationReturn, input.FactoryConfig)
	fields["request_id"], fields["trace_id"] = input.RequestID, input.TraceID
	fields["status"], fields["error_code"], fields["failure_class"] = string(result.Status), result.ErrorCode, failureClass
	t.log("warn", "factory session invocation failed", fields, nil)
}

func (t *sessionInvocationTelemetry) PackagedInvocationActive(sessionID string, input SessionInvocationWaitInput) {
	packaged := t.packaged(input.FactoryConfig)
	if packaged == nil {
		return
	}
	fields := t.packagedLogFields(sessionID, input.InputSource, input.InvocationReturn, input.FactoryConfig, packaged)
	fields["request_id"], fields["trace_id"] = input.RequestID, input.TraceID
	fields["readiness_outcome"] = packaged.LoadingClass
	t.log("info", "packaged tts invocation loading", fields, nil)
}

func (t *sessionInvocationTelemetry) PackagedInvocationCompleted(sessionID string, input SessionInvocationWaitInput, selection workinvocation.PrimaryResultSelection) {
	packaged := t.packaged(input.FactoryConfig)
	if packaged == nil {
		return
	}
	t.packagedMetric(packaged.SuccessMetric, input.InputSource, map[string]string{"readiness_outcome": packaged.SuccessClass})
	fields := t.packagedLogFields(sessionID, input.InputSource, input.InvocationReturn, input.FactoryConfig, packaged)
	fields["request_id"], fields["trace_id"] = input.RequestID, input.TraceID
	fields["status"] = string(interfaces.InvocationTerminalStatusCompleted)
	fields["resolved_work_id"], fields["resolved_work_type"] = selection.WorkID, selection.WorkTypeName
	fields["readiness_outcome"] = packaged.SuccessClass
	t.log("info", "packaged tts invocation completed", fields, nil)
}

func (t *sessionInvocationTelemetry) PackagedInvocationFailed(sessionID string, input SessionInvocationWaitInput, failure SessionInvocationSpecialFailure) {
	packaged := t.packaged(input.FactoryConfig)
	if packaged == nil {
		return
	}
	t.packagedMetric(packaged.FailureMetric, input.InputSource, map[string]string{"failure_class": failure.FailureClass})
	if failure.FailureClass == packaged.NotReadyClass {
		t.packagedMetric(packaged.NotReadyMetric, input.InputSource, nil)
	}
	fields := t.packagedLogFields(sessionID, input.InputSource, input.InvocationReturn, input.FactoryConfig, packaged)
	fields["request_id"], fields["trace_id"] = input.RequestID, input.TraceID
	fields["status"] = string(interfaces.InvocationTerminalStatusFailed)
	fields["error_code"], fields["failure_class"] = failure.ErrorCode, failure.FailureClass
	fields["readiness_outcome"] = failure.FailureClass
	t.log("warn", "packaged tts invocation failed", fields, nil)
}

func (t *sessionInvocationTelemetry) metric(name string, labels map[string]string) {
	if t == nil || t.deps.RecordMetric == nil || strings.TrimSpace(name) == "" {
		return
	}
	t.deps.RecordMetric(SessionInvocationMetric{Name: name, Labels: cloneMetricLabels(labels)})
}

func (t *sessionInvocationTelemetry) log(level, message string, fields map[string]any, err error) {
	if t == nil || t.deps.RecordLog == nil {
		return
	}
	t.deps.RecordLog(SessionInvocationLogRecord{Level: level, Message: message, Fields: fields, Error: err})
}

func (t *sessionInvocationTelemetry) packaged(cfg *interfaces.FactoryConfig) *PackagedInvocationTelemetry {
	if t == nil || t.deps.Packaged == nil || t.deps.Packaged.Active == nil || !t.deps.Packaged.Active(cfg) {
		return nil
	}
	return t.deps.Packaged
}

func (t *sessionInvocationTelemetry) packagedMetric(name string, source workinvocation.InputSourceLabel, extra map[string]string) {
	packaged := t.deps.Packaged
	t.metric(name, mergeMetricLabels(map[string]string{
		"input_source": string(source), "packaged_factory": packaged.FactoryName,
	}, extra))
}

func (t *sessionInvocationTelemetry) packagedLogFields(
	sessionID string,
	source workinvocation.InputSourceLabel,
	policy *interfaces.InvocationReturnConfig,
	cfg *interfaces.FactoryConfig,
	packaged *PackagedInvocationTelemetry,
) map[string]any {
	fields := invocationLogFields(sessionID, source, policy, cfg)
	fields["packaged_factory_name"], fields["tts_backend"] = packaged.FactoryName, packaged.Backend
	return fields
}

func addArgumentErrorFields(fields map[string]any, normalized *workinvocation.NormalizedArguments, argumentErr *workinvocation.ArgumentError) {
	if code := strings.TrimSpace(string(argumentErr.Code)); code != "" {
		fields["error_code"] = code
	}
	if parameter := strings.TrimSpace(argumentErr.Parameter); parameter != "" {
		fields["argument_name"] = parameter
	}
	if argument := strings.TrimSpace(argumentErr.Argument); argument != "" {
		fields["argument_key"] = argument
	}
	if sourceKind := strings.TrimSpace(string(argumentErr.SourceKind)); sourceKind != "" {
		fields["argument_source_kind"] = sourceKind
	} else if kinds := invocationArgumentSourceKinds(normalized, argumentErr.Parameter); kinds != "" {
		fields["argument_source_kind"] = kinds
	}
	if redacted, count := invocationArgumentRedactionState(normalized, argumentErr.Parameter); redacted || count > 0 {
		fields["argument_value_redacted"], fields["argument_value_count"] = redacted, count
	}
}

func invocationLogFields(sessionID string, source workinvocation.InputSourceLabel, policy *interfaces.InvocationReturnConfig, cfg *interfaces.FactoryConfig) map[string]any {
	fields := map[string]any{
		"session_id": sessionID, "input_source": string(source),
		"invocation_return_policy":      invocationPolicyName(policy),
		"invocation_return_policy_mode": policyModeForInvocation(policy),
		"policy_resolution_path":        invocationPolicyResolutionPath(policy),
	}
	for key, value := range invocationFactoryLabels(cfg) {
		fields[key] = value
	}
	return fields
}

func invocationMetricLabels(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel) map[string]string {
	return mergeMetricLabels(map[string]string{"input_source": string(source)}, invocationFactoryLabels(cfg))
}

func invocationFactoryLabels(cfg *interfaces.FactoryConfig) map[string]string {
	if cfg == nil {
		return nil
	}
	labels := map[string]string{}
	if value := strings.TrimSpace(cfg.Name); value != "" {
		labels["factory_name"] = value
	}
	if value := strings.TrimSpace(cfg.Project); value != "" {
		labels["factory_project"] = value
	}
	if value := workinvocation.InvocationSignatureHash(cfg.InvocationSignature); value != "" {
		labels["signature_hash"] = value
	}
	return labels
}

func invocationErrorMetricLabels(err error) map[string]string {
	var argumentErr *workinvocation.ArgumentError
	if errors.As(err, &argumentErr) && strings.TrimSpace(string(argumentErr.Code)) != "" {
		return map[string]string{"error_code": string(argumentErr.Code)}
	}
	return nil
}

func invocationArgumentSourceKinds(normalized *workinvocation.NormalizedArguments, parameter string) string {
	argument := invocationNormalizedArgument(normalized, parameter)
	if argument == nil {
		return ""
	}
	seen := map[string]struct{}{}
	for _, source := range argument.Sources {
		if kind := strings.TrimSpace(string(source.Kind)); kind != "" {
			seen[kind] = struct{}{}
		}
	}
	kinds := make([]string, 0, len(seen))
	for kind := range seen {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ",")
}

func invocationArgumentRedactionState(normalized *workinvocation.NormalizedArguments, parameter string) (bool, int) {
	argument := invocationNormalizedArgument(normalized, parameter)
	if argument == nil {
		return false, 0
	}
	redacted := argument.Sensitive
	for _, source := range argument.Sources {
		redacted = redacted || source.Redact
	}
	return redacted, len(argument.Values)
}

func invocationNormalizedArgument(normalized *workinvocation.NormalizedArguments, parameter string) *workinvocation.NormalizedArgument {
	if normalized == nil || strings.TrimSpace(parameter) == "" {
		return nil
	}
	argument, ok := normalized.Arguments[strings.TrimSpace(parameter)]
	if !ok {
		return nil
	}
	return &argument
}

func invocationPolicyName(cfg *interfaces.InvocationReturnConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Policy) == "" {
		return invocationPolicySubmittedWorkTerminal
	}
	return strings.TrimSpace(cfg.Policy)
}

func policyModeForInvocation(cfg *interfaces.InvocationReturnConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Policy) == "" {
		return invocationPolicyModeFallback
	}
	return invocationPolicyModeAuthored
}

func invocationPolicyResolutionPath(cfg *interfaces.InvocationReturnConfig) string {
	if invocationPolicyName(cfg) == invocationPolicyExplicit {
		return "explicit_scoped_terminal_match"
	}
	return "submitted_work_terminal"
}

func primaryResultMetricType(parts []work.WorkContentPart) string {
	if len(parts) == 0 {
		return "empty"
	}
	types := map[string]struct{}{}
	for _, part := range parts {
		partType := strings.TrimSpace(string(part.Type.Normalized()))
		if partType == "" {
			partType = "unknown"
		}
		types[partType] = struct{}{}
	}
	if len(types) == 1 {
		for partType := range types {
			return partType
		}
	}
	names := make([]string, 0, len(types))
	for partType := range types {
		names = append(names, partType)
	}
	sort.Strings(names)
	return "mixed:" + strings.Join(names, "+")
}

func mergeMetricLabels(parts ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, part := range parts {
		for key, value := range part {
			merged[key] = value
		}
	}
	return merged
}

func cloneMetricLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func invocationReturn(cfg *interfaces.FactoryConfig) *interfaces.InvocationReturnConfig {
	if cfg == nil {
		return nil
	}
	return cfg.InvocationReturn
}
