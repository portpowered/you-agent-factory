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

// DeliberateViolation identifies a smoke-only corruption applied to immutable
// contract snapshots. It proves diagnostics without executing commands or
// mutating the production Cobra tree.
type DeliberateViolation string

const (
	ViolationUncontractedCommand DeliberateViolation = "uncontracted-command"
	ViolationStaleMetadata       DeliberateViolation = "stale-generated-metadata"
	ViolationMissingHandler      DeliberateViolation = "missing-handler"
	ViolationAliasAsCanonical    DeliberateViolation = "compatibility-alias-as-canonical"
)

type compatibilityInventory struct {
	Records map[string]struct {
		PublicName     string `json:"publicName"`
		Classification string `json:"classification"`
		ApprovalStatus string `json:"approvalStatus"`
	} `json:"records"`
}

// CheckProduction loads committed contracts and validates root without command execution.
func CheckProduction(root *cobra.Command, repositoryRoot string) ([]Finding, error) {
	input, err := loadProductionInput(root, repositoryRoot)
	if err != nil {
		return nil, err
	}
	return Validate(input), nil
}

// CheckProductionViolation applies one deliberate structural violation to
// snapshots, then runs the production validator. The Cobra tree is unchanged.
func CheckProductionViolation(root *cobra.Command, repositoryRoot string, violation DeliberateViolation) ([]Finding, error) {
	input, err := loadProductionInput(root, repositoryRoot)
	if err != nil {
		return nil, err
	}
	if err := applyDeliberateViolation(&input, violation); err != nil {
		return nil, err
	}
	return Validate(input), nil
}

func loadProductionInput(root *cobra.Command, repositoryRoot string) (Input, error) {
	production, err := commandidentity.Walk(root)
	if err != nil {
		return Input{}, fmt.Errorf("inventory production CLI: %w", err)
	}
	canonical, err := climanifest.LoadProduction(filepath.Join(repositoryRoot, filepath.FromSlash(climanifest.ProductionManifestPath)))
	if err != nil {
		return Input{}, err
	}
	compatibility, err := climanifest.LoadCompatibility(filepath.Join(repositoryRoot, filepath.FromSlash(climanifest.CompatibilityManifestPath)))
	if err != nil {
		return Input{}, err
	}
	approved, err := LoadApprovedCompatibility(filepath.Join(repositoryRoot, filepath.FromSlash(CompatibilityInventoryPath)))
	if err != nil {
		return Input{}, err
	}
	canonicalGenerated, compatibilityGenerated, err := loadGeneratedManifests()
	if err != nil {
		return Input{}, err
	}
	return Input{
		Production: production, Canonical: canonical, Compatibility: compatibility,
		ApprovedCompatibility: approved, GeneratedCanonical: canonicalGenerated,
		GeneratedCompatibility: compatibilityGenerated,
	}, nil
}

func applyDeliberateViolation(input *Input, violation DeliberateViolation) error {
	switch violation {
	case ViolationUncontractedCommand:
		input.Production.Commands = append(input.Production.Commands, commandidentity.CommandRecord{
			IDCandidate: "you.experimental", Name: "experimental", Path: "you experimental",
			Visibility: "visible", Runnable: true, HandlerPresent: true,
		})
	case ViolationStaleMetadata:
		manifest := cloneManifestSnapshot(input.GeneratedCanonical[0])
		command := manifest.Commands["you"]
		command.Name = "stale"
		manifest.Commands["you"] = command
		input.GeneratedCanonical = append([]climanifest.Manifest(nil), input.GeneratedCanonical...)
		input.GeneratedCanonical[0] = manifest
	case ViolationMissingHandler:
		input.Production.Commands = append([]commandidentity.CommandRecord(nil), input.Production.Commands...)
		for index := range input.Production.Commands {
			if input.Production.Commands[index].Path == "you run" {
				input.Production.Commands[index].HandlerPresent = false
				return nil
			}
		}
		return fmt.Errorf("apply %s fixture: production command %q not found", violation, "you run")
	case ViolationAliasAsCanonical:
		compatibility, ok := input.Compatibility.Commands["you.workflow.preview"]
		if !ok {
			return fmt.Errorf("apply %s fixture: compatibility command %q not found", violation, "you.workflow.preview")
		}
		input.Canonical = cloneManifestSnapshot(input.Canonical)
		input.Canonical.Commands[compatibility.ID] = compatibility
		manifest := cloneManifestSnapshot(input.GeneratedCanonical[0])
		manifest.Commands[compatibility.ID] = compatibility
		input.GeneratedCanonical = append([]climanifest.Manifest(nil), input.GeneratedCanonical...)
		input.GeneratedCanonical[0] = manifest
	default:
		return fmt.Errorf("unknown deliberate CLI contract violation %q", violation)
	}
	return nil
}

func cloneManifestSnapshot(source climanifest.Manifest) climanifest.Manifest {
	commands := make(map[string]climanifest.Command, len(source.Commands))
	for id, command := range source.Commands {
		commands[id] = command
	}
	source.Commands = commands
	return source
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
