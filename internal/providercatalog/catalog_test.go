package providercatalog

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuildProducesDeterministicValidatedSortedArtifacts(t *testing.T) {
	source := repositoryFixture(t)
	first, err := Build(source)
	if err != nil {
		t.Fatalf("Build() first pass: %v", err)
	}
	second, err := Build(source)
	if err != nil {
		t.Fatalf("Build() second pass: %v", err)
	}
	for _, target := range []string{ManifestSchemaPath, CatalogSchemaPath, CatalogPath} {
		if !bytes.Equal(first.Files[target], second.Files[target]) {
			t.Fatalf("%s changed across identical generation passes", target)
		}
	}

	var catalog map[string]any
	if err := json.Unmarshal(first.Files[CatalogPath], &catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if got := catalog["formatVersion"]; got != FormatVersion {
		t.Fatalf("formatVersion = %v, want %s", got, FormatVersion)
	}
	if got := catalog["providerSchema"]; got != ManifestSchemaID {
		t.Fatalf("providerSchema = %v, want %s", got, ManifestSchemaID)
	}
	providers := catalog["providers"].([]any)
	var ids []string
	for _, value := range providers {
		ids = append(ids, value.(map[string]any)["id"].(string))
	}
	want := "agy, claude, codex, cursor, gemini, kiro, opencode, pi"
	if got := strings.Join(ids, ", "); got != want {
		t.Fatalf("provider order = %s, want %s", got, want)
	}
	assertSchemaIdentifier(t, first.Files[ManifestSchemaPath], ManifestSchemaID)
	assertSchemaIdentifier(t, first.Files[CatalogSchemaPath], CatalogSchemaID)
}

func TestBuildRejectsInvalidAuthoredManifestValues(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{name: "malformed canonical id", old: "id: agy", new: "id: Agy", wantErr: "schema validation failed"},
		{name: "malformed alias", old: "aliases: []", new: "aliases: [Bad_Alias]", wantErr: "schema validation failed"},
		{name: "unknown support level", old: "technicalSupportLevel: not-supported", new: "technicalSupportLevel: preview", wantErr: "schema validation failed"},
		{name: "secret value", old: "  configurationKeys: []", new: "  configurationKeys: []\n  credentialValue: secret", wantErr: "schema validation failed"},
		{name: "environment value", old: "  configurationKeys: []", new: "  configurationKeys: []\n  environmentValues: [TOKEN=secret]", wantErr: "schema validation failed"},
		{name: "live readiness", old: "  configurationKeys: []", new: "  configurationKeys: []\n  ready: true", wantErr: "schema validation failed"},
		{name: "pricing", old: "  configurationKeys: []", new: "  configurationKeys: []\n  pricing: free", wantErr: "schema validation failed"},
		{name: "machine local executable path", old: "    - agy", new: "    - C:\\\\Users\\\\local\\\\agy.exe", wantErr: "schema validation failed"},
		{name: "machine local documentation URL", old: "https://agylabs.github.io/agy-demos/", new: "file:///C:/Users/example/private.html", wantErr: "schema validation failed"},
		{name: "credential bearing documentation URL", old: "https://agylabs.github.io/agy-demos/", new: "https://user:secret@example.com/docs", wantErr: "schema validation failed"},
		{name: "localhost documentation URL", old: "https://agylabs.github.io/agy-demos/", new: "https://localhost/docs", wantErr: "schema validation failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := repositoryFixture(t)
			mutateFixture(t, source, "packages/model-providers/providers/agy/provider.yaml", test.old, test.new)
			_, err := Build(source)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Build() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateCatalogSemanticsRejectsIdentityCollisions(t *testing.T) {
	tests := []struct {
		name      string
		providers []any
		wantErr   string
	}{
		{
			name:      "duplicate ids",
			providers: []any{semanticManifest("codex", nil), semanticManifest("codex", nil)},
			wantErr:   `duplicate canonical id "codex"`,
		},
		{
			name:      "alias equals own id",
			providers: []any{semanticManifest("codex", []any{"codex"})},
			wantErr:   `alias "codex" duplicates its canonical id`,
		},
		{
			name:      "alias shadows canonical id",
			providers: []any{semanticManifest("codex", []any{"claude"}), semanticManifest("claude", nil)},
			wantErr:   `alias "claude" shadows a canonical provider id`,
		},
		{
			name:      "duplicate aliases",
			providers: []any{semanticManifest("codex", []any{"agent"}), semanticManifest("claude", []any{"agent"})},
			wantErr:   `provider alias collision`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCatalogSemantics(test.providers)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateCatalogSemantics() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateCatalogSemanticsRejectsImpossibleCapabilitiesAndDeprecation(t *testing.T) {
	streaming := semanticManifest("codex", nil)
	streaming["maximumResponseFidelityCapabilities"].(map[string]any)["messageDeltas"] = true
	if err := validateCatalogSemantics([]any{streaming}); err == nil || !strings.Contains(err.Error(), "messageDeltas requires nativeStreaming") {
		t.Fatalf("streaming capability error = %v", err)
	}

	deprecated := semanticManifest("codex", nil)
	deprecated["deprecation"] = map[string]any{"replacementProviderId": "codex"}
	if err := validateCatalogSemantics([]any{deprecated}); err == nil || !strings.Contains(err.Error(), "cannot identify") {
		t.Fatalf("self replacement error = %v", err)
	}

	missing := semanticManifest("codex", nil)
	missing["deprecation"] = map[string]any{"replacementProviderId": "missing"}
	if err := validateCatalogSemantics([]any{missing}); err == nil || !strings.Contains(err.Error(), "not a canonical provider id") {
		t.Fatalf("missing replacement error = %v", err)
	}
}

func assertSchemaIdentifier(t *testing.T, payload []byte, want string) {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(payload, &schema); err != nil {
		t.Fatalf("decode generated schema: %v", err)
	}
	if got := schema["$id"]; got != want {
		t.Fatalf("$id = %v, want %s", got, want)
	}
	if _, ok := schema["$defs"].(map[string]any); !ok {
		t.Fatal("generated schema does not contain explicit $defs")
	}
}

func repositoryFixture(t *testing.T) fstest.MapFS {
	t.Helper()
	root := repositoryRoot(t)
	paths := []string{openAPIPath}
	matches, err := filepath.Glob(filepath.Join(root, authoredProvidersDir, "*", "provider.yaml"))
	if err != nil {
		t.Fatalf("glob provider manifests: %v", err)
	}
	for _, match := range matches {
		relative, err := filepath.Rel(root, match)
		if err != nil {
			t.Fatalf("relativize %s: %v", match, err)
		}
		paths = append(paths, filepath.ToSlash(relative))
	}
	fixture := fstest.MapFS{}
	for _, name := range paths {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		fixture[name] = &fstest.MapFile{Data: payload, Mode: 0o644}
	}
	return fixture
}

func mutateFixture(t *testing.T, fixture fstest.MapFS, name, old, replacement string) {
	t.Helper()
	file := fixture[name]
	updated := strings.Replace(string(file.Data), old, replacement, 1)
	if updated == string(file.Data) {
		t.Fatalf("fixture %s does not contain %q", name, old)
	}
	fixture[name] = &fstest.MapFile{Data: []byte(updated), Mode: file.Mode}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func semanticManifest(id string, aliases []any) map[string]any {
	return map[string]any{
		"id":      id,
		"aliases": aliases,
		"maximumExecutionCapabilities": map[string]any{
			"sessionResume": false,
			"toolExecution": false,
		},
		"maximumResponseFidelityCapabilities": map[string]any{
			"nativeStreaming":   false,
			"messageDeltas":     false,
			"toolOutputDeltas":  false,
			"providerReconnect": false,
			"toolLifecycle":     false,
		},
	}
}

var _ fs.FS = fstest.MapFS{}
