// Package packagedfactorycatalog discovers and validates the authored
// first-party Factory inventory used by catalog generation.
package packagedfactorycatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswirevalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"gopkg.in/yaml.v3"
)

// Entry is one canonically decoded and validated authored Factory.
type Entry struct {
	Slug       string
	SourcePath string
	Factory    *factorydefinitions.FactoryConfig
}

// Inventory is the complete, stably ordered authored Factory inventory.
type Inventory struct {
	Entries []Entry
}

// Discover evaluates every immediate directory under root and returns the
// complete inventory sorted by directory slug.
func Discover(ctx context.Context, source fs.FS, root string) (Inventory, error) {
	if ctx == nil {
		return Inventory{}, errors.New("inventory: context is required")
	}
	if err := ctx.Err(); err != nil {
		return Inventory{}, fmt.Errorf("inventory: %w", err)
	}
	if source == nil {
		return Inventory{}, errors.New("inventory: source filesystem is required")
	}
	root = path.Clean(strings.TrimSpace(root))
	if root == "." || !fs.ValidPath(root) {
		return Inventory{}, fmt.Errorf("inventory: invalid authored root %q", root)
	}

	children, err := fs.ReadDir(source, root)
	if err != nil {
		return Inventory{}, fmt.Errorf("inventory: read authored root %s: %w", root, err)
	}

	var entries []Entry
	var diagnostics []string
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return Inventory{}, fmt.Errorf("inventory: %w", err)
		}
		if !child.IsDir() {
			continue
		}
		entry, err := discoverDirectory(source, root, child.Name())
		if err != nil {
			diagnostics = append(diagnostics, err.Error())
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 && len(diagnostics) == 0 {
		diagnostics = append(diagnostics, fmt.Sprintf("%s contains no authored Factory directories", root))
	}
	if len(diagnostics) == 0 {
		diagnostics = append(diagnostics, identityDiagnostics(entries)...)
	}
	if len(diagnostics) > 0 {
		sort.Strings(diagnostics)
		return Inventory{}, fmt.Errorf("inventory discovery failed:\n- %s", strings.Join(diagnostics, "\n- "))
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Slug < entries[j].Slug
	})
	return Inventory{Entries: entries}, nil
}

func discoverDirectory(source fs.FS, root, slug string) (Entry, error) {
	directory := path.Join(root, slug)
	if slug != strings.TrimSpace(slug) || !fs.ValidPath(slug) || strings.Contains(slug, "/") {
		return Entry{}, fmt.Errorf("%s has invalid directory slug %q", directory, slug)
	}
	sourcePath, err := resolveRootDocument(source, directory)
	if err != nil {
		return Entry{}, err
	}
	payload, err := fs.ReadFile(source, sourcePath)
	if err != nil {
		return Entry{}, fmt.Errorf("%s: read definition: %w", sourcePath, err)
	}
	cfg, err := decodeCanonicalFactory(source, directory, slug, sourcePath, payload)
	if err != nil {
		return Entry{}, err
	}
	if err := validateCanonicalFactory(sourcePath, cfg); err != nil {
		return Entry{}, err
	}

	return Entry{
		Slug:       slug,
		SourcePath: sourcePath,
		Factory:    cfg,
	}, nil
}

func resolveRootDocument(source fs.FS, directory string) (string, error) {
	children, err := fs.ReadDir(source, directory)
	if err != nil {
		return "", fmt.Errorf("%s: read Factory directory: %w", directory, err)
	}

	var supported []string
	var unsupported []string
	for _, child := range children {
		if child.IsDir() {
			continue
		}
		name := child.Name()
		lowerName := strings.ToLower(name)
		if isSupportedRootName(name) {
			supported = append(supported, path.Join(directory, name))
			continue
		}
		if strings.HasPrefix(lowerName, "factory.") || lowerName == "factory" {
			unsupported = append(unsupported, path.Join(directory, name))
		}
	}
	sort.Strings(supported)
	sort.Strings(unsupported)
	if len(unsupported) > 0 {
		return "", fmt.Errorf(
			"%s has unsupported root Factory candidate(s) %s; supported roots are factory.json, factory.yaml, factory.yml, and factory.js",
			directory,
			strings.Join(unsupported, ", "),
		)
	}
	if len(supported) != 1 {
		candidates := append(append([]string(nil), supported...), unsupported...)
		sort.Strings(candidates)
		if len(candidates) == 0 {
			return "", fmt.Errorf(
				"%s has no root Factory document; expected exactly one of %s/factory.json, %s/factory.yaml, %s/factory.yml, or %s/factory.js",
				directory,
				directory,
				directory,
				directory,
				directory,
			)
		}
		return "", fmt.Errorf(
			"%s has %d root Factory documents (%s); expected exactly one",
			directory,
			len(candidates),
			strings.Join(candidates, ", "),
		)
	}
	return supported[0], nil
}

