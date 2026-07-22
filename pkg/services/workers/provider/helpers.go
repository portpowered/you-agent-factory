package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/opencode"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

// Factory is a validated, reusable provider-worker construction component.
// Process composition selects its command and PTY edges once; runtime loading
// supplies only worker-specific options.
type Factory struct {
	commandRunner       CommandRunner
	commandClock        workerprocess.Clock
	agyPTYAllocator     agypty.PTYAllocator
	contentMaterializer work.ContentMaterializer
	resolveSymlinks     workerexecution.ResolveExecutableSymlinks
	executableLocator   platformprocess.ExecutableLocator
	executableInspector platformfilesystem.PathInspector
	executableFiles     platformfilesystem.ReadOpener
	operatingSystem     workerexecution.OperatingSystem
	temporaryFiles      platformfilesystem.TemporaryFileSystem
}

// NewFactory validates the process edges used by every provider-backed worker.
func NewFactory(
	commandRunner CommandRunner,
	commandClock workerprocess.Clock,
	agyPTYAllocator agypty.PTYAllocator,
	resolveSymlinks workerexecution.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workerexecution.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (*Factory, error) {
	if commandRunner == nil {
		return nil, errors.New("construct provider-backed worker factory: command runner is required")
	}
	if commandClock == nil {
		return nil, errors.New("construct provider-backed worker factory: command clock is required")
	}
	if agyPTYAllocator == nil {
		return nil, errors.New("construct provider-backed worker factory: Agy PTY allocator is required")
	}
	if resolveSymlinks == nil {
		return nil, errors.New("construct provider-backed worker factory: executable symlink resolver is required")
	}
	if executableLocator == nil {
		return nil, errors.New("construct provider-backed worker factory: executable locator is required")
	}
	if executableInspector == nil {
		return nil, errors.New("construct provider-backed worker factory: executable path inspector is required")
	}
	if executableFiles == nil {
		return nil, errors.New("construct provider-backed worker factory: executable file reader is required")
	}
	if strings.TrimSpace(string(operatingSystem)) == "" {
		return nil, errors.New("construct provider-backed worker factory: operating system is required")
	}
	var temporaryFiles platformfilesystem.TemporaryFileSystem
	if len(temporaryFileSystems) > 0 {
		temporaryFiles = temporaryFileSystems[0]
	}
	if temporaryFiles == nil {
		return nil, errors.New("construct provider-backed worker factory: temporary filesystem is required")
	}
	return &Factory{
		commandRunner: commandRunner, commandClock: commandClock, agyPTYAllocator: agyPTYAllocator,
		resolveSymlinks: resolveSymlinks, executableLocator: executableLocator,
		executableInspector: executableInspector,
		executableFiles:     executableFiles, operatingSystem: operatingSystem, temporaryFiles: temporaryFiles,
	}, nil
}

// New constructs one provider-backed worker from the factory's validated edges.
func (f *Factory) New(
	skipPermissions bool,
	logger logging.Logger,
	openCodeResolver *opencodeadapter.Resolver,
	progressPublisher InferenceProgressPublisher,
	responseStreamExecutor ResponseStreamExecutor,
	agyFactoryRoot string,
) (*ScriptWrapProvider, error) {
	if f == nil {
		return nil, errors.New("construct provider-backed worker: factory is required")
	}
	if f.commandClock == nil {
		return nil, errors.New("construct provider-backed worker: command clock is required")
	}
	if openCodeResolver == nil {
		var err error
		openCodeResolver, err = opencodeadapter.NewDefaultResolver(
			f.commandRunner, f.resolveSymlinks, f.executableLocator, f.executableFiles,
		)
		if err != nil {
			return nil, fmt.Errorf("construct provider-backed worker: %w", err)
		}
	}
	provider := NewScriptWrapProviderWithDependencies(
		skipPermissions,
		logger,
		workerprocess.CommandRunnerWithLogging(f.commandRunner, logger, f.commandClock),
		openCodeResolver,
		progressPublisher,
		responseStreamExecutor,
		agyFactoryRoot,
		f.agyPTYAllocator,
		f.contentMaterializer,
		f.commandClock,
	)
	provider.operatingSystem = string(f.operatingSystem)
	provider.temporaryFiles = f.temporaryFiles
	provider.agyExecutableDependencies = agyadapter.ExecutableDependencies{
		Locator: f.executableLocator, Inspector: f.executableInspector,
	}
	return provider, nil
}

// WithCommandRunner returns a validated copy with a per-runtime wrapper edge.
func (f *Factory) WithCommandRunner(runner CommandRunner) (*Factory, error) {
	if f == nil {
		return nil, errors.New("construct provider-backed worker factory: base factory is required")
	}
	if runner == nil {
		return f, nil
	}
	factory, err := NewFactory(
		runner, f.commandClock, f.agyPTYAllocator, f.resolveSymlinks,
		f.executableLocator, f.executableInspector, f.executableFiles, f.operatingSystem, f.temporaryFiles,
	)
	if err != nil {
		return nil, err
	}
	factory.contentMaterializer = f.contentMaterializer
	return factory, nil
}

// WithContentMaterializer returns a copy using the injected Work capability.
func (f *Factory) WithContentMaterializer(materializer work.ContentMaterializer) *Factory {
	if f == nil {
		return nil
	}
	copy := *f
	copy.contentMaterializer = materializer
	return &copy
}

type CommandRunner = workerprocess.CommandRunner
type CommandRequest = workerprocess.CommandRequest
type CommandResult = workerprocess.CommandResult
type ExecCommandRunner = workerprocess.ExecCommandRunner
type LoggingCommandRunner = workerprocess.LoggingCommandRunner

const (
	omitZeroWorkerEventExitCode = false
)

const (
	RedactedCommandEnvValue     = "<redacted>"
	MetadataOnlyCommandEnvValue = "<metadata-only>"
	ProviderDefaultModel        = "provider-default"
	ProviderInvocationPrepared  = "provider.invocation_prepared"
	ProviderFailureNormalized   = "provider.failure_normalized"
	RedactedProviderArgValue    = "<redacted>"
	RedactedProviderPrompt      = "<redacted:prompt>"
)

var providerFlagsWithSafeValues = map[string]struct{}{
	"--approval-mode": {}, "--model": {}, "--output-format": {}, "--sandbox": {},
}

var providerFlagsWithSensitiveValues = map[string]struct{}{
	"--agent": {}, "--cd": {}, "--dir": {}, "--image": {}, "--prompt": {},
	"--resume": {}, "--resume-id": {}, "--session": {}, "--system-prompt": {},
	"--workspace": {}, "--worktree": {},
}

func providerModelForLog(model string) string {
	if normalized := strings.TrimSpace(model); normalized != "" {
		return normalized
	}
	return ProviderDefaultModel
}

func sanitizeProviderArgs(provider string, args []string) []string {
	sanitized := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if providerInlineArgIsSensitive(arg) {
			sanitized = append(sanitized, strings.SplitN(arg, "=", 2)[0]+"="+RedactedProviderArgValue)
			continue
		}
		sanitized = append(sanitized, arg)
		if _, ok := providerFlagsWithSafeValues[arg]; ok && index+1 < len(args) {
			index++
			sanitized = append(sanitized, args[index])
			continue
		}
		if _, ok := providerFlagsWithSensitiveValues[arg]; ok && index+1 < len(args) {
			index++
			sanitized = append(sanitized, RedactedProviderArgValue)
			continue
		}
		if providerArgIsSensitivePositional(provider, args, index) {
			sanitized[len(sanitized)-1] = RedactedProviderPrompt
		}
	}
	return sanitized
}

