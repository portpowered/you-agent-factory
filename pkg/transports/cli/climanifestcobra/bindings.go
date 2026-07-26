package climanifestcobra

import (
	"context"
	"encoding/json"
	"fmt"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// CompletionRegistry maps stable manifest input IDs to transport-edge dynamic
// completion callbacks.
type CompletionRegistry map[string]cobra.CompletionFunc

// GenericHandler receives one invocation's normalized inputs keyed by stable
// manifest input ID. ResolvedPersistentInputsFromContext provides the
// invocation's inherited root/global snapshot without coupling the handler to
// Cobra or public command names.
type GenericHandler func(context.Context, map[string]any) error

// HandlerRegistry maps stable manifest handler IDs to transport-edge behavior.
type HandlerRegistry map[string]GenericHandler

// CobraHandler adapts an existing transport-owned Cobra handler while a command
// family migrates to normalized stable-ID inputs. New handler implementations
// should prefer GenericHandler so they remain independent of Cobra.
type CobraHandler func(*cobra.Command, []string, map[string]any, resolvedinput.Inputs) error

// CobraHandlerRegistry maps stable manifest handler IDs to migration adapters.
type CobraHandlerRegistry map[string]CobraHandler

// ResolvedCobraHandler is the transport-edge adapter for commands whose local
// inputs have migrated to the canonical resolved-input model. The second
// snapshot contains the invocation's inherited root inputs.
type ResolvedCobraHandler = func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error

// ResolvedCobraHandlerRegistry maps stable handler IDs to typed transport-edge
// adapters without exposing public argument spellings or raw Cobra arguments.
type ResolvedCobraHandlerRegistry map[string]ResolvedCobraHandler

// InputBinding receives a normalized value whenever Cobra parses the named
// stable input. It is a migration seam for handlers that still own typed
// option structs; generic handlers should consume their values map directly.
type InputBinding func(any) error

// InputBindingRegistry maps stable manifest input IDs to migration bindings.
type InputBindingRegistry map[string]InputBinding

// SourceCandidateProvider collects one non-CLI candidate through the
// manifest-owned source binding. Returning present=false leaves resolution to
// the next lower-precedence source.
type SourceCandidateProvider func(
	context.Context,
	climanifest.SourceBinding,
	resolvedinput.ValueKind,
) (value resolvedinput.Value, present bool, err error)

// ResolvedInputsBinding is the temporary family-migration seam that applies one
// complete persistent snapshot to handlers that still own typed option structs.
type ResolvedInputsBinding func(resolvedinput.Inputs) error

// GenericBindings supplies executable transport bindings used while projecting
// a generic manifest. Additional stable-ID registries can be added here without
// coupling manifest records to public command or input spellings.
type GenericBindings struct {
	Completions             CompletionRegistry
	Handlers                HandlerRegistry
	CobraHandlers           CobraHandlerRegistry
	ResolvedCobraHandlers   ResolvedCobraHandlerRegistry
	Inputs                  InputBindingRegistry
	SourceValues            SourceCandidateProvider
	RootInputs              ResolvedInputsBinding
	GuardUnknownSubcommands bool
}

// SessionFamilyBindings supplies the legacy typed option structs updated by
// stable input-ID bindings while Session handlers migrate to normalized values.
type SessionFamilyBindings struct {
	Create     *sessioncli.CreateConfig
	List       *sessioncli.ListConfig
	Delete     *sessioncli.DeleteConfig
	Dispatches *sessioncli.DispatchesConfig
	Pause      *sessioncli.LifecycleControlConfig
	Resume     *sessioncli.LifecycleControlConfig
	FlagUsages map[string]string
}

type persistentInputResolver struct {
	definitions    []resolvedinput.Definition
	defaults       []resolvedinput.Candidate
	targets        map[string]*genericFlagValue
	sourceBindings []climanifest.SourceBinding
	sourceValues   SourceCandidateProvider
	binding        ResolvedInputsBinding
}

type resolvedPersistentInputsContextKey struct{}

type persistentInputInvocation struct {
	resolver persistentInputResolver
	inputs   resolvedinput.Inputs
}

func newPersistentInputResolver(
	plan []plannedCommand,
	targets map[string]*genericFlagValue,
	sourceValues SourceCandidateProvider,
	binding ResolvedInputsBinding,
) (persistentInputResolver, error) {
	var root *plannedCommand
	for index := range plan {
		if plan[index].parentPath == "" {
			root = &plan[index]
			break
		}
	}
	if root == nil {
		return persistentInputResolver{}, fmt.Errorf("resolve persistent inputs: root command is unavailable")
	}
	resolver := persistentInputResolver{
		targets:      make(map[string]*genericFlagValue),
		sourceValues: sourceValues,
		binding:      binding,
	}
	for _, item := range root.flags {
		if item.record.Scope != "persistent" || len(item.record.AcceptedSources) == 0 {
			continue
		}
		definition, defaultCandidate, err := persistentInputContract(item.record, root.record.Precedence)
		if err != nil {
			return persistentInputResolver{}, genericFlagError(root.record.ID, item.record.ID, "resolved input: %v", err)
		}
		resolver.definitions = append(resolver.definitions, definition)
		if defaultCandidate != nil {
			resolver.defaults = append(resolver.defaults, *defaultCandidate)
		}
		resolver.targets[item.record.ID] = targets[item.record.ID]
	}
	for _, bindingID := range sortedKeys(root.record.SourceBindings) {
		binding := root.record.SourceBindings[bindingID]
		if _, tracked := resolver.targets[binding.InputID]; tracked {
			resolver.sourceBindings = append(resolver.sourceBindings, binding)
		}
	}
	return resolver, nil
}

func persistentInputContract(
	flag climanifest.Flag,
	precedence climanifest.Precedence,
) (resolvedinput.Definition, *resolvedinput.Candidate, error) {
	kind, err := resolvedValueKind(flag.ValueType)
	if err != nil {
		return resolvedinput.Definition{}, nil, err
	}
	sources := make([]resolvedinput.Source, 0, len(flag.AcceptedSources))
	accepted := make(map[string]bool, len(flag.AcceptedSources))
	for _, source := range flag.AcceptedSources {
		accepted[source] = true
	}
	for _, source := range precedence.Order {
		if !accepted[source] {
			continue
		}
		mapped, err := resolvedSource(source)
		if err != nil {
			return resolvedinput.Definition{}, nil, err
		}
		sources = append(sources, mapped)
	}
	definition := resolvedinput.Definition{
		ID:         flag.ID,
		Kind:       kind,
		Precedence: sources,
		Sensitive:  flag.Sensitivity != "" && flag.Sensitivity != "public",
	}
	if !accepted[climanifest.SourceManifestDefault] {
		return definition, nil, nil
	}
	value, err := genericFlagDefault(flag)
	if err != nil {
		return resolvedinput.Definition{}, nil, err
	}
	resolvedValue, err := resolvedValue(value)
	if err != nil {
		return resolvedinput.Definition{}, nil, err
	}
	return definition, &resolvedinput.Candidate{
		InputID: flag.ID,
		Source:  resolvedinput.SourceManifestDefault,
		Value:   resolvedValue,
	}, nil
}

func installPersistentInputResolution(
	root *cobra.Command,
	resolver persistentInputResolver,
	rootNoArgumentDiscovery bool,
) {
	if len(resolver.definitions) == 0 {
		return
	}
	previous := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if rootNoArgumentDiscovery && cmd == root && len(args) == 0 {
			inputs, err := resolver.resolve(cmd.Context(), false)
			if err != nil {
				return err
			}
			attachPersistentInputInvocation(cmd, resolver, inputs)
			return nil
		}
		inputs, err := resolver.resolve(cmd.Context(), !cmd.DisableFlagParsing)
		if err != nil {
			return err
		}
		if resolver.binding != nil {
			if err := resolver.binding(inputs); err != nil {
				return fmt.Errorf("bind resolved persistent inputs: %w", err)
			}
		}
		attachPersistentInputInvocation(cmd, resolver, inputs)
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
}

func attachPersistentInputInvocation(
	cmd *cobra.Command,
	resolver persistentInputResolver,
	inputs resolvedinput.Inputs,
) {
	cmd.SetContext(context.WithValue(
		cmd.Context(),
		resolvedPersistentInputsContextKey{},
		persistentInputInvocation{resolver: resolver, inputs: inputs},
	))
}

func (r persistentInputResolver) resolve(
	ctx context.Context,
	collectExternalSources bool,
) (resolvedinput.Inputs, error) {
	candidates := append([]resolvedinput.Candidate(nil), r.defaults...)
	cliInputs := make(map[string]bool, len(r.targets))
	for inputID, target := range r.targets {
		if target == nil || !target.changed {
			continue
		}
		value, err := resolvedValue(target.Get())
		if err != nil {
			return resolvedinput.Inputs{}, fmt.Errorf("resolve persistent input %q CLI value: %w", inputID, err)
		}
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: inputID,
			Source:  resolvedinput.SourceCLIFlag,
			Value:   value,
		})
		cliInputs[inputID] = true
	}
	if collectExternalSources && r.sourceValues != nil {
		kinds := make(map[string]resolvedinput.ValueKind, len(r.definitions))
		for _, definition := range r.definitions {
			kinds[definition.ID] = definition.Kind
		}
		for _, binding := range r.sourceBindings {
			if cliInputs[binding.InputID] {
				continue
			}
			value, present, err := r.sourceValues(ctx, binding, kinds[binding.InputID])
			if err != nil {
				return resolvedinput.Inputs{}, fmt.Errorf("resolve source binding %q: %w", binding.ID, err)
			}
			if !present {
				continue
			}
			source, err := resolvedSource(binding.Source)
			if err != nil {
				return resolvedinput.Inputs{}, fmt.Errorf("resolve source binding %q: %w", binding.ID, err)
			}
			candidates = append(candidates, resolvedinput.Candidate{
				InputID: binding.InputID,
				Source:  source,
				Value:   value,
			})
		}
	}
	inputs, err := resolvedinput.Resolve(r.definitions, candidates)
	if err != nil {
		return resolvedinput.Inputs{}, fmt.Errorf("resolve persistent inputs: %w", err)
	}
	return inputs, nil
}

