package packagedfactorycatalog

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const factorySchemaIdentity = "https://schemas.portpowered.com/you/config/factory.schema.json"

// DefinitionCatalog is an atomic, validated projection of packaged Factory
// definitions. Its methods return detached data in stable lexical order.
type DefinitionCatalog struct {
	definitions []factorydefinitions.PackagedDefinition
}

// LoadPublishedDefinitionCatalog validates the exact generated publication
// embedded in the packaged-factories module.
func LoadPublishedDefinitionCatalog() (DefinitionCatalog, error) {
	return LoadDefinitionCatalog(packagedfactories.Published())
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
		definitions = append(definitions, factorydefinitions.PackagedDefinition{
			Name:    entry.PublicName,
			Project: entry.Project,
			JSON:    append([]byte(nil), jsonPayload...),
		})
	}
	if len(definitions) == 0 {
		return DefinitionCatalog{}, errors.New("packaged definition catalog: manifest contains no Factories")
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})
	return DefinitionCatalog{definitions: definitions}, nil
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
	return definition
}
