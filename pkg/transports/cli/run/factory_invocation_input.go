package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/service"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
	"go.uber.org/zap"
)

type InvocationInputSource string

const (
	InvocationInputSourcePositional InvocationInputSource = "positional prompt"
	InvocationInputSourceStdin      InvocationInputSource = "stdin"
	InvocationInputSourceWorkFile   InvocationInputSource = "work file"
)

type FactoryInvocationInputConfig struct {
	PromptArgs []string
	Stdin      io.Reader
	StdinIsTTY func() bool
}

type FactoryInvocationInput struct {
	Source  InvocationInputSource
	Payload string
}

// ResolveFactoryInvocationInput resolves the one-shot invocation payload from
// positional prompt args, explicit "-" stdin, or piped non-TTY stdin.
func ResolveFactoryInvocationInput(cfg FactoryInvocationInputConfig) (FactoryInvocationInput, error) {
	positionalPrompt, explicitStdin, hasPositional := splitInvocationPromptArgs(cfg.PromptArgs)
	stdinPayload, hasStdin, err := resolveInvocationStdin(cfg, explicitStdin)
	if err != nil {
		return FactoryInvocationInput{}, err
	}

	if !hasPositional && !hasStdin {
		return FactoryInvocationInput{}, nil
	}

	sources := invocations.TextInputSources{}
	if hasPositional {
		sources.PositionalText = &positionalPrompt
	}
	if hasStdin {
		sources.StdinText = &stdinPayload
	}

	resolved, err := invocations.ResolveTextInput(sources)
	if err != nil {
		return FactoryInvocationInput{}, factoryInvocationInputError(err)
	}

	switch resolved.Source {
	case invocations.InputSourcePositionalText:
		return FactoryInvocationInput{Source: InvocationInputSourcePositional, Payload: resolved.Text}, nil
	case invocations.InputSourceStdinText:
		return FactoryInvocationInput{Source: InvocationInputSourceStdin, Payload: resolved.Text}, nil
	default:
		return FactoryInvocationInput{}, nil
	}
}

func splitInvocationPromptArgs(args []string) (prompt string, explicitStdin bool, hasPositional bool) {
	positional := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "-" {
			explicitStdin = true
			continue
		}
		hasPositional = true
		positional = append(positional, arg)
	}
	return strings.Join(positional, " "), explicitStdin, hasPositional
}

func resolveInvocationStdin(cfg FactoryInvocationInputConfig, explicitStdin bool) (string, bool, error) {
	if !explicitStdin && invocationStdinIsTTY(cfg) {
		return "", false, nil
	}

	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", false, fmt.Errorf("read invocation stdin: %w", err)
	}
	payload := string(data)
	if payload == "" {
		if explicitStdin {
			return "", true, nil
		}
		return "", false, nil
	}
	return payload, true, nil
}

