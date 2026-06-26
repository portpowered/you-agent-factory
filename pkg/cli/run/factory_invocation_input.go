package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryrun "github.com/portpowered/infinite-you/pkg/config/factoryrun"
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
	content := *workcontent.GeneratedPtrFromParts(resolved.Content)
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
	if streamRenderer != nil {
		streamRenderer.stopProgressRendering()
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

func ResolveFactoryInvocationSignature(dir string) (*interfaces.InvocationSignatureConfig, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	authored, err := factoryconfig.LoadAuthoredFactoryAPIFromPath(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		return nil, err
	}
	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(authored)
	if err != nil {
		return nil, err
	}
	if cfg.InvocationSignature == nil {
		return nil, nil
	}
	return cfg.InvocationSignature, nil
}

type factoryInvocationHelpData struct {
	factoryName   string
	selectionText string
	commandPrefix string
	signature     *interfaces.InvocationSignatureConfig
}

func WriteFactoryInvocationHelp(w io.Writer, cliName string, cfg RunConfig) (bool, error) {
	data, err := loadFactoryInvocationHelpData(cliName, cfg)
	if err != nil {
		return false, err
	}
	if data == nil || data.signature == nil {
		return false, nil
	}

	_, err = io.WriteString(w, formatFactoryInvocationHelp(*data))
	if err != nil {
		return false, err
	}
	return true, nil
}

func loadFactoryInvocationHelpData(cliName string, cfg RunConfig) (*factoryInvocationHelpData, error) {
	if strings.TrimSpace(cfg.NamedFactoryName) == "" && strings.TrimSpace(cfg.FactoryConfigPath) == "" {
		return nil, nil
	}

	switch {
	case strings.TrimSpace(cfg.NamedFactoryName) != "":
		configPath := filepath.Join(cfg.Dir, interfaces.FactoryConfigFile)
		loaded, err := factoryrun.LoadFactoryConfigFromConfigFile(configPath)
		if err != nil {
			return nil, err
		}
		return &factoryInvocationHelpData{
			factoryName:   selectedFactoryName(loaded, cfg.NamedFactoryName),
			selectionText: fmt.Sprintf("named factory %s", cfg.NamedFactoryName),
			commandPrefix: fmt.Sprintf("%s run --named %s", cliName, cfg.NamedFactoryName),
			signature:     loaded.InvocationSignature,
		}, nil
	case strings.TrimSpace(cfg.FactoryConfigPath) != "":
		loaded, err := factoryrun.LoadFactoryConfigFromConfigFile(cfg.FactoryConfigPath)
		if err != nil {
			return nil, err
		}
		return &factoryInvocationHelpData{
			factoryName:   selectedFactoryName(loaded, cfg.FactoryConfigPath),
			selectionText: fmt.Sprintf("factory config %s", cfg.FactoryConfigPath),
			commandPrefix: fmt.Sprintf("%s run --factory %s", cliName, cfg.FactoryConfigPath),
			signature:     loaded.InvocationSignature,
		}, nil
	default:
		return nil, nil
	}
}

func selectedFactoryName(cfg *interfaces.FactoryConfig, fallback string) string {
	if cfg != nil && strings.TrimSpace(cfg.Name) != "" {
		return cfg.Name
	}
	return fallback
}

func formatFactoryInvocationHelp(data factoryInvocationHelpData) string {
	var builder strings.Builder
	builder.WriteString("Factory invocation help\n\n")
	builder.WriteString("Selected factory: ")
	builder.WriteString(data.factoryName)
	builder.WriteString(" (")
	builder.WriteString(data.selectionText)
	builder.WriteString(")\n\n")
	builder.WriteString("Usage:\n")
	builder.WriteString("  ")
	builder.WriteString(data.commandPrefix)
	builder.WriteString(signatureUsageSuffix(data.signature))
	builder.WriteString("\n\n")
	builder.WriteString("Factory-defined arguments:\n")
	for _, parameter := range orderedSignatureParameters(data.signature) {
		builder.WriteString(formatInvocationParameter(parameter))
	}
	if data.signature.OutputContract != nil {
		builder.WriteString("\nOutput contract:\n")
		builder.WriteString(formatOutputContract(data.signature.OutputContract))
	}
	if len(data.signature.Examples) > 0 {
		builder.WriteString("\nExamples:\n")
		for _, example := range data.signature.Examples {
			builder.WriteString(formatInvocationExample(data.commandPrefix, example))
		}
	}
	builder.WriteString("\nRun-level flags:\n")
	builder.WriteString("  Existing operational flags such as `--no-record`, `--with-mock-workers`, `--server`, and `--json` still apply.\n")
	builder.WriteString("  Keep run-level flags on the same command; factory-defined `--argument` options come from the selected invocationSignature.\n")
	return builder.String()
}

func signatureUsageSuffix(signature *interfaces.InvocationSignatureConfig) string {
	if signature == nil {
		return ""
	}
	var parts []string
	for _, parameter := range orderedSignatureParameters(signature) {
		usagePart := parameterUsageToken(parameter)
		if usagePart == "" {
			continue
		}
		parts = append(parts, usagePart)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func orderedSignatureParameters(signature *interfaces.InvocationSignatureConfig) []interfaces.InvocationParameterConfig {
	if signature == nil {
		return nil
	}
	parameters := append([]interfaces.InvocationParameterConfig(nil), signature.Parameters...)
	slices.SortStableFunc(parameters, func(left, right interfaces.InvocationParameterConfig) int {
		leftSlot, leftPositional := positionalSlot(left)
		rightSlot, rightPositional := positionalSlot(right)
		switch {
		case leftPositional && rightPositional:
			if leftSlot != rightSlot {
				return leftSlot - rightSlot
			}
		case leftPositional:
			return -1
		case rightPositional:
			return 1
		}
		return strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	return parameters
}

func positionalSlot(parameter interfaces.InvocationParameterConfig) (int, bool) {
	slot := 0
	found := false
	for _, binding := range parameter.Bindings {
		if strings.TrimSpace(binding.Kind) != "POSITIONAL" {
			continue
		}
		if !found || binding.Position < slot {
			slot = binding.Position
		}
		found = true
	}
	return slot, found
}

func parameterUsageToken(parameter interfaces.InvocationParameterConfig) string {
	name := placeholderName(parameter)
	required := parameter.Required
	valueMode := strings.TrimSpace(parameter.ValueMode)
	_, positional := positionalSlot(parameter)
	if positional {
		switch valueMode {
		case "VARIADIC":
			if required {
				return "<" + name + "...>"
			}
			return "[<" + name + "...>]"
		default:
			if required {
				return "<" + name + ">"
			}
			return "[<" + name + ">]"
		}
	}

	flagName := "--" + namedParameterKey(parameter)
	valueToken := valuePlaceholder(parameter)
	switch valueMode {
	case "REPEATED":
		if required {
			return flagName + " " + valueToken + " [" + flagName + " " + valueToken + " ...]"
		}
		return "[" + flagName + " " + valueToken + "]"
	default:
		if required {
			return flagName + " " + valueToken
		}
		return "[" + flagName + " " + valueToken + "]"
	}
}

func placeholderName(parameter interfaces.InvocationParameterConfig) string {
	if external := strings.TrimSpace(parameter.ExternalName); external != "" {
		return external
	}
	return strings.TrimSpace(parameter.Name)
}

func namedParameterKey(parameter interfaces.InvocationParameterConfig) string {
	if external := strings.TrimSpace(parameter.ExternalName); external != "" {
		return external
	}
	return strings.TrimSpace(parameter.Name)
}

func valuePlaceholder(parameter interfaces.InvocationParameterConfig) string {
	switch strings.TrimSpace(parameter.TypeHint) {
	case "BOOLEAN_STRING":
		return "<true|false>"
	case "FILE_PATH":
		return "<file-path>"
	case "NUMBER_STRING":
		return "<number>"
	default:
		return "<value>"
	}
}

func formatInvocationParameter(parameter interfaces.InvocationParameterConfig) string {
	var builder strings.Builder
	builder.WriteString("  ")
	builder.WriteString(parameterSummary(parameter))
	builder.WriteString("\n")
	if description := strings.TrimSpace(parameter.Description); description != "" {
		builder.WriteString("    ")
		builder.WriteString(description)
		builder.WriteString("\n")
	}
	for _, detail := range parameterDetails(parameter) {
		builder.WriteString("    ")
		builder.WriteString(detail)
		builder.WriteString("\n")
	}
	return builder.String()
}

func parameterSummary(parameter interfaces.InvocationParameterConfig) string {
	var parts []string
	if slot, positional := positionalSlot(parameter); positional {
		label := "<" + placeholderName(parameter) + ">"
		if strings.TrimSpace(parameter.ValueMode) == "VARIADIC" {
			label = "<" + placeholderName(parameter) + "...>"
		}
		parts = append(parts, fmt.Sprintf("positional %d %s", slot, label))
	}
	if hasNamedBinding(parameter) {
		parts = append(parts, "--"+namedParameterKey(parameter)+" "+valuePlaceholder(parameter))
	}
	for _, alias := range parameter.Aliases {
		if trimmed := strings.TrimSpace(alias); trimmed != "" {
			parts = append(parts, "--"+trimmed+" "+valuePlaceholder(parameter)+" (alias)")
		}
	}
	if len(parts) == 0 {
		parts = append(parts, placeholderName(parameter))
	}
	return strings.Join(parts, " | ")
}

func hasNamedBinding(parameter interfaces.InvocationParameterConfig) bool {
	for _, binding := range parameter.Bindings {
		if strings.TrimSpace(binding.Kind) == "NAMED" {
			return true
		}
	}
	return false
}

func hasStdinBinding(parameter interfaces.InvocationParameterConfig) bool {
	for _, binding := range parameter.Bindings {
		if strings.TrimSpace(binding.Kind) == "STDIN" {
			return true
		}
	}
	return false
}

func parameterDetails(parameter interfaces.InvocationParameterConfig) []string {
	var details []string
	if parameter.Required {
		details = append(details, "Required.")
	} else {
		details = append(details, "Optional.")
	}
	if defaultValue := strings.TrimSpace(parameter.DefaultValue); defaultValue != "" {
		details = append(details, "Default: "+defaultValue+".")
	}
	if parameter.Sensitive {
		details = append(details, "Sensitive values are redacted in diagnostics.")
	}
	if len(parameter.Choices) > 0 {
		details = append(details, "Accepted values: "+strings.Join(parameter.Choices, ", ")+".")
	}
	if hasStdinBinding(parameter) {
		details = append(details, "Reads from stdin when provided.")
	}
	if strings.TrimSpace(parameter.TypeHint) == "BOOLEAN_STRING" && hasNamedBinding(parameter) {
		details = append(details, "Named form also accepts bare `--"+namedParameterKey(parameter)+"` as `true`.")
	}
	if valueMode := strings.TrimSpace(parameter.ValueMode); valueMode != "" && valueMode != "EXACT" {
		details = append(details, "Value mode: "+strings.ToLower(valueMode)+".")
	}
	if typeHint := strings.TrimSpace(parameter.TypeHint); typeHint != "" && typeHint != "STRING" {
		details = append(details, "Type hint: "+strings.ToLower(typeHint)+".")
	}
	return details
}

func formatOutputContract(contract *interfaces.InvocationOutputContractConfig) string {
	if contract == nil {
		return ""
	}
	mode := strings.TrimSpace(contract.Mode)
	if mode == "" {
		mode = "INLINE"
	}
	var builder strings.Builder
	builder.WriteString("  Mode: ")
	builder.WriteString(strings.ToLower(mode))
	builder.WriteString("\n")
	if pathParameter := strings.TrimSpace(contract.PathParameter); pathParameter != "" {
		builder.WriteString("  Path parameter: ")
		builder.WriteString(pathParameter)
		builder.WriteString("\n")
	}
	if description := strings.TrimSpace(contract.Description); description != "" {
		builder.WriteString("  ")
		builder.WriteString(description)
		builder.WriteString("\n")
	}
	return builder.String()
}

func formatInvocationExample(commandPrefix string, example interfaces.InvocationExampleConfig) string {
	var builder strings.Builder
	builder.WriteString("  ")
	if description := strings.TrimSpace(example.Description); description != "" {
		builder.WriteString("# ")
		builder.WriteString(description)
		builder.WriteString("\n  ")
	}
	if stdin := strings.TrimSpace(example.Stdin); stdin != "" {
		builder.WriteString("printf '%s\\n' ")
		builder.WriteString(shellQuoteArg(stdin))
		builder.WriteString(" | ")
		builder.WriteString(commandPrefix)
		if len(example.Argv) > 0 {
			builder.WriteString(" ")
			builder.WriteString(joinShellArgs(example.Argv))
		}
		builder.WriteString("\n")
		return builder.String()
	}
	builder.WriteString(commandPrefix)
	if len(example.Argv) > 0 {
		builder.WriteString(" ")
		builder.WriteString(joinShellArgs(example.Argv))
	}
	builder.WriteString("\n")
	return builder.String()
}

func joinShellArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuoteArg(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuoteArg(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
