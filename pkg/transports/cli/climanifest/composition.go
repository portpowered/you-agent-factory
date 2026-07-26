package climanifest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	CompositionCollisionCommandName = "cli.composition.command-name-collision"
	CompositionCollisionLongName    = "cli.composition.long-name-collision"
	CompositionCollisionAlias       = "cli.composition.alias-collision"
	CompositionCollisionShorthand   = "cli.composition.shorthand-collision"
	CompositionCollisionPosition    = "cli.composition.positional-collision"
	CompositionCollisionStdin       = "cli.composition.stdin-collision"
	CompositionCollisionBindingID   = "cli.composition.binding-id-collision"
)

const (
	EffectiveValueConsumptionSingle               = "single-value"
	EffectiveValueConsumptionRepeated             = "repeated-values"
	EffectiveValueConsumptionRemainingPositionals = "remaining-positionals"
	EffectiveValueConsumptionFileContents         = "file-contents"

	EffectiveUnboundedCardinality = -1
)

// EffectiveInputSchema is the immutable result of composing one static CLI
// command with one selected Factory invocation signature. Static inputs remain
// manifest-owned; the Factory contributes only invocation parameters.
type EffectiveInputSchema struct {
	CommandID                  string
	UnknownNamedArgumentPolicy string
	StaticInputs               []EffectiveStaticInput
	FactoryParameters          []EffectiveFactoryParameter
}

// EffectiveStaticInput is one manifest-owned input in the selected command.
type EffectiveStaticInput struct {
	ID               string
	Kind             string
	Scope            string
	HandlerBindingID string
	Position         int
	PublicSpellings  []string
	ConsumesStdin    bool
}

// EffectiveFactoryParameter is one selected-Factory dynamic input with the
// normalized facts consumed by help, validation, normalization, and completion.
// BindingID and CanonicalName are the stable key used by normalized invocation
// argument maps and handler interpolation.
type EffectiveFactoryParameter struct {
	BindingID             string
	CanonicalName         string
	PreferredExternalName string
	Aliases               []string
	Description           string
	Required              bool
	Choices               []string
	DefaultValue          *string
	DefaultValues         []string
	ValueMode             string
	ValueConsumption      string
	MinimumValues         int
	MaximumValues         int
	TypeHint              string
	Sensitive             bool
	Bindings              []work.InvocationParameterBindingConfig
}

// CompositionDiagnostic identifies both owners of one rejected collision.
type CompositionDiagnostic struct {
	Code         string
	Path         string
	Message      string
	StaticOwner  string
	FactoryOwner string
}

type reservedSpelling struct {
	kind  string
	owner string
}

// ComposeRunInputs combines a validated static command and selected Factory
// signature without mutating either contract. Any collision rejects the
// composition; callers must not use the returned schema when diagnostics are
// present.
func ComposeRunInputs(manifest Manifest, commandID string, signature work.InvocationSignatureConfig) (EffectiveInputSchema, []CompositionDiagnostic, error) {
	command, ok := manifest.Commands[commandID]
	if !ok {
		return EffectiveInputSchema{}, nil, fmt.Errorf("CLI manifest missing static command %q", commandID)
	}

	schema := EffectiveInputSchema{
		CommandID:                  commandID,
		UnknownNamedArgumentPolicy: normalizedUnknownNamedArgumentPolicy(signature.UnknownNamedArgumentPolicy),
		StaticInputs:               projectStaticInputs(command),
		FactoryParameters:          projectFactoryParameters(signature.Parameters),
	}
	diagnostics := compositionDiagnostics(manifest, command, signature.Parameters)
	if len(diagnostics) != 0 {
		return EffectiveInputSchema{}, diagnostics, nil
	}
	return schema, diagnostics, nil
}

func projectStaticInputs(command Command) []EffectiveStaticInput {
	inputs := make([]EffectiveStaticInput, 0, len(command.Arguments)+len(command.Flags))
	for _, argument := range command.Arguments {
		inputs = append(inputs, EffectiveStaticInput{
			ID:               argument.ID,
			Kind:             "argument",
			Scope:            argument.Scope,
			HandlerBindingID: argument.HandlerBindingID,
			Position:         argument.Position,
			PublicSpellings:  []string{argument.Name},
			ConsumesStdin:    containsString(argument.AcceptedSources, SourceStdin) || containsString(argument.Channels, SourceStdin),
		})
	}
	for _, flag := range command.Flags {
		spellings := append([]string{flag.Long}, flag.Aliases...)
		if flag.Shorthand != "" {
			spellings = append(spellings, flag.Shorthand)
		}
		inputs = append(inputs, EffectiveStaticInput{
			ID:               flag.ID,
			Kind:             "flag",
			Scope:            flag.Scope,
			HandlerBindingID: flag.HandlerBindingID,
			Position:         -1,
			PublicSpellings:  append([]string(nil), spellings...),
			ConsumesStdin:    containsString(flag.AcceptedSources, SourceStdin),
		})
	}
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Kind != inputs[j].Kind {
			return inputs[i].Kind < inputs[j].Kind
		}
		return inputs[i].ID < inputs[j].ID
	})
	return inputs
}

