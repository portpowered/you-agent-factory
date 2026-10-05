package analyzers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/portpowered/infinite-you/internal/providercatalog"
	"golang.org/x/tools/go/analysis/analysistest"
)

func providerCatalogFixture(t *testing.T) fstest.MapFS {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", providerInputs).Output()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := readProviderSnapshot(root, strings.Split(string(listed), "\x00"))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestProviderCatalogDiagnostics(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, want string
		mutate     func(fstest.MapFS)
	}{
		{"clean", "", func(fstest.MapFS) {}},
		{"unused-output", "", func(s fstest.MapFS) {
			s["packages/model-providers/generated/extra.txt"] = &fstest.MapFile{Data: []byte("unused")}
		}},
		{"missing", "missing: .*runtime-acp.json", func(s fstest.MapFS) { delete(s, providercatalog.RuntimeCatalogPath) }},
		{"stale", "stale: .*catalog.json", func(s fstest.MapFS) { s[providercatalog.CatalogPath].Data = []byte("stale") }},
		{"schema", "components.schemas is missing", func(s fstest.MapFS) { s["api/openapi.yaml"].Data = []byte("{}") }},
		{"manifest", "parse authored provider", func(s fstest.MapFS) { s[providerInputs+"/claude/provider.yaml"].Data = []byte("[") }},
		{"alias-collision", "identity collision.*codex", func(s fstest.MapFS) {
			s[providerInputs+"/claude/provider.yaml"].Data = bytes.Replace(s[providerInputs+"/claude/provider.yaml"].Data, []byte("aliases: []"), []byte("aliases: [codex]"), 1)
		}},
		{"impossible-streaming", "requires nativeStreaming", func(s fstest.MapFS) {
			s[providerInputs+"/claude/provider.yaml"].Data = bytes.Replace(s[providerInputs+"/claude/provider.yaml"].Data, []byte("nativeStreaming: true"), []byte("nativeStreaming: false"), 1)
		}},
		{"schema-privacy", "schema validation failed", func(s fstest.MapFS) {
			s[providerInputs+"/claude/provider.yaml"].Data = append(s[providerInputs+"/claude/provider.yaml"].Data, []byte("\ncredentialValue: secret\n")...)
		}},
		{"acp-harness", "requires harness.yaml", func(s fstest.MapFS) { delete(s, providerInputs+"/cursor/harness.yaml") }},
		{"populated-directory", "new/provider.yaml", func(s fstest.MapFS) {
			s[providerInputs+"/new/ignored.txt"] = &fstest.MapFile{Data: []byte("populated")}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := providerCatalogFixture(t)
			test.mutate(snapshot)
			source := "package providercatalog"
			if test.want != "" {
				source += fmt.Sprintf(" // want %q", "provider-catalog: .*"+test.want)
			}
			directory, cleanup, err := analysistest.WriteFiles(map[string]string{modulePrefix + "internal/providercatalog/source.go": source})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			analysistest.Run(t, directory, providerCatalogSnapshot(snapshot, nil), modulePrefix+"internal/providercatalog")
		})
	}
}

