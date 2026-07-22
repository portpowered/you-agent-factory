// Package removalgate records Batch 09 private-contract removal prerequisites
// and residual-use evidence before deleting migrated NDJSON record types.
package responsestreamremovalgate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

const (
	// PrerequisiteDocsStoryID is the merged S22 public/maintainer docs lane.
	PrerequisiteDocsStoryID = "stream-b09-public-maintainer-docs"
	// PrerequisiteParityStoryID is the merged S24 cross-provider parity lane.
	PrerequisiteParityStoryID = "stream-b09-cross-provider-parity"
)

// PrivateNDJSONRecordTypes are retired CLI response-stream JSON recordType values.
var PrivateNDJSONRecordTypes = []string{"progress", "compaction", "primary_result"}

// PublicNDJSONRecordTypes are the only supported CLI response-stream recordType values.
var PublicNDJSONRecordTypes = []string{"response_event", "invocation_result"}

var docsTopicRequiredMarkers = map[string][]string{
	"run": {
		"recordType=response_event",
		"recordType=invocation_result",
		"FactoryResponseEvent",
	},
	"sessions": {
		"FactoryResponseEvent",
		"STREAM_GAP",
		"GET /factory-sessions/{session_id}/response-events",
	},
	"workers": {
		"FactoryResponseEvent",
		"STREAM_GAP",
		"## Response-stream provider fidelity",
	},
}

var docsForbiddenMarkers = []string{
	"recordType=progress",
	"recordType=compaction",
	"recordType=primary_result",
	"PROGRESS_FRAGMENT",
	"STREAM_COMPACTION_SIGNAL",
}

var productionSurfaceRoots = []string{
	"pkg/transports/cli/run",
	"pkg/transports/mapping",
	"pkg/transports/http",
}

var privateNDJSONEmissionLiterals = []string{
	`"recordType":"progress"`,
	`"recordType":"compaction"`,
	`"recordType":"primary_result"`,
	`"recordType": "progress"`,
	`"recordType": "compaction"`,
	`"recordType": "primary_result"`,
}

// AssertClosure records Batch 09 Story 005 end-to-end closure evidence: the
// consolidated removal gate, release-note migration mapping, and private NDJSON
// contract negatives that prove only the canonical public vocabulary remains.
func AssertClosure(ctx context.Context, repoRoot string) error {
	if err := AssertGate(ctx, repoRoot); err != nil {
		return err
	}
	if err := AssertReleaseNotesMigrationMapping(repoRoot); err != nil {
		return fmt.Errorf("release notes migration mapping: %w", err)
	}
	if err := AssertPrivateNDJSONRecordTypesRejected(); err != nil {
		return fmt.Errorf("private NDJSON contract negatives: %w", err)
	}
	return nil
}

// AssertGate records Story 001 prerequisites and residual-use evidence. It fails
// closed when S22 docs, S24 parity, or public-surface residual-use checks fail.
func AssertGate(ctx context.Context, repoRoot string) error {
	_ = ctx
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if err := AssertDocsPrerequisite(repoRoot); err != nil {
		return fmt.Errorf("%s prerequisite: %w", PrerequisiteDocsStoryID, err)
	}
	if err := AssertProviderParityOwnerEvidence(repoRoot); err != nil {
		return fmt.Errorf("%s prerequisite: %w", PrerequisiteParityStoryID, err)
	}
	if err := AssertNoPrivateNDJSONInProductionSurfaces(repoRoot); err != nil {
		return fmt.Errorf("residual private NDJSON emission: %w", err)
	}
	if err := AssertPublicTransportLayersDoNotImportLegacyCompat(repoRoot); err != nil {
		return fmt.Errorf("public transport depends on legacy compat mapper: %w", err)
	}
	if err := AssertLegacyCompatMapperDeleted(repoRoot); err != nil {
		return fmt.Errorf("legacy compat mapper deletion: %w", err)
	}
	if err := AssertNoRetiredPrivateContractSymbolsInProductionSurfaces(repoRoot); err != nil {
		return fmt.Errorf("retired private-contract symbols: %w", err)
	}
	return nil
}

