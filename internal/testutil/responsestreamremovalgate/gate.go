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
)

// PrivateNDJSONRecordTypes are retired CLI response-stream JSON recordType values.
var PrivateNDJSONRecordTypes = []string{"progress", "compaction", "stream_gap", "primary_result"}

// PublicNDJSONRecordTypes are the only supported CLI response-stream recordType values.
var PublicNDJSONRecordTypes = []string{"factory_event", "invocation_result"}

var docsTopicRequiredMarkers = map[string][]string{
	"run": {
		"recordType=factory_event",
		"recordType=invocation_result",
		"FactoryEvent",
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
// closed when the public-surface residual-use checks fail.
func AssertGate(ctx context.Context, repoRoot string) error {
	_ = ctx
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if err := AssertDocsPrerequisite(repoRoot); err != nil {
		return fmt.Errorf("%s prerequisite: %w", PrerequisiteDocsStoryID, err)
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