func invocationStdinIsTTY(cfg FactoryInvocationInputConfig) bool {
	if cfg.StdinIsTTY != nil {
		return cfg.StdinIsTTY()
	}
	if cfg.Stdin != nil && cfg.Stdin != os.Stdin {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func factoryInvocationInputError(err error) error {
	inputErr, ok := err.(*invocations.InputError)
	if !ok {
		return err
	}
	switch inputErr.Code {
	case invocations.InputErrorCodeSourceConflict:
		return ambiguousInvocationInputError(inputErr)
	case invocations.InputErrorCodeEmpty:
		return &InvocationError{
			Code:    string(inputErr.Code),
			Message: inputErr.Message,
		}
	default:
		return &InvocationError{
			Code:    string(inputErr.Code),
			Message: inputErr.Message,
		}
	}
}

func ambiguousInvocationInputError(inputErr *invocations.InputError) error {
	sources := make([]InvocationInputSource, 0, len(inputErr.ConflictingSources))
	for _, label := range inputErr.ConflictingSources {
		switch label {
		case invocations.InputSourcePositionalText:
			sources = append(sources, InvocationInputSourcePositional)
		case invocations.InputSourceStdinText:
			sources = append(sources, InvocationInputSourceStdin)
		}
	}
	return &AmbiguousInvocationInputError{
		invocationErr: &InvocationError{
			Code:    string(inputErr.Code),
			Message: inputErr.Message,
		},
		Sources: sources,
	}
}

type AmbiguousInvocationInputError struct {
	invocationErr *InvocationError
	Sources       []InvocationInputSource
}

func (e *AmbiguousInvocationInputError) Error() string {
	if e == nil || e.invocationErr == nil {
		return ""
	}
	return e.invocationErr.Error()
}

func (e *AmbiguousInvocationInputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.invocationErr
}

// InvocationRunner is the already-constructed invocation collaborator consumed
// by one-shot CLI runs.
type InvocationRunner interface {
	factoryServiceRunner
	apisurface.InvocationAPI
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
	CloseFactorySession(context.Context, string) error
}

type sessionInvocationRunner = InvocationRunner

// InvocationBootstrapBuilder constructs the invocation collaborator at the
// application wiring boundary before the CLI application starts.
type InvocationBootstrapBuilder func(context.Context, *service.FactoryServiceConfig) (InvocationRunner, error)

// buildInvocationBootstrap is a compatibility test seam. Production process
// composition always supplies the builder owned by pkg/wire.
var buildInvocationBootstrap InvocationBootstrapBuilder

func buildInvocationApplication(
	ctx context.Context,
	cfg RunConfig,
	logger *zap.Logger,
	request *factoryapi.InvocationRequest,
	recordPath resolvedRunRecordPath,
	builder InvocationBootstrapBuilder,
	mockWorkersConfig *factoryconfig.MockWorkersConfig,
) (*Application, error) {
	if builder == nil {
		return nil, fmt.Errorf("construct factory invocation bootstrap: builder is required")
	}
	runner, err := builder(ctx, buildInvocationRunServiceConfig(cfg, logger, mockWorkersConfig))
	if err != nil {
		return nil, fmt.Errorf("construct factory invocation bootstrap: %w", err)
	}
	if runner == nil {
		return nil, fmt.Errorf("construct factory invocation bootstrap: builder returned nil runner")
	}
	return &Application{
		cfg: cfg, logger: logger, invocationRequest: request,
		invocationRunner: runner, invocationMode: true, recordPath: recordPath,
	}, nil
}

type sessionResponseEventInvocationRunner interface {
	sessionInvocationRunner
	sessionResponseEventAttachable
}

var _ sessionResponseEventInvocationRunner = (*service.InvocationBootstrap)(nil)

func resolveFactoryInvocationRequest(cfg RunConfig) (*factoryapi.InvocationRequest, bool, error) {
	if strings.TrimSpace(cfg.WorkFile) != "" {
		return nil, false, nil
	}
	if factoryInvocationRoot(cfg) == "" {
		return nil, false, nil
	}
	if cfg.InvocationNormalizedArguments != nil {
		return invocationRequestFromNormalizedArguments(*cfg.InvocationNormalizedArguments), true, nil
	}

	stdinTTY := stdinIsTTY(cfg)
	if cfg.InvocationPositionalText == nil && cfg.InvocationStdinText == nil && stdinTTY {
		return nil, false, nil
	}

	sources := invocations.TextInputSources{
		PositionalText: cfg.InvocationPositionalText,
	}
	if cfg.InvocationStdinText != nil {
		sources.StdinText = cfg.InvocationStdinText
	} else if !stdinTTY {
		stdinText, err := readInvocationStdin(cfg)
		if err != nil {
			return nil, true, err
		}
		sources.StdinText = &stdinText
	}

	resolved, err := invocations.ResolveTextInput(sources)
	if err != nil {
		recordCLIInvocationFailure(cfg, err)
		return nil, true, wrapInvocationInputError(err)
	}
	recordCLIInvocationResolved(cfg, resolved.Source)
	return invocationRequestFromResolvedInput(resolved), true, nil
}

func readInvocationStdin(cfg RunConfig) (string, error) {
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read invocation stdin: %w", err)
	}
	return string(data), nil
}

func stdinIsTTY(cfg RunConfig) bool {
	if cfg.StdinIsTTY != nil {
		return cfg.StdinIsTTY()
	}
	if cfg.Stdin != nil && cfg.Stdin != os.Stdin {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func factoryInvocationRoot(cfg RunConfig) string {
	if root := strings.TrimSpace(cfg.FactoryConfigPath); root != "" {
		return root
	}
	return strings.TrimSpace(cfg.Dir)
}

func invocationRequestFromResolvedInput(resolved invocations.ResolvedInput) *factoryapi.InvocationRequest {
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := *contentcontract.GeneratedPtrFromParts(resolved.Content)
	return &factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}
}

func invocationRequestFromNormalizedArguments(normalized invocations.NormalizedArguments) *factoryapi.InvocationRequest {
	args := make(map[string]any, len(normalized.Arguments))
	for name, argument := range normalized.Arguments {
		if len(argument.Values) == 1 {
			args[name] = argument.Values[0]
			continue
		}
		values := append([]string(nil), argument.Values...)
		args[name] = values
	}
	return &factoryapi.InvocationRequest{Args: &args}
}

func wrapInvocationInputError(err error) error {
	inputErr, ok := err.(*invocations.InputError)
	if !ok {
		return err
	}
	return invocationCLIError{
		Code:    string(inputErr.Code),
		Message: inputErr.Message,
	}
}

func runFactoryInvocation(
	ctx context.Context,
	cfg RunConfig,
	request factoryapi.InvocationRequest,
	invoker sessionInvocationRunner,
) error {
	if invoker == nil {
		return fmt.Errorf("run factory invocation: runner is required")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- invoker.Run(runCtx)
	}()

	logPackagedTTSInvocationStart(cfg)

	if err := waitForInvocationSessionReady(runCtx, invoker, runErrCh); err != nil {
		return err
	}

	streamRenderer, stopStreamAttachment := startInvocationResponseStream(ctx, cfg, invoker)
	if streamRenderer != nil {
		defer streamRenderer.stopProgressRendering()
	}

	result, err := invoker.InvokeFactorySession(runCtx, factorysessions.DefaultSessionID, request)
	if stopStreamAttachment != nil {
		stopStreamAttachment()
	}
	if releaseErr := releaseInvocationSession(runCtx, invoker, factorysessions.DefaultSessionID); releaseErr != nil && err == nil {
		err = releaseErr
	}
	cancel()
	runErr := <-runErrCh
	if err != nil {
		return err
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, streamRenderer)
	}
	return writeInvocationSuccess(cfg, result, streamRenderer)
}

func startInvocationResponseStream(
	ctx context.Context,
	cfg RunConfig,
	invoker sessionInvocationRunner,
) (responseStreamRenderer, func()) {
	if !isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return nil, nil
	}
	if cfg.JSONOutput {
		renderer := newJSONResponseStreamRenderer(cfg.Output)
		streamInvoker, ok := invoker.(sessionResponseEventInvocationRunner)
		if !ok {
			return renderer, nil
		}
		attachment := startResponseEventAttachment(ctx, streamInvoker, factorysessions.DefaultSessionID, renderer)
		if attachment == nil {
			return renderer, nil
		}
		return renderer, attachment.stop
	}

	renderer := newHumanResponseStreamRenderer(cfg.Output)
	streamInvoker, ok := invoker.(sessionResponseEventInvocationRunner)
	if !ok {
		return renderer, nil
	}
	attachment := startResponseEventAttachment(ctx, streamInvoker, factorysessions.DefaultSessionID, renderer)
	if attachment == nil {
		return renderer, nil
	}
	return renderer, attachment.stop
}