func TestProviderCatalogSnapshotIdentityAndIsolation(t *testing.T) {
	t.Parallel()
	snapshot := providerCatalogFixture(t)
	first := providerCatalogSnapshot(snapshot, nil)
	if first.Name != providerCatalogSnapshot(snapshot, nil).Name {
		t.Fatal("identical snapshots have different cache identities")
	}
	snapshot[providercatalog.RuntimeCatalogPath].Data = []byte("changed")
	if first.Name == providerCatalogSnapshot(snapshot, nil).Name {
		t.Fatal("output changes failed to invalidate cache")
	}
	delete(snapshot, providercatalog.RuntimeCatalogPath)
	if first.Name == providerCatalogSnapshot(snapshot, nil).Name {
		t.Fatal("missing output failed to invalidate cache")
	}
	if first.Name == providerCatalogSnapshot(snapshot, fmt.Errorf("permission denied")).Name {
		t.Fatal("read failure failed to invalidate cache")
	}
	directory, cleanup, err := analysistest.WriteFiles(map[string]string{modulePrefix + "internal/providercatalog/source.go": "package providercatalog"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, directory, first, modulePrefix+"internal/providercatalog")
}

func TestProviderCatalogEachOutputAndInputInvalidatesCache(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"api/openapi.yaml", providerInputs + "/claude/provider.yaml", providerInputs + "/cursor/harness.yaml",
		providercatalog.CatalogPath, providercatalog.ManifestSchemaPath, providercatalog.CatalogSchemaPath, providercatalog.RuntimeCatalogPath} {
		t.Run(name, func(t *testing.T) {
			snapshot := providerCatalogFixture(t)
			before := providerCatalogSnapshot(snapshot, nil).Name
			snapshot[name].Data = append(snapshot[name].Data, '\n')
			if before == providerCatalogSnapshot(snapshot, nil).Name {
				t.Fatal("data change retained cache identity")
			}
			delete(snapshot, name)
			if before == providerCatalogSnapshot(snapshot, nil).Name {
				t.Fatal("deletion retained cache identity")
			}
		})
	}
	for _, name := range []string{providercatalog.CatalogPath, providercatalog.ManifestSchemaPath, providercatalog.CatalogSchemaPath, providercatalog.RuntimeCatalogPath} {
		for _, kind := range []string{"missing", "stale"} {
			t.Run(kind+name, func(t *testing.T) {
				snapshot := providerCatalogFixture(t)
				if kind == "missing" {
					delete(snapshot, name)
				} else {
					snapshot[name].Data = []byte("stale")
				}
				source := fmt.Sprintf("package providercatalog // want %q", "provider-catalog: "+kind+": "+name+"; run")
				directory, cleanup, err := analysistest.WriteFiles(map[string]string{modulePrefix + "internal/providercatalog/source.go": source})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(cleanup)
				analysistest.Run(t, directory, providerCatalogSnapshot(snapshot, nil), modulePrefix+"internal/providercatalog")
			})
		}
	}
}

func TestProviderCatalogGitIncludesIgnoredAndDeletedInputs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	command := exec.Command("git", "init", "-q", root)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}
	for name, file := range providerCatalogFixture(t) {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, file.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := ProviderCatalogForDirectory(root)
	ignored := providerInputs + "/new/scaffold.txt"
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("scaffold.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(ignored))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	withIgnored := ProviderCatalogForDirectory(root)
	if before.Name == withIgnored.Name {
		t.Fatal("ignored populated directory was excluded")
	}
	directory, cleanup, err := analysistest.WriteFiles(map[string]string{modulePrefix + "internal/providercatalog/source.go": "package providercatalog // want `new/provider.yaml`"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, directory, withIgnored, modulePrefix+"internal/providercatalog")
	command = exec.Command("git", "-C", root, "add", "-f", ignored)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add fixture: %s: %v", out, err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if before.Name != ProviderCatalogForDirectory(root).Name {
		t.Fatal("deleted tracked scaffold remains in virtual tree")
	}
}

func TestProviderCatalogReadOnlyAndFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := providercatalog.CatalogPath
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("stale")
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readProviderSnapshot(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot[name].Data, data) {
		t.Fatal("snapshot changed bytes")
	}
	after, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("acquisition wrote output")
	}
	if _, err := readProviderSnapshot(root, []string{"../escape"}); err == nil {
		t.Fatal("escaping path accepted")
	}
	if _, err := readProviderSnapshot(root, []string{"packages/model-providers/generated"}); err == nil {
		t.Fatal("unreadable directory accepted")
	}
	directory, cleanup, err := analysistest.WriteFiles(map[string]string{
		modulePrefix + "internal/providercatalog/source.go": "package providercatalog // want `provider-catalog: permission denied`",
		modulePrefix + "internal/unrelated/source.go":       "package unrelated",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, directory, providerCatalogSnapshot(nil, fmt.Errorf("permission denied")), modulePrefix+"internal/providercatalog", modulePrefix+"internal/unrelated")
}
