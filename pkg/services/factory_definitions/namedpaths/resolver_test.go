package namedpaths

import (
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

var testNamedPaths = func() *Resolver {
	resolver, err := New(platformfilesystem.Local{})
	if err != nil {
		panic(err)
	}
	return resolver
}()

func TestNewRequiresFileSystem(t *testing.T) {
	t.Parallel()

	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) error = nil, want required filesystem error")
	}
}

func TestResolveCandidatePathsReturnsCanonicalPathsInCatalogPrecedence(t *testing.T) {
	t.Parallel()

	projectRoot := filepath.Join("repo", "factory")
	globalRoot := filepath.Join("home", "factories")
	got, err := testNamedPaths.ResolveCandidatePaths(projectRoot, globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveCandidatePaths: %v", err)
	}
	if want := filepath.Join(projectRoot, "@you", "goal"); got.Project != want {
		t.Fatalf("Project = %q, want %q", got.Project, want)
	}
	if want := filepath.Join(globalRoot, "@you", "goal"); got.Global != want {
		t.Fatalf("Global = %q, want %q", got.Global, want)
	}
}

func TestResolveCandidatePathsRejectsInvalidNameWithoutPartialResult(t *testing.T) {
	t.Parallel()

	got, err := testNamedPaths.ResolveCandidatePaths("project", "global", "../escape")
	if err == nil {
		t.Fatal("ResolveCandidatePaths error = nil, want invalid-name error")
	}
	if got != (CandidatePaths{}) {
		t.Fatalf("ResolveCandidatePaths result = %#v, want zero detached result", got)
	}
}

func TestResolveCandidatePathsRetainsValidFallbackCandidate(t *testing.T) {
	t.Parallel()

	got, err := testNamedPaths.ResolveCandidatePaths("", "global", "alpha")
	if err != nil {
		t.Fatalf("ResolveCandidatePaths: %v", err)
	}
	if got.Project != "" {
		t.Fatalf("Project = %q, want empty invalid candidate", got.Project)
	}
	if want := filepath.Join("global", "alpha"); got.Global != want {
		t.Fatalf("Global = %q, want fallback candidate %q", got.Global, want)
	}
}