func providerInlineArgIsSensitive(arg string) bool {
	name, _, ok := strings.Cut(arg, "=")
	if !ok {
		return false
	}
	normalized := strings.ToLower(name)
	return strings.Contains(normalized, "key") || strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "prompt") || strings.Contains(normalized, "credential")
}

func providerArgIsSensitivePositional(provider string, args []string, index int) bool {
	if strings.HasPrefix(args[index], "-") {
		return false
	}
	if index == 0 && (args[index] == "exec" || args[index] == "chat" || args[index] == "run") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case string(modelprovider.ProviderCodex):
		return args[index] != "-"
	default:
		return true
	}
}

func providerPreparedLogFields(ctx context.Context, req workerexecution.ProviderInferenceRequest, execReq CommandRequest) []any {
	fields := providerLogFields(req,
		"event_name", ProviderInvocationPrepared,
		"model", providerModelForLog(req.Model),
		"command", execReq.Command,
		"args", sanitizeProviderArgs(req.ModelProvider, execReq.Args),
		"working_dir", execReq.WorkDir,
		"stdin_bytes", len(execReq.Stdin),
		"stdin_sha256", sha256Hex(execReq.Stdin))
	if deadline, ok := ctx.Deadline(); ok {
		fields = append(fields, "deadline", deadline.UTC().Format(time.RFC3339Nano))
	}
	return fields
}

