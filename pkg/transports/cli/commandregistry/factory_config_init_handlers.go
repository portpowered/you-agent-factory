package commandregistry

import (
	"fmt"
	"io"

	initcmd "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/initsetup"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

const (
	factoryQueryPortInputID          = "you.factory.query.flag.port"
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
	initDirInputID                   = "you.init.flag.dir"
	initTypeInputID                  = "you.init.flag.type"
	initExecutorInputID              = "you.init.flag.executor"
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
	ConfigInit(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	Init(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
}

// FactoryConfigInitServices carries the injected effects used by the family
// adapter. Public spellings and Cobra storage deliberately stay out of it.
type FactoryConfigInitServices struct {
	QueryFactory          func(factorycli.QueryConfig) error
	ListFactories         func(factorycli.ListConfig) error
	CreateFactoryFromFile func(factorycli.CreateFromFileConfig) error
	UpdateFactoryFromFile func(factorycli.UpdateFromFileConfig) error
	DeleteFactory         func(factorycli.DeleteConfig) error
	ReplaceFactoryCurrent func(factorycli.ReplaceCurrentConfig) error
	ValidateFactory       func(factorycli.ValidateConfig) error
	FlattenFactoryConfig  func(configcli.FactoryConfigFlattenConfig) error
	ExpandFactoryConfig   func(configcli.FactoryConfigExpandConfig) error
	InitSystemConfig      func(configinitcmd.InitConfig) error
	InitFactory           func(initcmd.ScaffoldConfig) error
	ConfigureInit         func(initsetup.Config) error
	HomeDir               func() (string, error)
	DiagnosticsWriter     func(*cobra.Command) io.Writer
}

// FactoryConfigInitCommandHandler translates resolved manifest inputs into the
// existing transport request types at the CLI boundary.
type FactoryConfigInitCommandHandler struct {
	services FactoryConfigInitServices
}

func NewFactoryConfigInitCommandHandler(services FactoryConfigInitServices) *FactoryConfigInitCommandHandler {
	return &FactoryConfigInitCommandHandler{services: services}
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
	if h == nil || h.services.DiagnosticsWriter == nil {
		return nil
	}
	return h.services.DiagnosticsWriter(cmd)
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
	if h == nil || h.services.QueryFactory == nil {
		return fmt.Errorf("factory query service is required")
	}
	if err := rejectResolvedDeprecatedPort(inputs, factoryQueryPortInputID); err != nil {
		return err
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory query inputs: %w", err)
	}
	return h.services.QueryFactory(factorycli.QueryConfig{
		Context: cmd.Context(), Server: globals.server, JSON: globals.json,
		Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd),
		Verbose: globals.verbose, Debug: globals.debug,
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryList(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.ListFactories == nil {
		return fmt.Errorf("factory list service is required")
	}
	dir, err := inputs.String(factoryListDirInputID)
	if err != nil {
		return fmt.Errorf("resolve factory list inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory list inputs: %w", err)
	}
	return h.services.ListFactories(factorycli.ListConfig{
		Dir: dir, JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryCreate(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.CreateFactoryFromFile == nil {
		return fmt.Errorf("factory create service is required")
	}
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
	return h.services.CreateFactoryFromFile(factorycli.CreateFromFileConfig{
		Context: cmd.Context(), Name: name, Dir: dir, From: from,
		SetCurrent: setCurrent, JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryUpdate(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.UpdateFactoryFromFile == nil {
		return fmt.Errorf("factory update service is required")
	}
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
	return h.services.UpdateFactoryFromFile(factorycli.UpdateFromFileConfig{
		Context: cmd.Context(), Name: name, Dir: dir, From: from,
		JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryDelete(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.DeleteFactory == nil {
		return fmt.Errorf("factory delete service is required")
	}
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
	return h.services.DeleteFactory(factorycli.DeleteConfig{
		Name: name, Dir: dir, JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryReplaceCurrent(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.ReplaceFactoryCurrent == nil {
		return fmt.Errorf("factory replace-current service is required")
	}
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
	return h.services.ReplaceFactoryCurrent(factorycli.ReplaceCurrentConfig{
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
	if h == nil || h.services.ValidateFactory == nil {
		return fmt.Errorf("factory validate service is required")
	}
	path, err := inputs.String(factoryValidatePathInputID)
	if err != nil {
		return fmt.Errorf("resolve factory validate inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory validate inputs: %w", err)
	}
	return h.services.ValidateFactory(factorycli.ValidateConfig{
		Context: cmd.Context(), Path: path, JSON: globals.json, Output: cmd.OutOrStdout(),
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryConfigFlatten(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.FlattenFactoryConfig == nil {
		return fmt.Errorf("factory flatten service is required")
	}
	path, err := inputs.String(factoryFlattenPathInputID)
	if err != nil {
		return fmt.Errorf("resolve factory flatten inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory flatten inputs: %w", err)
	}
	return h.services.FlattenFactoryConfig(configcli.FactoryConfigFlattenConfig{
		Path: path, Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd),
		Verbose: globals.verbose, Debug: globals.debug,
	})
}

func (h *FactoryConfigInitCommandHandler) FactoryConfigExpand(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.ExpandFactoryConfig == nil {
		return fmt.Errorf("factory expand service is required")
	}
	path, err := inputs.String(factoryExpandPathInputID)
	if err != nil {
		return fmt.Errorf("resolve factory expand inputs: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve factory expand inputs: %w", err)
	}
	return h.services.ExpandFactoryConfig(configcli.FactoryConfigExpandConfig{
		Path: path, Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd),
		Verbose: globals.verbose, Debug: globals.debug,
	})
}

func (h *FactoryConfigInitCommandHandler) ConfigInit(
	cmd *cobra.Command,
	_ resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.InitSystemConfig == nil {
		return fmt.Errorf("system initialization service is required")
	}
	if h.services.HomeDir == nil {
		return fmt.Errorf("config init home directory resolver is required")
	}
	homeDir, err := h.services.HomeDir()
	if err != nil {
		return fmt.Errorf("resolve config init home directory: %w", err)
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve config init inputs: %w", err)
	}
	return h.services.InitSystemConfig(configinitcmd.InitConfig{
		Context: cmd.Context(), HomeDir: homeDir, JSON: globals.json,
		Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd), Verbose: globals.verbose,
	})
}

func (h *FactoryConfigInitCommandHandler) Init(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil {
		return fmt.Errorf("init handler is required")
	}
	globals, err := readFactoryConfigInitGlobals(inherited)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	if state, ok := inherited.State(factoryConfigInitJSONInputID); ok && state.Changed {
		return fmt.Errorf("--json is not supported by you init")
	}
	if resolvedInputChanged(inputs, initDirInputID, initTypeInputID, initExecutorInputID) {
		return h.initLegacyFactoryScaffold(cmd, inputs, globals)
	}
	if h.services.ConfigureInit == nil {
		return fmt.Errorf("init provider/model configuration service is required")
	}
	if h.services.HomeDir == nil {
		return fmt.Errorf("init home directory resolver is required")
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
	homeDir, err := h.services.HomeDir()
	if err != nil {
		return fmt.Errorf("resolve init home directory: %w", err)
	}
	return h.services.ConfigureInit(initsetup.Config{
		Context: cmd.Context(), HomeDir: homeDir, Provider: provider,
		Model: model, Output: cmd.OutOrStdout(),
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

func (h *FactoryConfigInitCommandHandler) initLegacyFactoryScaffold(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	globals factoryConfigInitGlobals,
) error {
	if h.services.InitFactory == nil {
		return fmt.Errorf("legacy init scaffold service is required")
	}
	dir, err := inputs.String(initDirInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	scaffoldType, err := inputs.String(initTypeInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	executor, err := inputs.String(initExecutorInputID)
	if err != nil {
		return fmt.Errorf("resolve init inputs: %w", err)
	}
	return h.services.InitFactory(initcmd.ScaffoldConfig{
		Dir: dir, Type: scaffoldType, Executor: executor, JSON: globals.json,
		Output: cmd.OutOrStdout(), Diagnostics: h.diagnostics(cmd),
		Verbose: globals.verbose, Debug: globals.debug,
	})
}