func buildInvocationRunServiceConfig(
	cfg RunConfig,
	logger *zap.Logger,
	mockWorkersConfig *factoryconfig.MockWorkersConfig,
) *service.FactoryServiceConfig {
	svcCfg := buildRunServiceConfig(cfg, logger, mockWorkersConfig, nil, make(chan struct{}), &sync.Once{})
	return service.NormalizeInvocationBootstrapConfig(svcCfg)
}

func releaseInvocationSession(
	ctx context.Context,
	invoker sessionInvocationRunner,
	sessionID string,
) error {
	if invoker == nil {
		return fmt.Errorf("factory invocation runner is required")
	}
	if err := invoker.CloseFactorySession(ctx, sessionID); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return nil
		}
		return fmt.Errorf("release factory invocation session: %w", err)
	}
	return nil
}

func waitForInvocationSessionReady(
	ctx context.Context,
	invoker sessionInvocationRunner,
	runErrCh <-chan error,
) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if _, err := invoker.GetCurrentFactoryForSession(ctx, factorysessions.DefaultSessionID); err == nil {
			return nil
		} else if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return err
		}

		select {
		case err := <-runErrCh:
			if err == nil || errors.Is(err, context.Canceled) {
				return fmt.Errorf("factory invocation session stopped before it became ready")
			}
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type invocationCLIError struct {
	Code      string
	Message   string
	SessionID string
	WorkID    string
	WorkName  string
	WorkState string
}

func (e invocationCLIError) Error() string {
	contextSuffix := e.contextSuffix()
	switch {
	case strings.TrimSpace(e.Code) == "":
		return strings.TrimSpace(e.Message) + contextSuffix
	case strings.TrimSpace(e.Message) == "":
		return strings.TrimSpace(e.Code) + contextSuffix
	default:
		return strings.TrimSpace(e.Code) + ": " + strings.TrimSpace(e.Message) + contextSuffix
	}
}

func (e invocationCLIError) contextSuffix() string {
	fields := make([]string, 0, 4)
	if sessionID := strings.TrimSpace(e.SessionID); sessionID != "" {
		fields = append(fields, "session="+sessionID)
	}
	if workID := strings.TrimSpace(e.WorkID); workID != "" {
		fields = append(fields, "workId="+workID)
	}
	if workName := strings.TrimSpace(e.WorkName); workName != "" {
		fields = append(fields, "workName="+workName)
	}
	if workState := strings.TrimSpace(e.WorkState); workState != "" {
		fields = append(fields, "workState="+workState)
	}
	if len(fields) == 0 {
		return ""
	}
	return " [" + strings.Join(fields, " ") + "]"
}

func invocationResultFailure(result apisurface.FactoryInvocationResult) error {
	return invocationCLIError{
		Code:      strings.TrimSpace(result.ErrorCode),
		Message:   strings.TrimSpace(result.Message),
		SessionID: strings.TrimSpace(result.SessionID),
		WorkID:    strings.TrimSpace(result.WorkID),
		WorkName:  strings.TrimSpace(result.WorkName),
		WorkState: strings.TrimSpace(result.WorkState),
	}
}

func writeInvocationFailure(
	cfg RunConfig,
	result apisurface.FactoryInvocationResult,
	streamRenderer responseStreamRenderer,
) error {
	if streamRenderer != nil {
		if err := streamRenderer.writeFinalInvocationResult(result); err != nil {
			return err
		}
	} else if cfg.JSONOutput {
		if err := writeInvocationJSON(cfg, result); err != nil {
			return err
		}
	}
	return invocationResultFailure(result)
}

func writeInvocationSuccess(
	cfg RunConfig,
	result apisurface.FactoryInvocationResult,
	streamRenderer responseStreamRenderer,
) error {
	if streamRenderer != nil {
		return streamRenderer.writeFinalInvocationResult(result)
	}
	if cfg.JSONOutput {
		return writeInvocationJSON(cfg, result)
	}

	text, err := invocationPrimaryResultText(result.PrimaryResult)
	if err != nil {
		return err
	}
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}
	_, err = fmt.Fprint(output, text)
	return err
}

