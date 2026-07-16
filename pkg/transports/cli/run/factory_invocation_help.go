package run

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryrun "github.com/portpowered/infinite-you/pkg/config/factoryrun"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
		if javascriptWorkflowPath(cfg.FactoryConfigPath) {
			return nil, nil
		}
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

func javascriptWorkflowPath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
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
