package climanifestgen

import (
	"encoding/json"
	"fmt"
	"go/format"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const (
	// ProductionManifestPath is the authored CLI command manifest input.
	ProductionManifestPath    = climanifest.ProductionManifestPath
	CompatibilityManifestPath = climanifest.CompatibilityManifestPath

	// RepresentativeFamilyJSONPath is the generated representative-family metadata artifact.
	RepresentativeFamilyJSONPath = "pkg/transports/cli/generated/representative_family.json"

	// SessionFamilyJSONPath is the generated complete session-family metadata artifact.
	SessionFamilyJSONPath = "pkg/transports/cli/generated/session_family.json"

	// SessionFamilyCommandIDsPath is the generated stable session command ID list.
	SessionFamilyCommandIDsPath = "pkg/transports/cli/generated/session_command_ids_gen.go"

	// WorkFamilyJSONPath is the generated work-family metadata artifact.
	WorkFamilyJSONPath = "pkg/transports/cli/generated/work_family.json"

	// RunSubmitFamilyJSONPath is the generated run/submit-family metadata artifact.
	RunSubmitFamilyJSONPath = "pkg/transports/cli/generated/run_submit_family.json"

	// RunSubmitFamilyCommandIDsPath is the generated run/submit stable command ID list.
	RunSubmitFamilyCommandIDsPath = "pkg/transports/cli/generated/run_submit_command_ids_gen.go"

	// RepresentativeFamilyCommandIDsPath is the generated stable command ID list.
	RepresentativeFamilyCommandIDsPath = "pkg/transports/cli/generated/command_ids_gen.go"

	// FactoryConfigInitFamilyJSONPath is the generated factory/config/init metadata artifact.
	FactoryConfigInitFamilyJSONPath = "pkg/transports/cli/generated/factory_config_init_family.json"

	// FactoryConfigInitFamilyCommandIDsPath is the generated factory/config/init command ID list.
	FactoryConfigInitFamilyCommandIDsPath = "pkg/transports/cli/generated/factory_config_init_command_ids_gen.go"

	// ModelsDocsFamilyJSONPath is the generated models/docs-family metadata artifact.
	ModelsDocsFamilyJSONPath = "pkg/transports/cli/generated/models_docs_family.json"

	// ModelsDocsFamilyCommandIDsPath is the generated models/docs stable command ID list.
	ModelsDocsFamilyCommandIDsPath = "pkg/transports/cli/generated/models_docs_command_ids_gen.go"

	MCPFamilyJSONPath          = "pkg/transports/cli/generated/mcp_family.json"
	MCPFamilyCommandIDsPath    = "pkg/transports/cli/generated/mcp_command_ids_gen.go"
	RuntimeFamilyManifestsPath = "pkg/transports/cli/generated/family_manifests_gen.go"
)

// RepresentativeFamilyArtifact returns deterministic generated representative-family metadata bytes.
func RepresentativeFamilyArtifact(store generatedartifacts.SourceStore, repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(store, manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractRepresentativeFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// SessionFamilyArtifact returns deterministic generated session-family metadata bytes.
func SessionFamilyArtifact(store generatedartifacts.SourceStore, repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(store, manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractSessionFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// WorkArtifact returns the deterministic generated work-family metadata bytes.
func WorkArtifact(store generatedartifacts.SourceStore, repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(store, manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractWorkFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// RunSubmitArtifact returns deterministic generated run/submit-family metadata bytes.
func RunSubmitArtifact(store generatedartifacts.SourceStore, repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(store, manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractRunSubmitFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// FactoryConfigInitFamilyArtifact returns deterministic generated factory/config/init metadata bytes.
func FactoryConfigInitFamilyArtifact(store generatedartifacts.SourceStore, repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(store, manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractFactoryConfigInitFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// ModelsDocsArtifact returns the deterministic generated models/docs-family metadata bytes.
func ModelsDocsArtifact(store generatedartifacts.SourceStore, repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(store, manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractModelsDocsFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// MCPArtifact returns canonical MCP family metadata and enforces source classification.
func MCPArtifact(store generatedartifacts.SourceStore, repositoryRoot string) ([]byte, error) {
	production, err := climanifest.LoadProduction(store, filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath)))
	if err != nil {
		return nil, err
	}
	family, err := ExtractMCPFamily(production)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// Artifacts returns the complete deterministic CLI family output set.
// Filesystem persistence and drift inspection belong to the command-selected
// Platform artifact store.
func Artifacts(store generatedartifacts.SourceStore, repositoryRoot string) ([]generatedartifacts.Artifact, error) {
	producers := []struct {
		path     string
		producer func(generatedartifacts.SourceStore, string) ([]byte, error)
	}{
		{RepresentativeFamilyJSONPath, RepresentativeFamilyArtifact},
		{WorkFamilyJSONPath, WorkArtifact},
		{SessionFamilyJSONPath, SessionFamilyArtifact},
		{RunSubmitFamilyJSONPath, RunSubmitArtifact},
		{FactoryConfigInitFamilyJSONPath, FactoryConfigInitFamilyArtifact},
		{ModelsDocsFamilyJSONPath, ModelsDocsArtifact},
		{MCPFamilyJSONPath, MCPArtifact},
	}
	payloads := make(map[string][]byte, len(producers))
	for _, item := range producers {
		payload, err := item.producer(store, repositoryRoot)
		if err != nil {
			return nil, err
		}
		payloads[item.path] = payload
	}
	runtimeSource, err := runtimeFamilyManifestsSource(payloads)
	if err != nil {
		return nil, err
	}
	// Preserve the established output order so path diagnostics remain stable
	// when an output location is unavailable.
	return []generatedartifacts.Artifact{
		{Path: RepresentativeFamilyJSONPath, Absent: true},
		{Path: WorkFamilyJSONPath, Absent: true},
		{Path: SessionFamilyJSONPath, Absent: true},
		{Path: SessionFamilyCommandIDsPath, Payload: sessionCommandIDsSource()},
		{Path: RepresentativeFamilyCommandIDsPath, Payload: representativeAndWorkCommandIDsSource()},
		{Path: RunSubmitFamilyJSONPath, Absent: true},
		{Path: RunSubmitFamilyCommandIDsPath, Payload: runSubmitCommandIDsSource()},
		{Path: FactoryConfigInitFamilyJSONPath, Absent: true},
		{Path: FactoryConfigInitFamilyCommandIDsPath, Payload: factoryConfigInitCommandIDsSource()},
		{Path: ModelsDocsFamilyJSONPath, Absent: true},
		{Path: ModelsDocsFamilyCommandIDsPath, Payload: modelsDocsCommandIDsSource()},
		{Path: MCPFamilyJSONPath, Absent: true},
		{Path: MCPFamilyCommandIDsPath, Payload: mcpCommandIDsSource()},
		{Path: RuntimeFamilyManifestsPath, Payload: runtimeSource},
	}, nil
}

func runtimeFamilyManifestsSource(payloads map[string][]byte) ([]byte, error) {
	families := []struct {
		functionName string
		path         string
	}{
		{functionName: "representativeFamilyManifestValue", path: RepresentativeFamilyJSONPath},
		{functionName: "sessionFamilyManifestValue", path: SessionFamilyJSONPath},
		{functionName: "workFamilyManifestValue", path: WorkFamilyJSONPath},
		{functionName: "factoryConfigInitFamilyManifestValue", path: FactoryConfigInitFamilyJSONPath},
		{functionName: "modelsDocsFamilyManifestValue", path: ModelsDocsFamilyJSONPath},
		{functionName: "runSubmitFamilyManifestValue", path: RunSubmitFamilyJSONPath},
		{functionName: "mcpFamilyManifestValue", path: MCPFamilyJSONPath},
	}

	var builder strings.Builder
	builder.WriteString("// Code generated by climanifestgen. DO NOT EDIT.\n\n")
	builder.WriteString("package generated\n\n")
	builder.WriteString("import \"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest\"\n\n")
	for _, family := range families {
		var manifest climanifest.Manifest
		if err := json.Unmarshal(payloads[family.path], &manifest); err != nil {
			return nil, fmt.Errorf("decode generated family %s for Go source: %w", family.path, err)
		}
		fmt.Fprintf(&builder, "func %s() climanifest.Manifest {\n\treturn %s\n}\n\n", family.functionName, renderManifestGoValue(reflect.ValueOf(manifest)))
	}
	formatted, err := format.Source([]byte(builder.String()))
	if err != nil {
		return nil, fmt.Errorf("format generated runtime CLI manifests: %w", err)
	}
	return formatted, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func renderManifestGoValue(value reflect.Value) string {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "nil"
		}
		elementType := renderManifestGoType(value.Type().Elem())
		return "func(value " + elementType + ") *" + elementType +
			" { return &value }(" + renderManifestGoValue(value.Elem()) + ")"
	}
	switch value.Kind() {
	case reflect.Struct:
		var builder strings.Builder
		builder.WriteString(renderManifestGoType(value.Type()))
		builder.WriteByte('{')
		for index := 0; index < value.NumField(); index++ {
			if index > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(value.Type().Field(index).Name)
			builder.WriteByte(':')
			builder.WriteString(renderManifestGoValue(value.Field(index)))
		}
		builder.WriteByte('}')
		return builder.String()
	case reflect.Map:
		if value.IsNil() {
			return "nil"
		}
		keys := value.MapKeys()
		sort.Slice(keys, func(left, right int) bool { return keys[left].String() < keys[right].String() })
		var builder strings.Builder
		builder.WriteString(renderManifestGoType(value.Type()))
		builder.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(strconv.Quote(key.String()))
			builder.WriteByte(':')
			builder.WriteString(renderManifestGoValue(value.MapIndex(key)))
		}
		builder.WriteByte('}')
		return builder.String()
	case reflect.Slice:
		if value.IsNil() {
			return "nil"
		}
		var builder strings.Builder
		builder.WriteString(renderManifestGoType(value.Type()))
		builder.WriteByte('{')
		for index := 0; index < value.Len(); index++ {
			if index > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(renderManifestGoValue(value.Index(index)))
		}
		builder.WriteByte('}')
		return builder.String()
	case reflect.String:
		return strconv.Quote(value.String())
	case reflect.Bool:
		return strconv.FormatBool(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10)
	default:
		panic(fmt.Sprintf("unsupported CLI manifest Go value kind %s", value.Kind()))
	}
}

func renderManifestGoType(valueType reflect.Type) string {
	if valueType.Name() != "" {
		if valueType.PkgPath() == "github.com/portpowered/infinite-you/pkg/transports/cli/climanifest" {
			return "climanifest." + valueType.Name()
		}
		return valueType.Name()
	}
	switch valueType.Kind() {
	case reflect.Map:
		return "map[" + renderManifestGoType(valueType.Key()) + "]" + renderManifestGoType(valueType.Elem())
	case reflect.Slice:
		return "[]" + renderManifestGoType(valueType.Elem())
	case reflect.Pointer:
		return "*" + renderManifestGoType(valueType.Elem())
	default:
		return valueType.String()
	}
}

func representativeAndWorkCommandIDsSource() []byte {
	var builder strings.Builder
	builder.WriteString(`// Code generated by climanifestgen. DO NOT EDIT.

package generated

`)
	writeCommandIDVar(&builder, "RepresentativeFamilyCommandIDs", ProductionManifestPath, "representative root/session-show family", RepresentativeFamilyCommandIDs)
	builder.WriteString("\n")
	writeCommandIDVar(&builder, "WorkFamilyCommandIDs", ProductionManifestPath, "work inspection/control family", WorkFamilyCommandIDs)
	return []byte(builder.String())
}

func factoryConfigInitCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"FactoryConfigInitFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the factory/config/init family.",
		"FactoryConfigInitFamilyCommandIDs",
		FactoryConfigInitFamilyCommandIDs,
	)
}

func modelsDocsCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"ModelsDocsFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the models/docs CLI family.",
		"ModelsDocsFamilyCommandIDs",
		ModelsDocsFamilyCommandIDs,
	)
}

func runSubmitCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"RunSubmitFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the run/submit CLI family.",
		"RunSubmitFamilyCommandIDs",
		RunSubmitFamilyCommandIDs,
	)
}

func mcpCommandIDsSource() []byte {
	var builder strings.Builder
	builder.WriteString("// Code generated by climanifestgen. DO NOT EDIT.\n\npackage generated\n\n")
	writeCommandIDVar(&builder, "MCPFamilyCommandIDs", ProductionManifestPath, "canonical MCP family", MCPFamilyCommandIDs)
	return []byte(builder.String())
}

func sessionCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"SessionFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the canonical Factory Session family.",
		"SessionFamilyCommandIDs",
		SessionFamilyCommandIDs,
	)
}

func renderCommandIDsSource(comment, varName string, ids []string) []byte {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("\t%q,", id)
	}
	return []byte(`// Code generated by climanifestgen. DO NOT EDIT.

package generated

// ` + comment + `
var ` + varName + ` = []string{
` + strings.Join(quoted, "\n") + `
}
`)
}

func writeCommandIDVar(builder *strings.Builder, varName, sourcePath, familyLabel string, ids []string) {
	fmt.Fprintf(builder, "// %s lists the stable command IDs emitted from\n", varName)
	fmt.Fprintf(builder, "// %s for the %s.\n", sourcePath, familyLabel)
	fmt.Fprintf(builder, "var %s = []string{\n", varName)
	for _, id := range ids {
		fmt.Fprintf(builder, "\t%q,\n", id)
	}
	builder.WriteString("}\n")
}