func isSupportedRootName(name string) bool {
	switch name {
	case "factory.json", "factory.yaml", "factory.yml", "factory.js":
		return true
	default:
		return false
	}
}

func validateCanonicalFactory(sourcePath string, cfg *factorydefinitions.FactoryConfig) error {
	if validation := factorydefinitionswirevalidation.ValidateFactoryDefinition(cfg); validation.HasBlockingTargets() {
		var findings []string
		for _, target := range validation.BlockingTargets() {
			findings = append(findings, fmt.Sprintf("%s %s: %s", target.Code, target.Path, target.Message))
		}
		sort.Strings(findings)
		return fmt.Errorf("%s: canonical Factory validation failed: %s", sourcePath, strings.Join(findings, "; "))
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("%s: canonical Factory name is empty", sourcePath)
	}
	if strings.TrimSpace(cfg.Project) == "" {
		return fmt.Errorf("%s: canonical Factory project/id is empty", sourcePath)
	}
	return nil
}

func decodeCanonicalFactory(
	source fs.FS,
	directory string,
	slug string,
	sourcePath string,
	payload []byte,
) (*factorydefinitions.FactoryConfig, error) {
	if path.Ext(sourcePath) == ".js" {
		return decodeJavaScriptFactory(sourcePath, payload)
	}
	jsonPayload := payload
	switch path.Ext(sourcePath) {
	case ".yaml", ".yml":
		var document any
		if err := yaml.Unmarshal(payload, &document); err != nil {
			return nil, fmt.Errorf("%s: decode authored YAML: %w", sourcePath, err)
		}
		var err error
		jsonPayload, err = yamlToJSON(document)
		if err != nil {
			return nil, fmt.Errorf("%s: map authored YAML to canonical Factory boundary: %w", sourcePath, err)
		}
	}

	assembled, err := factorydefinitions.AssemblePackagedFactoryAssets(factorydefinitions.PackagedFactoryAssetDefinition{
		Package:     slug,
		FactoryJSON: jsonPayload,
		Assets:      source,
		AssetRoot:   directory,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: asset flattening: normalize authored Factory assets: %w", sourcePath, err)
	}

	cfg, err := factorymapping.NewFactoryConfigMapper().Expand(assembled)
	if err != nil {
		return nil, fmt.Errorf("%s: decode and map canonical Factory: %w", sourcePath, err)
	}
	return cfg, nil
}

// javascriptFactoryMetadata keeps a standalone packaged JavaScript Factory
// self-describing. A leading comment keeps the metadata inert when the same
// file is executed directly by the JavaScript runtime. The authored source is
// the only file needed; catalog generation projects it into portable artifacts.
type javascriptFactoryMetadata struct {
	Name                string                                        `json:"name"`
	Version             int                                           `json:"version"`
	ID                  string                                        `json:"id"`
	Description         *factorydefinitions.NameValueConfig           `json:"description,omitempty"`
	InvocationSignature *factorydefinitions.InvocationSignatureConfig `json:"invocationSignature,omitempty"`
	Examples            []factorydefinitions.InvocationExampleConfig  `json:"examples,omitempty"`
	ArgsSchema          json.RawMessage                               `json:"argsSchema,omitempty"`
	DefaultPolicy       json.RawMessage                               `json:"defaultPolicy,omitempty"`
}

func decodeJavaScriptFactory(sourcePath string, payload []byte) (*factorydefinitions.FactoryConfig, error) {
	metadata, err := decodeJavaScriptFactoryMetadata(payload)
	if err != nil {
		return nil, fmt.Errorf("%s: decode @you-factory-meta metadata: %w", sourcePath, err)
	}
	if strings.TrimSpace(metadata.Name) == "" || strings.TrimSpace(metadata.ID) == "" || metadata.Version < 1 {
		return nil, fmt.Errorf("%s: @you-factory-meta requires non-empty name and id plus a positive version", sourcePath)
	}
	return &factorydefinitions.FactoryConfig{
		Name:                metadata.Name,
		Project:             metadata.ID,
		Description:         metadata.Description,
		InvocationSignature: metadata.InvocationSignature,
		Examples:            metadata.Examples,
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: factorydefinitions.OrchestratorKindJavaScript,
			JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
				InlineSource: &factorydefinitions.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factorydefinitions.OrchestratorInlineEncoding,
					Inline:   string(payload),
				},
				Metadata: map[string]string{
					"name":    metadata.Name,
					"version": strconv.Itoa(metadata.Version),
				},
				ArgsSchema:    append(json.RawMessage(nil), metadata.ArgsSchema...),
				DefaultPolicy: append(json.RawMessage(nil), metadata.DefaultPolicy...),
			},
		},
		WorkTypes:    []factorydefinitions.WorkTypeConfig{},
		Resources:    nil,
		Workers:      nil,
		Workstations: nil,
	}, nil
}

