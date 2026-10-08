package packagedfactorycatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"sync"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const factorySchemaIdentity = "https://schemas.portpowered.com/you/config/factory.schema.json"

// DefinitionCatalog is an atomic, validated projection of packaged Factory
// definitions. Its methods return detached data in stable lexical order.
type DefinitionCatalog struct {
	definitions []factorydefinitions.PackagedDefinition
	conversions map[[sha256.Size]byte][]byte
	canonical   map[[sha256.Size]byte][]byte
}

// The embedded publication cannot change during this process. Validate it
// once; catalog accessors keep every caller's definitions detached.
var publishedDefinitionCatalog = sync.OnceValues(func() (DefinitionCatalog, error) {
	return LoadDefinitionCatalog(packagedfactories.Published())
})

// LoadPublishedDefinitionCatalog validates the exact generated publication
// embedded in the packaged-factories module.
func LoadPublishedDefinitionCatalog() (DefinitionCatalog, error) {
	return publishedDefinitionCatalog()
}

// LoadDefinitionCatalog validates a generated package publication before
// returning any packaged definition. The injectable filesystem keeps package
// reads inert and makes every failure mode testable without process state.
func LoadDefinitionCatalog(source fs.FS) (DefinitionCatalog, error) {
	if source == nil {
		return DefinitionCatalog{}, errors.New("packaged definition catalog: source filesystem is required")
	}

	manifest, err := readPublishedManifest(source)
	if err != nil {
		return DefinitionCatalog{}, err
	}
	schema, err := compileFactorySchema(source, "schemas/factory.schema.json")
	if err != nil {
		return DefinitionCatalog{}, fmt.Errorf("packaged definition catalog: %w", err)
	}
	schemaID, err := readSchemaIdentity(source, "schemas/factory.schema.json")
	if err != nil {
		return DefinitionCatalog{}, fmt.Errorf("packaged definition catalog: %w", err)
	}
	if schemaID != factorySchemaIdentity {
		return DefinitionCatalog{}, fmt.Errorf(
			"packaged definition catalog: Factory schema $id %q is unsupported; expected %q",
			schemaID,
			factorySchemaIdentity,
		)
	}

	definitions := make([]factorydefinitions.PackagedDefinition, 0, len(manifest.Factories))
	conversions := make(map[[sha256.Size]byte][]byte, len(manifest.Factories))
	canonical := make(map[[sha256.Size]byte][]byte, len(manifest.Factories))
	identityOwners := newDefinitionIdentityOwners()
	locatorOwners := make(map[string]string, len(manifest.Factories)*2)
	for index, entry := range manifest.Factories {
		context := fmt.Sprintf("manifest factories[%d] %q", index, entry.PublicName)
		if err := validateManifestEntryIdentity(entry, context, identityOwners); err != nil {
			return DefinitionCatalog{}, err
		}
		if err := validateManifestEntryLocators(entry, context, locatorOwners); err != nil {
			return DefinitionCatalog{}, err
		}

		jsonPayload, err := readVerifiedArtifact(source, entry.JSON, context+" JSON")
		if err != nil {
			return DefinitionCatalog{}, err
		}
		yamlPayload, err := readVerifiedArtifact(source, entry.YAML, context+" YAML")
		if err != nil {
			return DefinitionCatalog{}, err
		}
		if err := validatePublishedArtifactPair(schema, entry, jsonPayload, yamlPayload, context); err != nil {
			return DefinitionCatalog{}, err
		}
		if conversion, err := fs.ReadFile(source, factorydefinitions.SerializedFactoryConfigInput(jsonPayload).Path()); err == nil {
			conversions[sha256.Sum256(jsonPayload)] = indexCanonicalConversion(canonical, jsonPayload, conversion)
		}
		definitions = append(definitions, factorydefinitions.PackagedDefinition{
			Name:    entry.PublicName,
			Project: entry.Project,
			JSON:    append([]byte(nil), jsonPayload...),
			YAML:    append([]byte(nil), yamlPayload...),
			Formats: []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
				factorydefinitions.PackagedFactoryFormatYAML,
			},
		})
	}
	if len(definitions) == 0 {
		return DefinitionCatalog{}, errors.New("packaged definition catalog: manifest contains no Factories")
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})
	return DefinitionCatalog{definitions: definitions, conversions: conversions, canonical: canonical}, nil
}

func indexCanonicalConversion(output map[[sha256.Size]byte][]byte, payload, data []byte) []byte {
	var conversion factorydefinitions.SerializedFactoryConfig
	if json.Unmarshal(data, &conversion) != nil {
		return data
	}
	hash := sha256.Sum256(payload)
	if conversion.Version != factorydefinitions.SerializedFactoryConfigVersion || conversion.PayloadSHA256 != hex.EncodeToString(hash[:]) || len(conversion.Canonical) == 0 || conversion.Canonical[0] != '{' || !json.Valid(conversion.Canonical) {
		return data
	}
	key, err := hex.DecodeString(conversion.CanonicalInputSHA256)
	if err != nil || len(key) != sha256.Size {
		return data
	}
	output[[sha256.Size]byte(key)] = conversion.Canonical
	// Decoders should scan only their native conversion, not the separately
	// indexed canonical output. Split immutable assets once at catalog load.
	conversion.Canonical = nil
	conversion.CanonicalInputSHA256 = ""
	native, err := json.Marshal(conversion)
	if err != nil {
		return data
	}
	return native
}

// ReadCanonicalFactoryConfig returns detached output for an exact normalized
// native input. Missing optional assets retain the canonical encoder.
func (catalog DefinitionCatalog) ReadCanonicalFactoryConfig(payload []byte) ([]byte, error) {
	data, ok := catalog.canonical[sha256.Sum256(payload)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

// ReadSerializedFactoryConfig returns a detached optional conversion of a
// schema-validated published input. Callers retain canonical mapping on a miss.
func (catalog DefinitionCatalog) ReadSerializedFactoryConfig(payload []byte) ([]byte, error) {
	data, ok := catalog.conversions[sha256.Sum256(payload)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

// Names returns packaged Factory names in stable lexical order.
func (catalog DefinitionCatalog) Names() []string {
	names := make([]string, len(catalog.definitions))
	for index, definition := range catalog.definitions {
		names[index] = definition.Name
	}
	return names
}

// All returns detached packaged definitions in stable lexical order.
func (catalog DefinitionCatalog) All() []factorydefinitions.PackagedDefinition {
	definitions := make([]factorydefinitions.PackagedDefinition, len(catalog.definitions))
	for index, definition := range catalog.definitions {
		definitions[index] = clonePackagedDefinition(definition)
	}
	return definitions
}

// Lookup returns a detached packaged definition by public name.
func (catalog DefinitionCatalog) Lookup(name string) (factorydefinitions.PackagedDefinition, bool) {
	index := sort.Search(len(catalog.definitions), func(index int) bool {
		return catalog.definitions[index].Name >= name
	})
	if index == len(catalog.definitions) || catalog.definitions[index].Name != name {
		return factorydefinitions.PackagedDefinition{}, false
	}
	return clonePackagedDefinition(catalog.definitions[index]), true
}

func clonePackagedDefinition(definition factorydefinitions.PackagedDefinition) factorydefinitions.PackagedDefinition {
	definition.JSON = append([]byte(nil), definition.JSON...)
	definition.YAML = append([]byte(nil), definition.YAML...)
	definition.Formats = append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...)
	return definition
}
