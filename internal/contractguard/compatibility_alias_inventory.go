package contractguard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	mcpDeprecatedInventoryPath = "contracts/mcp/deprecated.json"
	cliDeprecatedInventoryPath = "contracts/cli/deprecated.json"
	apiDeprecatedInventoryPath = "contracts/api/deprecated.json"
)

// CompatibilityAliasTerm is one inventoried compatibility alias surface that new
// first-party internal code must not adopt outside approved boundaries.
type CompatibilityAliasTerm struct {
	ItemID      string
	Family      string
	PublicName  string
	MatchValues []string
}

type compatibilityInventoryDocument struct {
	Family  string                         `json:"family"`
	Records map[string]compatibilityRecord `json:"records"`
}

type compatibilityRecord struct {
	ItemID     string `json:"itemId"`
	Family     string `json:"family"`
	PublicName string `json:"publicName"`
}

var compatibilityAliasLintExcludedItemIDs = map[string]struct{}{
	"api.selector.default-session": {},
	"api.route.get.events":         {},
}

// LoadCompatibilityAliasTerms reads the family compatibility inventories and
// returns deduplicated match values for each inventoried compatibility surface
// that must not be adopted in new first-party internal code.
func LoadCompatibilityAliasTerms(repoRoot string) ([]CompatibilityAliasTerm, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	termsByItemID := make(map[string]CompatibilityAliasTerm)
	for _, inventoryPath := range []string{
		mcpDeprecatedInventoryPath,
		cliDeprecatedInventoryPath,
		apiDeprecatedInventoryPath,
	} {
		document, err := readCompatibilityInventoryDocument(filepath.Join(repoRoot, filepath.FromSlash(inventoryPath)))
		if err != nil {
			return nil, err
		}
		for recordKey, record := range document.Records {
			itemID := strings.TrimSpace(record.ItemID)
			if itemID == "" {
				itemID = recordKey
			}
			family := strings.TrimSpace(record.Family)
			if family == "" {
				family = document.Family
			}
			publicName := strings.TrimSpace(record.PublicName)
			if publicName == "" {
				return nil, fmt.Errorf("%s: record %q is missing publicName", inventoryPath, recordKey)
			}
			if _, excluded := compatibilityAliasLintExcludedItemIDs[itemID]; excluded {
				continue
			}
			termsByItemID[itemID] = CompatibilityAliasTerm{
				ItemID:      itemID,
				Family:      family,
				PublicName:  publicName,
				MatchValues: derivedCompatibilityAliasMatchValues(itemID, family, publicName),
			}
		}
	}

	terms := make([]CompatibilityAliasTerm, 0, len(termsByItemID))
	for _, term := range termsByItemID {
		terms = append(terms, term)
	}
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].ItemID == terms[j].ItemID {
			return terms[i].PublicName < terms[j].PublicName
		}
		return terms[i].ItemID < terms[j].ItemID
	})
	return terms, nil
}

func readCompatibilityInventoryDocument(path string) (compatibilityInventoryDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return compatibilityInventoryDocument{}, fmt.Errorf("read compatibility inventory %s: %w", filepath.ToSlash(path), err)
	}
	var document compatibilityInventoryDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return compatibilityInventoryDocument{}, fmt.Errorf("decode compatibility inventory %s: %w", filepath.ToSlash(path), err)
	}
	if document.Records == nil {
		document.Records = map[string]compatibilityRecord{}
	}
	return document, nil
}

func derivedCompatibilityAliasMatchValues(itemID, family, publicName string) []string {
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		seen[value] = struct{}{}
	}

	add(publicName)
	switch family {
	case "mcp":
		if strings.HasPrefix(itemID, "mcp.alias.") {
			add(strings.TrimPrefix(itemID, "mcp.alias."))
		}
	case "cli":
		if strings.HasPrefix(itemID, "cli.command.") {
			add("you." + strings.TrimPrefix(itemID, "cli.command."))
		}
	}

	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
