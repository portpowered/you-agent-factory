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

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/workcontent"
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

type sessionInvocationRunner interface {
	factoryServiceRunner
	apisurface.InvocationAPI
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
}

type sessionResponseStreamInvocationRunner interface {
	sessionInvocationRunner
	sessionResponseStreamAttachable
}

func resolveFactoryInvocationRequest(cfg RunConfig) (*factoryapi.InvocationRequest, bool, error) {
	if strings.TrimSpace(cfg.WorkFile) != "" {
		return nil, false, nil
	}
	if factoryInvocationRoot(cfg) == "" {
		return nil, false, nil
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
	return &factoryapi.InvocationRequest{
		SourceKind: factoryapi.InvocationInputSourceKindText,
		Content:    *workcontent.GeneratedPtrFromParts(resolved.Content),
	}
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
	logger *zap.Logger,
	mockWorkersConfig *factoryconfig.MockWorkersConfig,
	reservedAPIServer *reservedAPIServerListener,
) error {
	svcCfg := buildRunServiceConfig(cfg, logger, mockWorkersConfig, reservedAPIServer, make(chan struct{}), &sync.Once{})
	svcCfg.RuntimeMode = interfaces.RuntimeModeService
	svcCfg.WorkFile = ""
	svcCfg.SimpleDashboardRenderer = nil

	factorySvc, err := buildFactoryService(ctx, svcCfg)
	if err != nil {
		return err
	}
	invoker, ok := factorySvc.(sessionInvocationRunner)
	if !ok {
		return fmt.Errorf("factory invocation runner does not support session invocation")
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

	var streamAttachment *responseStreamAttachment
	var streamRenderer responseStreamRenderer
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		streamRenderer = newResponseStreamRenderer(cfg.Output, cfg.JSONOutput)
		if streamInvoker, ok := invoker.(sessionResponseStreamInvocationRunner); ok {
			streamAttachment = startResponseStreamAttachment(
				ctx,
				streamInvoker,
				factorysessions.DefaultSessionID,
				streamRenderer,
			)
		}
	}

	result, err := invoker.InvokeFactorySession(runCtx, factorysessions.DefaultSessionID, request)
	if streamAttachment != nil {
		streamAttachment.stop()
	}
	cancel()
	runErr := <-runErrCh
	if err != nil {
		return err
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, streamRenderer)
	}
	return writeInvocationSuccess(cfg, result, streamRenderer)
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

func invocationPrimaryResultText(parts []interfaces.WorkContentPart) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("invocation primary result is empty")
	}

	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type.Normalized() != interfaces.WorkContentPartTypeText {
			return "", fmt.Errorf("invocation primary result is not plain text; use --json")
		}
		textParts = append(textParts, part.Text)
	}
	return strings.Join(textParts, "\n"), nil
}
