// Package climanifestcobra builds representative, session, work,
// factory/config/init, models/docs, run/submit, and workflow/MCP Cobra trees
// from generated manifest metadata and handwritten handler registries.
package climanifestcobra

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const deprecatedPortFlagMessage = "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"

func rejectDeprecatedPortFlag(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Lookup("port") != nil && cmd.Flags().Changed("port") {
		return fmt.Errorf("%s", deprecatedPortFlagMessage)
	}
	return nil
}

func registerDeprecatedPortFlag(cmd *cobra.Command, target *int) {
	cmd.Flags().IntVar(target, "port", 0, "deprecated; use --server")
	_ = cmd.Flags().MarkHidden("port")
}

// RepresentativeFamilyComponents holds detached representative-family commands
// before the session/show subtree is attached to the generated root.
type RepresentativeFamilyComponents struct {
	Root    *cobra.Command
	Session *cobra.Command
	Show    *cobra.Command
}

// NewRepresentativeFamilyCommand builds the representative you → session → show tree
// from generated metadata and attaches handwritten handlers by stable command ID.
// Only contracted representative-family commands are constructed.
func NewRepresentativeFamilyCommand(registry *commandregistry.Registry, bindings PersistentFlagBindings) (*cobra.Command, error) {
	components, err := NewRepresentativeFamilyComponents(registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Session.AddCommand(components.Show)
	components.Root.AddCommand(components.Session)
	return components.Root, nil
}

// NewRepresentativeFamilyComponents builds detached representative-family commands
// so production wiring can attach additional handwritten session siblings in order.
func NewRepresentativeFamilyComponents(registry *commandregistry.Registry, bindings PersistentFlagBindings) (RepresentativeFamilyComponents, error) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	return NewRepresentativeFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewRepresentativeFamilyCommandFromManifest builds the representative tree from one
// generated manifest snapshot. Manifest command IDs must stay within the representative family.
func NewRepresentativeFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings PersistentFlagBindings,
) (*cobra.Command, error) {
	components, err := NewRepresentativeFamilyComponentsFromManifest(manifest, registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Session.AddCommand(components.Show)
	components.Root.AddCommand(components.Session)
	return components.Root, nil
}

// NewRepresentativeFamilyComponentsFromManifest builds detached representative-family
// commands from one generated manifest snapshot.
func NewRepresentativeFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings PersistentFlagBindings,
) (RepresentativeFamilyComponents, error) {
	if registry == nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: registry is required")
	}
	if err := validateBindings(bindings); err != nil {
		return RepresentativeFamilyComponents{}, err
	}
	if err := validateRepresentativeManifest(manifest); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}

	rootRecord, sessionRecord, showRecord, err := representativeManifestRecords(manifest)
	if err != nil {
		return RepresentativeFamilyComponents{}, err
	}

	root, err := buildCommandFromRecord(rootRecord)
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	root.SilenceUsage = true
	if err := registerPersistentFlags(root, rootRecord, bindings); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if err := registry.AttachRunE(root, rootRecord.ID); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}

	session, err := buildCommandFromRecord(sessionRecord)
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if sessionRecord.Runnable {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %q must remain non-runnable", sessionRecord.ID)
	}

	show, err := buildCommandFromRecord(showRecord)
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	show.Args = positionalArgsFromManifest(showRecord)
	show.PreRunE = rejectDeprecatedPortFlag
	if err := registerLocalFlags(show, showRecord, bindings); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if err := registry.AttachRunE(show, showRecord.ID); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}

	return RepresentativeFamilyComponents{
		Root:    root,
		Session: session,
		Show:    show,
	}, nil
}

func representativeManifestRecords(manifest climanifest.Manifest) (root, session, show climanifest.Command, err error) {
	root, err = manifest.CommandByID("you")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build representative family command: %w", err)
	}
	session, err = manifest.CommandByID("you.session")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build representative family command: %w", err)
	}
	show, err = manifest.CommandByID("you.session.show")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build representative family command: %w", err)
	}
	return root, session, show, nil
}

func validateRepresentativeManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.RepresentativeFamilyCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d representative-family commands",
			len(manifest.Commands),
			len(climanifestgen.RepresentativeFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertRepresentativeFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.RepresentativeFamilyCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing representative-family command %q", commandID)
		}
	}
	return nil
}

func validateBindings(bindings PersistentFlagBindings) error {
	required := []struct {
		name string
		ok   bool
	}{
		{"Verbose", bindings.Verbose != nil},
		{"Debug", bindings.Debug != nil},
		{"Server", bindings.Server != nil},
		{"JSON", bindings.JSON != nil},
		{"DefaultWorkerModelProvider", bindings.DefaultWorkerModelProvider != nil},
		{"DefaultWorkerModel", bindings.DefaultWorkerModel != nil},
	}
	for _, field := range required {
		if !field.ok {
			return fmt.Errorf("build representative family command: bindings.%s is required", field.name)
		}
	}
	return nil
}

func buildCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
	if err := climanifestgen.AssertRepresentativeFamilyCommandID(record.ID); err != nil {
		return nil, err
	}
	cmd := &cobra.Command{
		Use:     record.Usage.Line,
		Short:   record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    record.Documentation.Documentation.Description.CanonicalEnglish,
		Example: record.Usage.Example,
		Aliases: append([]string(nil), record.Aliases...),
	}
	if record.Visibility == "hidden" {
		cmd.Hidden = true
	}
	return cmd, nil
}

func registerPersistentFlags(root *cobra.Command, record climanifest.Command, bindings PersistentFlagBindings) error {
	flags := sortedFlags(record.Flags)
	for _, flag := range flags {
		if flag.Scope != "persistent" {
			continue
		}
		target, err := persistentBindingTarget(flag.Long, bindings)
		if err != nil {
			return err
		}
		usage := flagUsage(bindings, flag.Long)
		if err := registerFlag(root.PersistentFlags(), flag, target, usage); err != nil {
			return fmt.Errorf("register root persistent flag %q: %w", flag.Long, err)
		}
		if err := applyFlagContract(root.PersistentFlags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply root persistent flag %q contract: %w", flag.Long, err)
		}
	}
	return nil
}

func registerLocalFlags(cmd *cobra.Command, record climanifest.Command, bindings PersistentFlagBindings) error {
	var deprecatedPort int
	flags := sortedFlags(record.Flags)
	for _, flag := range flags {
		if flag.Scope != "local" {
			continue
		}
		if flag.Long == "port" {
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			if err := applyFlagContract(cmd.Flags().Lookup("port"), flag); err != nil {
				return fmt.Errorf("apply port flag contract: %w", err)
			}
			continue
		}
		target, err := localBindingTarget(flag)
		if err != nil {
			return err
		}
		usage := flagUsage(bindings, flag.Long)
		if err := registerFlag(cmd.Flags(), flag, target, usage); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply local flag %q contract: %w", flag.Long, err)
		}
	}
	return nil
}

func flagUsage(bindings PersistentFlagBindings, longName string) string {
	if bindings.FlagUsages == nil {
		return ""
	}
	return bindings.FlagUsages[longName]
}

type flagTarget struct {
	boolValue   *bool
	stringValue *string
	intValue    *int
}

func persistentBindingTarget(longName string, bindings PersistentFlagBindings) (flagTarget, error) {
	switch longName {
	case "verbose":
		return flagTarget{boolValue: bindings.Verbose}, nil
	case "debug":
		return flagTarget{boolValue: bindings.Debug}, nil
	case "server":
		return flagTarget{stringValue: bindings.Server}, nil
	case "json":
		return flagTarget{boolValue: bindings.JSON}, nil
	case "default-worker-model-provider":
		return flagTarget{stringValue: bindings.DefaultWorkerModelProvider}, nil
	case "default-worker-model":
		return flagTarget{stringValue: bindings.DefaultWorkerModel}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported root persistent flag %q", longName)
	}
}