// ResolvedPersistentInputs returns the invocation-local root/global inputs
// inherited by the selected command, keyed only by canonical manifest input ID.
func ResolvedPersistentInputs(cmd *cobra.Command) (resolvedinput.Inputs, error) {
	if cmd == nil {
		return resolvedinput.Inputs{}, fmt.Errorf("read resolved persistent inputs: command is required")
	}
	return ResolvedPersistentInputsFromContext(cmd.Context())
}

// ResolvedPersistentInputsFromContext returns the same invocation-local
// snapshot for schema-neutral handlers that do not depend on Cobra.
func ResolvedPersistentInputsFromContext(ctx context.Context) (resolvedinput.Inputs, error) {
	if ctx == nil {
		return resolvedinput.Inputs{}, fmt.Errorf("read resolved persistent inputs: context is required")
	}
	invocation, ok := ctx.Value(resolvedPersistentInputsContextKey{}).(persistentInputInvocation)
	if !ok {
		return resolvedinput.Inputs{}, fmt.Errorf("read resolved persistent inputs: invocation has not resolved root inputs")
	}
	return invocation.inputs, nil
}

// ResolvePersistentInputsForObservation runs only the root input-resolution
// boundary after a caller has parsed and validated argv. It never dispatches
// the selected command handler.
func ResolvePersistentInputsForObservation(
	cmd *cobra.Command,
	args []string,
) (resolvedinput.Inputs, error) {
	if cmd == nil {
		return resolvedinput.Inputs{}, fmt.Errorf("observe resolved persistent inputs: command is required")
	}
	root := cmd.Root()
	if root.PersistentPreRunE == nil {
		return resolvedinput.Inputs{}, fmt.Errorf("observe resolved persistent inputs: root resolver is unavailable")
	}
	if cmd.Context() == nil {
		cmd.SetContext(root.Context())
	}
	if err := root.PersistentPreRunE(cmd, args); err != nil {
		return resolvedinput.Inputs{}, err
	}
	if cmd.DisableFlagParsing {
		if err := RefreshResolvedPersistentInputs(cmd); err != nil {
			return resolvedinput.Inputs{}, err
		}
	}
	return ResolvedPersistentInputs(cmd)
}