func projectFactoryParameters(parameters []work.InvocationParameterConfig) []EffectiveFactoryParameter {
	projected := make([]EffectiveFactoryParameter, 0, len(parameters))
	for _, parameter := range parameters {
		canonicalName := strings.TrimSpace(parameter.Name)
		preferredExternalName := strings.TrimSpace(parameter.ExternalName)
		if preferredExternalName == "" {
			preferredExternalName = canonicalName
		}
		valueMode := work.NormalizeInvocationValueMode(parameter.ValueMode)
		minimumValues, maximumValues, consumption := effectiveValueFacts(valueMode, parameter.Required)
		projected = append(projected, EffectiveFactoryParameter{
			BindingID:             canonicalName,
			CanonicalName:         canonicalName,
			PreferredExternalName: preferredExternalName,
			Aliases:               normalizedNonEmptyStrings(parameter.Aliases),
			Description:           parameter.Description,
			Required:              parameter.Required,
			Choices:               append([]string(nil), parameter.Choices...),
			DefaultValue:          cloneDefaultValue(parameter.DefaultValue),
			DefaultValues:         append([]string(nil), parameter.DefaultValues...),
			ValueMode:             valueMode,
			ValueConsumption:      consumption,
			MinimumValues:         minimumValues,
			MaximumValues:         maximumValues,
			TypeHint:              strings.TrimSpace(parameter.TypeHint),
			Sensitive:             parameter.Sensitive,
			Bindings:              normalizedBindings(parameter.Bindings),
		})
	}
	sort.Slice(projected, func(i, j int) bool { return projected[i].BindingID < projected[j].BindingID })
	return projected
}

func normalizedUnknownNamedArgumentPolicy(policy string) string {
	trimmed := strings.TrimSpace(policy)
	if trimmed == "" {
		return work.InvocationUnknownNamedArgumentPolicyReject
	}
	return trimmed
}

func normalizedNonEmptyStrings(values []string) []string {
	var normalized []string
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}

func normalizedBindings(bindings []work.InvocationParameterBindingConfig) []work.InvocationParameterBindingConfig {
	normalized := make([]work.InvocationParameterBindingConfig, len(bindings))
	for index, binding := range bindings {
		normalized[index] = work.InvocationParameterBindingConfig{
			Kind:     strings.TrimSpace(binding.Kind),
			Position: binding.Position,
		}
	}
	return normalized
}

func cloneDefaultValue(value string) *string {
	if value == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func effectiveValueFacts(valueMode string, required bool) (int, int, string) {
	minimumValues := 0
	if required {
		minimumValues = 1
	}
	switch valueMode {
	case work.InvocationParameterValueModeRepeated:
		return minimumValues, EffectiveUnboundedCardinality, EffectiveValueConsumptionRepeated
	case work.InvocationParameterValueModeVariadic:
		return minimumValues, EffectiveUnboundedCardinality, EffectiveValueConsumptionRemainingPositionals
	case work.InvocationParameterValueModeFileContents:
		return minimumValues, 1, EffectiveValueConsumptionFileContents
	default:
		return minimumValues, 1, EffectiveValueConsumptionSingle
	}
}

func compositionDiagnostics(manifest Manifest, command Command, parameters []work.InvocationParameterConfig) []CompositionDiagnostic {
	spellings := reservedStaticSpellings(manifest, command)
	positions, stdinOwner, bindingIDs := reservedStaticInputs(command)
	var diagnostics []CompositionDiagnostic
	for index, parameter := range parameters {
		owner := parameter.Name
		path := fmt.Sprintf("/invocationSignature/parameters/%d", index)
		if staticOwner, collision := bindingIDs[parameter.Name]; collision {
			diagnostics = append(diagnostics, newCompositionDiagnostic(CompositionCollisionBindingID, path+"/name", parameter.Name, staticOwner, owner))
		}
		for field, value := range factoryNamedSpellings(parameter) {
			for _, reserved := range spellings[value] {
				diagnostics = append(diagnostics, newCompositionDiagnostic(collisionCode(reserved.kind), path+field, value, reserved.owner, owner))
			}
		}
		for bindingIndex, binding := range parameter.Bindings {
			bindingPath := fmt.Sprintf("%s/bindings/%d", path, bindingIndex)
			switch binding.Kind {
			case work.InvocationParameterBindingKindPositional:
				if staticOwner, collision := positions[binding.Position]; collision {
					diagnostics = append(diagnostics, newCompositionDiagnostic(CompositionCollisionPosition, bindingPath+"/position", fmt.Sprint(binding.Position), staticOwner, owner))
				}
			case work.InvocationParameterBindingKindStdin:
				if stdinOwner != "" {
					diagnostics = append(diagnostics, newCompositionDiagnostic(CompositionCollisionStdin, bindingPath+"/kind", SourceStdin, stdinOwner, owner))
				}
			}
		}
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.StaticOwner != right.StaticOwner {
			return left.StaticOwner < right.StaticOwner
		}
		return left.FactoryOwner < right.FactoryOwner
	})
	return diagnostics
}