func decodeJavaScriptFactoryMetadata(payload []byte) (javascriptFactoryMetadata, error) {
	const (
		prefix = "/* @you-factory-meta\n"
		suffix = "\n*/"
	)
	trimmed := bytes.TrimSpace(payload)
	if !bytes.HasPrefix(trimmed, []byte(prefix)) {
		return javascriptFactoryMetadata{}, errors.New("standalone packaged JavaScript source must begin with an @you-factory-meta JSON comment")
	}
	end := bytes.Index(trimmed[len(prefix):], []byte(suffix))
	if end < 0 {
		return javascriptFactoryMetadata{}, errors.New("@you-factory-meta JSON comment must end with */ on its own line")
	}
	metadataPayload := trimmed[len(prefix) : len(prefix)+end]
	decoder := json.NewDecoder(bytes.NewReader(metadataPayload))
	decoder.DisallowUnknownFields()
	var metadata javascriptFactoryMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return javascriptFactoryMetadata{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return javascriptFactoryMetadata{}, errors.New("@you-factory-meta comment must contain exactly one JSON object")
	}
	return metadata, nil
}

func identityDiagnostics(entries []Entry) []string {
	var diagnostics []string
	diagnostics = append(diagnostics, duplicateDiagnostics(entries, "public Factory name", func(entry Entry) string {
		return strings.TrimSpace(entry.Factory.Name)
	})...)
	diagnostics = append(diagnostics, duplicateDiagnostics(entries, "Factory project/id", func(entry Entry) string {
		return strings.TrimSpace(entry.Factory.Project)
	})...)
	diagnostics = append(diagnostics, duplicateDiagnostics(entries, "directory slug", func(entry Entry) string {
		return strings.ToLower(entry.Slug)
	})...)
	return diagnostics
}

func duplicateDiagnostics(entries []Entry, label string, value func(Entry) string) []string {
	pathsByValue := make(map[string][]string, len(entries))
	for _, entry := range entries {
		key := value(entry)
		pathsByValue[key] = append(pathsByValue[key], entry.SourcePath)
	}
	var diagnostics []string
	for repeated, paths := range pathsByValue {
		if len(paths) < 2 {
			continue
		}
		sort.Strings(paths)
		diagnostics = append(diagnostics, fmt.Sprintf(
			"duplicate %s %q in %s",
			label,
			repeated,
			strings.Join(paths, ", "),
		))
	}
	sort.Strings(diagnostics)
	return diagnostics
}
