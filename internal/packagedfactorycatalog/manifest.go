package packagedfactorycatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	// ManifestFormatVersion is the supported packaged Factory catalog contract.
	ManifestFormatVersion = "1"
	generatedCatalogRoot  = "generated/factories"
)

// Manifest is the deterministic package-public index of generated Factories.
// Factories are ordered lexically by PublicName, then Slug.
type Manifest struct {
	FormatVersion string          `json:"formatVersion"`
	FactorySchema string          `json:"factorySchema"`
	Factories     []ManifestEntry `json:"factories"`
}

// ManifestEntry projects one generated Factory and its exact-byte integrity.
type ManifestEntry struct {
	PublicName  string                              `json:"name"`
	Project     string                              `json:"project"`
	Slug        string                              `json:"slug"`
	JSON        ManifestArtifact                    `json:"json"`
	YAML        ManifestArtifact                    `json:"yaml"`
	Description *factorydefinitions.NameValueConfig `json:"description,omitempty"`
	Examples    []ManifestExample                   `json:"examples,omitempty"`
}

// ManifestArtifact identifies one package-public generated representation.
type ManifestArtifact struct {
	Locator string `json:"locator"`
	SHA256  string `json:"sha256"`
}

// ManifestExample is an inert, typed projection of one runnable invocation.
type ManifestExample struct {
	Name        string                                        `json:"name"`
	Description factorydefinitions.NameValueConfig            `json:"description"`
	Args        factorydefinitions.InvocationExampleArguments `json:"args"`
}

// ManifestResult carries both the typed projection and its canonical JSON.
type ManifestResult struct {
	Manifest Manifest
	JSON     []byte
}

// GenerateManifest builds the catalog index from the same validated portable
// artifacts and package-local schema used by artifact generation.
func GenerateManifest(
	ctx context.Context,
	source fs.FS,
	root string,
	schemaPath string,
) (ManifestResult, error) {
	artifacts, err := GenerateArtifacts(ctx, source, root, schemaPath)
	if err != nil {
		return ManifestResult{}, err
	}
	schemaIdentity, err := readSchemaIdentity(source, schemaPath)
	if err != nil {
		return ManifestResult{}, err
	}
	return ProjectManifest(artifacts, schemaIdentity)
}

// ProjectManifest validates and projects a complete in-memory artifact set.
func ProjectManifest(artifacts []ArtifactPair, schemaIdentity string) (ManifestResult, error) {
	if strings.TrimSpace(schemaIdentity) == "" {
		return ManifestResult{}, errors.New("manifest projection: Factory schema identity is empty")
	}
	entries := make([]ManifestEntry, 0, len(artifacts))
	locatorOwners := make(map[string][]string, len(artifacts)*2)
	for _, artifact := range artifacts {
		entry, err := projectManifestEntry(artifact)
		if err != nil {
			return ManifestResult{}, err
		}
		entries = append(entries, entry)
		owner := fmt.Sprintf("%s (%s)", entry.PublicName, artifact.SourcePath)
		for _, locator := range []string{entry.JSON.Locator, entry.YAML.Locator} {
			key := strings.ToLower(locator)
			locatorOwners[key] = append(locatorOwners[key], owner+" -> "+locator)
		}
	}
	if err := rejectLocatorCollisions(locatorOwners); err != nil {
		return ManifestResult{}, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PublicName != entries[j].PublicName {
			return entries[i].PublicName < entries[j].PublicName
		}
		return entries[i].Slug < entries[j].Slug
	})
	manifest := Manifest{
		FormatVersion: ManifestFormatVersion,
		FactorySchema: schemaIdentity,
		Factories:     entries,
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return ManifestResult{}, fmt.Errorf("manifest serialization: %w", err)
	}
	payload, err = indentedJSON(payload)
	if err != nil {
		return ManifestResult{}, fmt.Errorf("manifest serialization: format JSON: %w", err)
	}
	return ManifestResult{Manifest: manifest, JSON: payload}, nil
}

func projectManifestEntry(artifact ArtifactPair) (ManifestEntry, error) {
	if artifact.Factory == nil {
		return ManifestEntry{}, fmt.Errorf("manifest projection: %s has no canonical Factory value", artifact.SourcePath)
	}
	if strings.TrimSpace(artifact.PublicName) == "" || strings.TrimSpace(artifact.Factory.Project) == "" {
		return ManifestEntry{}, fmt.Errorf(
			"manifest projection: %s has empty public Factory name or project/id",
			artifact.SourcePath,
		)
	}
	jsonLocator, yamlLocator, err := artifactLocators(artifact.Slug)
	if err != nil {
		return ManifestEntry{}, fmt.Errorf(
			"manifest collision: Factory %q from %s: %w",
			artifact.PublicName,
			artifact.SourcePath,
			err,
		)
	}
	description, examples, err := projectMetadata(artifact)
	if err != nil {
		return ManifestEntry{}, err
	}
	return ManifestEntry{
		PublicName:  artifact.PublicName,
		Project:     artifact.Factory.Project,
		Slug:        artifact.Slug,
		JSON:        integrity(jsonLocator, artifact.JSON),
		YAML:        integrity(yamlLocator, artifact.YAML),
		Description: description,
		Examples:    examples,
	}, nil
}

