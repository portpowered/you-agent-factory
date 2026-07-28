package acp_test

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"testing"
)

//go:embed testdata/json_golden/manifest.json testdata/json_golden/upstream/*.json
var acpGoldenFiles embed.FS

type goldenManifest struct {
	Module  string            `json:"module"`
	Version string            `json:"version"`
	Commit  string            `json:"commit"`
	License string            `json:"license"`
	Files   []string          `json:"files"`
	SHA256  map[string]string `json:"sha256"`
}

func TestPinnedACPSDKGoldenManifestIsCompleteAndParseable(t *testing.T) {
	data, err := acpGoldenFiles.ReadFile("testdata/json_golden/manifest.json")
	if err != nil {
		t.Fatalf("read ACP golden manifest: %v", err)
	}
	var manifest goldenManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode ACP golden manifest: %v", err)
	}
	if manifest.Module != "github.com/coder/acp-go-sdk" || manifest.Version != "v0.13.5" || manifest.Commit != "0845a3bb9eddda5bfc22a94dd3598c90cb842451" || manifest.License != "Apache-2.0" {
		t.Fatalf("unexpected ACP golden source: %#v", manifest)
	}
	if len(manifest.Files) == 0 {
		t.Fatal("ACP golden manifest has no allowlisted fixtures")
	}
	seen := map[string]bool{}
	for _, name := range manifest.Files {
		if seen[name] {
			t.Fatalf("duplicate ACP golden fixture %q", name)
		}
		seen[name] = true
		fixture, err := acpGoldenFiles.ReadFile("testdata/json_golden/upstream/" + name)
		if err != nil {
			t.Fatalf("read ACP golden fixture %q: %v", name, err)
		}
		if !json.Valid(fixture) {
			digest := sha256.Sum256(fixture)
			t.Fatalf("ACP golden fixture %q is not valid JSON (sha256 %s)", name, hex.EncodeToString(digest[:]))
		}
		digest := sha256.Sum256(fixture)
		if got := hex.EncodeToString(digest[:]); manifest.SHA256[name] != got {
			t.Fatalf("ACP golden fixture %q sha256 = %s, manifest = %s", name, got, manifest.SHA256[name])
		}
	}
	if len(manifest.SHA256) != len(manifest.Files) {
		t.Fatalf("ACP golden checksum count = %d, fixture count = %d", len(manifest.SHA256), len(manifest.Files))
	}
}

func readGoldenJSON(t testing.TB, name string) json.RawMessage {
	t.Helper()
	data, err := acpGoldenFiles.ReadFile("testdata/json_golden/upstream/" + name)
	if err != nil {
		t.Fatalf("read ACP golden %q: %v", name, err)
	}
	return data
}
