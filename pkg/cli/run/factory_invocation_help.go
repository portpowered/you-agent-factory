package run

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	factoryrun "github.com/portpowered/infinite-you/pkg/config/factoryrun"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		parts = append(parts, "--"+trimmed)
	}
	if hasStdinBinding(parameter) {
		parts = append(parts, "stdin")
	}
	if len(parts) == 0 {
		parts = append(parts, parameter.Name)
	}
	return strings.Join(parts, ", ")
}

func parameterDetails(parameter interfaces.InvocationParameterConfig) []string {
	var details []string
	if parameter.Required {
		details = append(details, "Required.")
	} else {
		details = append(details, "Optional.")
	}
	if typeHint := strings.TrimSpace(parameter.TypeHint); typeHint != "" {
		details = append(details, "Type hint: "+typeHint+".")
	}
	if valueMode := strings.TrimSpace(parameter.ValueMode); valueMode != "" {
		details = append(details, "Value mode: "+valueMode+".")
	}
	if len(parameter.Choices) > 0 {
		details = append(details, "Accepted values: "+strings.Join(parameter.Choices, ", ")+".")
	}
	if defaultValue := strings.TrimSpace(parameter.DefaultValue); defaultValue != "" {
		details = append(details, "Default: "+defaultValue+".")
	}
	if len(parameter.DefaultValues) > 0 {
		details = append(details, "Default values: "+strings.Join(parameter.DefaultValues, ", ")+".")
	}
	if parameter.Sensitive {
		details = append(details, "Sensitive values are redacted in downstream diagnostics.")
	}
	if hasBooleanConvenience(parameter) {
		details = append(details, "Named form also accepts bare `--"+namedParameterKey(parameter)+"` as `true`.")
	}
	if hasNamedRestBinding(parameter) {
		details = append(details, "Collects unknown named arguments when the signature policy allows it.")
	}
	return details
}

func hasNamedBinding(parameter interfaces.InvocationParameterConfig) bool {
	for _, binding := range parameter.Bindings {
		if strings.TrimSpace(binding.Kind) == "NAMED" {
			return true
		}
	}
	return false
}

func hasNamedRestBinding(parameter interfaces.InvocationParameterConfig) bool {
	for _, binding := range parameter.Bindings {
		if strings.TrimSpace(binding.Kind) == "NAMED_REST" {
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

func hasBooleanConvenience(parameter interfaces.InvocationParameterConfig) bool {
	return hasNamedBinding(parameter) && strings.TrimSpace(parameter.TypeHint) == "BOOLEAN_STRING"
}

func formatOutputContract(output *interfaces.InvocationOutputContractConfig) string {
	var builder strings.Builder
	if mode := strings.TrimSpace(output.Mode); mode != "" {
		builder.WriteString("  Mode: ")
		builder.WriteString(mode)
		builder.WriteString("\n")
	}
	if pathParameter := strings.TrimSpace(output.PathParameter); pathParameter != "" {
		builder.WriteString("  Path parameter: ")
		builder.WriteString(pathParameter)
		builder.WriteString("\n")
	}
	if contentType := strings.TrimSpace(output.ContentType); contentType != "" {
		builder.WriteString("  Content type: ")
		builder.WriteString(contentType)
		builder.WriteString("\n")
	}
	if fileExtension := strings.TrimSpace(output.FileExtension); fileExtension != "" {
		builder.WriteString("  File extension: ")
		builder.WriteString(fileExtension)
		builder.WriteString("\n")
	}
	if description := strings.TrimSpace(output.Description); description != "" {
		builder.WriteString("  Description: ")
		builder.WriteString(description)
		builder.WriteString("\n")
	}
	return builder.String()
}

func formatInvocationExample(commandPrefix string, example interfaces.InvocationExampleConfig) string {
	var builder strings.Builder
	builder.WriteString("  ")
	if name := strings.TrimSpace(example.Name); name != "" {
		builder.WriteString(name)
		builder.WriteString(":\n")
	} else {
		builder.WriteString("example:\n")
	}
	if description := strings.TrimSpace(example.Description); description != "" {
		builder.WriteString("    ")
		builder.WriteString(description)
		builder.WriteString("\n")
	}
	if stdin := strings.TrimSpace(example.Stdin); stdin != "" {
		builder.WriteString("    printf '%s\\n' ")
		builder.WriteString(shellQuote(stdin))
		builder.WriteString(" | ")
		builder.WriteString(commandPrefix)
		if len(example.Argv) > 0 {
			builder.WriteString(" ")
			builder.WriteString(strings.Join(quoteArgs(example.Argv), " "))
		}
		builder.WriteString("\n")
		return builder.String()
	}
	builder.WriteString("    ")
	builder.WriteString(commandPrefix)
	if len(example.Argv) > 0 {
		builder.WriteString(" ")
		builder.WriteString(strings.Join(quoteArgs(example.Argv), " "))
	}
	builder.WriteString("\n")
	return builder.String()
}

func quoteArgs(args []string) []string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return quoted
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\|&;<>()[{}*$?!`") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