func artifactLocators(slug string) (string, string, error) {
	if slug != strings.TrimSpace(slug) || !fs.ValidPath(slug) || strings.Contains(slug, "/") {
		return "", "", fmt.Errorf("unsafe directory slug %q cannot form a package-public locator", slug)
	}
	base := path.Join(generatedCatalogRoot, slug, "factory")
	jsonLocator := base + ".json"
	yamlLocator := base + ".yaml"
	if err := validatePublicLocator(jsonLocator); err != nil {
		return "", "", err
	}
	if err := validatePublicLocator(yamlLocator); err != nil {
		return "", "", err
	}
	return jsonLocator, yamlLocator, nil
}

func validatePublicLocator(locator string) error {
	if !fs.ValidPath(locator) || path.Clean(locator) != locator || strings.Contains(locator, "\\") {
		return fmt.Errorf("unsafe package-public locator %q", locator)
	}
	return nil
}

func rejectLocatorCollisions(owners map[string][]string) error {
	var diagnostics []string
	for normalized, conflicts := range owners {
		if len(conflicts) < 2 {
			continue
		}
		sort.Strings(conflicts)
		diagnostics = append(diagnostics, fmt.Sprintf(
			"normalized package-public locator %q conflicts between %s",
			normalized,
			strings.Join(conflicts, ", "),
		))
	}
	if len(diagnostics) == 0 {
		return nil
	}
	sort.Strings(diagnostics)
	return fmt.Errorf("manifest collision:\n- %s", strings.Join(diagnostics, "\n- "))
}

func integrity(locator string, payload []byte) ManifestArtifact {
	sum := sha256.Sum256(payload)
	return ManifestArtifact{Locator: locator, SHA256: hex.EncodeToString(sum[:])}
}

func projectMetadata(
	artifact ArtifactPair,
) (*factorydefinitions.NameValueConfig, []ManifestExample, error) {
	description, err := cloneNameValue(artifact.Factory.Description)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"manifest projection: Factory %q from %s description: %w",
			artifact.PublicName,
			artifact.SourcePath,
			err,
		)
	}
	var examples []ManifestExample
	for index, example := range artifact.Factory.Examples {
		if strings.TrimSpace(example.Name) == "" {
			return nil, nil, fmt.Errorf(
				"manifest projection: Factory %q from %s examples[%d].name is empty",
				artifact.PublicName,
				artifact.SourcePath,
				index,
			)
		}
		exampleDescription, err := cloneNameValue(&example.Description)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"manifest projection: Factory %q from %s examples[%d].description: %w",
				artifact.PublicName,
				artifact.SourcePath,
				index,
				err,
			)
		}
		args, err := cloneInvocationArgs(example.Args)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"manifest projection: Factory %q from %s examples[%d]: %w",
				artifact.PublicName,
				artifact.SourcePath,
				index,
				err,
			)
		}
		examples = append(examples, ManifestExample{
			Name:        example.Name,
			Description: *exampleDescription,
			Args:        args,
		})
	}
	return description, examples, nil
}

func cloneNameValue(
	value *factorydefinitions.NameValueConfig,
) (*factorydefinitions.NameValueConfig, error) {
	if value == nil {
		return nil, nil
	}
	if err := value.Validate(); err != nil {
		return nil, err
	}
	cloned := *value
	cloned.Locales = append([]string(nil), value.Locales...)
	if value.Values != nil {
		cloned.Values = make(map[string]string, len(value.Values))
		for locale, localized := range value.Values {
			cloned.Values[locale] = localized
		}
	}
	return &cloned, nil
}

func cloneInvocationArgs(
	args factorydefinitions.InvocationExampleArguments,
) (factorydefinitions.InvocationExampleArguments, error) {
	cloned := make(factorydefinitions.InvocationExampleArguments, len(args))
	for name, value := range args {
		switch typed := value.(type) {
		case string:
			cloned[name] = typed
		case []string:
			cloned[name] = append([]string(nil), typed...)
		default:
			return nil, fmt.Errorf("args.%s must be a string or array of strings", name)
		}
	}
	return cloned, nil
}

func readSchemaIdentity(source fs.FS, schemaPath string) (string, error) {
	payload, err := fs.ReadFile(source, schemaPath)
	if err != nil {
		return "", fmt.Errorf("manifest projection: read package Factory schema %s: %w", schemaPath, err)
	}
	var header struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(payload, &header); err != nil {
		return "", fmt.Errorf("manifest projection: decode package Factory schema %s: %w", schemaPath, err)
	}
	if strings.TrimSpace(header.ID) == "" {
		return "", fmt.Errorf("manifest projection: package Factory schema %s has no $id", schemaPath)
	}
	return header.ID, nil
}
