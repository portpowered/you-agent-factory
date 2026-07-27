package contractstaging_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

// exportMapRawTargets lists every truthfully producible raw projection from the
// reviewed B06 package export map. Later-phase families such as components are
// intentionally omitted when no canonical owner exists yet.
var exportMapRawTargets = []string{
	"packages/api/generated/cli/command-manifest.schema.json",
	"packages/api/generated/cli/commands.json",
	"packages/api/generated/javascript/runtime-api.json",
	"packages/api/generated/mcp/tools.json",
	"packages/api/generated/openapi/openapi.yaml",
	"packages/api/generated/schemas/factory.schema.json",
	"packages/api/generated/schemas/mock-workers.schema.json",
	"packages/api/generated/schemas/you-config.schema.json",
}

func TestExportMapRawTargetsAreStagedByContractStaging(t *testing.T) {
	t.Parallel()

	allowed := contractstaging.AllowedArtifacts()
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, path := range allowed {
		allowedSet[path] = struct{}{}
	}
	for _, target := range exportMapRawTargets {
		if _, ok := allowedSet[target]; !ok {
			t.Errorf("export map target %s is missing from AllowedArtifacts()", target)
		}
	}
	for _, path := range allowed {
		if strings.Contains(path, "/components/") {
			t.Fatalf("AllowedArtifacts() must not invent later-phase component topology: %s", path)
		}
	}
}

func TestRepositoryStagedRawArtifacts_MatchCanonicalSources(t *testing.T) {
	t.Parallel()
	defer contractstaging.LockRepositoryStagingForTest()()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	for _, artifact := range contractstaging.RawArtifacts() {
		if artifact.Source == contractstaging.CanonicalOpenAPIPath {
			continue
		}
		artifact := artifact
		t.Run(artifact.Target, func(t *testing.T) {
			t.Parallel()
			canonicalPath := filepath.Join(repositoryRoot, filepath.FromSlash(artifact.Source))
			stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash(artifact.Target))

			canonical, err := os.ReadFile(canonicalPath)
			if err != nil {
				t.Fatalf("read canonical raw artifact %s: %v", artifact.Source, err)
			}
			staged, err := os.ReadFile(stagedPath)
			if err != nil {
				t.Fatalf("read staged raw artifact %s: %v", artifact.Target, err)
			}
			if !bytes.Equal(canonical, staged) {
				t.Fatalf(
					"staged raw artifact %s diverges from canonical source %s; raw projections must remain byte-identical copies",
					artifact.Target,
					artifact.Source,
				)
			}
		})
	}
}

func TestRepositoryStagedFactorySchema_MatchesOpenAPIDerivedProjection(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	artifacts := testArtifactsForRepository(t, repositoryRoot)
	const target = "packages/api/generated/schemas/factory.schema.json"
	projected := artifacts[target]
	stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash(target))
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged factory schema: %v", err)
	}
	if !bytes.Equal(projected, staged) {
		t.Fatalf("staged factory schema diverges from OpenAPI-derived projection at %s", target)
	}
}

func TestRepositoryAuthoredFactorySchema_MatchesStagedProjection(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	authoredPath := filepath.Join(repositoryRoot, filepath.FromSlash(contractstaging.FactorySchemaAuthoredPath))
	stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash("packages/api/generated/schemas/factory.schema.json"))
	authored, err := os.ReadFile(authoredPath)
	if err != nil {
		t.Fatalf("read authored factory schema: %v", err)
	}
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged factory schema: %v", err)
	}
	if !bytes.Equal(authored, staged) {
		t.Fatalf(
			"authored factory schema %s diverges from staged projection %s; both must be generated from ConvertFailClosedSchema",
			contractstaging.FactorySchemaAuthoredPath,
			"packages/api/generated/schemas/factory.schema.json",
		)
	}
}
