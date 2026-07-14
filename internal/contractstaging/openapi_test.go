package contractstaging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestRepositoryStagedOpenAPI_MatchesReviewedBytePolicy(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	canonicalPath := filepath.Join(repositoryRoot, filepath.FromSlash(contractstaging.CanonicalOpenAPIPath))
	stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash(contractstaging.StagedOpenAPIPath))

	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical OpenAPI: %v", err)
	}
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged OpenAPI: %v", err)
	}

	if err := contractstaging.VerifyStagedOpenAPIParity(
		canonical,
		staged,
		contractstaging.ReviewedOpenAPIBytePolicy,
	); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyStagedOpenAPIParity_RejectsDivergentStagedBytes(t *testing.T) {
	t.Parallel()

	canonical := []byte("openapi: 3.1.0\n")
	staged := []byte("openapi: 3.0.0\n")

	err := contractstaging.VerifyStagedOpenAPIParity(
		canonical,
		staged,
		contractstaging.ReviewedOpenAPIBytePolicy,
	)
	if err == nil {
		t.Fatal("expected staged OpenAPI parity verification to fail")
	}
	if !strings.Contains(err.Error(), contractstaging.StagedOpenAPIPath) {
		t.Fatalf("error = %q, want staged path %q", err, contractstaging.StagedOpenAPIPath)
	}
}

func TestGenerateAPIInputsRemainLocalSourceDriven(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	hardcodedLocalSources := []string{
		"Makefile",
		"pkg/transports/http/generate.go",
	}
	stagedOpenAPIReference := filepath.ToSlash(contractstaging.StagedOpenAPIPath)
	localOpenAPIReference := contractstaging.CanonicalOpenAPIPath

	for _, source := range hardcodedLocalSources {
		source := source
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			assertGeneratorSourceUsesLocalOpenAPI(
				t,
				filepath.Join(repositoryRoot, filepath.FromSlash(source)),
				source,
				localOpenAPIReference,
				stagedOpenAPIReference,
			)
		})
	}

	t.Run("ui/scripts/generate-openapi-types.mjs", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(repositoryRoot, "ui", "scripts", "generate-openapi-types.mjs")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read generator source %s: %v", path, err)
		}
		if strings.Contains(string(payload), stagedOpenAPIReference) {
			t.Fatalf("ui/scripts/generate-openapi-types.mjs must not read staged package OpenAPI at %s", stagedOpenAPIReference)
		}
	})
}

func assertGeneratorSourceUsesLocalOpenAPI(
	t *testing.T,
	path, label, localOpenAPIReference, stagedOpenAPIReference string,
) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generator source %s: %v", label, err)
	}
	contents := string(payload)
	if !strings.Contains(contents, localOpenAPIReference) {
		t.Fatalf("%s must reference canonical OpenAPI at %s", label, localOpenAPIReference)
	}
	if strings.Contains(contents, stagedOpenAPIReference) {
		t.Fatalf("%s must not read staged package OpenAPI at %s", label, stagedOpenAPIReference)
	}
}
