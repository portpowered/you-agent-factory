package commandregistry

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	configcli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/config"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli/initsetup"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

const (
	factoryShowPortInputID           = "you.factory.show.flag.port"
	factoryShowSessionInputID        = "you.factory.show.flag.session"
	factoryListDirInputID            = "you.factory.list.flag.dir"
	factoryCreateNameInputID         = "you.factory.create.arg.0"
	factoryCreateDirInputID          = "you.factory.create.flag.dir"
	factoryCreateFromInputID         = "you.factory.create.flag.from"
	factoryCreateSetCurrentInputID   = "you.factory.create.flag.set-current"
	factoryUpdateNameInputID         = "you.factory.update.arg.0"
	factoryUpdateDirInputID          = "you.factory.update.flag.dir"
	factoryUpdateFromInputID         = "you.factory.update.flag.from"
	factoryDeleteNameInputID         = "you.factory.delete.arg.0"
	factoryDeleteDirInputID          = "you.factory.delete.flag.dir"
	factoryReplacePortInputID        = "you.factory.replace-current.flag.port"
	factoryReplaceSessionInputID     = "you.factory.replace-current.flag.session"
	factoryValidatePathInputID       = "you.factory.config.validate.arg.0"
	factoryFlattenPathInputID        = "you.factory.config.flatten.arg.0"
	factoryExpandPathInputID         = "you.factory.config.expand.arg.0"
	initPackageInputID               = "you.init.flag.package"
	initDirInputID                   = "you.init.flag.dir"
	initFormatInputID                = "you.init.flag.format"
	initReplaceInputID               = "you.init.flag.replace"
	initProviderInputID              = "you.init.flag.provider"
	initModelInputID                 = "you.init.flag.model"
	factoryConfigInitServerInputID   = "you.flag.server"
	factoryConfigInitJSONInputID     = "you.flag.json"
	factoryConfigInitVerboseInputID  = "you.flag.verbose"
	factoryConfigInitDebugInputID    = "you.flag.debug"
	deprecatedFactoryPortFlagMessage = "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"
)

