package contractstaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const factorySchemaB16GapsRelativePath = "docs/internal/contract/factory-schema-b16-gaps.json"

type factorySchemaB16Gaps struct {
	Profile                    string                      `json:"profile"`
	Status                     string                      `json:"status"`
	Summary                    string                      `json:"summary"`
	BlockingCategories         []blockingCategory          `json:"blockingCategories"`
	ApprovedResidualExclusions []approvedResidualExclusion `json:"approvedResidualExclusions,omitempty"`
}

type blockingCategory struct {
	Code          string   `json:"code"`
	Keywords      []string `json:"keywords,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	Why           string   `json:"why"`
	InstanceCount int      `json:"instanceCount"`
}

type approvedResidualExclusion struct {
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	Why      string `json:"why"`
	Approval string `json:"approval"`
}

func loadFactorySchemaB16Gaps(repositoryRoot string) (factorySchemaB16Gaps, error) {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(factorySchemaB16GapsRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return factorySchemaB16Gaps{}, fmt.Errorf("read factory schema B16 gap record: %w", err)
	}
	var record factorySchemaB16Gaps
	if err := json.Unmarshal(payload, &record); err != nil {
		return factorySchemaB16Gaps{}, fmt.Errorf("decode factory schema B16 gap record: %w", err)
	}
	switch record.Profile {
	case "fail-closed":
	default:
		return factorySchemaB16Gaps{}, fmt.Errorf("factory schema B16 gap record has unexpected profile/status")
	}
	switch record.Status {
	case "blocks_full_endorsement", "converter_endorsed":
	default:
		return factorySchemaB16Gaps{}, fmt.Errorf("factory schema B16 gap record has unexpected profile/status")
	}
	return record, nil
}

func factorySchemaGapRecordEndorsesConverter(record factorySchemaB16Gaps) bool {
	return record.Status == "converter_endorsed"
}

func factorySchemaConverterFailureExpected(
	repositoryRoot string,
	diagnostics []contractvalidator.Diagnostic,
) (bool, error) {
	if len(diagnostics) == 0 {
		return false, nil
	}
	record, err := loadFactorySchemaB16Gaps(repositoryRoot)
	if err != nil {
		return false, err
	}
	for _, diagnostic := range diagnostics {
		if !factorySchemaDiagnosticMatchesGapRecord(diagnostic, record) {
			return false, fmt.Errorf(
				"factory schema converter diagnostic %#v is not covered by the B16 gap record",
				diagnostic,
			)
		}
	}
	return true, nil
}

func factorySchemaDiagnosticMatchesGapRecord(
	diagnostic contractvalidator.Diagnostic,
	record factorySchemaB16Gaps,
) bool {
	for _, category := range record.BlockingCategories {
		if diagnostic.Code != category.Code {
			continue
		}
		switch category.Code {
		case "openapi.convert.unsupported_keyword":
			keyword := unsupportedKeywordFromDiagnostic(diagnostic)
			for _, allowed := range category.Keywords {
				if keyword == allowed {
					return true
				}
			}
		case "openapi.convert.unsupported_reference":
			if strings.Contains(diagnostic.Message, "$ref must be the only keyword") {
				return true
			}
		}
	}
	for _, exclusion := range record.ApprovedResidualExclusions {
		if diagnostic.Code != exclusion.Code {
			continue
		}
		if exclusion.Path != "" && diagnostic.Path != exclusion.Path {
			continue
		}
		if exclusion.Pattern != "" && !strings.Contains(diagnostic.Message, exclusion.Pattern) {
			continue
		}
		return true
	}
	return false
}

func unsupportedKeywordFromDiagnostic(diagnostic contractvalidator.Diagnostic) string {
	const prefix = `keyword "`
	if !strings.HasPrefix(diagnostic.Message, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(diagnostic.Message, prefix)
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// CollectFactorySchemaConverterDiagnosticsForTest enumerates every fail-closed
// diagnostic blocking conversion of the supplied Factory graph copy.
func CollectFactorySchemaConverterDiagnosticsForTest(
	factory map[string]any,
	components map[string]any,
) []contractvalidator.Diagnostic {
	factoryCopy := deepCopyValue(factory).(map[string]any)
	componentsCopy := deepCopyValue(components).(map[string]any)
	return collectFactorySchemaConverterDiagnostics(factoryCopy, componentsCopy)
}

// GenerateFactorySchemaFromGraphForTest runs the reviewed Factory schema
// generation policy against an in-memory Factory graph copy.
func GenerateFactorySchemaFromGraphForTest(
	repositoryRoot string,
	factory map[string]any,
	components map[string]any,
) ([]byte, error) {
	return generateFactorySchemaFromGraph(repositoryRoot, factory, components)
}

func deepCopyValue(value any) any {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var copied any
	if err := json.Unmarshal(payload, &copied); err != nil {
		panic(err)
	}
	return copied
}

// DeepCopyValueForTest returns a deep copy of a YAML/JSON-decoded value.
func DeepCopyValueForTest(value any) any {
	return deepCopyValue(value)
}