func localBindingTarget(flag climanifest.Flag) (flagTarget, error) {
	switch flag.ValueType {
	case "int":
		value, err := strconv.Atoi(flag.Default)
		if err != nil {
			return flagTarget{}, fmt.Errorf("parse default for local flag %q: %w", flag.Long, err)
		}
		heap := value
		return flagTarget{intValue: &heap}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported local flag %q with value type %q", flag.Long, flag.ValueType)
	}
}

func registerFlag(flagSet *pflag.FlagSet, contract climanifest.Flag, target flagTarget, usage string) error {
	switch contract.ValueType {
	case "bool":
		if target.boolValue == nil {
			return fmt.Errorf("missing bool binding for flag %q", contract.Long)
		}
		defaultValue, err := strconv.ParseBool(contract.Default)
		if err != nil {
			return fmt.Errorf("parse default for flag %q: %w", contract.Long, err)
		}
		if contract.Shorthand != "" {
			flagSet.BoolVarP(target.boolValue, contract.Long, contract.Shorthand, defaultValue, usage)
		} else {
			flagSet.BoolVar(target.boolValue, contract.Long, defaultValue, usage)
		}
	case "string":
		if target.stringValue == nil {
			return fmt.Errorf("missing string binding for flag %q", contract.Long)
		}
		if contract.Shorthand != "" {
			return fmt.Errorf("string flag %q does not support shorthand in generated constructor", contract.Long)
		}
		flagSet.StringVar(target.stringValue, contract.Long, contract.Default, usage)
	case "int":
		if target.intValue == nil {
			return fmt.Errorf("missing int binding for flag %q", contract.Long)
		}
		defaultValue, err := strconv.Atoi(contract.Default)
		if err != nil {
			return fmt.Errorf("parse default for flag %q: %w", contract.Long, err)
		}
		flagSet.IntVar(target.intValue, contract.Long, defaultValue, usage)
	default:
		return fmt.Errorf("unsupported flag value type %q for %q", contract.ValueType, contract.Long)
	}
	return nil
}

func applyFlagContract(flag *pflag.Flag, contract climanifest.Flag) error {
	if flag == nil {
		return fmt.Errorf("flag %q was not registered", contract.Long)
	}
	if contract.Visibility == "hidden" {
		flag.Hidden = true
	}
	if contract.NoOptionDefault != "" {
		flag.NoOptDefVal = contract.NoOptionDefault
	}
	return nil
}

func sortedFlags(flags map[string]climanifest.Flag) []climanifest.Flag {
	if len(flags) == 0 {
		return nil
	}
	ordered := make([]climanifest.Flag, 0, len(flags))
	for _, flag := range flags {
		ordered = append(ordered, flag)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Long == ordered[j].Long {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Long < ordered[j].Long
	})
	return ordered
}

func positionalArgsFromManifest(record climanifest.Command) cobra.PositionalArgs {
	if len(record.Arguments) == 0 {
		return nil
	}
	args := make([]climanifest.Argument, 0, len(record.Arguments))
	for _, arg := range record.Arguments {
		args = append(args, arg)
	}
	sort.Slice(args, func(i, j int) bool { return args[i].Position < args[j].Position })

	variadic := false
	totalMin := 0
	totalMax := 0
	unboundedMax := false
	for _, arg := range args {
		if arg.Variadic {
			variadic = true
		}
		totalMin += arg.MinCardinality
		if arg.MaxCardinality < 0 {
			unboundedMax = true
			continue
		}
		if arg.MaxCardinality == 0 {
			continue
		}
		totalMax += arg.MaxCardinality
	}
	if variadic {
		if totalMin > 0 {
			return cobra.MinimumNArgs(totalMin)
		}
		return cobra.ArbitraryArgs
	}
	if unboundedMax {
		if totalMin > 0 {
			return cobra.MinimumNArgs(totalMin)
		}
		return cobra.ArbitraryArgs
	}
	if totalMax > 0 && totalMin == totalMax {
		return cobra.ExactArgs(totalMin)
	}
	if totalMax > 0 {
		return cobra.MaximumNArgs(totalMax)
	}
	if totalMin > 0 {
		return cobra.MinimumNArgs(totalMin)
	}
	return nil
}