// RefreshResolvedPersistentInputs re-resolves the root/global snapshot after a
// compatibility command performs its own Cobra-permitted flag parsing.
func RefreshResolvedPersistentInputs(cmd *cobra.Command) error {
	if cmd == nil {
		return fmt.Errorf("refresh resolved persistent inputs: command is required")
	}
	invocation, ok := cmd.Context().Value(resolvedPersistentInputsContextKey{}).(persistentInputInvocation)
	if !ok {
		return fmt.Errorf("refresh resolved persistent inputs: invocation has not initialized root inputs")
	}
	inputs, err := invocation.resolver.resolve(cmd.Context(), true)
	if err != nil {
		return err
	}
	if invocation.resolver.binding != nil {
		if err := invocation.resolver.binding(inputs); err != nil {
			return fmt.Errorf("bind resolved persistent inputs: %w", err)
		}
	}
	invocation.inputs = inputs
	cmd.SetContext(context.WithValue(cmd.Context(), resolvedPersistentInputsContextKey{}, invocation))
	return nil
}

func resolvedValueKind(valueType string) (resolvedinput.ValueKind, error) {
	switch valueType {
	case "bool":
		return resolvedinput.ValueKindBool, nil
	case "string":
		return resolvedinput.ValueKindString, nil
	case "int":
		return resolvedinput.ValueKindInt, nil
	case "int64":
		return resolvedinput.ValueKindInt64, nil
	case "stringArray":
		return resolvedinput.ValueKindStringArray, nil
	default:
		return "", fmt.Errorf("unsupported value type %q", valueType)
	}
}