func providerFailureLogFields(
	req workerexecution.ProviderInferenceRequest,
	providerErr *ProviderError,
	result CommandResult,
	duration time.Duration,
) []any {
	decision := WorkFailureDecisionFromProviderError(providerErr)
	fields := providerLogFields(req,
		"event_name", ProviderFailureNormalized,
		"model", providerModelForLog(req.Model),
		"failure_reason", providerErr.Type,
		"failure_message", safeProviderFailureLogMessage(req.ModelProvider, providerErr),
		"retryable", decision.Retryable,
		"duration_ms", duration.Milliseconds())
	fields = appendProviderSessionLogFields(fields, providerErr.ProviderSession)
	if result.ExitCode != 0 {
		fields = append(fields, "exit_code", result.ExitCode)
	}
	return fields
}

func safeProviderFailureLogMessage(provider string, providerErr *ProviderError) string {
	// Codex exit failures are parsed into bounded, audited messages. Execution
	// errors retain raw command diagnostics in the returned error, so they must
	// use the same fixed reason-based messages as the other providers.
	if strings.EqualFold(strings.TrimSpace(provider), string(modelprovider.ProviderCodex)) && !isProviderExecutionCause(providerErr.Cause) {
		return providerErr.Message
	}
	switch providerErr.Type {
	case workerexecution.WorkFailureTypeAuthFailure:
		return "Provider authentication failed."
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return "Provider rejected the request as invalid."
	case workerexecution.WorkFailureTypeThrottled:
		return "Provider is temporarily unavailable due to usage or capacity limits."
	case workerexecution.WorkFailureTypeInternalServerError:
		return "Provider encountered a temporary server error."
	case workerexecution.WorkFailureTypeTimeout:
		return "Provider request timed out."
	case workerexecution.WorkFailureTypeMisconfigured:
		return "Provider command could not be started."
	case workerexecution.WorkFailureTypeMissingExecutable:
		return "Provider executable could not be found."
	case workerexecution.WorkFailureTypeCommandLineTooLong:
		return "Provider command exceeded the operating system command-line limit."
	default:
		return "Provider execution failed."
	}
}

func isProviderExecutionCause(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var execErr *exec.Error
	return errors.As(err, &execErr)
}

// SafeProviderFailureDetail returns the allowlisted public diagnostic for a
// normalized provider failure. It intentionally excludes causes and raw
// provider output so durable projections can persist it safely.
func SafeProviderFailureDetail(providerErr *ProviderError) *workerexecution.FailureDetail {
	if providerErr == nil {
		return nil
	}
	return &workerexecution.FailureDetail{
		Reason:  providerErr.Type,
		Message: safeProviderFailureLogMessage("", providerErr),
	}
}