const supportedManifestFormatVersion = "1.0.0"

type plannedCommand struct {
	record     climanifest.Command
	parentPath string
}

// NewCommandTree constructs a detached Cobra tree from one resolved CLI
// manifest snapshot. It validates the complete command topology before
// allocating any Cobra commands, so callers never receive a partial tree.
func NewCommandTree(manifest climanifest.Manifest) (*cobra.Command, error) {
	plan, err := planCommandTree(manifest)
	if err != nil {
		return nil, fmt.Errorf("build generic command tree: %w", err)
	}

	built := make(map[string]*cobra.Command, len(plan))
	for _, item := range plan {
		built[item.record.Path] = projectCommand(item.record)
	}
	for _, item := range plan {
		if item.parentPath == "" {
			continue
		}
		built[item.parentPath].AddCommand(built[item.record.Path])
	}
	return built[manifest.RootPath], nil
}

func planCommandTree(manifest climanifest.Manifest) ([]plannedCommand, error) {
	if err := validateManifestHeader(manifest); err != nil {
		return nil, err
	}
	byPath, err := indexCommandRecords(manifest.Commands)
	if err != nil {
		return nil, err
	}
	plan, err := planCommandTopology(manifest.RootPath, byPath)
	if err != nil {
		return nil, err
	}
	sortCommandPlan(plan)
	return plan, nil
}

func validateManifestHeader(manifest climanifest.Manifest) error {
	if manifest.FormatVersion == "" {
		return fmt.Errorf("manifest formatVersion is required")
	}
	if manifest.FormatVersion != supportedManifestFormatVersion {
		return fmt.Errorf("unsupported manifest formatVersion %q", manifest.FormatVersion)
	}
	if err := validateCommandPath(manifest.RootPath, "manifest rootPath"); err != nil {
		return err
	}
	if len(manifest.Commands) == 0 {
		return fmt.Errorf("manifest commands are required")
	}
	return nil
}

func indexCommandRecords(commands map[string]climanifest.Command) (map[string]climanifest.Command, error) {
	byPath := make(map[string]climanifest.Command, len(commands))
	for mapID, record := range commands {
		if err := validateCommandRecord(mapID, record); err != nil {
			return nil, err
		}
		if previous, exists := byPath[record.Path]; exists {
			return nil, fmt.Errorf(
				"commands %q and %q declare duplicate path %q",
				previous.ID,
				record.ID,
				record.Path,
			)
		}
		byPath[record.Path] = record
	}
	return byPath, nil
}

func planCommandTopology(rootPath string, byPath map[string]climanifest.Command) ([]plannedCommand, error) {
	root, ok := byPath[rootPath]
	if !ok {
		return nil, fmt.Errorf("manifest rootPath %q has no command record", rootPath)
	}

	plan := make([]plannedCommand, 0, len(byPath))
	for _, record := range byPath {
		parentPath := commandParentPath(record.Path)
		if record.Path == rootPath {
			parentPath = ""
		} else {
			if !strings.HasPrefix(record.Path, rootPath+" ") {
				return nil, fmt.Errorf(
					"command %q path %q is outside rootPath %q",
					record.ID,
					record.Path,
					rootPath,
				)
			}
			if _, exists := byPath[parentPath]; !exists {
				return nil, fmt.Errorf(
					"command %q path %q has missing parent %q",
					record.ID,
					record.Path,
					parentPath,
				)
			}
		}
		plan = append(plan, plannedCommand{record: record, parentPath: parentPath})
	}
	if commandParentPath(root.Path) != "" {
		return nil, fmt.Errorf("root command %q path %q must not have a parent", root.ID, root.Path)
	}
	return plan, nil
}

