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
	unfinishedMovesStage           = "pss-unfinished-package-moves"
	unfinishedMovesRelativePath    = "docs/internal/baselines/unfinished-package-moves.json"
	edgesArchitectureExceptionNote = "Process Edges (pkg/services/edges) is the sole broad external-effect architecture exception for the Packaged Service Structure program."
)

// DestinationVocabulary is the closed set of destinations inventory rows may claim.
type DestinationVocabulary struct {
	ProductOwners          []string `json:"productOwners"`
	NonServiceFamilies     []string `json:"nonServiceFamilies"`
	ArchitectureExceptions []string `json:"architectureExceptions"`
}

// PackageMapping is one open-move row from the consolidated move ledger.
//
// Destination is the owner-relative committed destination bucket; Successor is
// the repository-relative package path that replaces PackagePath once the move
// lands. DeletionCondition names the closing cutover proof when the migration
// packet named one.
type PackageMapping struct {
	PackagePath       string `json:"packagePath"`
	Destination       string `json:"destination"`
	Successor         string `json:"successor"`
	DeletionCondition string `json:"deletionCondition,omitempty"`
}

// Manifest is the package-target destination vocabulary and future-debt document.
//
// It no longer carries package rows. Open moves live in one consolidated
// ledger (unfinishedMovesRelativePath) shared with the ownership-inventory
// checker, so a package that still has to move is recorded exactly once.
type Manifest struct {
	Version                    int                   `json:"version"`
	Stage                      string                `json:"stage"`
	DestinationVocabulary      DestinationVocabulary `json:"destinationVocabulary"`
	ArchitectureExceptionNotes map[string]string     `json:"architectureExceptionNotes"`
	// FutureDebt records deferred migration work intentionally left outside
	// this packet (for example FND-06 Edges narrowing).
	FutureDebt []FutureDebt `json:"futureDebt"`
}

// UnfinishedMoves is the consolidated open-move ledger. The ledger only
// shrinks: landing a move deletes its row, and when Moves is empty the file and
// its checks are deleted outright.
type UnfinishedMoves struct {
	Version int              `json:"version"`
	Stage   string           `json:"stage"`
	Moves   []PackageMapping `json:"moves"`
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
			"events",
			"worker_sessions",
			"webhooks",
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

func loadUnfinishedMoves(path string) (UnfinishedMoves, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Every migration landed. That is this ledger's intended end state.
			return UnfinishedMoves{}, nil
		}
		return UnfinishedMoves{}, fmt.Errorf("read unfinished package moves %s: %w", path, err)
	}
	var moves UnfinishedMoves
	if err := json.Unmarshal(data, &moves); err != nil {
		return UnfinishedMoves{}, fmt.Errorf("decode unfinished package moves %s: %w", path, err)
	}
	return moves, nil
}

func validateManifest(manifest Manifest) error {
	// Schema-only validation used by unit fixtures that do not bind a repo tree.
	return validateManifestSchema(manifest)
}

func validateManifestAt(repoRoot string, manifest Manifest, moves UnfinishedMoves) error {
	if err := validateManifestSchema(manifest); err != nil {
		return err
	}
	if err := validateUnfinishedMovesSchema(moves); err != nil {
		return err
	}
	return validateRowsNamePackagesThatExist(repoRoot, moves.Moves)
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
	return validateFutureDebt(manifest.FutureDebt)
}

func validateUnfinishedMovesSchema(moves UnfinishedMoves) error {
	if len(moves.Moves) == 0 {
		return nil
	}
	if moves.Version != 1 {
		return fmt.Errorf("unfinished package moves version %d is unsupported; want 1", moves.Version)
	}
	if moves.Stage != unfinishedMovesStage {
		return fmt.Errorf("unfinished package moves stage %q is unsupported; want %q", moves.Stage, unfinishedMovesStage)
	}
	closed := closedDestinationSet()
	for i, row := range moves.Moves {
		if err := validatePackageMapping(i, row, closed); err != nil {
			return err
		}
	}
	return validatePackageCoverage(moves)
}

func validateVocabulary(got DestinationVocabulary) error {
	want := closedDestinationVocabulary()
	if !slices.Equal(got.ProductOwners, want.ProductOwners) {
		return fmt.Errorf("destination vocabulary productOwners must exactly match the closed product-owner set")
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
	prefix := fmt.Sprintf("moves[%d]", index)
	packagePath := strings.TrimSpace(row.PackagePath)
	if packagePath == "" {
		return fmt.Errorf("%s.packagePath is required", prefix)
	}
	if strings.TrimSpace(row.Destination) == "" {
		return fmt.Errorf("%s.destination is required", prefix)
	}
	if err := validateDestination(row.Destination, closed); err != nil {
		return fmt.Errorf("%s.destination: %w", prefix, err)
	}
	successor := strings.TrimSpace(row.Successor)
	if successor == "" {
		return fmt.Errorf("%s.successor is required: an open move must name the package path that replaces it", prefix)
	}
	// A successor may equal its own packagePath: the transitional top-level
	// fold rows record that a package folds into the tree it already sits in.
	owner, _, _ := splitDestination(row.Destination)
	ownerRoot := "pkg/services/" + owner
	if successor != ownerRoot && !strings.HasPrefix(successor, ownerRoot+"/") {
		return fmt.Errorf("%s.successor %q must sit under %s for destination %q", prefix, successor, ownerRoot, row.Destination)
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