func resolvedValue(value any) (resolvedinput.Value, error) {
	switch typed := value.(type) {
	case bool:
		return resolvedinput.BoolValue(typed), nil
	case string:
		return resolvedinput.StringValue(typed), nil
	case int:
		return resolvedinput.IntValue(typed), nil
	case int64:
		return resolvedinput.Int64Value(typed), nil
	case []string:
		return resolvedinput.StringArrayValue(typed), nil
	default:
		return resolvedinput.Value{}, fmt.Errorf("unsupported canonical value type %T", value)
	}
}

func resolvedCommandInputs(
	cmd *cobra.Command,
	record climanifest.Command,
	values map[string]any,
) (resolvedinput.Inputs, error) {
	definitions := make([]resolvedinput.Definition, 0, len(record.Arguments)+len(record.Flags))
	candidates := make([]resolvedinput.Candidate, 0, len(record.Arguments)+len(record.Flags))
	if err := appendResolvedArguments(
		cmd,
		record,
		values,
		&definitions,
		&candidates,
	); err != nil {
		return resolvedinput.Inputs{}, err
	}
	if err := appendResolvedLocalFlags(
		cmd,
		record,
		values,
		&definitions,
		&candidates,
	); err != nil {
		return resolvedinput.Inputs{}, err
	}
	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		return resolvedinput.Inputs{}, fmt.Errorf("resolve command inputs: %w", err)
	}
	return inputs, nil
}

func appendResolvedArguments(
	cmd *cobra.Command,
	record climanifest.Command,
	values map[string]any,
	definitions *[]resolvedinput.Definition,
	candidates *[]resolvedinput.Candidate,
) error {
	for _, argument := range record.Arguments {
		if argument.HandlerBindingID == "" {
			continue
		}
		definition, err := resolvedCommandInputDefinition(
			argument.ID,
			argument.ValueType,
			argument.AcceptedSources,
			record.Precedence,
			true,
			false,
		)
		if err != nil {
			return fmt.Errorf("resolve argument %q: %w", argument.ID, err)
		}
		*definitions = append(*definitions, definition)
		if argument.DefaultValue != nil {
			value, err := typedGenericInputValue(argument.ValueType, argument.DefaultValue)
			if err != nil {
				return fmt.Errorf("resolve argument %q default: %w", argument.ID, err)
			}
			candidate, err := resolvedCommandCandidate(argument.ID, resolvedinput.SourceManifestDefault, value)
			if err != nil {
				return err
			}
			*candidates = append(*candidates, candidate)
		}
		present, err := storedArgumentPresent(cmd, argument.ID)
		if err != nil {
			return fmt.Errorf("resolve argument %q: %w", argument.ID, err)
		}
		if present {
			candidate, err := resolvedCommandCandidate(
				argument.ID,
				resolvedinput.SourcePositionalArgument,
				values[argument.ID],
			)
			if err != nil {
				return err
			}
			*candidates = append(*candidates, candidate)
		}
	}
	return nil
}

