// Package clidiag owns shared helpers for customer-facing CLI diagnostics.
package clidiag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	DefaultSessionID = "~default"

	// DefaultFailureCode is the stable fallback used when a command returns an
	// error that does not carry a CLI diagnostic contract.
	DefaultFailureCode    = "CLI_COMMAND_FAILED"
	defaultFailureMessage = "command failed"

	// LocalInputFailureCode identifies a safe diagnostic for a user-supplied
	// local file that could not be prepared before runtime execution.
	LocalInputFailureCode = "CLI_LOCAL_INPUT_FAILED"
	// FlagConflictFailureCode identifies an incompatible local flag selection.
	FlagConflictFailureCode = "CLI_FLAG_CONFLICT"
)

// CodedError is the small presentation contract shared by command-owned
// errors. Implementations keep their own concrete type and cause; the CLI
// boundary only asks for the already-sanitized code and message.
type CodedError interface {
	error
	CLIErrorCode() string
	CLIErrorMessage() string
}

// FamilyCodedError optionally supplies the public ErrorResponse family along
// with the already-sanitized CLI code and message. Command packages that
// preserve a transport-owned bad-request diagnostic can use this seam without
// making the central renderer inspect or unwrap private causes.
type FamilyCodedError interface {
	CodedError
	CLIErrorFamily() factoryapi.ErrorFamily
}

// ResponseCodedError exposes the complete public ErrorResponse that crossed
// the CLI transport boundary. It is intentionally structural so the central
// renderer does not depend on a concrete HTTP client or command package.
type ResponseCodedError interface {
	error
	CLIErrorResponse() factoryapi.ErrorResponse
}

// HTTPDiagnostic exposes request metadata that is safe to render only from
// the explicit debug channel. The contract is structural so the central CLI
// renderer does not depend on the concrete HTTP adapter.
type HTTPDiagnostic interface {
	error
	CLIHTTPMethod() string
	CLIHTTPURL() string
	CLIHTTPStatus() int
}

// InvocationCodedError supports the existing invocation error vocabulary
// without forcing older command packages to rename their methods.
type InvocationCodedError interface {
	error
	InvocationErrorCode() string
	InvocationErrorMessage() string
}

// Failure is the typed fallback for an unclassified command error. Its public
// message is deliberately constant so the diagnostic cannot expose an
// underlying payload, credential, or implementation detail.
type Failure struct {
	Code    string
	Message string
	Cause   error
}

func (failure *Failure) Error() string {
	if failure == nil {
		return ""
	}
	code := strings.TrimSpace(failure.Code)
	message := strings.TrimSpace(failure.Message)
	if code == "" {
		code = DefaultFailureCode
	}
	if message == "" {
		message = defaultFailureMessage
	}
	if failure.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", code, message, failure.Cause)
	}
	return fmt.Sprintf("%s: %s", code, message)
}

