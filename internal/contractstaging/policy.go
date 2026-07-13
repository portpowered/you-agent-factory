// Package contractstaging defines the repository-owned contract package staging
// policy. It is build tooling and must not be imported by runtime packages.
package contractstaging

import (
	"path/filepath"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

const joinedOutputDirectory = "packages/api/generated/joined"

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

// AllowedArtifacts returns the complete reviewed generated artifact set in
// deterministic repository-relative path order.
func AllowedArtifacts() []string {
	artifacts := make([]string, len(joinedRoots))
	for index, root := range joinedRoots {
		artifacts[index] = filepath.ToSlash(filepath.Join(joinedOutputDirectory, root))
	}
	return artifacts
}
