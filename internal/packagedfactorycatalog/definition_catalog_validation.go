package packagedfactorycatalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswirevalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func readPublishedManifest(source fs.FS) (Manifest, error) {
	payload, err := fs.ReadFile(source, CatalogManifestPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("packaged definition catalog: read manifest %s: %w", CatalogManifestPath, err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("packaged definition catalog: decode manifest %s: %w", CatalogManifestPath, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return Manifest{}, fmt.Errorf("packaged definition catalog: decode manifest %s: %w", CatalogManifestPath, err)
	}
	if manifest.FormatVersion != ManifestFormatVersion {
		return Manifest{}, fmt.Errorf(
			"packaged definition catalog: manifest formatVersion %q is unsupported; expected %q",
			manifest.FormatVersion,
			ManifestFormatVersion,
		)
	}
	if manifest.FactorySchema != factorySchemaIdentity {
		return Manifest{}, fmt.Errorf(
			"packaged definition catalog: manifest factorySchema %q is unsupported; expected %q",
			manifest.FactorySchema,
			factorySchemaIdentity,
		)
	}
	return manifest, nil
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

type definitionIdentityOwners struct {
	names    map[string]string
	projects map[string]string
	slugs    map[string]string
}

func newDefinitionIdentityOwners() definitionIdentityOwners {
	return definitionIdentityOwners{
		names:    make(map[string]string),
		projects: make(map[string]string),
		slugs:    make(map[string]string),
	}
}

func validateManifestEntryIdentity(
	entry ManifestEntry,
	context string,
	owners definitionIdentityOwners,
) error {
	name := strings.TrimSpace(entry.PublicName)
	project := strings.TrimSpace(entry.Project)
	slug := strings.TrimSpace(entry.Slug)
	if name == "" || project == "" || slug == "" {
		return fmt.Errorf("%s: name, project, and slug must be non-empty", context)
	}
	if entry.PublicName != name || entry.Project != project || entry.Slug != slug ||
		!fs.ValidPath(slug) || strings.Contains(slug, "/") {
		return fmt.Errorf("%s: invalid Factory name, project, or directory slug %q", context, entry.Slug)
	}
	if name != "@you/"+slug {
		return fmt.Errorf("%s: public name %q does not agree with portable Factory slug %q", context, name, slug)
	}
	for _, identity := range []struct {
		label  string
		value  string
		key    string
		owners map[string]string
	}{
		{label: "public name", value: name, key: name, owners: owners.names},
		{label: "project", value: project, key: project, owners: owners.projects},
		{label: "slug", value: slug, key: strings.ToLower(slug), owners: owners.slugs},
	} {
		if prior, exists := identity.owners[identity.key]; exists {
			return fmt.Errorf("%s: duplicate %s %q also used by %s", context, identity.label, identity.value, prior)
		}
		identity.owners[identity.key] = context
	}
	return nil
}

func validateManifestEntryLocators(
	entry ManifestEntry,
	context string,
	owners map[string]string,
) error {
	expected := []struct {
		format   string
		artifact ManifestArtifact
		locator  string
	}{
		{format: "JSON", artifact: entry.JSON, locator: path.Join(generatedCatalogRoot, entry.Slug, "factory.json")},
		{format: "YAML", artifact: entry.YAML, locator: path.Join(generatedCatalogRoot, entry.Slug, "factory.yaml")},
	}
	for _, item := range expected {
		if err := validatePublicLocator(item.artifact.Locator); err != nil {
			return fmt.Errorf("%s %s: %w", context, item.format, err)
		}
		if item.artifact.Locator != item.locator {
			return fmt.Errorf(
				"%s %s: locator %q does not resolve to expected package path %q",
				context,
				item.format,
				item.artifact.Locator,
				item.locator,
			)
		}
		key := strings.ToLower(item.artifact.Locator)
		if prior, exists := owners[key]; exists {
			return fmt.Errorf("%s %s: duplicate locator %q also used by %s", context, item.format, item.artifact.Locator, prior)
		}
		owners[key] = context + " " + item.format
	}
	return nil
}

func readVerifiedArtifact(source fs.FS, artifact ManifestArtifact, context string) ([]byte, error) {
	expectedHash, err := decodeManifestHash(artifact.SHA256)
	if err != nil {
		return nil, fmt.Errorf("%s artifact %q: %w", context, artifact.Locator, err)
	}
	payload, err := fs.ReadFile(source, artifact.Locator)
	if err != nil {
		return nil, fmt.Errorf("%s artifact %q: read: %w", context, artifact.Locator, err)
	}
	actualHash := sha256.Sum256(payload)
	if !bytes.Equal(actualHash[:], expectedHash) {
		return nil, fmt.Errorf(
			"%s artifact %q: SHA-256 mismatch: manifest=%s actual=%s",
			context,
			artifact.Locator,
			artifact.SHA256,
			hex.EncodeToString(actualHash[:]),
		)
	}
	return payload, nil
}

func decodeManifestHash(value string) ([]byte, error) {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return nil, fmt.Errorf("invalid SHA-256 %q; expected 64 lowercase hexadecimal characters", value)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid SHA-256 %q: %w", value, err)
	}
	return decoded, nil
}

func validatePublishedArtifactPair(
	schema interface{ Validate(any) error },
	entry ManifestEntry,
	jsonPayload []byte,
	yamlPayload []byte,
	context string,
) error {
	yamlJSON, err := decodedYAMLJSON(yamlPayload)
	if err != nil {
		return fmt.Errorf("%s YAML artifact %q: decode: %w", context, entry.YAML.Locator, err)
	}
	jsonFactory, err := validatePublishedArtifact(schema, entry, jsonPayload, "JSON", entry.JSON.Locator, context)
	if err != nil {
		return err
	}
	yamlFactory, err := validatePublishedArtifact(schema, entry, yamlJSON, "YAML", entry.YAML.Locator, context)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(jsonFactory, yamlFactory) {
		return fmt.Errorf("%s: generated JSON and YAML artifacts decode to different canonical Factory values", context)
	}
	return nil
}

func validatePublishedArtifact(
	schema interface{ Validate(any) error },
	entry ManifestEntry,
	payload []byte,
	format string,
	locator string,
	context string,
) (*factorydefinitions.FactoryConfig, error) {
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("%s %s artifact %q: decode JSON boundary: %w", context, format, locator, err)
	}
	if err := schema.Validate(document); err != nil {
		return nil, fmt.Errorf("%s %s artifact %q: Factory schema validation: %w", context, format, locator, err)
	}
	factory, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		return nil, fmt.Errorf("%s %s artifact %q: Factory Definitions mapping: %w", context, format, locator, err)
	}
	if factory.Name != entry.Slug || factory.Project != entry.Project {
		return nil, fmt.Errorf(
			"%s %s artifact %q: decoded identity name=%q project=%q does not agree with manifest slug=%q project=%q",
			context,
			format,
			locator,
			factory.Name,
			factory.Project,
			entry.Slug,
			entry.Project,
		)
	}
	if validation := factorydefinitionswirevalidation.ValidateFactoryDefinition(factory); validation.HasBlockingTargets() {
		var findings []string
		for _, target := range validation.BlockingTargets() {
			findings = append(findings, fmt.Sprintf("%s %s: %s", target.Code, target.Path, target.Message))
		}
		sort.Strings(findings)
		return nil, fmt.Errorf(
			"%s %s artifact %q: Factory Definitions validation: %s",
			context,
			format,
			locator,
			strings.Join(findings, "; "),
		)
	}
	return factory, nil
}
