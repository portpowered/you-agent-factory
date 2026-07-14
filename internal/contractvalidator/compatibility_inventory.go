package contractvalidator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const compatibilityInventorySchemaID = "https://schemas.portpowered.com/you/contracts/compatibility-inventory.schema.json"

const (
	compatibilityDocumentationSchemaID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
	compatibilityDeprecationsSchemaID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	compatibilityVocabularySchemaID    = "https://schemas.portpowered.com/you/contracts/common/compatibility-inventory.schema.json"
)

// CompatibilityInventoryRegistry registers compatibility inventory schemas and
// reviewed valid fixtures.
func CompatibilityInventoryRegistry() Registry {
	return NewRegistry(Entry{
		Family:        "compatibility-inventory",
		FormatVersion: "1.0.0",
		Schemas: []Schema{
			{ID: compatibilityDocumentationSchemaID, Path: "contracts/common/documentation.schema.json"},
			{ID: compatibilityDeprecationsSchemaID, Path: "contracts/common/deprecations.schema.json"},
			{ID: compatibilityVocabularySchemaID, Path: "contracts/common/compatibility-inventory.schema.json"},
			{ID: compatibilityInventorySchemaID, Path: "contracts/compatibility-inventory.schema.json"},
		},
		Documents: []Document{
			{Path: "contracts/testdata/compatibility-inventory/valid-mcp-retain.json", SchemaID: compatibilityInventorySchemaID},
			{Path: "contracts/testdata/compatibility-inventory/valid-api-remove-now.json", SchemaID: compatibilityInventorySchemaID},
			{Path: "contracts/testdata/compatibility-inventory/valid-cli-separately-approved.json", SchemaID: compatibilityInventorySchemaID},
		},
	})
}

// CompatibilityInventorySemanticsDiagnostics applies inventory-specific semantic
// checks after schema validation succeeds.
func CompatibilityInventorySemanticsDiagnostics(document string, value any) []Diagnostic {
	return compatibilityInventorySemanticsDiagnostics(document, value)
}

func compatibilityInventorySemanticsDiagnostics(document string, value any) []Diagnostic {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	family, _ := root["family"].(string)
	records, ok := root["records"].(map[string]any)
	if !ok {
		return nil
	}

	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	seenItemIDs := make(map[string]string)
	var diagnostics []Diagnostic
	for _, key := range keys {
		recordValue, ok := records[key].(map[string]any)
		if !ok {
			continue
		}
		recordPath := "/records/" + escapeJSONPointerToken(key)
		itemID, _ := recordValue["itemId"].(string)
		if itemID != key {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.record_key_mismatch",
				recordPath+"/itemId",
				fmt.Sprintf("record itemId %s must match records key %s", strconv.Quote(itemID), strconv.Quote(key)),
				document,
			))
		}
		if previousKey, exists := seenItemIDs[itemID]; exists {
			message := fmt.Sprintf("stable ID %s appears more than once", strconv.Quote(itemID))
			diagnostics = append(diagnostics,
				newDiagnostic("identity.duplicate", recordPath+"/itemId", message, document),
				newDiagnostic("identity.duplicate", "/records/"+escapeJSONPointerToken(previousKey)+"/itemId", message, document),
			)
		} else {
			seenItemIDs[itemID] = key
		}
		recordFamily, _ := recordValue["family"].(string)
		if family != "" && recordFamily != family {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.family_mismatch",
				recordPath+"/family",
				fmt.Sprintf("record family %q must match inventory family %q", recordFamily, family),
				document,
			))
		}
		if lifecycle, ok := recordValue["lifecycle"].(map[string]any); ok {
			lifecycleItemID, _ := lifecycle["itemId"].(string)
			if itemID != "" && lifecycleItemID != itemID {
				diagnostics = append(diagnostics, newDiagnostic(
					"inventory.lifecycle_item_mismatch",
					recordPath+"/lifecycle/itemId",
					fmt.Sprintf("lifecycle itemId %s must match record itemId %s", strconv.Quote(lifecycleItemID), strconv.Quote(itemID)),
					document,
				))
			}
		}
	}
	sortDiagnostics(diagnostics)
	return diagnostics
}

func escapeJSONPointerToken(token string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(token)
}
