package commandregistry

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/spf13/cobra"
)

// RunE is a handwritten Cobra handler bound to one stable command ID.
type RunE func(cmd *cobra.Command, args []string) error

// CommandHandlers is the handwritten lifecycle attached to one runnable Cobra command.
type CommandHandlers struct {
	PreRunE RunE
	RunE    RunE
}

// Registry maps stable command IDs to handwritten command handlers.
type Registry struct {
	handlers map[string]CommandHandlers
}

// NewRegistry constructs an empty handler registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]CommandHandlers)}
}

// Register binds one stable command ID to a handwritten handler.
// Duplicate registration fails observably.
func (r *Registry) Register(commandID string, handler RunE) error {
	return r.RegisterHandlers(commandID, CommandHandlers{RunE: handler})
}

// RegisterHandlers binds one stable command ID to its handwritten lifecycle.
// Duplicate registration fails observably.
func (r *Registry) RegisterHandlers(commandID string, handlers CommandHandlers) error {
	if r == nil {
		return fmt.Errorf("register %q: registry is nil", commandID)
	}
	if commandID == "" {
		return fmt.Errorf("register handler: command ID is required")
	}
	if handlers.RunE == nil {
		return fmt.Errorf("register %q: handler is required", commandID)
	}
	if _, exists := r.handlers[commandID]; exists {
		return fmt.Errorf("register %q: duplicate handler registration", commandID)
	}
	r.handlers[commandID] = handlers
	return nil
}

// Lookup returns the handwritten handler for one stable command ID.
func (r *Registry) Lookup(commandID string) (RunE, error) {
	if r == nil {
		return nil, fmt.Errorf("lookup %q: registry is nil", commandID)
	}
	handlers, ok := r.handlers[commandID]
	if !ok {
		return nil, fmt.Errorf("lookup %q: handler not registered", commandID)
	}
	return handlers.RunE, nil
}

// LookupHandlers returns the handwritten lifecycle for one stable command ID.
func (r *Registry) LookupHandlers(commandID string) (CommandHandlers, error) {
	if r == nil {
		return CommandHandlers{}, fmt.Errorf("lookup %q: registry is nil", commandID)
	}
	handlers, ok := r.handlers[commandID]
	if !ok {
		return CommandHandlers{}, fmt.Errorf("lookup %q: handler not registered", commandID)
	}
	return handlers, nil
}

// AttachRunE sets cmd.RunE from the registry entry for commandID.
func (r *Registry) AttachRunE(cmd *cobra.Command, commandID string) error {
	if cmd == nil {
		return fmt.Errorf("attach %q: command is nil", commandID)
	}
	handler, err := r.Lookup(commandID)
	if err != nil {
		return err
	}
	cmd.RunE = handler
	return nil
}

// AttachHandlers sets cmd.PreRunE and cmd.RunE from the registry entry.
func (r *Registry) AttachHandlers(cmd *cobra.Command, commandID string) error {
	if cmd == nil {
		return fmt.Errorf("attach %q: command is nil", commandID)
	}
	handlers, err := r.LookupHandlers(commandID)
	if err != nil {
		return err
	}
	cmd.PreRunE = handlers.PreRunE
	cmd.RunE = handlers.RunE
	return nil
}

const (
	SubmitHandlerID      = "you.submit.handler"
	SubmitBatchHandlerID = "you.submit.batch.handler"

	submitNameInputID     = "you.submit.flag.name"
	submitWorkTypeInputID = "you.submit.flag.work-type-name"
	submitPayloadInputID  = "you.submit.flag.payload"
	submitSessionInputID  = "you.submit.flag.session"
	batchArgumentInputID  = "you.submit.batch.arg.0"
	batchFileInputID      = "you.submit.batch.flag.file"
	batchDryRunInputID    = "you.submit.batch.flag.dry-run"
	batchSessionInputID   = "you.submit.batch.flag.session"
	rootServerInputID     = "you.flag.server"
	rootJSONInputID       = "you.flag.json"
	rootVerboseInputID    = "you.flag.verbose"
	rootDebugInputID      = "you.flag.debug"
)

// SubmitHandler consumes one invocation's local and inherited resolved inputs.
type SubmitHandler func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error

// SubmitHandlers supplies the two executable submit-family bindings.
type SubmitHandlers struct {
	Submit      SubmitHandler
	SubmitBatch SubmitHandler
}

// SubmitRegistry contains exactly the stable handlers accepted by the submit family.
type SubmitRegistry struct {
	handlers map[string]SubmitHandler
}