func (failure *Failure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func (failure *Failure) CLIErrorCode() string {
	if failure == nil || strings.TrimSpace(failure.Code) == "" {
		return DefaultFailureCode
	}
	return strings.TrimSpace(failure.Code)
}

func (failure *Failure) CLIErrorMessage() string {
	if failure == nil || strings.TrimSpace(failure.Message) == "" {
		return defaultFailureMessage
	}
	return strings.TrimSpace(failure.Message)
}

// LocalFailure preserves a safe, actionable diagnostic for a client-side
// pre-flight failure. Its message is authored from the submitted input, while
// Cause remains available to callers without being rendered in the default
// diagnostic envelope.
type LocalFailure struct {
	Code    string
	Message string
	Cause   error
}

func (failure *LocalFailure) Error() string {
	if failure == nil {
		return ""
	}
	return strings.TrimSpace(failure.Message)
}

func (failure *LocalFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func (failure *LocalFailure) CLIErrorCode() string {
	if failure == nil || strings.TrimSpace(failure.Code) == "" {
		return LocalInputFailureCode
	}
	return strings.TrimSpace(failure.Code)
}

func (failure *LocalFailure) CLIErrorFamily() factoryapi.ErrorFamily {
	return factoryapi.ErrorFamilyBadRequest
}

func (failure *LocalFailure) CLIErrorMessage() string {
	if failure == nil || strings.TrimSpace(failure.Message) == "" {
		return "local command input could not be prepared"
	}
	return strings.TrimSpace(failure.Message)
}

// NewLocalInputFailure constructs the shared safe presentation for a failed
// local file input. Only the flag and submitted path cross into the default
// output; filesystem and wrapped implementation details remain a cause.
func NewLocalInputFailure(flag, path string, cause error) error {
	flag = strings.TrimSpace(flag)
	path = strings.TrimSpace(path)
	if flag == "" {
		flag = "local"
	}
	return &LocalFailure{
		Code:    LocalInputFailureCode,
		Message: fmt.Sprintf("failed to load %s input %q", flag, path),
		Cause:   cause,
	}
}

// NewFlagConflictFailure constructs a shared safe presentation for two
// incompatible local flags while retaining the original error as a cause.
func NewFlagConflictFailure(first, second string, cause error) error {
	first = strings.TrimSpace(first)
	second = strings.TrimSpace(second)
	return &LocalFailure{
		Code:    FlagConflictFailureCode,
		Message: fmt.Sprintf("%s cannot be used with %s", first, second),
		Cause:   cause,
	}
}

// UsageError preserves a Cobra parsing or argument-validation failure while
// carrying the command path whose help can correct the invocation. It is
// deliberately not a coded ErrorResponse: usage mistakes are client input
// failures, not command failures or server failures.
type UsageError struct {
	CommandPath string
	Cause       error
}

func (failure *UsageError) Error() string {
	if failure == nil || failure.Cause == nil {
		return ""
	}
	return failure.Cause.Error()
}

func (failure *UsageError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// NewUsageError adds the command path needed for the concise help hint. An
// existing usage error is retained so nested boundaries do not duplicate it.
func NewUsageError(commandPath string, cause error) error {
	if cause == nil {
		return nil
	}
	var usage *UsageError
	if errors.As(cause, &usage) {
		return cause
	}
	return &UsageError{CommandPath: strings.TrimSpace(commandPath), Cause: cause}
}

// WriteUsageError renders the standard Cobra-style usage diagnostic and
// reports whether err carries usage metadata.
func WriteUsageError(output io.Writer, err error) bool {
	var usage *UsageError
	if !errors.As(err, &usage) {
		return false
	}
	commandPath := strings.TrimSpace(usage.CommandPath)
	if commandPath == "" {
		commandPath = "you"
	}
	if output != nil {
		_, _ = fmt.Fprintf(output, "Error: %s\nRun '%s --help' for usage.\n", usage.Error(), commandPath)
	}
	MarkDiagnosticRendered(output)
	return true
}

// DiagnosticWriter records whether a command has already rendered its
// structured failure. The writer remains transparent to command handlers, so
// existing command-specific sanitization and response formats are preserved.
type DiagnosticWriter struct {
	output   io.Writer
	rendered bool
	debug    bool
}

type centralDiagnosticsContextKey struct{}

// WithCentralDiagnostics tells command handlers that the process command
// adapter owns the final fallback diagnostic. Direct Cobra callers omit this
// value and retain their historical handler-local presentation behavior.
func WithCentralDiagnostics(ctx context.Context, enabled bool) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, centralDiagnosticsContextKey{}, enabled)
}

func CentralDiagnosticsEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(centralDiagnosticsContextKey{}).(bool)
	return enabled
}

func NewDiagnosticWriter(output io.Writer, debug ...bool) *DiagnosticWriter {
	debugEnabled := len(debug) > 0 && debug[0]
	return &DiagnosticWriter{output: output, debug: debugEnabled}
}

func (writer *DiagnosticWriter) Write(payload []byte) (int, error) {
	if writer == nil || writer.output == nil {
		return len(payload), nil
	}
	return writer.output.Write(payload)
}

func (writer *DiagnosticWriter) MarkDiagnosticRendered() {
	if writer != nil {
		writer.rendered = true
	}
}

func (writer *DiagnosticWriter) DiagnosticRendered() bool {
	return writer != nil && writer.rendered
}

// DebugEnabled reports whether this invocation explicitly selected --debug.
// It is kept on the process-local writer so command handlers can continue to
// use the ordinary io.Writer boundary without receiving a second mode object.
func (writer *DiagnosticWriter) DebugEnabled() bool {
	return writer != nil && writer.debug
}

// MarkDiagnosticRendered marks a writer supplied by the process boundary when
// a command-specific renderer has already emitted its one coded diagnostic.
func MarkDiagnosticRendered(output io.Writer) {
	marker, ok := output.(interface{ MarkDiagnosticRendered() })
	if ok {
		marker.MarkDiagnosticRendered()
	}
}