// FactoryConfigInitHandler is the transport-owned stable-ID handler surface for
// the complete factory/config/init command family.
type FactoryConfigInitHandler interface {
	FactoryQuery(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryList(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryCreate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryUpdate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryDelete(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryReplaceCurrent(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryConfigValidate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryConfigFlatten(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	FactoryConfigExpand(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	Init(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
}

// FactoryConfigInitCommandHandler translates resolved inputs using prebound operations.
type FactoryConfigInitCommandHandler struct {
	queryFactory           func(factorycli.QueryConfig) error
	listFactories          func(factorycli.ListConfig) error
	createFactoryFromFile  func(factorycli.CreateFromFileConfig) error
	updateFactoryFromFile  func(factorycli.UpdateFromFileConfig) error
	deleteFactory          func(factorycli.DeleteConfig) error
	replaceFactoryCurrent  func(factorycli.ReplaceCurrentConfig) error
	validateFactory        func(factorycli.ValidateConfig) error
	flattenFactoryConfig   func(configcli.FactoryConfigFlattenConfig) error
	expandFactoryConfig    func(configcli.FactoryConfigExpandConfig) error
	configureInit          func(initsetup.Config) error
	installPackagedFactory func(factorydefinitionscli.InstallPackagedFactoryConfig) error
	homeDir                func(*cobra.Command) (string, error)
	resolveFactoryRoots    func(string, string) (factorydefinitions.NamedFactoryRoots, error)
	diagnosticsWriter      func(*cobra.Command) io.Writer
}

func NewFactoryConfigInitCommandHandler(
	queryFactory func(factorycli.QueryConfig) error,
	listFactories func(factorycli.ListConfig) error,
	createFactoryFromFile func(factorycli.CreateFromFileConfig) error,
	updateFactoryFromFile func(factorycli.UpdateFromFileConfig) error,
	deleteFactory func(factorycli.DeleteConfig) error,
	replaceFactoryCurrent func(factorycli.ReplaceCurrentConfig) error,
	validateFactory func(factorycli.ValidateConfig) error,
	flattenFactoryConfig func(configcli.FactoryConfigFlattenConfig) error,
	expandFactoryConfig func(configcli.FactoryConfigExpandConfig) error,
	configureInit func(initsetup.Config) error,
	installPackagedFactory func(factorydefinitionscli.InstallPackagedFactoryConfig) error,
	homeDir func(*cobra.Command) (string, error),
	resolveFactoryRoots func(string, string) (factorydefinitions.NamedFactoryRoots, error),
	diagnosticsWriter func(*cobra.Command) io.Writer,
) *FactoryConfigInitCommandHandler {
	return &FactoryConfigInitCommandHandler{
		queryFactory:           queryFactory,
		listFactories:          listFactories,
		createFactoryFromFile:  createFactoryFromFile,
		updateFactoryFromFile:  updateFactoryFromFile,
		deleteFactory:          deleteFactory,
		replaceFactoryCurrent:  replaceFactoryCurrent,
		validateFactory:        validateFactory,
		flattenFactoryConfig:   flattenFactoryConfig,
		expandFactoryConfig:    expandFactoryConfig,
		configureInit:          configureInit,
		installPackagedFactory: installPackagedFactory,
		homeDir:                homeDir,
		resolveFactoryRoots:    resolveFactoryRoots,
		diagnosticsWriter:      diagnosticsWriter,
	}
}

type factoryConfigInitGlobals struct {
	server  string
	json    bool
	verbose bool
	debug   bool
}

func readFactoryConfigInitGlobals(inputs resolvedinput.Inputs) (factoryConfigInitGlobals, error) {
	server, err := inputs.String(factoryConfigInitServerInputID)
	if err != nil {
		return factoryConfigInitGlobals{}, err
	}
	jsonOutput, err := inputs.Bool(factoryConfigInitJSONInputID)
	if err != nil {
		return factoryConfigInitGlobals{}, err
	}
	verbose, err := inputs.Bool(factoryConfigInitVerboseInputID)
	if err != nil {
		return factoryConfigInitGlobals{}, err
	}
	debug, err := inputs.Bool(factoryConfigInitDebugInputID)
	if err != nil {
		return factoryConfigInitGlobals{}, err
	}
	return factoryConfigInitGlobals{server: server, json: jsonOutput, verbose: verbose, debug: debug}, nil
}

func (h *FactoryConfigInitCommandHandler) diagnostics(cmd *cobra.Command) io.Writer {
	return h.diagnosticsWriter(cmd)
}

func rejectResolvedDeprecatedPort(inputs resolvedinput.Inputs, inputID string) error {
	state, ok := inputs.State(inputID)
	if ok && state.Changed {
		return fmt.Errorf("%s", deprecatedFactoryPortFlagMessage)
	}
	return nil
}

func (h *FactoryConfigInitCommandHandler) FactoryQuery(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if err := rejectResolvedDeprecatedPort(inputs, factoryShowPortInputID); err != nil {
		return err
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory show inputs: %w", err)
	}
	sessionID, err := inputs.String(factoryShowSessionInputID)
	if err != nil {
		return fmt.Errorf("resolve factory show inputs: %w", err)
	}
	return h.queryFactory(factorycli.QueryConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID, JSON: globals.json,
		Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd),
		Verbose: globals.verbose, Debug: globals.debug,
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryList(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	dir, err := inputs.String(factoryListDirInputID)
	if err != nil {
		return fmt.Errorf("resolve factory list inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory list inputs: %w", err)
	}
	home, err := h.homeDir(cmd)
	if err != nil {
		return fmt.Errorf("resolve factory list home: %w", err)
	}
	workingDirectory := startupcli.WorkingDirectory(cmd.Context())
	if strings.TrimSpace(workingDirectory) == "" {
		return fmt.Errorf("resolve factory list roots: process working directory is required")
	}
	roots, err := h.resolveFactoryRoots(home, workingDirectory)
	if err != nil {
		return fmt.Errorf("resolve factory list roots: %w", err)
	}
	if state, ok := inputs.State(factoryListDirInputID); ok && state.Changed {
		if filepath.IsAbs(dir) {
			roots.Project = dir
		} else {
			roots.Project = filepath.Join(workingDirectory, dir)
		}
	}
	return h.listFactories(factorycli.ListConfig{
		Context: cmd.Context(), ProjectRoot: roots.Project, GlobalRoot: roots.Global,
		JSON: globals.json, Output: cmd.OutOrStdout(), Diagnostics: cmd.ErrOrStderr(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryCreate(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	name, err := inputs.String(factoryCreateNameInputID)
	if err != nil {
		return fmt.Errorf("resolve factory create inputs: %w", err)
	}
	dir, err := inputs.String(factoryCreateDirInputID)
	if err != nil {
		return fmt.Errorf("resolve factory create inputs: %w", err)
	}
	from, err := inputs.String(factoryCreateFromInputID)
	if err != nil {
		return fmt.Errorf("resolve factory create inputs: %w", err)
	}
	setCurrent, err := inputs.Bool(factoryCreateSetCurrentInputID)
	if err != nil {
		return fmt.Errorf("resolve factory create inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory create inputs: %w", err)
	}
	return h.createFactoryFromFile(factorycli.CreateFromFileConfig{
		Context: cmd.Context(), Name: name, Dir: dir, From: from,
		SetCurrent: setCurrent, JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryUpdate(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	name, err := inputs.String(factoryUpdateNameInputID)
	if err != nil {
		return fmt.Errorf("resolve factory update inputs: %w", err)
	}
	dir, err := inputs.String(factoryUpdateDirInputID)
	if err != nil {
		return fmt.Errorf("resolve factory update inputs: %w", err)
	}
	from, err := inputs.String(factoryUpdateFromInputID)
	if err != nil {
		return fmt.Errorf("resolve factory update inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory update inputs: %w", err)
	}
	return h.updateFactoryFromFile(factorycli.UpdateFromFileConfig{
		Context: cmd.Context(), Name: name, Dir: dir, From: from,
		JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryDelete(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	name, err := inputs.String(factoryDeleteNameInputID)
	if err != nil {
		return fmt.Errorf("resolve factory delete inputs: %w", err)
	}
	dir, err := inputs.String(factoryDeleteDirInputID)
	if err != nil {
		return fmt.Errorf("resolve factory delete inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory delete inputs: %w", err)
	}
	return h.deleteFactory(factorycli.DeleteConfig{
		Name: name, Dir: dir, JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryReplaceCurrent(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if err := rejectResolvedDeprecatedPort(inputs, factoryReplacePortInputID); err != nil {
		return err
	}
	sessionID, err := inputs.String(factoryReplaceSessionInputID)
	if err != nil {
		return fmt.Errorf("resolve factory replace-current inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory replace-current inputs: %w", err)
	}
	return h.replaceFactoryCurrent(factorycli.ReplaceCurrentConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		JSON: globals.json, Output: cmd.OutOrStdout(),
		Diagnostics: h.diagnostics(cmd), Verbose: globals.verbose,
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryConfigValidate(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	path, err := inputs.String(factoryValidatePathInputID)
	if err != nil {
		return fmt.Errorf("resolve factory validate inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory validate inputs: %w", err)
	}
	return h.validateFactory(factorycli.ValidateConfig{
		Context: cmd.Context(), Path: path, JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryConfigFlatten(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	path, err := inputs.String(factoryFlattenPathInputID)
	if err != nil {
		return fmt.Errorf("resolve factory flatten inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory flatten inputs: %w", err)
	}
	return h.flattenFactoryConfig(configcli.FactoryConfigFlattenConfig{
		Path: path, Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd),
		Verbose: globals.verbose, Debug: globals.debug,
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryConfigExpand(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	path, err := inputs.String(factoryExpandPathInputID)
	if err != nil {
		return fmt.Errorf("resolve factory expand inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory expand inputs: %w", err)
	}
	return h.expandFactoryConfig(configcli.FactoryConfigExpandConfig{
		Path: path, Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd),
		Verbose: globals.verbose, Debug: globals.debug,
	})
}

func (h *FactoryConfigInitCommandHandler) Init(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	if resolvedInputChanged(inputs, initPackageInputID) {
		return h.initPackagedFactory(cmd, inputs, globals)
	}
	if globals.json {
		return fmt.Errorf("--json is not supported by you init")
	}
	provider, err := inputs.String(initProviderInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	modelValue, err := inputs.String(initModelInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	var model *string
	if state, ok := inputs.State(initModelInputID); ok && state.Changed {
		model = &modelValue
	}
	homeDir, err := h.homeDir(cmd)
	if err != nil {
		return fmt.Errorf("resolve init home directory: %w", err)
	}
	ctx := cmd.Context()
	interactive := ctx != nil &&
		startupcli.StdinIsTTY(ctx) &&
		startupcli.StdoutIsTTY(ctx)
	return h.configureInit(initsetup.Config{
		Context: ctx, HomeDir: homeDir, Provider: provider,
		Model: model, Input: cmd.InOrStdin(), Output: cmd.OutOrStdout(),
		Interactive: interactive,
	})
}

func resolvedInputChanged(inputs resolvedinput.Inputs, inputIDs ...string) bool {
	for _, inputID := range inputIDs {
		if state, ok := inputs.State(inputID); ok && state.Changed {
			return true
		}
	}
	return false
}

func (h *FactoryConfigInitCommandHandler) initPackagedFactory(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	globals factoryConfigInitGlobals,
) error {
	packageName, err := inputs.String(initPackageInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	dir, err := inputs.String(initDirInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	format, err := inputs.String(initFormatInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	replace, err := inputs.Bool(initReplaceInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	homeDir, err := h.homeDir(cmd)
	if err != nil {
		return fmt.Errorf("resolve init home directory: %w", err)
	}
	dirChanged := false
	if state, ok := inputs.State(initDirInputID); ok {
		dirChanged = state.Changed
	}
	formatChanged := false
	if state, ok := inputs.State(initFormatInputID); ok {
		formatChanged = state.Changed
	}
	return h.installPackagedFactory(factorydefinitionscli.InstallPackagedFactoryConfig{
		Context:       cmd.Context(),
		HomeDir:       homeDir,
		Package:       packageName,
		Dir:           dir,
		DirChanged:    dirChanged,
		Format:        format,
		FormatChanged: formatChanged,
		Replace:       replace,
		JSON:          globals.json,
		Output:        cmd.OutOrStdout(),
		Diagnostics:   h.diagnostics(cmd),
		Verbose:       globals.verbose,
	})
}

// InvocationHome is resolved profile metadata, including a deferred lookup error.
// It contains no invocation collaborator.
type InvocationHome struct {
	Path string
	Err  error
}

type invocationHomeContextKey struct{}

func WithInvocationHome(ctx context.Context, home InvocationHome) context.Context {
	return context.WithValue(ctx, invocationHomeContextKey{}, home)
}

func InvocationHomeFromContext(ctx context.Context) InvocationHome {
	var home InvocationHome
	var ok bool
	if ctx != nil {
		home, ok = ctx.Value(invocationHomeContextKey{}).(InvocationHome)
	}
	if !ok {
		return InvocationHome{Err: fmt.Errorf("invocation home metadata is required")}
	}
	return home
}

func ResolveInvocationHome(cmd *cobra.Command) (string, error) {
	home := InvocationHomeFromContext(cmd.Context())
	return home.Path, home.Err
}
