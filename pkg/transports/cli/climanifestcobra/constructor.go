// Package climanifestcobra builds representative, session, work,
// factory/config/init, models/docs, run/submit, and workflow/MCP Cobra trees
// from generated manifest metadata and handwritten handler registries.
package climanifestcobra

import (
	"fmt"
	"reflect"
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
	record        climanifest.Command
	parentPath    string
	flags         []plannedFlag
	arguments     []climanifest.Argument
	relationships []plannedRelationship
}

type plannedFlag struct {
	record      climanifest.Flag
	canonicalID string
}

// GenericConstructor is the stateless transport role that projects a resolved
// manifest snapshot into a detached Cobra command tree.
type GenericConstructor struct{}

// NewCommandTree constructs a detached Cobra tree from one resolved CLI
// manifest snapshot. It validates the complete command topology before
// allocating any Cobra commands, so callers never receive a partial tree.
func NewCommandTree(manifest climanifest.Manifest, bindingSets ...GenericBindings) (*cobra.Command, error) {
	return (GenericConstructor{}).Construct(manifest, bindingSets...)
}

// Construct validates and projects one resolved manifest snapshot.
func (GenericConstructor) Construct(manifest climanifest.Manifest, bindingSets ...GenericBindings) (*cobra.Command, error) {
	bindings, err := resolveGenericBindings(bindingSets)
	if err != nil {
		return nil, fmt.Errorf("build generic command tree: %w", err)
	}
	plan, err := planCommandTree(manifest, bindings)
	if err != nil {
		return nil, fmt.Errorf("build generic command tree: %w", err)
	}

	built := make(map[string]*cobra.Command, len(plan))
	targets := make(map[string]*genericFlagValue)
	for _, item := range plan {
		built[item.record.Path] = projectCommand(item.record, item.arguments)
	}
	for _, item := range plan {
		if err := projectFlags(built[item.record.Path], item, targets); err != nil {
			return nil, fmt.Errorf("build generic command tree: %w", err)
		}
		projectArgumentAndRelationshipRules(built[item.record.Path], item)
		if err := projectGenericPresentation(built[item.record.Path], item, bindings); err != nil {
			return nil, fmt.Errorf("build generic command tree: %w", err)
		}
		projectGenericHandler(built[item.record.Path], item.record, bindings)
	}
	for _, item := range plan {
		if item.parentPath == "" {
			continue
		}
		built[item.parentPath].AddCommand(built[item.record.Path])
	}
	return built[manifest.RootPath], nil
}