func appendResolvedLocalFlags(
	cmd *cobra.Command,
	record climanifest.Command,
	values map[string]any,
	definitions *[]resolvedinput.Definition,
	candidates *[]resolvedinput.Candidate,
) error {
	for _, flag := range record.Flags {
		if !isCanonicalFlagRecord(flag) || flag.Scope != "local" {
			continue
		}
		definition, err := resolvedCommandInputDefinition(
			flag.ID,
			flag.ValueType,
			flag.AcceptedSources,
			record.Precedence,
			false,
			flag.Sensitivity != "" && flag.Sensitivity != "public",
		)
		if err != nil {
			return fmt.Errorf("resolve flag %q: %w", flag.ID, err)
		}
		*definitions = append(*definitions, definition)
		if flag.DefaultValue != nil {
			value, err := typedGenericInputValue(flag.ValueType, flag.DefaultValue)
			if err != nil {
				return fmt.Errorf("resolve flag %q default: %w", flag.ID, err)
			}
			candidate, err := resolvedCommandCandidate(flag.ID, resolvedinput.SourceManifestDefault, value)
			if err != nil {
				return err
			}
			*candidates = append(*candidates, candidate)
		}
		if parsed := lookupCommandFlag(cmd, flag.Long); parsed != nil && parsed.Changed {
			candidate, err := resolvedCommandCandidate(flag.ID, resolvedinput.SourceCLIFlag, values[flag.ID])
			if err != nil {
				return err
			}
			*candidates = append(*candidates, candidate)
		}
	}
	return nil
}

func resolvedCommandInputDefinition(
	id string,
	valueType string,
	acceptedSources []string,
	precedence climanifest.Precedence,
	positional bool,
	sensitive bool,
) (resolvedinput.Definition, error) {
	kind, err := resolvedValueKind(valueType)
	if err != nil {
		return resolvedinput.Definition{}, err
	}
	accepted := make(map[string]bool, len(acceptedSources))
	for _, source := range acceptedSources {
		accepted[source] = true
	}
	sources := make([]resolvedinput.Source, 0, len(acceptedSources))
	for _, source := range precedence.Order {
		if !accepted[source] {
			continue
		}
		mapped, err := resolvedCommandSource(source, positional)
		if err != nil {
			return resolvedinput.Definition{}, err
		}
		sources = append(sources, mapped)
	}
	return resolvedinput.Definition{
		ID: id, Kind: kind, Precedence: sources, Sensitive: sensitive,
	}, nil
}

func resolvedCommandSource(source string, positional bool) (resolvedinput.Source, error) {
	if source == climanifest.SourceCLI && positional {
		return resolvedinput.SourcePositionalArgument, nil
	}
	return resolvedSource(source)
}

func resolvedCommandCandidate(
	inputID string,
	source resolvedinput.Source,
	value any,
) (resolvedinput.Candidate, error) {
	resolved, err := resolvedValue(value)
	if err != nil {
		return resolvedinput.Candidate{}, fmt.Errorf("resolve input %q value: %w", inputID, err)
	}
	return resolvedinput.Candidate{InputID: inputID, Source: source, Value: resolved}, nil
}

func storedArgumentPresent(cmd *cobra.Command, inputID string) (bool, error) {
	if cmd == nil {
		return false, fmt.Errorf("command is required")
	}
	encoded, exists := cmd.Annotations[genericArgumentAnnotationPrefix+inputID]
	if !exists {
		return false, fmt.Errorf("parsed value is unavailable")
	}
	var value encodedArgumentValue
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return false, fmt.Errorf("decode parsed value: %w", err)
	}
	return value.Present, nil
}

func resolvedSource(source string) (resolvedinput.Source, error) {
	switch source {
	case climanifest.SourceCLI:
		return resolvedinput.SourceCLIFlag, nil
	case climanifest.SourceStdin:
		return resolvedinput.SourceStdin, nil
	case climanifest.SourceEnvironment:
		return resolvedinput.SourceEnvironment, nil
	case climanifest.SourceOperatorConfig:
		return resolvedinput.SourceOperatorConfig, nil
	case climanifest.SourceManifestDefault:
		return resolvedinput.SourceManifestDefault, nil
	case climanifest.SourceFactorySignatureDefault:
		return resolvedinput.SourceFactorySignatureDefault, nil
	default:
		return "", fmt.Errorf("unsupported source %q", source)
	}
}
