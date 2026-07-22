package clicontract

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
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

// CheckProduction loads committed contracts and validates a detached production observation.
func CheckProduction(store generatedartifacts.SourceStore, production commandidentity.Inventory, repositoryRoot string) ([]Finding, error) {
	input, err := loadProductionInput(store, production, repositoryRoot)
	if err != nil {
		return nil, err
	}
	return Validate(input), nil
}

// CheckProductionViolation applies one deliberate structural violation to
// snapshots, then runs the production validator. The Cobra tree is unchanged.
func CheckProductionViolation(store generatedartifacts.SourceStore, production commandidentity.Inventory, repositoryRoot string, violation DeliberateViolation) ([]Finding, error) {
	input, err := loadProductionInput(store, production, repositoryRoot)
	if err != nil {
		return nil, err
	}
	if err := applyDeliberateViolation(&input, violation); err != nil {
		return nil, err
	}
	return Validate(input), nil
}

func loadProductionInput(store generatedartifacts.SourceStore, production commandidentity.Inventory, repositoryRoot string) (Input, error) {
	canonical, err := climanifest.LoadProduction(store, filepath.Join(repositoryRoot, filepath.FromSlash(climanifest.ProductionManifestPath)))
	if err != nil {
		return Input{}, err
	}
	compatibility, err := climanifest.LoadCompatibility(store, filepath.Join(repositoryRoot, filepath.FromSlash(climanifest.CompatibilityManifestPath)))
	if err != nil {
		return Input{}, err
	}
	approved, err := LoadApprovedCompatibility(store, filepath.Join(repositoryRoot, filepath.FromSlash(CompatibilityInventoryPath)))
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
		compatibility := addSyntheticCompatibility(input)
		manifest := cloneManifestSnapshot(input.GeneratedCanonical[0])
		manifest.Commands[compatibility.ID] = compatibility
		input.GeneratedCanonical = append([]climanifest.Manifest(nil), input.GeneratedCanonical...)
		input.GeneratedCanonical[0] = manifest
	default:
		return fmt.Errorf("unknown deliberate CLI contract violation %q", violation)
	}
	return nil
}

func addSyntheticCompatibility(input *Input) climanifest.Command {
	command := input.Canonical.Commands["you.run"]
	command.ID = "you.compatibility-preview"
	command.Name = "compatibility-preview"
	command.Path = "you compatibility-preview"

	input.Compatibility = cloneManifestSnapshot(input.Compatibility)
	input.Compatibility.Commands[command.ID] = command
	input.ApprovedCompatibility = append(input.ApprovedCompatibility, CompatibilityRecord{
		InventoryID:    "synthetic-compatibility-preview",
		StableID:       command.ID,
		Path:           command.Path,
		Classification: "compatibility",
	})
	input.Production.Commands = append(input.Production.Commands, commandidentity.CommandRecord{
		IDCandidate:    command.ID,
		Name:           command.Name,
		Path:           command.Path,
		Visibility:     "visible",
		Runnable:       command.Runnable,
		HandlerPresent: command.Handler != nil,
	})
	input.GeneratedCompatibility = append(input.GeneratedCompatibility, climanifest.Manifest{
		Commands: map[string]climanifest.Command{command.ID: command},
	})
	return command
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
func LoadApprovedCompatibility(store generatedartifacts.SourceStore, path string) ([]CompatibilityRecord, error) {
	if store == nil {
		return nil, fmt.Errorf("CLI compatibility inventory source store is required")
	}
	payload, err := store.Read(path)
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
	return canonical, nil, nil
}