func sortCommandPlan(plan []plannedCommand) {
	sort.Slice(plan, func(i, j int) bool {
		leftDepth := strings.Count(plan[i].record.Path, " ")
		rightDepth := strings.Count(plan[j].record.Path, " ")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		if plan[i].record.Path != plan[j].record.Path {
			return plan[i].record.Path < plan[j].record.Path
		}
		return plan[i].record.ID < plan[j].record.ID
	})
}

func validateCommandRecord(mapID string, record climanifest.Command) error {
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("command map entry %q is missing id", mapID)
	}
	if mapID != record.ID {
		return fmt.Errorf("command map key %q does not match record id %q", mapID, record.ID)
	}
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("command %q is missing name", record.ID)
	}
	if strings.ContainsAny(record.Name, " \t\r\n") {
		return fmt.Errorf("command %q name %q must be one path segment", record.ID, record.Name)
	}
	if err := validateCommandPath(record.Path, fmt.Sprintf("command %q path", record.ID)); err != nil {
		return err
	}
	pathParts := strings.Split(record.Path, " ")
	if pathParts[len(pathParts)-1] != record.Name {
		return fmt.Errorf(
			"command %q name %q does not match path %q",
			record.ID,
			record.Name,
			record.Path,
		)
	}
	if strings.TrimSpace(record.Usage.Line) == "" {
		return fmt.Errorf("command %q is missing usage line", record.ID)
	}
	if strings.Fields(record.Usage.Line)[0] != record.Name {
		return fmt.Errorf(
			"command %q usage line %q must start with name %q",
			record.ID,
			record.Usage.Line,
			record.Name,
		)
	}
	if strings.TrimSpace(record.Documentation.Documentation.Title.CanonicalEnglish) == "" {
		return fmt.Errorf("command %q is missing documentation title", record.ID)
	}
	if strings.TrimSpace(record.Documentation.Documentation.Description.CanonicalEnglish) == "" {
		return fmt.Errorf("command %q is missing documentation description", record.ID)
	}
	switch record.Visibility {
	case "visible", "hidden":
	default:
		return fmt.Errorf("command %q has unsupported visibility %q", record.ID, record.Visibility)
	}
	switch record.Completeness {
	case "", "authoritative":
	default:
		return fmt.Errorf("command %q has unsupported completeness mode %q", record.ID, record.Completeness)
	}
	return validateCommandAliases(record)
}

func validateCommandPath(path, field string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.Join(strings.Fields(path), " ") != path {
		return fmt.Errorf("%s %q must use single spaces between segments", field, path)
	}
	return nil
}

func validateCommandAliases(record climanifest.Command) error {
	seen := map[string]struct{}{record.Name: {}}
	for _, alias := range record.Aliases {
		if strings.TrimSpace(alias) == "" {
			return fmt.Errorf("command %q has an empty alias", record.ID)
		}
		if strings.ContainsAny(alias, " \t\r\n") {
			return fmt.Errorf("command %q alias %q must be one path segment", record.ID, alias)
		}
		if _, exists := seen[alias]; exists {
			return fmt.Errorf("command %q has duplicate name or alias %q", record.ID, alias)
		}
		seen[alias] = struct{}{}
	}
	return nil
}

func commandParentPath(path string) string {
	index := strings.LastIndex(path, " ")
	if index < 0 {
		return ""
	}
	return path[:index]
}

func projectCommand(record climanifest.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:     record.Usage.Line,
		Short:   record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    record.Documentation.Documentation.Description.CanonicalEnglish,
		Example: record.Usage.Example,
		Aliases: append([]string(nil), record.Aliases...),
		Hidden:  record.Visibility == "hidden",
	}
	if record.Runnable {
		commandID := record.ID
		cmd.RunE = func(*cobra.Command, []string) error {
			return fmt.Errorf("command %q has no executable handler attached", commandID)
		}
	}
	return cmd
}