// AssertProviderParityOwnerEvidence verifies that the cross-provider contract
// proof remains Workers-owned and retains every required fidelity fixture. The
// provider adapters themselves are executed by that package's tests; this
// removal gate inspects only static source and fixture evidence.
func AssertProviderParityOwnerEvidence(repoRoot string) error {
	const ownerRoot = "pkg/services/workers/provider/paritytests"
	requiredFiles := []string{
		"catalog_test.go",
		"harness_test.go",
		"transport_parity_test.go",
		"mode_parity_test.go",
		"testdata/full_stream_claude.jsonl",
		"testdata/partial_stream_codex.jsonl",
		"testdata/snapshot_only_opencode.jsonl",
		"testdata/final_only_opencode.txt",
		"testdata/tool_lifecycle_claude.jsonl",
		"testdata/agy_final_only.txt",
	}
	for _, relPath := range requiredFiles {
		path := filepath.Join(repoRoot, filepath.FromSlash(ownerRoot), filepath.FromSlash(relPath))
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("Workers provider parity evidence %q: %w", relPath, err)
		}
		if info.IsDir() {
			return fmt.Errorf("Workers provider parity evidence %q is a directory", relPath)
		}
	}

	catalogPath := filepath.Join(repoRoot, filepath.FromSlash(ownerRoot), "catalog_test.go")
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		return fmt.Errorf("read Workers provider parity catalog: %w", err)
	}
	for _, marker := range []string{
		"FidelityFullStream",
		"FidelityPartialStream",
		"FidelitySnapshotOnly",
		"FidelityFinalOnly",
		"FixtureToolLifecycleClaude",
		"FixtureAgyFinalOnly",
	} {
		if !strings.Contains(string(catalog), marker) {
			return fmt.Errorf("Workers provider parity catalog missing %q", marker)
		}
	}
	return nil
}

// AssertDocsPrerequisite proves the canonical packaged-doc sources advertise
// only the public response-stream vocabulary and omit retired private NDJSON
// record types.
func AssertDocsPrerequisite(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	for _, topic := range []string{"run", "sessions", "workers"} {
		path := filepath.Join(repoRoot, "docs", "reference", topic+".md")
		doc, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read canonical docs topic %q: %w", topic, err)
		}
		text := string(doc)
		for _, marker := range docsTopicRequiredMarkers[topic] {
			if !strings.Contains(text, marker) {
				return fmt.Errorf("docs topic %q missing public marker %q", topic, marker)
			}
		}
		for _, retired := range docsForbiddenMarkers {
			if strings.Contains(text, retired) {
				return fmt.Errorf("docs topic %q advertises retired private-contract marker %q", topic, retired)
			}
		}
	}
	return nil
}

// AssertNoPrivateNDJSONInProductionSurfaces scans supported CLI/API transport
// production code for literals that would emit or accept private NDJSON records.
func AssertNoPrivateNDJSONInProductionSurfaces(repoRoot string) error {
	for _, relRoot := range productionSurfaceRoots {
		absRoot := filepath.Join(repoRoot, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if contractguard.ShouldSkipDir(
					absRoot,
					path,
					"generated",
					"contracttests",
					"servertests",
				) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(contents)
			for _, literal := range privateNDJSONEmissionLiterals {
				if strings.Contains(text, literal) {
					return fmt.Errorf("%s contains private NDJSON literal %q", path, literal)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("scan %s: %w", relRoot, err)
		}
	}
	return nil
}

// AssertPublicTransportLayersDoNotImportLegacyCompat proves supported public
// transport emitters do not depend on the legacy fragment compatibility mapper.
func AssertPublicTransportLayersDoNotImportLegacyCompat(repoRoot string) error {
	const legacyCompatImport = "responsestream/compat"
	for _, relRoot := range productionSurfaceRoots {
		absRoot := filepath.Join(repoRoot, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if contractguard.ShouldSkipDir(absRoot, path, "generated", "contracttests", "servertests") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(contents), legacyCompatImport) {
				return fmt.Errorf("%s imports legacy compat mapper %q", path, legacyCompatImport)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("scan %s: %w", relRoot, err)
		}
	}
	return nil
}

// RepoRoot locates the repository root from the current working directory.
func RepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current := filepath.Clean(cwd)
	for {
		goModPath := filepath.Join(current, "go.mod")
		if info, statErr := os.Stat(goModPath); statErr == nil && !info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found from %s", cwd)
		}
		current = parent
	}
}
