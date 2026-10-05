package analyzers

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
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
	// Owned fixture inventory; Git discovery is exercised only by static smoke.
	names := []string{
		"api/openapi.yaml",
		"packages/model-providers/generated/catalog.json",
		"packages/model-providers/generated/provider-manifest.schema.json",
		"packages/model-providers/generated/provider-catalog.schema.json",
		"packages/model-providers/generated/runtime-acp.json",
		"packages/model-providers/providers/antigravity/provider.yaml",
		"packages/model-providers/providers/claude/provider.yaml",
		"packages/model-providers/providers/codex/provider.yaml",
		"packages/model-providers/providers/copilot-acp/provider.yaml",
		"packages/model-providers/providers/cursor/provider.yaml",
		"packages/model-providers/providers/droid-acp/provider.yaml",
		"packages/model-providers/providers/fast-agent-acp/provider.yaml",
		"packages/model-providers/providers/gemini/provider.yaml",
		"packages/model-providers/providers/grok-build-acp/provider.yaml",
		"packages/model-providers/providers/iflow-acp/provider.yaml",
		"packages/model-providers/providers/kilocode-acp/provider.yaml",
		"packages/model-providers/providers/kimi-acp/provider.yaml",
		"packages/model-providers/providers/kiro/provider.yaml",
		"packages/model-providers/providers/mux-acp/provider.yaml",
		"packages/model-providers/providers/openclaw-acp/provider.yaml",
		"packages/model-providers/providers/opencode/provider.yaml",
		"packages/model-providers/providers/pi/provider.yaml",
		"packages/model-providers/providers/pool-acp/provider.yaml",
		"packages/model-providers/providers/qoder-acp/provider.yaml",
		"packages/model-providers/providers/qwen-acp/provider.yaml",
		"packages/model-providers/providers/reasonix-acp/provider.yaml",
		"packages/model-providers/providers/trae-acp/provider.yaml",
		"packages/model-providers/providers/zeroclaw-acp/provider.yaml",
		"packages/model-providers/providers/copilot-acp/harness.yaml",
		"packages/model-providers/providers/cursor/harness.yaml",
		"packages/model-providers/providers/droid-acp/harness.yaml",
		"packages/model-providers/providers/fast-agent-acp/harness.yaml",
		"packages/model-providers/providers/gemini/harness.yaml",
		"packages/model-providers/providers/grok-build-acp/harness.yaml",
		"packages/model-providers/providers/iflow-acp/harness.yaml",
		"packages/model-providers/providers/kilocode-acp/harness.yaml",
		"packages/model-providers/providers/kimi-acp/harness.yaml",
		"packages/model-providers/providers/kiro/harness.yaml",
		"packages/model-providers/providers/mux-acp/harness.yaml",
		"packages/model-providers/providers/openclaw-acp/harness.yaml",
		"packages/model-providers/providers/opencode/harness.yaml",
		"packages/model-providers/providers/pi/harness.yaml",
		"packages/model-providers/providers/pool-acp/harness.yaml",
		"packages/model-providers/providers/qoder-acp/harness.yaml",
		"packages/model-providers/providers/qwen-acp/harness.yaml",
		"packages/model-providers/providers/reasonix-acp/harness.yaml",
		"packages/model-providers/providers/trae-acp/harness.yaml",
		"packages/model-providers/providers/zeroclaw-acp/harness.yaml",
	}
	snapshot := fstest.MapFS{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		snapshot[name] = &fstest.MapFile{Data: data, Mode: 0o644}
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

func TestProviderCatalogAcquisition(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, want                                 string
		failCommand                                int
		ignored, deleted, readFailure, statFailure bool
	}{
		{name: "clean"},
		{name: "ignored", ignored: true, want: "new/provider.yaml"},
		{name: "deleted", deleted: true},
		{name: "root-failure", failCommand: 1, want: "locate provider repository"},
		{name: "list-failure", failCommand: 2, want: "list provider inputs"},
		{name: "ignored-list-failure", failCommand: 3, want: "list ignored provider inputs"},
		{name: "read-failure", readFailure: true, want: "read api/openapi.yaml: permission denied"},
		{name: "stat-failure", statFailure: true, want: "inspect .*scaffold.txt: permission denied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			snapshot := providerCatalogFixture(t)
			var names []string
			for name := range snapshot {
				if strings.HasPrefix(name, providerInputs+"/") {
					names = append(names, name)
				}
			}
			scaffold := providerInputs + "/new/scaffold.txt"
			if test.ignored || test.statFailure {
				snapshot[scaffold] = &fstest.MapFile{Data: []byte("unread content")}
			}
			if test.deleted {
				names = append(names, scaffold)
			}
			names = append(names, providerInputs+"/claude/provider.yaml") // Duplicate Git entries read once.
			calls := 0
			git := providerCatalogGitFake(t, names, test.ignored || test.statFailure, test.failCommand, &calls)
			reads := map[string]int{}
			files := providerAcquisitionFS{MapFS: snapshot, reads: reads}
			if test.readFailure {
				files.denied = "api/openapi.yaml"
			}
			if test.statFailure {
				files.denied = scaffold
			}
			bound := providerCatalogForDirectory("fixture", git, func(root string) fs.FS {
				if root != "fixture" {
					t.Fatalf("unexpected root %q", root)
				}
				return files
			})
			assertProviderAcquisition(t, bound.Name, snapshot, reads, test.want == "", calls, test.failCommand)
			source := "package providercatalog"
			if test.want != "" {
				source += fmt.Sprintf(" // want %q", "provider-catalog: .*"+test.want)
			}
			directory, cleanup, err := analysistest.WriteFiles(map[string]string{modulePrefix + "internal/providercatalog/source.go": source})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			analysistest.Run(t, directory, bound, modulePrefix+"internal/providercatalog")
			for name, count := range reads {
				if count != 1 {
					t.Fatalf("read %s %d times", name, count)
				}
			}
		})
	}
}