func sha256Hex(input []byte) string {
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

type CommandEnvClassification string

const (
	CommandEnvClassificationSafe         CommandEnvClassification = "safe"
	CommandEnvClassificationRedacted     CommandEnvClassification = "redacted"
	CommandEnvClassificationMetadataOnly CommandEnvClassification = "metadata_only"
)

type CommandEnvDiagnosticProjection struct {
	Count  int               `json:"count"`
	Keys   []string          `json:"keys,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

var safeCommandEnvKeys = map[string]struct{}{
	"CI":                  {},
	"CGO_ENABLED":         {},
	"EDITOR":              {},
	"FORCE_COLOR":         {},
	"GIT_EDITOR":          {},
	"GIT_MERGE_AUTOEDIT":  {},
	"GIT_SEQUENCE_EDITOR": {},
	"GIT_TERMINAL_PROMPT": {},
	"GOARCH":              {},
	"GOOS":                {},
	"NO_COLOR":            {},
	"OS":                  {},
	"RUNNER_OS":           {},
	"TERM":                {},
	"VISUAL":              {},
}

var sensitiveCommandEnvNameFragments = []string{
	"TOKEN",
	"SECRET",
	"PASSWORD",
	"PASS",
	"KEY",
	"CREDENTIAL",
	"CREDENTIALS",
	"AUTH",
	"ANTHROPIC",
	"OPENAI",
	"GEMINI",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
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

func workLogFields(metadata work.ExecutionMetadata, keysAndValues ...any) []any {
	fields := []any{
		"request_id", metadata.RequestID,
		"trace_id", metadata.TraceID,
		"work_id", primaryWorkID(metadata.WorkIDs),
		"work_ids", cloneWorkIDs(metadata.WorkIDs),
	}
	return append(fields, keysAndValues...)
}

func providerLogFields(req workerexecution.ProviderInferenceRequest, keysAndValues ...any) []any {
	fields := workLogFields(req.Dispatch.Execution,
		"dispatch_id", req.Dispatch.DispatchID,
		"provider", workerexecution.CanonicalProviderSessionProvider(req.ModelProvider),
		"worker_type", firstNonEmpty(req.WorkerType, req.Dispatch.WorkerType),
		"workstation", req.Dispatch.WorkstationName)
	return append(fields, keysAndValues...)
}

func appendProviderSessionLogFields(fields []any, session *workerexecution.ProviderSessionMetadata) []any {
	if session == nil {
		return fields
	}
	return append(fields,
		"provider_session_provider", workerexecution.CanonicalProviderSessionProvider(session.Provider),
		"provider_session_kind", session.Kind,
		"provider_session_id", session.ID)
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

func firstImageContentPart(rawTokens []any) (int, int, work.WorkContentPart, bool) {
	for tokenIndex, token := range cloneInputTokens(rawTokens) {
		for partIndex, part := range token.Color.Content {
			if part.Type == work.WorkContentPartTypeImage {
				return tokenIndex, partIndex, part, true
			}
		}
	}
	return 0, 0, work.WorkContentPart{}, false
}

func unsupportedImageContentError(rawTokens []any, executionPath string) error {
	tokenIndex, partIndex, part, ok := firstImageContentPart(rawTokens)
	if !ok {
		return nil
	}
	if part.File == "" {
		return fmt.Errorf("input_tokens[%d].color.content[%d]: image content is not supported by %s; configure modelProvider codex for image-capable execution", tokenIndex, partIndex, executionPath)
	}
	return fmt.Errorf("input_tokens[%d].color.content[%d].file: image content %q is not supported by %s; configure modelProvider codex for image-capable execution", tokenIndex, partIndex, part.File, executionPath)
}

func ClassifyCommandEnvKey(name string) CommandEnvClassification {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if normalized == "" {
		return CommandEnvClassificationMetadataOnly
	}
	for _, fragment := range sensitiveCommandEnvNameFragments {
		if strings.Contains(normalized, fragment) {
			return CommandEnvClassificationRedacted
		}
	}
	if _, ok := safeCommandEnvKeys[normalized]; ok {
		return CommandEnvClassificationSafe
	}
	return CommandEnvClassificationMetadataOnly
}

func ProjectCommandEnvForDiagnostics(env []string) CommandEnvDiagnosticProjection {
	projection := CommandEnvDiagnosticProjection{}
	if len(env) == 0 {
		return projection
	}

	seenKeys := make(map[string]struct{}, len(env))
	values := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		projection.Count++
		seenKeys[name] = struct{}{}
		switch ClassifyCommandEnvKey(name) {
		case CommandEnvClassificationSafe:
			values[name] = value
		case CommandEnvClassificationRedacted:
			values[name] = RedactedCommandEnvValue
		default:
			values[name] = MetadataOnlyCommandEnvValue
		}
	}

	if len(seenKeys) > 0 {
		projection.Keys = make([]string, 0, len(seenKeys))
		for name := range seenKeys {
			projection.Keys = append(projection.Keys, name)
		}
		sort.Strings(projection.Keys)
	}
	if len(values) > 0 {
		projection.Values = values
	}
	return projection
}

func commandEnvDiagnosticMetadata(projection CommandEnvDiagnosticProjection) map[string]string {
	if projection.Count == 0 && len(projection.Keys) == 0 {
		return nil
	}
	return map[string]string{
		"env_count": fmt.Sprintf("%d", projection.Count),
		"env_keys":  strings.Join(projection.Keys, ","),
	}
}

func workDiagnosticsForInferenceRequest(req workerexecution.ProviderInferenceRequest) *workerexecution.WorkDiagnostics {
	requestMetadata := map[string]string{
		"worker_type":       firstNonEmpty(req.WorkerType, req.Dispatch.WorkerType),
		"workstation_type":  req.WorkstationType,
		"worktree":          req.Worktree,
		"working_directory": req.WorkingDirectory,
		"session_id":        req.SessionID,
		"output_schema":     req.OutputSchema,
	}
	if req.OpenCodeAgent != "" {
		requestMetadata["opencode_agent"] = req.OpenCodeAgent
	}
	return &workerexecution.WorkDiagnostics{
		RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
			SystemPromptHash: hashText(req.SystemPrompt),
			UserMessageHash:  hashText(req.UserMessage),
		},
		Provider: &workerexecution.ProviderDiagnostic{
			Provider:        req.ModelProvider,
			Model:           req.Model,
			RequestMetadata: requestMetadata,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func withInferenceResponseDiagnostics(base *workerexecution.WorkDiagnostics, resp workerexecution.InferenceResponse, retryCount int) *workerexecution.WorkDiagnostics {
	diagnostics := workerexecution.CloneWorkDiagnostics(base)
	diagnostics = mergeWorkDiagnostics(diagnostics, resp.Diagnostics)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["content_bytes"] = fmt.Sprintf("%d", len(resp.Content))
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	if resp.ProviderSession != nil {
		diagnostics.Provider.ResponseMetadata["provider_session_provider"] = workerexecution.CanonicalProviderSessionProvider(resp.ProviderSession.Provider)
		diagnostics.Provider.ResponseMetadata["provider_session_kind"] = resp.ProviderSession.Kind
		diagnostics.Provider.ResponseMetadata["provider_session_id"] = resp.ProviderSession.ID
	}
	return diagnostics
}

func withInferenceErrorDiagnostics(base *workerexecution.WorkDiagnostics, err error, retryCount int) *workerexecution.WorkDiagnostics {
	diagnostics := workerexecution.CloneWorkDiagnostics(base)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["error"] = err.Error()
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	return diagnostics
}

func commandDiagnostics(req CommandRequest, result CommandResult, duration time.Duration, timedOut bool) *workerexecution.WorkDiagnostics {
	envProjection := ProjectCommandEnvForDiagnostics(req.Env)
	return &workerexecution.WorkDiagnostics{
		Command: &workerexecution.CommandDiagnostic{
			Command:    req.Command,
			Args:       append([]string(nil), req.Args...),
			Stdin:      string(req.Stdin),
			Env:        envProjection.Values,
			Stdout:     string(result.Stdout),
			Stderr:     string(result.Stderr),
			ExitCode:   result.ExitCode,
			TimedOut:   timedOut,
			Duration:   duration,
			WorkingDir: req.WorkDir,
		},
		Metadata: commandEnvDiagnosticMetadata(envProjection),
	}
}

func mergeWorkDiagnostics(base, overlay *workerexecution.WorkDiagnostics) *workerexecution.WorkDiagnostics {
	if base == nil {
		return workerexecution.CloneWorkDiagnostics(overlay)
	}
	if overlay == nil {
		return base
	}
	overlay = workerexecution.CloneWorkDiagnostics(overlay)
	if overlay.RenderedPrompt != nil {
		base.RenderedPrompt = overlay.RenderedPrompt
	}
	if overlay.Provider != nil {
		base.Provider = mergeProviderDiagnostic(base.Provider, overlay.Provider)
	}
	if overlay.Invocation != nil {
		base.Invocation = workerexecution.CloneWorkDiagnostics(&workerexecution.WorkDiagnostics{Invocation: overlay.Invocation}).Invocation
	}
	if overlay.Command != nil {
		base.Command = overlay.Command
	}
	if overlay.Panic != nil {
		base.Panic = overlay.Panic
	}
	if len(overlay.Metadata) > 0 {
		if base.Metadata == nil {
			base.Metadata = make(map[string]string, len(overlay.Metadata))
		}
		for k, v := range overlay.Metadata {
			base.Metadata[k] = v
		}
	}
	return base
}

func mergeProviderDiagnostic(base, overlay *workerexecution.ProviderDiagnostic) *workerexecution.ProviderDiagnostic {
	if base == nil {
		return overlay
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if overlay.Model != "" {
		base.Model = overlay.Model
	}
	if len(overlay.RequestMetadata) > 0 {
		if base.RequestMetadata == nil {
			base.RequestMetadata = make(map[string]string, len(overlay.RequestMetadata))
		}
		for k, v := range overlay.RequestMetadata {
			base.RequestMetadata[k] = v
		}
	}
	if len(overlay.ResponseMetadata) > 0 {
		if base.ResponseMetadata == nil {
			base.ResponseMetadata = make(map[string]string, len(overlay.ResponseMetadata))
		}
		for k, v := range overlay.ResponseMetadata {
			base.ResponseMetadata[k] = v
		}
	}
	return base
}

func hashText(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func workerEventExitCode(exitCode int, present bool, includeZero bool) *int {
	if !present {
		return nil
	}
	if exitCode == 0 && !includeZero {
		return nil
	}
	exitCodeCopy := exitCode
	return &exitCodeCopy
}