// BatchSubmitEffects supplies invocation-independent process effects at the
// CLI composition boundary.
type BatchSubmitEffects struct {
	FileSystem submitcli.BatchInputFileSystem
	StdinIsTTY func(context.Context) bool
}

// UnarySubmitHandler adapts one invocation's typed stable-ID inputs to the
// existing unary submit operation without retaining mutable command state.
func UnarySubmitHandler(operation func(submitcli.SubmitConfig) error) SubmitHandler {
	return func(
		cmd *cobra.Command,
		local resolvedinput.Inputs,
		inherited resolvedinput.Inputs,
	) error {
		if operation == nil {
			return fmt.Errorf("execute unary submit: operation is required")
		}
		cfg, err := unarySubmitConfig(cmd, local, inherited)
		if err != nil {
			return fmt.Errorf("execute unary submit: %w", err)
		}
		return operation(cfg)
	}
}

func unarySubmitConfig(
	cmd *cobra.Command,
	local resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) (submitcli.SubmitConfig, error) {
	if cmd == nil {
		return submitcli.SubmitConfig{}, fmt.Errorf("command is required")
	}
	name, err := requiredSubmitString(local, submitNameInputID)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	workType, err := requiredSubmitString(local, submitWorkTypeInputID)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	payload, err := requiredSubmitString(local, submitPayloadInputID)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	sessionID, err := submitString(local, submitSessionInputID)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	server, err := submitServer(inherited)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	jsonOutput, err := submitBool(inherited, rootJSONInputID)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	verbose, err := submitBool(inherited, rootVerboseInputID)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	debug, err := submitBool(inherited, rootDebugInputID)
	if err != nil {
		return submitcli.SubmitConfig{}, err
	}
	diagnostics := submitDiagnostics(cmd, verbose || debug)
	return submitcli.SubmitConfig{
		Context:      cmd.Context(),
		Name:         name,
		WorkTypeName: workType,
		Payload:      payload,
		Server:       server,
		SessionID:    sessionID,
		JSON:         jsonOutput,
		Output:       cmd.OutOrStdout(),
		Verbose:      verbose || debug,
		Debug:        debug,
		Diagnostics:  diagnostics,
	}, nil
}

// BatchSubmitHandler adapts one invocation's typed stable-ID inputs and
// streams to the existing batch operation without retaining mutable state.
func BatchSubmitHandler(
	operation func(submitcli.BatchConfig) error,
	effects BatchSubmitEffects,
) SubmitHandler {
	return func(
		cmd *cobra.Command,
		local resolvedinput.Inputs,
		inherited resolvedinput.Inputs,
	) error {
		if operation == nil {
			return fmt.Errorf("execute batch submit: operation is required")
		}
		cfg, err := batchSubmitConfig(cmd, local, inherited, effects)
		if err != nil {
			return fmt.Errorf("execute batch submit: %w", err)
		}
		return operation(cfg)
	}
}

func batchSubmitConfig(
	cmd *cobra.Command,
	local resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
	effects BatchSubmitEffects,
) (submitcli.BatchConfig, error) {
	if cmd == nil {
		return submitcli.BatchConfig{}, fmt.Errorf("command is required")
	}
	argument, err := optionalSubmitString(local, batchArgumentInputID)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	file, err := submitString(local, batchFileInputID)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	dryRun, err := submitBool(local, batchDryRunInputID)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	sessionID, err := submitString(local, batchSessionInputID)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	server, err := submitServer(inherited)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	jsonOutput, err := submitBool(inherited, rootJSONInputID)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	verbose, err := submitBool(inherited, rootVerboseInputID)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	debug, err := submitBool(inherited, rootDebugInputID)
	if err != nil {
		return submitcli.BatchConfig{}, err
	}
	if effects.FileSystem == nil {
		return submitcli.BatchConfig{}, fmt.Errorf("batch input file system is required")
	}
	if effects.StdinIsTTY == nil {
		return submitcli.BatchConfig{}, fmt.Errorf("batch stdin TTY detector is required")
	}
	diagnostics := submitDiagnostics(cmd, verbose || debug)
	args := []string{}
	if argument != "" {
		args = append(args, argument)
	}
	return submitcli.BatchConfig{
		Context:     cmd.Context(),
		FileFlag:    file,
		DryRun:      dryRun,
		Server:      server,
		SessionID:   sessionID,
		JSON:        jsonOutput,
		Verbose:     verbose || debug,
		Debug:       debug,
		Args:        args,
		Stdin:       cmd.InOrStdin(),
		StdinIsTTY:  func() bool { return effects.StdinIsTTY(cmd.Context()) },
		Output:      cmd.OutOrStdout(),
		Diagnostics: diagnostics,
		FileSystem:  effects.FileSystem,
	}, nil
}

