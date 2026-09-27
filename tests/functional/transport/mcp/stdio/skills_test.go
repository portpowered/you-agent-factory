package stdio_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestMCPSkillsExtensionListsGetsAndReadsPublishedSkills proves legacy
// 2024-11-05 clients can still call the Skills methods and read published
// resources. The Skills-specific 2026 discovery handshake is covered below.
func TestMCPSkillsExtensionListsGetsAndReadsPublishedSkills(t *testing.T) {
	server := startFixtureBackedMCPServer(t)
	defer server.cleanup()

	initResult := server.client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "skills-functional-test", "version": "test"},
	})
	if initResult.Error != nil {
		t.Fatalf("initialize error = %#v", initResult.Error)
	}
	assertMCPSkillsCapabilities(t, initResult.Result)

	listing := server.client.call("skills/list", map[string]any{})
	if listing.Error != nil {
		t.Fatalf("skills/list error = %#v", listing.Error)
	}
	assertMCPCompleteCacheableResult(t, listing.Result, "skills/list")
	entries, ok := listing.Result["skills"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("skills/list skills = %#v, want non-empty array", listing.Result["skills"])
	}

	var operatorSkill map[string]any
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("skills/list entry = %#v, want object", value)
		}
		if entry["uri"] == "skill://subagent-configuration/SKILL.md" {
			operatorSkill = entry
		}
		assertSkillManifest(t, entry)
	}
	if operatorSkill == nil {
		t.Fatalf("skills/list did not publish skill://subagent-configuration/SKILL.md: %#v", entries)
	}

	skillURI, _ := operatorSkill["uri"].(string)
	got := server.client.call("skills/get", map[string]any{"uri": skillURI})
	if got.Error != nil {
		t.Fatalf("skills/get(%q) error = %#v", skillURI, got.Error)
	}
	assertMCPCompleteCacheableResult(t, got.Result, "skills/get")
	gotSkill, ok := got.Result["skill"].(map[string]any)
	if !ok {
		t.Fatalf("skills/get skill = %#v, want object", got.Result["skill"])
	}
	assertSkillManifest(t, gotSkill)
	if gotSkill["uri"] != skillURI {
		t.Fatalf("skills/get uri = %#v, want %q", gotSkill["uri"], skillURI)
	}

	files, ok := gotSkill["resources"].([]any)
	if !ok {
		t.Fatalf("skills/get resources = %#v, want complete array", gotSkill["resources"])
	}
	assertSkillResourcesReadable(t, server, files)

	resources := server.client.call("resources/list", map[string]any{})
	if resources.Error != nil {
		t.Fatalf("resources/list error = %#v", resources.Error)
	}
	// The legacy handshake used by this stdio fixture predates cacheable
	// resources/list results; the 2026 handshake is covered by server tests.
	assertOperatorConfigResourcesPublished(t, server, resources.Result)
}

func assertSkillResourcesReadable(t *testing.T, server *stdioMCPServer, files []any) {
	t.Helper()
	for _, value := range files {
		file, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("skill resource = %#v, want object", value)
		}
		uri, _ := file["uri"].(string)
		read := server.client.call("resources/read", map[string]any{"uri": uri})
		if read.Error != nil {
			t.Fatalf("resources/read(%q) error = %#v", uri, read.Error)
		}
		contents, ok := read.Result["contents"].([]any)
		if !ok || len(contents) != 1 {
			t.Fatalf("resources/read(%q) contents = %#v, want one item", uri, read.Result["contents"])
		}
		content, ok := contents[0].(map[string]any)
		if !ok {
			t.Fatalf("resources/read(%q) item = %#v, want object", uri, contents[0])
		}
		text, _ := content["text"].(string)
		if text == "" {
			t.Fatalf("resources/read(%q) text is empty or absent", uri)
		}
		if content["uri"] != uri {
			t.Fatalf("resources/read(%q) returned uri %#v", uri, content["uri"])
		}
		digest := sha256.Sum256([]byte(text))
		wantDigest := "sha256:" + hex.EncodeToString(digest[:])
		if file["digest"] != wantDigest {
			t.Fatalf("manifest digest for %q = %#v, content digest = %q", uri, file["digest"], wantDigest)
		}
		if size, ok := file["size"].(float64); !ok || int(size) != len([]byte(text)) {
			t.Fatalf("manifest size for %q = %#v, content bytes = %d", uri, file["size"], len([]byte(text)))
		}
	}

}