func providerCatalogGitFake(t *testing.T, names []string, ignored bool, failCommand int, calls *int) func(...string) ([]byte, error) {
	t.Helper()
	return func(args ...string) ([]byte, error) {
		*calls++
		expected := [][]string{
			{"-C", "fixture", "rev-parse", "--show-toplevel"},
			{"-C", "fixture", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", providerInputs},
			{"-C", "fixture", "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--", providerInputs},
		}
		if *calls > len(expected) || strings.Join(args, "\x00") != strings.Join(expected[*calls-1], "\x00") {
			t.Fatalf("unexpected Git acquisition: %v", args)
		}
		if *calls == failCommand {
			return []byte("controlled failure"), fmt.Errorf("command failed")
		}
		if *calls == 1 {
			return []byte("fixture\n"), nil
		}
		if *calls == 2 {
			return []byte(strings.Join(names, "\x00") + "\x00"), nil
		}
		if ignored {
			return []byte(providerInputs + "/new/scaffold.txt\x00"), nil
		}
		return nil, nil
	}
}

func assertProviderAcquisition(t *testing.T, identity string, snapshot fstest.MapFS, reads map[string]int, clean bool, calls, failCommand int) {
	t.Helper()
	if clean && identity != providerCatalogSnapshot(snapshot, nil).Name {
		t.Fatal("acquisition changed snapshot identity")
	}
	if reads[providerInputs+"/new/scaffold.txt"] != 0 {
		t.Fatal("acquisition read unrelated scaffold content")
	}
	for name := range reads {
		if snapshot[name] == nil {
			t.Fatalf("read outside fixture: %s", name)
		}
	}
	if failCommand != 0 && (calls != failCommand || len(reads) != 0) {
		t.Fatal("failed Git acquisition continued")
	}
}

// A per-invocation filesystem records reads and injects acquisition failures.
type providerAcquisitionFS struct {
	fstest.MapFS
	reads  map[string]int
	denied string
}

func (s providerAcquisitionFS) ReadFile(name string) ([]byte, error) {
	if name == s.denied {
		return nil, fs.ErrPermission
	}
	data, err := s.MapFS.ReadFile(name)
	if err == nil {
		s.reads[name]++
	}
	return data, err
}

func (s providerAcquisitionFS) Stat(name string) (fs.FileInfo, error) {
	if name == s.denied {
		return nil, fs.ErrPermission
	}
	return s.MapFS.Stat(name)
}

func TestProviderCatalogReadOnlyAndFailure(t *testing.T) {
	t.Parallel()
	name := providercatalog.CatalogPath
	data := []byte("stale")
	source := fstest.MapFS{name: &fstest.MapFile{Data: bytes.Clone(data)}}
	snapshot, err := readProviderSnapshot(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot[name].Data, data) || !bytes.Equal(source[name].Data, data) {
		t.Fatal("acquisition changed output bytes")
	}
	if _, err := readProviderSnapshot(source, []string{"../escape"}); err == nil {
		t.Fatal("escaping path accepted")
	}
	source["packages/model-providers/generated"] = &fstest.MapFile{Mode: fs.ModeDir}
	if _, err := readProviderSnapshot(source, []string{"packages/model-providers/generated"}); err == nil {
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