func submitDiagnostics(cmd *cobra.Command, enabled bool) io.Writer {
	if !enabled {
		return nil
	}
	return cmd.ErrOrStderr()
}

func submitServer(inputs resolvedinput.Inputs) (string, error) {
	server, err := submitString(inputs, rootServerInputID)
	if err != nil {
		return "", err
	}
	base, err := cliserver.ResolveBase(server)
	if err != nil {
		return "", fmt.Errorf(
			"resolved submit input %q is not a valid server URI",
			rootServerInputID,
		)
	}
	return base.String(), nil
}

func requiredSubmitString(inputs resolvedinput.Inputs, inputID string) (string, error) {
	value, err := submitString(inputs, inputID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("resolved submit input %q is required", inputID)
	}
	return value, nil
}

func submitString(inputs resolvedinput.Inputs, inputID string) (string, error) {
	value, err := inputs.String(inputID)
	if err != nil {
		return "", fmt.Errorf("resolved submit input %q must be a string", inputID)
	}
	return value, nil
}

func optionalSubmitString(inputs resolvedinput.Inputs, inputID string) (string, error) {
	if _, ok := inputs.Lookup(inputID); !ok {
		return "", nil
	}
	return submitString(inputs, inputID)
}

func submitBool(inputs resolvedinput.Inputs, inputID string) (bool, error) {
	value, err := inputs.Bool(inputID)
	if err != nil {
		return false, fmt.Errorf("resolved submit input %q must be a boolean", inputID)
	}
	return value, nil
}

// NewSubmitRegistry constructs a complete submit-only handler registry.
func NewSubmitRegistry(handlers SubmitHandlers) (*SubmitRegistry, error) {
	if handlers.Submit == nil {
		return nil, fmt.Errorf("build submit handler registry: handler %q is required", SubmitHandlerID)
	}
	if handlers.SubmitBatch == nil {
		return nil, fmt.Errorf("build submit handler registry: handler %q is required", SubmitBatchHandlerID)
	}
	return &SubmitRegistry{handlers: map[string]SubmitHandler{
		SubmitHandlerID:      handlers.Submit,
		SubmitBatchHandlerID: handlers.SubmitBatch,
	}}, nil
}

// Lookup returns the executable binding for one stable submit handler ID.
func (r *SubmitRegistry) Lookup(handlerID string) (SubmitHandler, error) {
	if r == nil {
		return nil, fmt.Errorf("lookup submit handler %q: registry is required", handlerID)
	}
	handler, ok := r.handlers[handlerID]
	if !ok {
		return nil, fmt.Errorf("lookup submit handler %q: handler is not registered", handlerID)
	}
	return handler, nil
}

// Verify rejects missing, extra, duplicate, or unknown submit handler metadata.
func (r *SubmitRegistry) Verify(manifest climanifest.Manifest) error {
	if r == nil {
		return fmt.Errorf("verify submit handler coverage: registry is required")
	}
	expected := map[string]bool{
		SubmitHandlerID:      true,
		SubmitBatchHandlerID: true,
	}
	seen := make(map[string]bool, len(expected))
	var unknown []string
	for _, record := range manifest.Commands {
		if !record.Runnable || record.Handler == nil || record.Handler.ID == "" {
			return fmt.Errorf("verify submit handler coverage: command %q must declare a runnable handler", record.ID)
		}
		handlerID := record.Handler.ID
		if !expected[handlerID] {
			unknown = append(unknown, handlerID)
			continue
		}
		if seen[handlerID] {
			return fmt.Errorf("verify submit handler coverage: handler %q is declared more than once", handlerID)
		}
		if _, err := r.Lookup(handlerID); err != nil {
			return fmt.Errorf("verify submit handler coverage: %w", err)
		}
		seen[handlerID] = true
	}
	var missing []string
	for handlerID := range expected {
		if !seen[handlerID] {
			missing = append(missing, handlerID)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 || len(unknown) > 0 {
		return fmt.Errorf(
			"verify submit handler coverage: missing=%v unknown=%v",
			missing,
			unknown,
		)
	}
	return nil
}