func writeInvocationJSON(cfg RunConfig, result apisurface.FactoryInvocationResult) error {
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}
	encoded, err := json.Marshal(apisurface.InvocationResponseFromResult(result))
	if err != nil {
		return fmt.Errorf("marshal invocation response: %w", err)
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func invocationPrimaryResultText(parts []work.WorkContentPart) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("invocation primary result is empty")
	}

	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			return "", fmt.Errorf("invocation primary result is not plain text; use --json")
		}
		textParts = append(textParts, part.Text)
	}
	return strings.Join(textParts, "\n"), nil
}

type SignatureFactoryInvocationInputConfig struct {
	PromptArgs []string
	Signature  *interfaces.InvocationSignatureConfig
	Stdin      io.Reader
	StdinIsTTY func() bool
}

func ResolveSignatureFactoryInvocationInput(cfg SignatureFactoryInvocationInputConfig) (invocations.NormalizedArguments, error) {
	positionalArgs, namedArgs, explicitStdin, err := splitSignatureInvocationArgs(cfg.PromptArgs, cfg.Signature)
	if err != nil {
		return invocations.NormalizedArguments{}, err
	}
	stdinPayload, hasStdin, err := resolveInvocationStdin(FactoryInvocationInputConfig{
		Stdin:      cfg.Stdin,
		StdinIsTTY: cfg.StdinIsTTY,
	}, explicitStdin)
	if err != nil {
		return invocations.NormalizedArguments{}, err
	}

	input := invocations.NormalizeArgumentsInput{
		Signature:      cfg.Signature,
		PositionalArgs: positionalArgs,
		NamedArgs:      namedArgs,
	}
	if hasStdin {
		input.StdinText = &stdinPayload
	}

	normalized, err := invocations.NormalizeArguments(input)
	if err != nil {
		return invocations.NormalizedArguments{}, signatureInvocationInputError(err)
	}
	return normalized, nil
}

