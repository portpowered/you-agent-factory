package runtimetests

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

func TestCrossPlatformNamedFactoryPathTable_RejectUnsafeNames(t *testing.T) {
	rejectCases := []struct {
		name       string
		wantSubstr string
	}{
		{name: "", wantSubstr: "factory name is required"},
		{name: "../alpha", wantSubstr: "cannot contain path separators"},
		{name: "alpha/beta", wantSubstr: "cannot contain path separators"},
		{name: `alpha\beta`, wantSubstr: "cannot contain path separators"},
		{name: ".", wantSubstr: "not a valid directory name"},
		{name: "..", wantSubstr: "not a valid directory name"},
		{name: "@you", wantSubstr: "must be scoped as @scope/name"},
		{name: "@you/../goal", wantSubstr: "must be scoped as @scope/name"},
		{name: `@you/foo\bar`, wantSubstr: "cannot contain path separators"},
	}

	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()

			_, err := namedfactorypath.MapDir(rootDir, tc.name)
			if err == nil {
				t.Fatalf("MapNamedFactoryDir(%q) expected error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("MapNamedFactoryDir error = %v, want substring %q", err, tc.wantSubstr)
			}

			_, err = factorydefinitioncomposition.NamedPaths().ResolveExistingDir(rootDir, tc.name)
			if err == nil {
				t.Fatalf("ResolveNamedFactoryDir(%q) expected error", tc.name)
			}
			if !errors.Is(err, ErrInvalidNamedFactoryName) {
				t.Fatalf("ResolveNamedFactoryDir error = %v, want ErrInvalidNamedFactoryName", err)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("ResolveNamedFactoryDir error = %v, want substring %q", err, tc.wantSubstr)
			}

			_, err = factorydefinitioncomposition.PersistNamedFactory(rootDir, tc.name, namedFactoryPayload(t, "reject"), ownerFactoryDefinitionValidator())
			if err == nil {
				t.Fatalf("PersistNamedFactory(%q) expected error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("PersistNamedFactory error = %v, want substring %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestCrossPlatformNamedFactoryPathTable_MapHierarchicalContract(t *testing.T) {
	resolveCases := []struct {
		name         string
		wantSegments []string
	}{
		{name: "alpha", wantSegments: []string{"alpha"}},
		{name: "@you/goal", wantSegments: []string{"@you", "goal"}},
		{name: "@you/tts", wantSegments: []string{"@you", "tts"}},
	}

	for _, tc := range resolveCases {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()

			segments, err := namedfactorypath.PathSegments(tc.name)
			if err != nil {
				t.Fatalf("NamedFactoryPathSegments(%q): %v", tc.name, err)
			}
			if len(segments) != len(tc.wantSegments) {
				t.Fatalf("segments = %#v, want %#v", segments, tc.wantSegments)
			}
			for i := range tc.wantSegments {
				if segments[i] != tc.wantSegments[i] {
					t.Fatalf("segment[%d] = %q, want %q", i, segments[i], tc.wantSegments[i])
				}
			}

			mappedDir, err := namedfactorypath.MapDir(rootDir, tc.name)
			if err != nil {
				t.Fatalf("MapNamedFactoryDir(%q): %v", tc.name, err)
			}
			wantDir := filepath.Join(append([]string{rootDir}, tc.wantSegments...)...)
			if mappedDir != wantDir {
				t.Fatalf("MapNamedFactoryDir(%q) = %q, want %q", tc.name, mappedDir, wantDir)
			}
			if strings.Contains(mappedDir, "%2F") {
				t.Fatalf("mapper dir %q must not use percent-encoded scoped leaf names", mappedDir)
			}

			factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, tc.name, namedFactoryPayload(t, "hierarchical"), ownerFactoryDefinitionValidator())
			if err != nil {
				t.Fatalf("PersistNamedFactory(%q): %v", tc.name, err)
			}
			if factoryDir != wantDir {
				t.Fatalf("PersistNamedFactory(%q) = %q, want %q", tc.name, factoryDir, wantDir)
			}
			resolvedDir, err := factorydefinitioncomposition.NamedPaths().ResolveExistingDir(rootDir, tc.name)
			if err != nil {
				t.Fatalf("ResolveNamedFactoryDir(%q): %v", tc.name, err)
			}
			if resolvedDir != wantDir {
				t.Fatalf("ResolveNamedFactoryDir(%q) = %q, want %q", tc.name, resolvedDir, wantDir)
			}
		})
	}
}