// DiagnosticRendered reports whether a command-specific renderer has already
// emitted a coded failure to output.
func DiagnosticRendered(output io.Writer) bool {
	marker, ok := output.(interface{ DiagnosticRendered() bool })
	return ok && marker.DiagnosticRendered()
}

// Normalize preserves a command-owned coded error. Unclassified failures are
// wrapped in a safe typed fallback without inspecting terminal output.
func Normalize(err error) error {
	if err == nil {
		return nil
	}
	if hasCode, ok := diagnosticFields(err); ok && strings.TrimSpace(hasCode.code) != "" {
		return err
	}
	return &Failure{
		Code:    DefaultFailureCode,
		Message: defaultFailureMessage,
		Cause:   err,
	}
}

// HasCodedDiagnostic reports whether an error already carries an authored
// presentation contract. Callers that add a more specific local context can
// use it to avoid replacing an existing command or transport diagnostic.
func HasCodedDiagnostic(err error) bool {
	values, ok := diagnosticFields(err)
	return ok && strings.TrimSpace(values.code) != ""
}

type fields struct {
	code        string
	message     string
	family      factoryapi.ErrorFamily
	response    factoryapi.ErrorResponse
	hasResponse bool
}

func diagnosticFields(err error) (fields, bool) {
	if err == nil {
		return fields{}, false
	}
	var responseCoded ResponseCodedError
	if errors.As(err, &responseCoded) {
		response := responseCoded.CLIErrorResponse()
		return fields{
			code:        strings.TrimSpace(string(response.Code)),
			message:     strings.TrimSpace(response.Message),
			family:      response.Family,
			response:    response,
			hasResponse: true,
		}, true
	}
	var familyCoded FamilyCodedError
	if errors.As(err, &familyCoded) {
		return fields{
			code:    strings.TrimSpace(familyCoded.CLIErrorCode()),
			message: strings.TrimSpace(familyCoded.CLIErrorMessage()),
			family:  familyCoded.CLIErrorFamily(),
		}, true
	}
	var coded CodedError
	if errors.As(err, &coded) {
		return fields{
			code:    strings.TrimSpace(coded.CLIErrorCode()),
			message: strings.TrimSpace(coded.CLIErrorMessage()),
		}, true
	}
	var invocation InvocationCodedError
	if errors.As(err, &invocation) {
		return fields{
			code:    strings.TrimSpace(invocation.InvocationErrorCode()),
			message: strings.TrimSpace(invocation.InvocationErrorMessage()),
		}, true
	}
	return fields{}, false
}

// WriteFailure writes the standard single JSON diagnostic for a normalized
// failure. It returns true when err has a coded contract (including Failure).
func WriteFailure(output io.Writer, err error) bool {
	values, ok := diagnosticFields(err)
	if !ok || strings.TrimSpace(values.code) == "" {
		return false
	}
	if strings.TrimSpace(values.message) == "" {
		values.message = defaultFailureMessage
	}
	payload := values.response
	if !values.hasResponse {
		payload = factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCode(values.code),
			Family:  values.family,
			Message: values.message,
		}
	} else {
		payload.Code = factoryapi.ErrorResponseCode(values.code)
		payload.Family = values.family
		payload.Message = values.message
	}
	if payload.Family == "" {
		payload.Family = factoryapi.ErrorFamilyInternalServerError
	}
	if output != nil {
		if encoded, marshalErr := json.Marshal(payload); marshalErr == nil {
			_, _ = fmt.Fprintln(output, string(encoded))
		}
	}
	MarkDiagnosticRendered(output)
	return true
}

// WriteDebugFailure appends safe, line-oriented diagnostics for one failure.
// The normal ErrorResponse remains unchanged; callers opt into this renderer
// only after the explicit --debug flag has been resolved. Cause text is
// bounded and redacted, and HTTP query strings, credentials, and fragments
// are never emitted.
func WriteDebugFailure(output io.Writer, err error) bool {
	if output == nil || err == nil {
		return false
	}
	causes := debugCauseChain(err)
	for index, cause := range causes {
		_, _ = fmt.Fprintf(output, "debug: cause[%d]=%s\n", index, cause)
	}

	var httpFailure HTTPDiagnostic
	if !errors.As(err, &httpFailure) {
		return len(causes) > 0
	}
	method := strings.TrimSpace(httpFailure.CLIHTTPMethod())
	if method == "" {
		method = "<unknown>"
	}
	status := "<unavailable>"
	if code := httpFailure.CLIHTTPStatus(); code != 0 {
		status = strconv.Itoa(code)
	}
	_, _ = fmt.Fprintf(
		output,
		"debug: http method=%s url=%s status=%s\n",
		method,
		sanitizeURL(httpFailure.CLIHTTPURL()),
		status,
	)
	return true
}

