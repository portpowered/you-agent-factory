package authoredlayout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"gopkg.in/yaml.v3"
)

func preparedRootFileName(rootFileName string) (string, error) {
	switch strings.TrimSpace(rootFileName) {
	case "":
		return factorydefinitions.FactoryConfigFile, nil
	case factorydefinitions.FactoryConfigFile, "factory.yaml", "factory.yml":
		return rootFileName, nil
	default:
		return "", fmt.Errorf(
			"unsupported Factory Definition root file %q; expected %s",
			rootFileName,
			factorydefinitions.SupportedAuthoredFactoryRootFiles,
		)
	}
}

func formatCanonicalFactory(
	data []byte,
	sourcePath string,
	rootFileName string,
) ([]byte, error) {
	if rootFileName == factorydefinitions.FactoryConfigFile {
		return formatCanonicalFactoryJSON(data, sourcePath)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf(
			"format canonical factory config %s as YAML: %w",
			sourcePath,
			err,
		)
	}
	clearYAMLPresentationStyles(&document)
	var formatted bytes.Buffer
	encoder := yaml.NewEncoder(&formatted)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, fmt.Errorf(
			"format canonical factory config %s as YAML: %w",
			sourcePath,
			err,
		)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf(
			"format canonical factory config %s as YAML: %w",
			sourcePath,
			err,
		)
	}
	return formatted.Bytes(), nil
}

func formatCanonicalFactoryJSON(data []byte, sourcePath string) ([]byte, error) {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return nil, fmt.Errorf(
			"format canonical factory config %s: %w",
			sourcePath,
			err,
		)
	}
	formatted.WriteByte('\n')
	return formatted.Bytes(), nil
}

func clearYAMLPresentationStyles(node *yaml.Node) {
	if node == nil {
		return
	}
	node.Style = 0
	for _, child := range node.Content {
		clearYAMLPresentationStyles(child)
	}
}