// TestMCP2026SkillsDiscovery proves the current protocol discovery request and
// server/discover response advertise Skills, and that the advertised methods
// work through the public stdio server. The 2026 protocol uses per-request
// metadata and does not begin with the legacy initialize handshake.
func TestMCP2026SkillsDiscovery(t *testing.T) {
	server := startFixtureBackedMCPServer(t)
	defer server.cleanup()

	requestMeta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "skills-2026-functional-test", "version": "test"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	discover := server.client.call("server/discover", map[string]any{
		"_meta": requestMeta,
	})
	if discover.Error != nil {
		t.Fatalf("server/discover error = %#v", discover.Error)
	}
	versions, ok := discover.Result["supportedVersions"].([]any)
	if !ok || !containsString(versions, "2026-07-28") {
		t.Fatalf("server/discover supportedVersions = %#v, want 2026-07-28", discover.Result["supportedVersions"])
	}
	discoveryJSON, err := json.Marshal(discover.Result)
	if err != nil {
		t.Fatalf("marshal server/discover result: %v", err)
	}
	if !strings.Contains(string(discoveryJSON), "io.modelcontextprotocol/skills") {
		t.Fatalf("server/discover result = %s, want Skills extension declaration", discoveryJSON)
	}

	listing := server.client.call("skills/list", map[string]any{"_meta": requestMeta})
	if listing.Error != nil {
		t.Fatalf("skills/list error = %#v", listing.Error)
	}
	assertMCPCompleteCacheableResult(t, listing.Result, "skills/list")
	entries, ok := listing.Result["skills"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("skills/list skills = %#v, want non-empty array", listing.Result["skills"])
	}
	var skill map[string]any
	for _, entry := range entries {
		candidate, ok := entry.(map[string]any)
		if ok && candidate["uri"] == "skill://subagent-configuration/SKILL.md" {
			skill = candidate
			break
		}
	}
	if skill == nil {
		t.Fatalf("skills/list omitted subagent configuration skill: %#v", entries)
	}
	get := server.client.call("skills/get", map[string]any{"uri": skill["uri"], "_meta": requestMeta})
	if get.Error != nil {
		t.Fatalf("skills/get error = %#v", get.Error)
	}
	assertMCPCompleteCacheableResult(t, get.Result, "skills/get")
}

