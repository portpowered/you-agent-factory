package packagedfactorycatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// ArtifactPair is one self-contained Factory serialized in equivalent public
// JSON and YAML representations.
type ArtifactPair struct {
	Slug       string
	PublicName string
	SourcePath string
	Factory    *factorydefinitions.FactoryConfig
	JSON       []byte
	YAML       []byte
}

// GenerateArtifacts discovers the complete authored inventory and projects
// every Factory through the canonical portability and public schema boundaries.
func GenerateArtifacts(
	ctx context.Context,
	source fs.FS,
	root string,
	schemaPath string,
) ([]ArtifactPair, error) {
	inventory, err := Discover(ctx, source, root)
	if err != nil {
		return nil, err
	}
	schema, err := compileFactorySchema(source, schemaPath)
	if err != nil {
		return nil, err
	}

	artifacts := make([]ArtifactPair, 0, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		artifact, err := generateArtifactPair(entry, schema)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func generateArtifactPair(entry Entry, schema *jsonschema.Schema) (ArtifactPair, error) {
	portableFactory := *entry.Factory
	portableFactory.Name = entry.Slug
	jsonPayload, err := factorymapping.MarshalCanonicalFactoryConfig(&portableFactory)
	if err != nil {
		return ArtifactPair{}, fmt.Errorf("%s: serialize portable JSON artifact: %w", entry.SourcePath, err)
	}
	jsonPayload, err = indentedJSON(jsonPayload)
	if err != nil {
		return ArtifactPair{}, fmt.Errorf("%s: format portable JSON artifact: %w", entry.SourcePath, err)
	}
	if err := validateArtifactSchema(schema, jsonPayload); err != nil {
		return ArtifactPair{}, fmt.Errorf("%s: validate JSON artifact against package Factory schema: %w", entry.SourcePath, err)
	}

	var document any
	if err := json.Unmarshal(jsonPayload, &document); err != nil {
		return ArtifactPair{}, fmt.Errorf("%s: decode portable JSON artifact for YAML serialization: %w", entry.SourcePath, err)
	}
	yamlPayload, err := yaml.Marshal(document)
	if err != nil {
		return ArtifactPair{}, fmt.Errorf("%s: serialize portable YAML artifact: %w", entry.SourcePath, err)
	}
	yamlJSON, err := decodedYAMLJSON(yamlPayload)
	if err != nil {
		return ArtifactPair{}, fmt.Errorf("%s: decode portable YAML artifact: %w", entry.SourcePath, err)
	}
	if err := validateArtifactSchema(schema, yamlJSON); err != nil {
		return ArtifactPair{}, fmt.Errorf("%s: validate YAML artifact against package Factory schema: %w", entry.SourcePath, err)
	}
	if err := validateArtifactEquivalence(entry.SourcePath, jsonPayload, yamlJSON); err != nil {
		return ArtifactPair{}, err
	}

	return ArtifactPair{
		Slug:       entry.Slug,
		PublicName: entry.Factory.Name,
		SourcePath: entry.SourcePath,
		Factory:    &portableFactory,
		JSON:       jsonPayload,
		YAML:       yamlPayload,
	}, nil
}

func compileFactorySchema(source fs.FS, schemaPath string) (*jsonschema.Schema, error) {
	payload, err := fs.ReadFile(source, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("schema validation: read package Factory schema %s: %w", schemaPath, err)
	}
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("schema validation: decode package Factory schema %s: %w", schemaPath, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(schemaPath, document); err != nil {
		return nil, fmt.Errorf("schema validation: register package Factory schema %s: %w", schemaPath, err)
	}
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("schema validation: compile package Factory schema %s: %w", schemaPath, err)
	}
	return schema, nil
}

func validateArtifactSchema(schema *jsonschema.Schema, payload []byte) error {
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		return err
	}
	normalizeSchemaValidationCompatibility(document)
	return schema.Validate(document)
}

// The canonical Factory input mapper intentionally accepts exact invocation
// placeholders on enum-backed worker modelProvider fields. The accepted
// OpenAPI schema remains concrete-provider-only, so schema validation uses a
// concrete representative while canonical mapping and domain validation above
// continue to validate and preserve the authored placeholder itself.
func normalizeSchemaValidationCompatibility(document any) {
	root, ok := document.(map[string]any)
	if !ok {
		return
	}
	workers, _ := root["workers"].([]any)
	for _, value := range workers {
		worker, ok := value.(map[string]any)
		if !ok {
			continue
		}
		provider, _ := worker["modelProvider"].(string)
		if strings.HasPrefix(provider, "${") && strings.HasSuffix(provider, "}") {
			worker["modelProvider"] = "CODEX"
		}
	}
}

func validateArtifactEquivalence(sourcePath string, jsonPayload, yamlJSON []byte) error {
	mapper := factorymapping.NewFactoryConfigMapper()
	jsonFactory, err := mapper.Expand(jsonPayload)
	if err != nil {
		return fmt.Errorf("%s: decode portable JSON artifact through canonical Factory boundary: %w", sourcePath, err)
	}
	yamlFactory, err := mapper.Expand(yamlJSON)
	if err != nil {
		return fmt.Errorf("%s: decode portable YAML artifact through canonical Factory boundary: %w", sourcePath, err)
	}
	for format, factory := range map[string]*factorydefinitions.FactoryConfig{"JSON": jsonFactory, "YAML": yamlFactory} {
		if validation := factoryvalidation.Validate(factory); validation.HasBlockingTargets() {
			var findings []string
			for _, target := range validation.BlockingTargets() {
				findings = append(findings, fmt.Sprintf("%s %s: %s", target.Code, target.Path, target.Message))
			}
			return fmt.Errorf("%s: validate decoded %s artifact: %s", sourcePath, format, findings)
		}
	}
	if !reflect.DeepEqual(jsonFactory, yamlFactory) {
		return fmt.Errorf("%s: JSON and YAML artifacts decode to different canonical Factory values", sourcePath)
	}
	return nil
}

func indentedJSON(payload []byte) ([]byte, error) {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, payload, "", "  "); err != nil {
		return nil, err
	}
	formatted.WriteByte('\n')
	return formatted.Bytes(), nil
}

func decodedYAMLJSON(payload []byte) ([]byte, error) {
	var document any
	if err := yaml.Unmarshal(payload, &document); err != nil {
		return nil, err
	}
	return yamlToJSON(document)
}