func reservedStaticSpellings(manifest Manifest, command Command) map[string][]reservedSpelling {
	reserved := make(map[string][]reservedSpelling)
	for _, commandID := range sortedCommandIDs(manifest.Commands) {
		record := manifest.Commands[commandID]
		addReservedSpelling(reserved, record.Name, "command", record.ID)
		for _, alias := range record.Aliases {
			addReservedSpelling(reserved, alias, "command", record.ID)
		}
	}
	for _, flagID := range sortedFlagIDs(command.Flags) {
		flag := command.Flags[flagID]
		addReservedSpelling(reserved, flag.Long, "long", flag.ID)
		for _, alias := range flag.Aliases {
			addReservedSpelling(reserved, alias, "alias", flag.ID)
		}
		addReservedSpelling(reserved, flag.Shorthand, "shorthand", flag.ID)
	}
	return reserved
}

func reservedStaticInputs(command Command) (map[int]string, string, map[string]string) {
	positions := make(map[int]string)
	bindingIDs := make(map[string]string)
	stdinOwner := ""
	for _, argument := range command.Arguments {
		// Factory positions are 1-based while static manifest slots are 0-based.
		positions[argument.Position+1] = argument.ID
		addBindingOwners(bindingIDs, argument.ID, argument.HandlerBindingID)
		if stdinOwner == "" && (containsString(argument.AcceptedSources, SourceStdin) || containsString(argument.Channels, SourceStdin)) {
			stdinOwner = argument.ID
		}
	}
	for _, flag := range command.Flags {
		addBindingOwners(bindingIDs, flag.ID, flag.HandlerBindingID)
		if stdinOwner == "" && containsString(flag.AcceptedSources, SourceStdin) {
			stdinOwner = flag.ID
		}
	}
	return positions, stdinOwner, bindingIDs
}

func factoryNamedSpellings(parameter work.InvocationParameterConfig) map[string]string {
	hasNamedBinding := false
	for _, binding := range parameter.Bindings {
		if binding.Kind == work.InvocationParameterBindingKindNamed || binding.Kind == work.InvocationParameterBindingKindNamedRest {
			hasNamedBinding = true
			break
		}
	}
	if !hasNamedBinding {
		return nil
	}
	primary := parameter.ExternalName
	if primary == "" {
		primary = parameter.Name
	}
	spellings := map[string]string{"/externalName": primary}
	if parameter.ExternalName == "" {
		spellings = map[string]string{"/name": primary}
	}
	for index, alias := range parameter.Aliases {
		spellings[fmt.Sprintf("/aliases/%d", index)] = alias
	}
	return spellings
}

func collisionCode(kind string) string {
	switch kind {
	case "command":
		return CompositionCollisionCommandName
	case "alias":
		return CompositionCollisionAlias
	case "shorthand":
		return CompositionCollisionShorthand
	default:
		return CompositionCollisionLongName
	}
}

func newCompositionDiagnostic(code, path, value, staticOwner, factoryOwner string) CompositionDiagnostic {
	return CompositionDiagnostic{
		Code:         code,
		Path:         path,
		Message:      fmt.Sprintf("Factory input %q collides with reserved static owner %q on %q", factoryOwner, staticOwner, value),
		StaticOwner:  staticOwner,
		FactoryOwner: factoryOwner,
	}
}

func addReservedSpelling(reserved map[string][]reservedSpelling, value, kind, owner string) {
	if value == "" {
		return
	}
	reserved[value] = append(reserved[value], reservedSpelling{kind: kind, owner: owner})
}

func addBindingOwners(owners map[string]string, inputID, handlerBindingID string) {
	if inputID != "" {
		owners[inputID] = inputID
	}
	if handlerBindingID != "" {
		owners[handlerBindingID] = inputID
	}
}

func sortedCommandIDs(commands map[string]Command) []string {
	ids := make([]string, 0, len(commands))
	for id := range commands {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedFlagIDs(flags map[string]Flag) []string {
	ids := make([]string, 0, len(flags))
	for id := range flags {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
