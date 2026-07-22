// Package contractstaging defines the repository-owned contract package staging
// policy. It is build tooling and must not be imported by runtime packages.
package contractstaging

import (
	"path/filepath"
	"sort"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

const joinedOutputDirectory = "packages/api/generated/joined"

const (
	manifestTarget               = "packages/api/generated/manifest.json"
	FactorySchemaAuthoredPath    = "contracts/config/factory.schema.json"
	factorySchemaTarget          = "packages/api/generated/schemas/factory.schema.json"
	factoryEventSchemaTarget     = "packages/api/generated/schemas/factory-event.schema.json"
	factoryRecordingSchemaTarget = "packages/api/generated/schemas/factory-recording.schema.json"
)

// RawArtifact maps one canonical repository artifact into its package-facing
// generated projection. Source and Target are repository-relative slash paths.
type RawArtifact struct {
	Source string
	Target string
}

var (
	joinedRoots = []string{
		"contracts/common/deprecations.schema.json",
		"contracts/common/documentation.schema.json",
		"contracts/manifest.schema.json",
	}
	joinedComponents = []string{
		"contracts/common/deprecations.schema.json",
		"contracts/common/documentation.schema.json",
	}
	rawArtifacts = []RawArtifact{
		{Source: CanonicalOpenAPIPath, Target: StagedOpenAPIPath},
		{Source: "contracts/cli/commands.json", Target: "packages/api/generated/cli/commands.json"},
		{Source: "contracts/testdata/baseline/mcp-tools.json", Target: "packages/api/generated/mcp/tools.json"},
		{Source: "contracts/config/you-config.schema.json", Target: "packages/api/generated/schemas/you-config.schema.json"},
		{Source: "contracts/config/mock-workers.schema.json", Target: "packages/api/generated/schemas/mock-workers.schema.json"},
		{Source: "contracts/javascript/runtime-api.json", Target: "packages/api/generated/javascript/runtime-api.json"},
	}
)

// JoinInput returns the complete reviewed input set for contract package
// generation. Returning copies keeps the package policy immutable to callers.
func JoinInput(repositoryRoot string) contractjoiner.Input {
	return contractjoiner.Input{
		RepositoryRoot: repositoryRoot,
		Roots:          append([]string(nil), joinedRoots...),
		Components:     append([]string(nil), joinedComponents...),
	}
}

// RawArtifacts returns independent copies of the reviewed raw projection map.
func RawArtifacts() []RawArtifact {
	return append([]RawArtifact(nil), rawArtifacts...)
}

// SourceIdentityPaths returns the canonical repository paths whose latest change
// identifies the package source commit recorded in the publication manifest.
func SourceIdentityPaths() []string {
	paths := make([]string, 0, len(joinedRoots)+len(joinedComponents)+len(rawArtifacts)+3)
	paths = append(paths, joinedRoots...)
	paths = append(paths, joinedComponents...)
	for _, artifact := range rawArtifacts {
		paths = append(paths, artifact.Source)
	}
	paths = append(paths,
		FactorySchemaAuthoredPath,
		"docs/internal/contract/factory-schema-b16-gaps.json",
		"internal/contractstaging/factory_schema.go",
		"internal/contractstaging/factory_schema_b16_gaps.go",
		"internal/contractstaging/factory_recording_schema.go",
		"internal/contractstaging/manifest.go",
		"internal/contractstaging/openapi.go",
		"internal/contractstaging/policy.go",
	)
	sort.Strings(paths)
	compact := paths[:0]
	var previous string
	for _, path := range paths {
		if path == previous {
			continue
		}
		compact = append(compact, path)
		previous = path
	}
	return compact
}

// AllowedArtifacts returns the complete reviewed generated artifact set in
// deterministic repository-relative path order.
func AllowedArtifacts() []string {
	artifacts := make([]string, 0, len(joinedRoots)+len(rawArtifacts)+4)
	for _, root := range joinedRoots {
		artifacts = append(artifacts, filepath.ToSlash(filepath.Join(joinedOutputDirectory, root)))
	}
	for _, artifact := range rawArtifacts {
		artifacts = append(artifacts, artifact.Target)
	}
	artifacts = append(
		artifacts,
		manifestTarget,
		factorySchemaTarget,
		factoryEventSchemaTarget,
		factoryRecordingSchemaTarget,
	)
	sort.Strings(artifacts)
	return artifacts
}
