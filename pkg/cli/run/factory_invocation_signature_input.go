package run

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
)

type SignatureFactoryInvocationInputConfig struct {
	PromptArgs []string
	Signature  *interfaces.InvocationSignatureConfig
	Stdin      io.Reader
	StdinIsTTY func() bool
}

func ResolveSignatureFactoryInvocationInput(cfg SignatureFactoryInvocationInputConfig) (invocations.NormalizedArguments, error) {
	positionalArgs, namedArgs, explicitStdin, err := splitSignatureInvocationArgs(cfg.PromptArgs, cfg.Signature)
	if err != nil {
		return invocations.NormalizedArguments{}, err
	}
	stdinPayload, hasStdin, err := resolveInvocationStdin(FactoryInvocationInputConfig{
		Stdin:      cfg.Stdin,
		StdinIsTTY: cfg.StdinIsTTY,
	}, explicitStdin)
	if err != nil {
		return invocations.NormalizedArguments{}, err
	}

	input := invocations.NormalizeArgumentsInput{
		Signature:      cfg.Signature,
		PositionalArgs: positionalArgs,
		NamedArgs:      namedArgs,
	}
	if hasStdin {
		input.StdinText = &stdinPayload
	}

	normalized, err := invocations.NormalizeArguments(input)
	if err != nil {
		return invocations.NormalizedArguments{}, signatureInvocationInputError(err)
	}
	return normalized, nil
}

func splitSignatureInvocationArgs(args []string, signature *interfaces.InvocationSignatureConfig) ([]string, []invocations.NamedArgumentInput, bool, error) {
	positional := make([]string, 0, len(args))
	named := make([]invocations.NamedArgumentInput, 0)
	explicitStdin := false
	booleanNamedKeys := signatureBooleanNamedKeys(signature)

	for index := 0; index < len(args); index++ {
		token := args[index]
		if strings.TrimSpace(token) == "-" {
			explicitStdin = true
			continue
		}
		if !strings.HasPrefix(token, "--") || token == "--" {
			positional = append(positional, token)
			continue
		}

		raw := strings.TrimPrefix(token, "--")
		if name, value, ok := strings.Cut(raw, "="); ok {
			named = append(named, invocations.NamedArgumentInput{Key: strings.TrimSpace(name), Values: []string{value}})
			continue
		}

		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, nil, false, fmt.Errorf("factory argument name is required after --")
		}

		if booleanNamedKeys[name] {
			if index+1 < len(args) && isExplicitBooleanStringValue(args[index+1]) {
				index++
				named = append(named, invocations.NamedArgumentInput{Key: name, Values: []string{args[index]}})
				continue
			}
			named = append(named, invocations.NamedArgumentInput{Key: name, Values: []string{"true"}})
			continue
		}

		if index+1 >= len(args) {
			return nil, nil, false, fmt.Errorf("factory argument --%s requires a value", name)
		}
		index++
		named = append(named, invocations.NamedArgumentInput{Key: name, Values: []string{args[index]}})
	}

	return positional, named, explicitStdin, nil
}

func signatureBooleanNamedKeys(signature *interfaces.InvocationSignatureConfig) map[string]bool {
	keys := map[string]bool{}
	if signature == nil {
		return keys
	}
	for _, parameter := range signature.Parameters {
		if strings.TrimSpace(parameter.TypeHint) != string(factoryapi.FactoryInvocationParameterTypeHintBooleanString) {
			continue
		}
		hasNamedBinding := false
		for _, binding := range parameter.Bindings {
			if strings.TrimSpace(binding.Kind) == string(factoryapi.FactoryInvocationParameterBindingKindNamed) {
				hasNamedBinding = true
				break
			}
		}
		if !hasNamedBinding {
			continue
		}
		if name := strings.TrimSpace(parameter.Name); name != "" {
			keys[name] = true
		}
		if external := strings.TrimSpace(parameter.ExternalName); external != "" {
			keys[external] = true
		}
		for _, alias := range parameter.Aliases {
			if trimmed := strings.TrimSpace(alias); trimmed != "" {
				keys[trimmed] = true
			}
		}
	}
	return keys
}

func isExplicitBooleanStringValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "1", "0", "yes", "no", "on", "off":
		return true
	default:
		return false
	}
}

func signatureInvocationInputError(err error) error {
	argumentErr, ok := err.(*invocations.ArgumentError)
	if !ok {
		return err
	}
	return &InvocationError{
		Code:    string(argumentErr.Code),
		Message: argumentErr.Message,
	}
}

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
