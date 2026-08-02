package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	manifestStage                  = "pss-fnd-01-package-target-manifest"
	manifestRelativePath           = "docs/internal/packaged-service-structure/package-target-manifest.json"
	edgesArchitectureExceptionNote = "Process Edges (pkg/services/edges) is the sole broad external-effect architecture exception for the Packaged Service Structure program."
	DispositionRetain              = "retain"
	DispositionMove                = "move"
	DispositionDelete              = "delete"
)

// DestinationVocabulary is the closed set of destinations inventory rows may claim.
type DestinationVocabulary struct {
	ProductOwners          []string `json:"productOwners"`
	NonServiceFamilies     []string `json:"nonServiceFamilies"`
	ArchitectureExceptions []string `json:"architectureExceptions"`
}

// PackageMapping maps one production pkg path to a destination or deletion queue entry.
type PackageMapping struct {
	PackagePath       string `json:"packagePath"`
	Disposition       string `json:"disposition"`
	Destination       string `json:"destination"`
	DeletionSuccessor string `json:"deletionSuccessor,omitempty"`
	DeletionCondition string `json:"deletionCondition,omitempty"`
}

// Manifest is the package-to-target and deletion inventory document.
type Manifest struct {
	Version                    int                   `json:"version"`
	Stage                      string                `json:"stage"`
	DestinationVocabulary      DestinationVocabulary `json:"destinationVocabulary"`
	ArchitectureExceptionNotes map[string]string     `json:"architectureExceptionNotes"`
	// FutureDebt records deferred migration work intentionally left outside
	// this packet (for example FND-06 Edges narrowing).
	FutureDebt []FutureDebt `json:"futureDebt"`
	// Inventory is the stable-sorted ledger seed of every production pkg package
	// path (repository-relative, slash-separated). Package destination rows are
	// filled separately under Packages.
	Inventory []string         `json:"inventory"`
	Packages  []PackageMapping `json:"packages"`
}

func closedDestinationVocabulary() DestinationVocabulary {
	return DestinationVocabulary{
		ProductOwners: []string{
			"factory_definitions",
			"factory_sessions",
			"factory_runtime",
			"work",
			"workers",
			"providers",
			"provider_sessions",
			"models",
			"automations",
			"recordings",
			"factory_visualization",
			"operator_settings",
			"system_initialization",
			"chat_sessions",
		},
		NonServiceFamilies: []string{
			"initializer",
			"root",
			"wire",
			"platform",
			"transports",
		},
		ArchitectureExceptions: []string{
			"edges",
		},
	}
}

func closedDestinationSet() map[string]struct{} {
	vocab := closedDestinationVocabulary()
	set := make(map[string]struct{}, len(vocab.ProductOwners)+len(vocab.NonServiceFamilies)+len(vocab.ArchitectureExceptions))
	for _, name := range vocab.ProductOwners {
		set[name] = struct{}{}
	}
	for _, name := range vocab.NonServiceFamilies {
		set[name] = struct{}{}
	}
	for _, name := range vocab.ArchitectureExceptions {
		set[name] = struct{}{}
	}
	return set
}

func loadManifest(relativePath string) (Manifest, error) {
	data, err := os.ReadFile(relativePath)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", relativePath, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest %s: %w", relativePath, err)
	}
	return manifest, nil
}

func validateManifest(manifest Manifest) error {
	// Schema-only validation used by unit fixtures that do not bind a repo tree.
	return validateManifestSchema(manifest)
}

func validateManifestAt(repoRoot string, manifest Manifest) error {
	if err := validateManifestSchema(manifest); err != nil {
		return err
	}
	return validateInventory(repoRoot, manifest.Inventory)
}

