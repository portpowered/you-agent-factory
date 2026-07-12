package invocations

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ContentFromNormalizedArguments materializes signature inputs that represent
// work content. Positional and stdin parameters are text input; FILE_PATH and
// FILE_CONTENTS parameters are materialized regardless of binding kind.
func ContentFromNormalizedArguments(signature *interfaces.InvocationSignatureConfig, normalized *NormalizedArguments) ([]interfaces.WorkContentPart, error) {
	if signature == nil || normalized == nil {
		return nil, nil
	}
	content := make([]interfaces.WorkContentPart, 0, len(signature.Parameters))
	for _, parameter := range signature.Parameters {
		argument, ok := normalized.Arguments[strings.TrimSpace(parameter.Name)]
		if !ok || len(argument.Values) == 0 || !parameterProducesContent(parameter) {
			continue
		}
		for _, value := range argument.Values {
			part, err := invocationParameterContent(parameter, value)
			if err != nil {
				return nil, err
			}
			content = append(content, part)
		}
	}
	return content, nil
}

func parameterProducesContent(parameter interfaces.InvocationParameterConfig) bool {
	switch strings.TrimSpace(parameter.ValueMode) {
	case string(factoryapi.FactoryInvocationParameterValueModeFileContents):
		return true
	}
	if strings.TrimSpace(parameter.TypeHint) == string(factoryapi.FactoryInvocationParameterTypeHintFilePath) {
		return true
	}
	for _, binding := range parameter.Bindings {
		switch strings.TrimSpace(binding.Kind) {
		case string(factoryapi.FactoryInvocationParameterBindingKindPositional), string(factoryapi.FactoryInvocationParameterBindingKindStdin):
			return true
		}
	}
	return false
}

func invocationParameterContent(parameter interfaces.InvocationParameterConfig, value string) (interfaces.WorkContentPart, error) {
	name := strings.TrimSpace(parameter.Name)
	if strings.TrimSpace(parameter.ValueMode) == string(factoryapi.FactoryInvocationParameterValueModeFileContents) {
		data, err := os.ReadFile(value)
		if err != nil {
			return interfaces.WorkContentPart{}, fmt.Errorf("read FILE_CONTENTS invocation parameter %q from %q: %w", name, value, err)
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return interfaces.WorkContentPart{}, fmt.Errorf("FILE_CONTENTS invocation parameter %q from %q is empty", name, value)
		}
		return interfaces.WorkContentPart{Type: interfaces.WorkContentPartTypeText, Text: text, Label: name}, nil
	}
	if strings.TrimSpace(parameter.TypeHint) == string(factoryapi.FactoryInvocationParameterTypeHintFilePath) {
		path, err := filepath.Abs(value)
		if err != nil {
			return interfaces.WorkContentPart{}, fmt.Errorf("resolve FILE_PATH invocation parameter %q: %w", name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return interfaces.WorkContentPart{}, fmt.Errorf("read FILE_PATH invocation parameter %q from %q: %w", name, value, err)
		}
		if !info.Mode().IsRegular() {
			return interfaces.WorkContentPart{}, fmt.Errorf("FILE_PATH invocation parameter %q from %q is not a regular file", name, value)
		}
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
		partType := interfaces.WorkContentPartTypeBinary
		if strings.HasPrefix(contentType, "audio/") {
			partType = interfaces.WorkContentPartTypeAudio
		}
		if strings.HasPrefix(contentType, "image/") {
			partType = interfaces.WorkContentPartTypeImage
		}
		return interfaces.WorkContentPart{Type: partType, File: path, ContentType: contentType, Label: name}, nil
	}
	return interfaces.WorkContentPart{Type: interfaces.WorkContentPartTypeText, Text: value, Label: name}, nil
}