const maxDebugCauseDepth = 16

func debugCauseChain(err error) []string {
	causes := make([]string, 0, 2)
	previous := ""
	for depth := 0; err != nil && depth < maxDebugCauseDepth; depth++ {
		message := sanitizeDebugMessage(err.Error())
		if message != previous {
			causes = append(causes, message)
			previous = message
		}
		err = errors.Unwrap(err)
	}
	if err != nil {
		causes = append(causes, "<cause chain truncated>")
	}
	return causes
}

var (
	debugURLPattern                 = regexp.MustCompile(`(?i)https?://[^\s]+`)
	debugSensitiveAssignmentPattern = regexp.MustCompile(
		`(?i)(\b(?:authorization|cookie|set-cookie|password|passwd|secret|token|credential|api[-_]?key|access[-_]?token|refresh[-_]?token|payload|body|request[-_]?body|response[-_]?body|query|prompt|input|content|path|file|filename|filepath|directory|dir|cache|cachepath|environment|env|home|userprofile|homedrive|homepath)\b\s*[:=]\s*)(?:"(?:\\.|[^"])*"|'(?:\\.|[^'])*'|[^\s,;]+(?:\s+[^\s,;]+)?)`,
	)
	debugWindowsPathPattern  = regexp.MustCompile(`(?i)(?:[A-Za-z]:[\\/]|\\\\)[^\s"',;)\]}]+`)
	debugUnixPathPattern     = regexp.MustCompile(`(^|[\s=(])/(?:[^/\s"',;)\]}]+/)*[^/\s"',;)\]}]+`)
	debugRelativePathPattern = regexp.MustCompile(`(^|[\s=(])(?:\.\.?[\\/]|~[\\/])[^\s"',;)\]}]+`)
)

func sanitizeDebugMessage(message string) string {
	message = debugURLPattern.ReplaceAllStringFunc(message, sanitizeDebugURLMatch)
	message = debugSensitiveAssignmentPattern.ReplaceAllString(message, `${1}<redacted>`)
	message = debugWindowsPathPattern.ReplaceAllString(message, `<redacted>`)
	message = debugUnixPathPattern.ReplaceAllString(message, `${1}<redacted>`)
	message = debugRelativePathPattern.ReplaceAllString(message, `${1}<redacted>`)
	message = strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(message)
	message = strings.TrimSpace(message)
	if message == "" {
		return "<empty>"
	}
	if len(message) > 512 {
		return message[:512] + "..."
	}
	return message
}

func sanitizeDebugURLMatch(raw string) string {
	suffix := ""
	for len(raw) > 0 && strings.ContainsRune(".,;:)]}", rune(raw[len(raw)-1])) {
		suffix = string(raw[len(raw)-1]) + suffix
		raw = raw[:len(raw)-1]
	}
	return sanitizeURL(raw) + suffix
}

func sanitizeURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "<unavailable>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

// DebugFlagEnabled resolves the explicit CLI --debug switch from raw argv.
// This is intentionally limited to CLI input; environment and config values
// must not silently turn on disclosure of cause or HTTP metadata.
func DebugFlagEnabled(args []string) bool {
	enabled := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch {
		case arg == "--debug" || arg == "-d" || arg == "--debug=true" || arg == "-d=true":
			enabled = true
		case arg == "--debug=false" || arg == "-d=false":
			enabled = false
		}
	}
	return enabled
}

// DebugEnabled reports whether output is the process boundary's explicit
// debug writer.
func DebugEnabled(output io.Writer) bool {
	debugWriter, ok := output.(interface{ DebugEnabled() bool })
	return ok && debugWriter.DebugEnabled()
}

// Printf writes one verbose diagnostic line when diagnostics are enabled.
func Printf(output io.Writer, enabled bool, format string, args ...any) {
	if !enabled || output == nil {
		return
	}
	_, _ = fmt.Fprintf(output, format+"\n", args...)
}

// SessionLabel returns the user-facing session label used in diagnostics.
func SessionLabel(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return DefaultSessionID
	}
	return sessionID
}

// PayloadType classifies a submitted file without reading or logging content.
func PayloadType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".md", ".markdown":
		return "markdown"
	default:
		return "file"
	}
}