func validateManifestSchema(manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("manifest version %d is unsupported; want 1", manifest.Version)
	}
	if manifest.Stage != manifestStage {
		return fmt.Errorf("manifest stage %q is unsupported; want %q", manifest.Stage, manifestStage)
	}
	if err := validateVocabulary(manifest.DestinationVocabulary); err != nil {
		return err
	}
	if note := manifest.ArchitectureExceptionNotes["edges"]; note != edgesArchitectureExceptionNote {
		return fmt.Errorf("architectureExceptionNotes.edges must record the Process Edges exception exactly")
	}
	if err := validateFutureDebt(manifest.FutureDebt); err != nil {
		return err
	}
	closed := closedDestinationSet()
	for i, row := range manifest.Packages {
		if err := validatePackageMapping(i, row, closed); err != nil {
			return err
		}
	}
	if err := validateEdgesExceptionCoverage(manifest); err != nil {
		return err
	}
	if err := validateResidualCoverage(manifest); err != nil {
		return err
	}
	// Complete one-destination coverage is required once the inventory ledger
	// seed is present; schema-only fixtures may omit inventory.
	if len(manifest.Inventory) > 0 {
		if err := validatePackageCoverage(manifest); err != nil {
			return err
		}
	}
	return nil
}

func validateVocabulary(got DestinationVocabulary) error {
	want := closedDestinationVocabulary()
	if !slices.Equal(got.ProductOwners, want.ProductOwners) {
		return fmt.Errorf("destination vocabulary productOwners must exactly match the closed 13-owner set")
	}
	if !slices.Equal(got.NonServiceFamilies, want.NonServiceFamilies) {
		return fmt.Errorf("destination vocabulary nonServiceFamilies must exactly match the closed approved family set")
	}
	if !slices.Equal(got.ArchitectureExceptions, want.ArchitectureExceptions) {
		return fmt.Errorf("destination vocabulary architectureExceptions must exactly match the closed exception set")
	}
	return nil
}

func validatePackageMapping(index int, row PackageMapping, closed map[string]struct{}) error {
	prefix := fmt.Sprintf("packages[%d]", index)
	if strings.TrimSpace(row.PackagePath) == "" {
		return fmt.Errorf("%s.packagePath is required", prefix)
	}
	switch row.Disposition {
	case DispositionRetain, DispositionMove, DispositionDelete:
	default:
		return fmt.Errorf("%s.disposition %q is invalid; want retain, move, or delete", prefix, row.Disposition)
	}
	if strings.TrimSpace(row.Destination) == "" {
		return fmt.Errorf("%s.destination is required", prefix)
	}
	if err := validateDestination(row.Destination, closed); err != nil {
		return fmt.Errorf("%s.destination: %w", prefix, err)
	}
	if row.Disposition == DispositionDelete {
		if strings.TrimSpace(row.DeletionSuccessor) == "" {
			return fmt.Errorf("%s.deletionSuccessor is required when disposition is delete", prefix)
		}
		if strings.TrimSpace(row.DeletionCondition) == "" {
			return fmt.Errorf("%s.deletionCondition is required when disposition is delete", prefix)
		}
	} else if row.DeletionSuccessor != "" || row.DeletionCondition != "" {
		return fmt.Errorf("%s deletionSuccessor/deletionCondition are only valid when disposition is delete", prefix)
	}
	return nil
}

func validateDestination(destination string, closed map[string]struct{}) error {
	root, nested, ok := splitDestination(destination)
	if !ok {
		return fmt.Errorf("%q is outside closed destination set", destination)
	}
	if _, allowed := closed[root]; !allowed {
		return fmt.Errorf("%q is outside closed destination set", root)
	}
	if nested == "" {
		return nil
	}
	// Transitional service/ packages fold into the owner's private internal tree.
	if nested == "internal" {
		return nil
	}
	// Nested destinations may only name committed internal/services/<subservice> targets.
	if !strings.HasPrefix(nested, "internal/services/") {
		return fmt.Errorf("%q uses nested path %q; only internal/services/<subservice> nesting is allowed", destination, nested)
	}
	subservice := strings.TrimPrefix(nested, "internal/services/")
	if subservice == "" || strings.Contains(subservice, "/") {
		return fmt.Errorf("%q must name exactly one internal/services/<subservice> segment", destination)
	}
	if _, isOwner := productOwnerSet()[root]; isOwner && !isCommittedNestedSubservice(root, subservice) {
		return fmt.Errorf("%q uses nested subservice %q outside the committed plan tree for %s", destination, subservice, root)
	}
	return nil
}

func splitDestination(destination string) (root, nested string, ok bool) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return "", "", false
	}
	parts := strings.SplitN(destination, "/", 2)
	root = parts[0]
	if root == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return root, "", true
	}
	return root, parts[1], true
}

func resolveManifestPath(repoRoot, relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(repoRoot, relativePath)
}
