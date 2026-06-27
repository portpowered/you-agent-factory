package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestMarkdown_PackagedGoalReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("packaged-goal")
	if err != nil {
		t.Fatalf("Markdown(packaged-goal) error = %v", err)
	}

	for _, want := range []string{
		"# Packaged Goal (`@you/goal`)",
		"you run --named @you/goal",
		"~/.you-agent-factory/factories",
		"@you%2Fgoal",
		"MODEL_WORKER",
		"MODEL_WORKSTATION",
		"goal-executor",
		"execute-goal",
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"INVOCATION_BLOCKED",
		"INVOCATION_NEEDS_HUMAN",
		"INVOCATION_PAUSED",
		"INVOCATION_INTERRUPTED",
		"INVOCATION_RUNTIME_FAILURE",
		"INVOCATION_TIMED_OUT",
		"INVOCATION_PRIMARY_RESULT_UNRESOLVED",
		"sessionId",
		"workId",
		"workState",
		"you session resume <session-id>",
		"make docs-reference-smoke",
		"editable",
		"`you docs authoring-factories`",
		"`you docs config`",
		"`you docs sessions`",
		"`you docs workstations`",
		"`you docs workers`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(packaged-goal) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs packaged-goal`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(packaged-goal) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_PackagedTTSReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("packaged-tts")
	if err != nil {
		t.Fatalf("Markdown(packaged-tts) error = %v", err)
	}

	for _, want := range []string{
		"# Packaged TTS (`@you/tts`)",
		"you run --named @you/tts",
		"~/.you-agent-factory/factories",
		"@you%2Ftts",
		"artifactPath",
		"mediaType",
		"backend",
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"editable",
		"raw audio",
		"shared invocation contract",
		"INVOCATION_TTS_MODEL_NOT_READY",
		"INVOCATION_TTS_GENERATION_FAILED",
		"`you docs authoring-factories`",
		"`you docs config`",
		"`you docs sessions`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(packaged-tts) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs packaged-tts`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(packaged-tts) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_PackagedReferenceTopicsHaveNoPackagedTopicMarkdownLinks(t *testing.T) {
	t.Parallel()

	packagedTopicMD := regexp.MustCompile(`\[[^\]]+\]\((?:\./|\.\./reference/)?([a-z0-9-]+)\.md(?:#[^)]*)?\)`)
	exempt := map[string]bool{"README": true}

	repoRoot := testutil.MustRepoRoot(t)
	referenceDir := filepath.Join(repoRoot, "docs", "reference")
	entries, err := os.ReadDir(referenceDir)
	if err != nil {
		t.Fatalf("ReadDir(docs/reference) error = %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "README.md" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(referenceDir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), err)
		}
		doc := string(content)
		for _, match := range packagedTopicMD.FindAllString(doc, -1) {
			stem := packagedTopicMD.FindStringSubmatch(match)[1]
			if exempt[stem] {
				continue
			}
			t.Fatalf("%s contains packaged-topic markdown link %q; use `you docs %s` instead", entry.Name(), match, stem)
		}
		if strings.Contains(doc, "you docs authoring-agents-md") {
			t.Fatalf("%s references authoring-agents-md as a docs topic; use docs/reference/authoring-agents-md.md path instead", entry.Name())
		}
	}
}