func splitSignatureInvocationArgs(args []string, signature *interfaces.InvocationSignatureConfig) ([]string, []invocations.NamedArgumentInput, bool, error) {
	positional := make([]string, 0, len(args))
	named := make([]invocations.NamedArgumentInput, 0)
	explicitStdin := false
	booleanNamedKeys := signatureBooleanNamedKeys(signature)

	for index := 0; index < len(args); index++ {
		token := args[index]
		if strings.TrimSpace(token) == "-" {
			explicitStdin = true
			continue
		}
		if !strings.HasPrefix(token, "--") || token == "--" {
			positional = append(positional, token)
			continue
		}

		raw := strings.TrimPrefix(token, "--")
		if name, value, ok := strings.Cut(raw, "="); ok {
			named = append(named, invocations.NamedArgumentInput{Key: strings.TrimSpace(name), Values: []string{value}})
			continue
		}

		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, nil, false, fmt.Errorf("factory argument name is required after --")
		}

		if booleanNamedKeys[name] {
			if index+1 < len(args) && isExplicitBooleanStringValue(args[index+1]) {
				index++
				named = append(named, invocations.NamedArgumentInput{Key: name, Values: []string{args[index]}})
				continue
			}
			named = append(named, invocations.NamedArgumentInput{Key: name, Values: []string{"true"}})
			continue
		}

		if index+1 >= len(args) {
			return nil, nil, false, fmt.Errorf("factory argument --%s requires a value", name)
		}
		index++
		named = append(named, invocations.NamedArgumentInput{Key: name, Values: []string{args[index]}})
	}

	return positional, named, explicitStdin, nil
}

func signatureBooleanNamedKeys(signature *interfaces.InvocationSignatureConfig) map[string]bool {
	keys := map[string]bool{}
	if signature == nil {
		return keys
	}
	for _, parameter := range signature.Parameters {
		if strings.TrimSpace(parameter.TypeHint) != string(factoryapi.FactoryInvocationParameterTypeHintBooleanString) {
			continue
		}
		hasNamedBinding := false
		for _, binding := range parameter.Bindings {
			if strings.TrimSpace(binding.Kind) == string(factoryapi.FactoryInvocationParameterBindingKindNamed) {
				hasNamedBinding = true
				break
			}
		}
		if !hasNamedBinding {
			continue
		}
		if name := strings.TrimSpace(parameter.Name); name != "" {
			keys[name] = true
		}
		if external := strings.TrimSpace(parameter.ExternalName); external != "" {
			keys[external] = true
		}
		for _, alias := range parameter.Aliases {
			if trimmed := strings.TrimSpace(alias); trimmed != "" {
				keys[trimmed] = true
			}
		}
	}
	return keys
}

func isExplicitBooleanStringValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "1", "0", "yes", "no", "on", "off":
		return true
	default:
		return false
	}
}

func signatureInvocationInputError(err error) error {
	argumentErr, ok := err.(*invocations.ArgumentError)
	if !ok {
		return err
	}
	return &InvocationError{
		Code:    string(argumentErr.Code),
		Message: argumentErr.Message,
	}
}
