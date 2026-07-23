package climanifest

import (
	"fmt"
	"strings"
)

// ValidateRootContract validates the manifest-owned root discovery contract and
// every root/global input before generation or Cobra construction.
func ValidateRootContract(manifest Manifest) error {
	root, found := rootCommand(manifest)
	if !found {
		if manifest.RootPath == "you" {
			return fmt.Errorf("rootPath %q has no matching root command record", manifest.RootPath)
		}
		return nil
	}
	if root.ID != "you" {
		return nil
	}
	if err := validateRootLifecycle(root); err != nil {
		return err
	}
	return validateRootFlags(root)
}

func rootCommand(manifest Manifest) (Command, bool) {
	for _, command := range manifest.Commands {
		if command.Path == manifest.RootPath {
			return command, true
		}
	}
	return Command{}, false
}

func validateRootLifecycle(root Command) error {
	if root.RootLifecycle == nil {
		return fmt.Errorf("root command %q is missing rootLifecycle", root.ID)
	}
	lifecycle := root.RootLifecycle
	if lifecycle.NoArguments != "help" || lifecycle.HelpOutput != "stdout" ||
		lifecycle.ExitCode != 0 || lifecycle.SideEffects != "none" {
		return fmt.Errorf("root command %q has unsupported no-argument lifecycle", root.ID)
	}
	if lifecycle.Ownership != (RootOwnership{
		Help: root.ID, Init: root.ID + ".init", Run: root.ID + ".run",
		Server: root.ID + ".flag.server",
	}) {
		return fmt.Errorf("root command %q has invalid lifecycle ownership boundaries", root.ID)
	}
	return nil
}

func validateRootFlags(root Command) error {
	longNames := make(map[string]string)
	shorthands := make(map[string]string)
	for _, key := range sortedFlagIDs(root.Flags) {
		flag := root.Flags[key]
		if err := validateRootFlag(root, key, flag, longNames, shorthands); err != nil {
			return err
		}
	}
	if server, exists := root.Flags[root.RootLifecycle.Ownership.Server]; !exists || server.Long != "server" {
		return fmt.Errorf("root command %q server ownership does not reference the active server input", root.ID)
	}
	return nil
}

func validateRootFlag(root Command, key string, flag Flag, longNames, shorthands map[string]string) error {
	if err := validateRootFlagShape(root, key, flag); err != nil {
		return err
	}
	if err := validateRootFlagSpellings(root, flag, longNames, shorthands); err != nil {
		return err
	}
	binding, bound := root.HandlerBindings[flag.HandlerBindingID]
	if !bound || binding.ID != flag.HandlerBindingID || binding.InputID != flag.ID {
		return fmt.Errorf("root command %q input %q has missing stable handler binding %q", root.ID, flag.ID, flag.HandlerBindingID)
	}
	return nil
}

func validateRootFlagShape(root Command, key string, flag Flag) error {
	if key != flag.ID || strings.TrimSpace(flag.ID) == "" {
		return fmt.Errorf("root command %q flag map key %q does not match stable input ID %q", root.ID, key, flag.ID)
	}
	if flag.Scope != "persistent" {
		return fmt.Errorf("root command %q input %q must use persistent scope", root.ID, flag.ID)
	}
	if flag.Kind != "named" || flag.MinCardinality != 0 || flag.MaxCardinality != 1 ||
		flag.DefaultValue == nil || len(flag.AcceptedSources) == 0 ||
		strings.TrimSpace(flag.HandlerBindingID) == "" {
		return fmt.Errorf("root command %q input %q must use the complete canonical input contract", root.ID, flag.ID)
	}
	if err := validateRootInputValues(flag); err != nil {
		return fmt.Errorf("root command %q input %q: %w", root.ID, flag.ID, err)
	}
	if strings.TrimSpace(flag.Usage) == "" {
		return fmt.Errorf("root command %q input %q is missing manifest-owned usage", root.ID, flag.ID)
	}
	if flag.Sensitivity != "public" && flag.Sensitivity != "sensitive" {
		return fmt.Errorf("root command %q input %q has unsupported sensitivity %q", root.ID, flag.ID, flag.Sensitivity)
	}
	if flag.Lifecycle.State != "active" {
		return fmt.Errorf("root command %q input %q is not an active global input", root.ID, flag.ID)
	}
	return nil
}

func validateRootFlagSpellings(root Command, flag Flag, longNames, shorthands map[string]string) error {
	if err := recordRootSpelling(longNames, flag.Long, flag.ID); err != nil {
		return err
	}
	for _, alias := range flag.Aliases {
		if err := recordRootSpelling(longNames, alias, flag.ID); err != nil {
			return err
		}
	}
	if flag.Shorthand == "h" || len([]rune(flag.Shorthand)) > 1 {
		return fmt.Errorf("root command %q input %q has unsupported shorthand %q", root.ID, flag.ID, flag.Shorthand)
	}
	if flag.Shorthand == "" {
		return nil
	}
	if owner, duplicate := shorthands[flag.Shorthand]; duplicate {
		return fmt.Errorf("root inputs %q and %q duplicate shorthand %q", owner, flag.ID, flag.Shorthand)
	}
	shorthands[flag.Shorthand] = flag.ID
	return nil
}

func recordRootSpelling(owners map[string]string, spelling, inputID string) error {
	if strings.TrimSpace(spelling) == "" || strings.ContainsAny(spelling, " \t\r\n") ||
		strings.HasPrefix(spelling, "-") || spelling == "help" {
		return fmt.Errorf("root input %q has unsupported public spelling %q", inputID, spelling)
	}
	if owner, duplicate := owners[spelling]; duplicate {
		return fmt.Errorf("root inputs %q and %q duplicate public spelling %q", owner, inputID, spelling)
	}
	owners[spelling] = inputID
	return nil
}

func validateRootInputValues(flag Flag) error {
	if rootInputValueKind(flag.DefaultValue) != flag.ValueType {
		return fmt.Errorf("typed default does not match value type %q", flag.ValueType)
	}
	if flag.NoOptionValue == nil {
		return nil
	}
	if flag.ValueType != "bool" && flag.ValueType != "string" {
		return fmt.Errorf("no-option default is unsupported for value type %q", flag.ValueType)
	}
	if rootInputValueKind(flag.NoOptionValue) != flag.ValueType {
		return fmt.Errorf("typed no-option default does not match value type %q", flag.ValueType)
	}
	return nil
}

func rootInputValueKind(value *InputValue) string {
	if value == nil {
		return ""
	}
	kind := ""
	count := 0
	for candidate, present := range map[string]bool{
		"bool": value.Boolean != nil, "string": value.String != nil,
		"int": value.Int != nil, "int64": value.Int64 != nil,
		"stringArray": value.StringArray != nil,
	} {
		if present {
			kind = candidate
			count++
		}
	}
	if count != 1 {
		return ""
	}
	return kind
}