func containsString(values []any, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func assertMCPSkillsCapabilities(t *testing.T, initialize map[string]any) {
	t.Helper()
	capabilities, ok := initialize["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("initialize capabilities = %#v, want object", initialize["capabilities"])
	}
	if _, ok := capabilities["resources"]; !ok {
		t.Fatalf("initialize capabilities missing resources: %#v", capabilities)
	}
	extensions, ok := capabilities["extensions"].(map[string]any)
	if !ok {
		t.Fatalf("initialize capabilities.extensions = %#v, want object", capabilities["extensions"])
	}
	if _, ok := extensions["io.modelcontextprotocol/skills"]; !ok {
		t.Fatalf("initialize capabilities missing io.modelcontextprotocol/skills extension: %#v", extensions)
	}
}

func assertMCPCompleteCacheableResult(t *testing.T, result map[string]any, method string) {
	t.Helper()
	if result["resultType"] != "complete" {
		t.Fatalf("%s resultType = %#v, want complete", method, result["resultType"])
	}
	if ttl, ok := result["ttlMs"].(float64); !ok || ttl < 0 {
		t.Fatalf("%s ttlMs = %#v, want non-negative number", method, result["ttlMs"])
	}
	if result["cacheScope"] == nil {
		t.Fatalf("%s cacheScope missing: %#v", method, result)
	}
}

func assertSkillManifest(t *testing.T, skill map[string]any) {
	t.Helper()
	uri, ok := skill["uri"].(string)
	if !ok || !strings.HasSuffix(uri, "/SKILL.md") {
		t.Fatalf("skill uri = %#v, want SKILL.md resource URI", skill["uri"])
	}
	frontmatter, ok := skill["frontmatter"].(map[string]any)
	if !ok || frontmatter["name"] == nil || frontmatter["description"] == nil {
		t.Fatalf("skill frontmatter = %#v, want name and description", skill["frontmatter"])
	}
	files, ok := skill["resources"].([]any)
	if !ok || len(files) == 0 {
		t.Fatalf("skill resources = %#v, want complete non-empty array", skill["resources"])
	}
	containsSkillFile := false
	for _, value := range files {
		file, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("skill resource = %#v, want object", value)
		}
		fileURI, _ := file["uri"].(string)
		digest, _ := file["digest"].(string)
		if fileURI == uri {
			containsSkillFile = true
		}
		if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
			t.Fatalf("skill resource %q digest = %q, want sha256 plus 64 hex characters", fileURI, digest)
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:")); err != nil {
			t.Fatalf("skill resource %q digest = %q, invalid hex: %v", fileURI, digest, err)
		}
		if size, ok := file["size"].(float64); !ok || size < 0 {
			t.Fatalf("skill resource %q size = %#v, want non-negative number", fileURI, file["size"])
		}
	}
	if !containsSkillFile {
		t.Fatalf("skill manifest %q does not include its SKILL.md URI", uri)
	}
}

func assertOperatorConfigResourcesPublished(t *testing.T, server *stdioMCPServer, result map[string]any) {
	t.Helper()
	resources, ok := result["resources"].([]any)
	if !ok {
		t.Fatalf("resources/list resources = %#v, want array", result["resources"])
	}
	var hasSchema, hasCurrentConfig bool
	for _, value := range resources {
		resource, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("resources/list entry = %#v, want object", value)
		}
		uri := strings.ToLower(fmt.Sprint(resource["uri"]))
		if uri == "you://operator/config/schema" {
			hasSchema = true
		}
		if uri == "you://operator/config/current" {
			hasCurrentConfig = true
		}
	}
	if !hasSchema || !hasCurrentConfig {
		t.Fatalf("resources/list missing you://operator/config/schema or you://operator/config/current (schema=%t current=%t): %#v", hasSchema, hasCurrentConfig, resources)
	}
	for _, uri := range []string{"you://operator/config/schema", "you://operator/config/current"} {
		read := server.client.call("resources/read", map[string]any{"uri": uri})
		if read.Error != nil {
			t.Fatalf("resources/read(%q) error = %#v", uri, read.Error)
		}
		contents, ok := read.Result["contents"].([]any)
		if !ok || len(contents) != 1 {
			t.Fatalf("resources/read(%q) contents = %#v, want one item", uri, read.Result["contents"])
		}
		item, ok := contents[0].(map[string]any)
		if !ok {
			t.Fatalf("resources/read(%q) item = %#v, want object", uri, contents[0])
		}
		content, _ := item["text"].(string)
		if content == "" {
			t.Fatalf("resources/read(%q) returned empty content", uri)
		}
		if uri == "you://operator/config/schema" {
			var schema map[string]any
			if err := json.Unmarshal([]byte(content), &schema); err != nil {
				t.Fatalf("decode operator config schema resource: %v", err)
			}
			if schema["type"] != "object" {
				t.Fatalf("operator config schema type = %#v, want object", schema["type"])
			}
		}
	}
}
