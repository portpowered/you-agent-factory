package clicontract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

const CompatibilityInventoryPath = "contracts/cli/deprecated.json"

type compatibilityInventory struct {
	Records map[string]struct {
		PublicName     string `json:"publicName"`
		Classification string `json:"classification"`
		ApprovalStatus string `json:"approvalStatus"`
	} `json:"records"`
}

// CheckProduction loads committed contracts and validates root without command execution.
func CheckProduction(root *cobra.Command, repositoryRoot string) ([]Finding, error) {
	production, err := commandidentity.Walk(root)
	if err != nil {
		return nil, fmt.Errorf("inventory production CLI: %w", err)
	}
	canonical, err := climanifest.LoadProduction(filepath.Join(repositoryRoot, filepath.FromSlash(climanifest.ProductionManifestPath)))
	if err != nil {
		return nil, err
	}
	compatibility, err := climanifest.LoadCompatibility(filepath.Join(repositoryRoot, filepath.FromSlash(climanifest.CompatibilityManifestPath)))
	if err != nil {
		return nil, err
	}
	approved, err := LoadApprovedCompatibility(filepath.Join(repositoryRoot, filepath.FromSlash(CompatibilityInventoryPath)))
	if err != nil {
		return nil, err
	}
	canonicalGenerated, compatibilityGenerated, err := loadGeneratedManifests()
	if err != nil {
		return nil, err
	}
	return Validate(Input{
		Production: production, Canonical: canonical, Compatibility: compatibility,
		ApprovedCompatibility: approved, GeneratedCanonical: canonicalGenerated,
		GeneratedCompatibility: compatibilityGenerated,
	}), nil
}

// LoadApprovedCompatibility returns callable, explicitly approved CLI inventory entries.
func LoadApprovedCompatibility(path string) ([]CompatibilityRecord, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CLI compatibility inventory %s: %w", path, err)
	}
	var inventory compatibilityInventory
	if err := json.Unmarshal(payload, &inventory); err != nil {
		return nil, fmt.Errorf("decode CLI compatibility inventory: %w", err)
	}
	records := make([]CompatibilityRecord, 0, len(inventory.Records))
	for inventoryID, record := range inventory.Records {
		if record.ApprovalStatus != "approved" || record.Classification == "remove-now" {
			continue
		}
		path := strings.Join(strings.Fields(record.PublicName), " ")
		if path == "" {
			return nil, fmt.Errorf("approved CLI compatibility record %q missing publicName", inventoryID)
		}
		records = append(records, CompatibilityRecord{
			InventoryID: inventoryID, StableID: strings.ReplaceAll(path, " ", "."),
			Path: path, Classification: record.Classification,
		})
	}
	return records, nil
}

func loadGeneratedManifests() ([]climanifest.Manifest, []climanifest.Manifest, error) {
	loaders := []func() (climanifest.Manifest, error){
		generated.RepresentativeFamilyManifest, generated.SessionFamilyManifest,
		generated.WorkFamilyManifest, generated.FactoryConfigInitFamilyManifest,
		generated.ModelsDocsFamilyManifest, generated.RunSubmitFamilyManifest,
		generated.MCPFamilyManifest,
	}
	canonical := make([]climanifest.Manifest, 0, len(loaders))
	for _, load := range loaders {
		manifest, err := load()
		if err != nil {
			return nil, nil, err
		}
		canonical = append(canonical, manifest)
	}
	compatibility, err := generated.WorkflowCompatibilityFamilyManifest()
	if err != nil {
		return nil, nil, err
	}
	return canonical, []climanifest.Manifest{compatibility}, nil
}
