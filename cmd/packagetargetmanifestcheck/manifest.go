package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	unfinishedMovesStage        = "pss-unfinished-package-moves"
	unfinishedMovesRelativePath = "docs/internal/baselines/unfinished-package-moves.json"
)

// DestinationVocabulary is the closed set of destinations move rows may claim.
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

// UnfinishedMoves is the consolidated open-move ledger. The ledger only
// shrinks: landing a move deletes its row, and when Moves is empty the file and
// its checks are deleted outright.
type UnfinishedMoves struct {
	Version int              `json:"version"`
	Stage   string           `json:"stage"`
	Moves   []PackageMapping `json:"moves"`
}

// derivedDestinationVocabulary builds the destination vocabulary for repoRoot.
//
// The product-owner half is derived from the live pkg/services directory by the
// same ownershipinventory helper the ownership-inventory checker uses, so the two
// tools cannot disagree about which services exist. Before this, both carried the
// service names as hand-maintained Go literals and adding a service meant editing
// each one; now a service is a directory and neither tool needs a code change.
//
// The non-service families stay closed on purpose: they encode the approved
// top-level pkg/ families architectural rule, not a service roster.
func derivedDestinationVocabulary(repoRoot string) (DestinationVocabulary, error) {
	owners, err := ownershipinventory.DiscoverProductOwners(repoRoot)
	if err != nil {
		return DestinationVocabulary{}, err
	}
	return DestinationVocabulary{
		ProductOwners:          owners,
		NonServiceFamilies:     slices.Clone(ownershipinventory.NonServiceFamilies),
		ArchitectureExceptions: []string{"edges"},
	}, nil
}

func destinationSet(vocab DestinationVocabulary) map[string]struct{} {
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

// validateOpenMoveLedger checks the consolidated open-move ledger's schema and
// that every remaining row still names a package that exists on disk.
func validateOpenMoveLedger(repoRoot string, moves UnfinishedMoves) error {
	vocabulary, err := derivedDestinationVocabulary(repoRoot)
	if err != nil {
		return err
	}
	if err := validateUnfinishedMovesSchema(moves, vocabulary); err != nil {
		return err
	}
	return validateRowsNamePackagesThatExist(repoRoot, moves.Moves)
}

func validateUnfinishedMovesSchema(moves UnfinishedMoves, vocabulary DestinationVocabulary) error {
	if len(moves.Moves) == 0 {
		return nil
	}
	if moves.Version != 1 {
		return fmt.Errorf("unfinished package moves version %d is unsupported; want 1", moves.Version)
	}
	if moves.Stage != unfinishedMovesStage {
		return fmt.Errorf("unfinished package moves stage %q is unsupported; want %q", moves.Stage, unfinishedMovesStage)
	}
	closed := destinationSet(vocabulary)
	for i, row := range moves.Moves {
		if err := validatePackageMapping(i, row, vocabulary, closed); err != nil {
			return err
		}
	}
	return validatePackageCoverage(moves)
}

func validatePackageMapping(index int, row PackageMapping, vocabulary DestinationVocabulary, closed map[string]struct{}) error {
	prefix := fmt.Sprintf("moves[%d]", index)
	packagePath := strings.TrimSpace(row.PackagePath)
	if packagePath == "" {
		return fmt.Errorf("%s.packagePath is required", prefix)
	}
	if strings.TrimSpace(row.Destination) == "" {
		return fmt.Errorf("%s.destination is required", prefix)
	}
	if err := validateDestination(row.Destination, vocabulary, closed); err != nil {
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

func validateDestination(destination string, vocabulary DestinationVocabulary, closed map[string]struct{}) error {
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
	if _, isOwner := productOwnerSet(vocabulary)[root]; isOwner && !isCommittedNestedSubservice(root, subservice) {
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

func resolveRepoPath(repoRoot, relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(repoRoot, relativePath)
}
