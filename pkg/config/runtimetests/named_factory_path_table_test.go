package runtimetests

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
)

func TestCrossPlatformNamedFactoryPathTable_ResolveHierarchicalPaths(t *testing.T) {
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
			project := strings.TrimPrefix(strings.ReplaceAll(tc.name, "/", "-"), "@")

			factoryDir, err := PersistNamedFactory(rootDir, tc.name, namedFactoryPayload(t, project))
			if err != nil {
				t.Fatalf("PersistNamedFactory(%q): %v", tc.name, err)
			}

			wantDir := filepath.Join(append([]string{rootDir}, tc.wantSegments...)...)
			if factoryDir != wantDir {
				t.Fatalf("persisted dir = %q, want hierarchical %q", factoryDir, wantDir)
			}
			if strings.Contains(factoryDir, "%2F") {
				t.Fatalf("persisted dir %q must not use percent-encoded scoped leaf names", factoryDir)
			}

			resolvedDir, err := ResolveNamedFactoryDir(rootDir, tc.name)
			if err != nil {
				t.Fatalf("ResolveNamedFactoryDir(%q): %v", tc.name, err)
			}
			if resolvedDir != wantDir {
				t.Fatalf("resolved dir = %q, want %q", resolvedDir, wantDir)
			}

			entries, err := ListNamedFactories(rootDir)
			if err != nil {
				t.Fatalf("ListNamedFactories: %v", err)
			}
			if len(entries) != 1 || entries[0].Name != tc.name {
				t.Fatalf("list entries = %#v, want canonical name %q", entries, tc.name)
			}
		})
	}
}

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

			_, err := MapNamedFactoryDir(rootDir, tc.name)
			if err == nil {
				t.Fatalf("MapNamedFactoryDir(%q) expected error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("MapNamedFactoryDir error = %v, want substring %q", err, tc.wantSubstr)
			}

			_, err = ResolveNamedFactoryDir(rootDir, tc.name)
			if err == nil {
				t.Fatalf("ResolveNamedFactoryDir(%q) expected error", tc.name)
			}
			if !errors.Is(err, ErrInvalidNamedFactoryName) {
				t.Fatalf("ResolveNamedFactoryDir error = %v, want ErrInvalidNamedFactoryName", err)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("ResolveNamedFactoryDir error = %v, want substring %q", err, tc.wantSubstr)
			}

			_, err = PersistNamedFactory(rootDir, tc.name, namedFactoryPayload(t, "reject"))
			if err == nil {
				t.Fatalf("PersistNamedFactory(%q) expected error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("PersistNamedFactory error = %v, want substring %q", err, tc.wantSubstr)
			}
		})
	}
}