func planCommandTree(manifest climanifest.Manifest, bindings GenericBindings) ([]plannedCommand, error) {
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
	if err := planCommandFlags(plan); err != nil {
		return nil, err
	}
	if err := planCommandArgumentsAndRelationships(plan); err != nil {
		return nil, err
	}
	if err := validateGenericPresentation(plan, bindings); err != nil {
		return nil, err
	}
	if err := validateGenericHandlers(plan, bindings); err != nil {
		return nil, err
	}
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

func projectCommand(record climanifest.Command, arguments []climanifest.Argument) *cobra.Command {
	return &cobra.Command{
		Use:     projectedCommandUsage(record, arguments),
		Short:   record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    commandLong(record),
		Example: commandExamples(record),
		Aliases: append([]string(nil), record.Aliases...),
		Hidden:  record.Visibility == "hidden",
	}
}

func planCommandFlags(plan []plannedCommand) error {
	type declaration struct {
		flag        climanifest.Flag
		commandPath string
	}
	declarations := make(map[string]declaration)
	inputOwners := make(map[string]string)
	for index := range plan {
		for _, mapID := range sortedFlagMapKeys(plan[index].record.Flags) {
			record := plan[index].record.Flags[mapID]
			if mapID != record.ID {
				return fmt.Errorf(
					"command %q flag map key %q does not match input id %q",
					plan[index].record.ID,
					mapID,
					record.ID,
				)
			}
		}
		records := sortedFlags(plan[index].record.Flags)
		seenNames := make(map[string]string)
		seenShorthands := make(map[string]string)
		for _, record := range records {
			if owner, exists := inputOwners[record.ID]; exists {
				return genericFlagError(plan[index].record.ID, record.ID, "stable input ID is already declared by command %q", owner)
			}
			inputOwners[record.ID] = plan[index].record.ID
			if err := validateGenericFlag(plan[index].record.ID, record, seenNames, seenShorthands); err != nil {
				return err
			}
			flagPlan := plannedFlag{record: record, canonicalID: record.ID}
			switch record.Scope {
			case "local", "persistent":
				if _, exists := declarations[record.ID]; exists {
					return genericFlagError(plan[index].record.ID, record.ID, "stable input ID is duplicated")
				}
				declarations[record.ID] = declaration{flag: record, commandPath: plan[index].record.Path}
			case "inherited":
				target, exists := declarations[record.InheritedFromID]
				if !exists {
					return genericFlagError(
						plan[index].record.ID,
						record.ID,
						"inherited input %q does not reference a declared ancestor flag",
						record.InheritedFromID,
					)
				}
				if target.flag.Scope != "persistent" ||
					!strings.HasPrefix(plan[index].record.Path, target.commandPath+" ") {
					return genericFlagError(
						plan[index].record.ID,
						record.ID,
						"inheritance target %q is not persistent on an ancestor command",
						record.InheritedFromID,
					)
				}
				if err := validateInheritedFlag(record, target.flag); err != nil {
					return genericFlagError(plan[index].record.ID, record.ID, "%v", err)
				}
				flagPlan.canonicalID = target.flag.ID
			}
			plan[index].flags = append(plan[index].flags, flagPlan)
		}
	}
	return nil
}

func sortedFlagMapKeys(flags map[string]climanifest.Flag) []string {
	keys := make([]string, 0, len(flags))
	for key := range flags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validateGenericFlag(
	commandID string,
	flag climanifest.Flag,
	seenNames map[string]string,
	seenShorthands map[string]string,
) error {
	if strings.TrimSpace(flag.ID) == "" {
		return genericFlagError(commandID, flag.ID, "stable input ID is required")
	}
	if strings.TrimSpace(flag.Long) == "" || strings.ContainsAny(flag.Long, " \t\r\n") {
		return genericFlagError(commandID, flag.ID, "long name %q must be one non-empty segment", flag.Long)
	}
	switch flag.Scope {
	case "local", "persistent":
		if flag.InheritedFromID != "" {
			return genericFlagError(commandID, flag.ID, "scope %q cannot declare inheritance target %q", flag.Scope, flag.InheritedFromID)
		}
	case "inherited":
		if flag.InheritedFromID == "" {
			return genericFlagError(commandID, flag.ID, "inherited scope requires inheritedFromInputId")
		}
	default:
		return genericFlagError(commandID, flag.ID, "unsupported scope %q", flag.Scope)
	}
	if err := validateGenericFlagNames(commandID, flag, seenNames, seenShorthands); err != nil {
		return err
	}
	if err := validateGenericFlagShape(commandID, flag); err != nil {
		return err
	}
	return validateGenericFlagDefaults(commandID, flag)
}

func validateGenericFlagNames(
	commandID string,
	flag climanifest.Flag,
	seenNames map[string]string,
	seenShorthands map[string]string,
) error {
	names := append([]string{flag.Long}, flag.Aliases...)
	for _, name := range names {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, " \t\r\n") {
			return genericFlagError(commandID, flag.ID, "flag name or alias %q must be one non-empty segment", name)
		}
		if owner, exists := seenNames[name]; exists {
			return genericFlagError(commandID, flag.ID, "public name %q conflicts with input %q", name, owner)
		}
		if name == "help" {
			return genericFlagError(commandID, flag.ID, "public name %q is reserved by Cobra", name)
		}
		seenNames[name] = flag.ID
	}
	if flag.Shorthand != "" && len([]rune(flag.Shorthand)) != 1 {
		return genericFlagError(commandID, flag.ID, "shorthand %q must be one character", flag.Shorthand)
	}
	if flag.Shorthand == "h" {
		return genericFlagError(commandID, flag.ID, "shorthand %q is reserved by Cobra", flag.Shorthand)
	}
	if owner, exists := seenShorthands[flag.Shorthand]; flag.Shorthand != "" && exists {
		return genericFlagError(commandID, flag.ID, "shorthand %q conflicts with input %q", flag.Shorthand, owner)
	}
	if flag.Shorthand != "" {
		seenShorthands[flag.Shorthand] = flag.ID
	}
	return nil
}

func validateGenericFlagDefaults(commandID string, flag climanifest.Flag) error {
	defaultValue, err := genericFlagDefault(flag)
	if err != nil {
		return genericFlagError(commandID, flag.ID, "invalid typed default: %v", err)
	}
	if hasGenericFlagDefault(flag) {
		if err := validateEnumValue(flag, defaultValue); err != nil {
			return genericFlagError(commandID, flag.ID, "invalid typed default: %v", err)
		}
	}
	if flag.NoOptionValue == nil && flag.NoOptionDefault == "" {
		return nil
	}
	noOptionValue, err := genericNoOptionValue(flag)
	if err != nil {
		return genericFlagError(commandID, flag.ID, "invalid no-option default: %v", err)
	}
	if flag.ValueType == "stringArray" {
		return genericFlagError(commandID, flag.ID, "no-option default is incompatible with repeated-string flags")
	}
	if err := validateEnumValue(flag, noOptionValue); err != nil {
		return genericFlagError(commandID, flag.ID, "invalid no-option default: %v", err)
	}
	return nil
}

func validateInheritedFlag(inherited, declared climanifest.Flag) error {
	comparable := inherited
	comparable.ID = declared.ID
	comparable.Scope = declared.Scope
	comparable.InheritedFromID = declared.InheritedFromID
	comparable.Lifecycle.ItemID = declared.Lifecycle.ItemID
	if !reflect.DeepEqual(comparable, declared) {
		return fmt.Errorf("inheritance target %q has incompatible flag metadata", declared.ID)
	}
	return nil
}

func genericFlagError(commandID, inputID, format string, args ...any) error {
	return fmt.Errorf("command %q input %q: %s", commandID, inputID, fmt.Sprintf(format, args...))
}

func projectFlags(cmd *cobra.Command, plan plannedCommand, targets map[string]*genericFlagValue) error {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	for _, item := range plan.flags {
		cmd.Annotations[genericInputAnnotationPrefix+item.record.ID] = item.record.Long
		if item.record.Scope == "inherited" {
			if targets[item.canonicalID] == nil {
				return genericFlagError(plan.record.ID, item.record.ID, "canonical inherited storage %q is unavailable", item.canonicalID)
			}
			continue
		}
		value, err := newGenericFlagValue(item.record)
		if err != nil {
			return genericFlagError(plan.record.ID, item.record.ID, "allocate typed storage: %v", err)
		}
		targets[item.canonicalID] = value
		flagSet := cmd.Flags()
		if item.record.Scope == "persistent" {
			flagSet = cmd.PersistentFlags()
		}
		if err := registerGenericFlag(flagSet, item.record, value); err != nil {
			return genericFlagError(plan.record.ID, item.record.ID, "register flag: %v", err)
		}
	}
	return nil
}

func registerGenericFlag(flagSet *pflag.FlagSet, record climanifest.Flag, value *genericFlagValue) error {
	flagSet.VarP(value, record.Long, record.Shorthand, projectedFlagUsage(record))
	registered := flagSet.Lookup(record.Long)
	registered.Hidden = record.Visibility == "hidden"
	registered.Annotations = map[string][]string{"infinite-you/input-id": {record.ID}}
	if record.NoOptionValue != nil || record.NoOptionDefault != "" {
		noOption, err := genericNoOptionValue(record)
		if err != nil {
			return err
		}
		registered.NoOptDefVal = genericFlagString(noOption)
	}
	for _, alias := range record.Aliases {
		flagSet.Var(value, alias, "")
		aliasFlag := flagSet.Lookup(alias)
		aliasFlag.Hidden = true
		aliasFlag.NoOptDefVal = registered.NoOptDefVal
	}
	return nil
}

func validateRequiredGenericFlags(cmd *cobra.Command, plan plannedCommand) error {
	for _, item := range plan.flags {
		if !item.record.Required {
			continue
		}
		names := append([]string{item.record.Long}, item.record.Aliases...)
		for _, name := range names {
			if flag := lookupCommandFlag(cmd, name); flag != nil && flag.Changed {
				goto nextFlag
			}
		}
		return fmt.Errorf("required flag(s) %q not set", "--"+item.record.Long)
	nextFlag:
	}
	return nil
}
