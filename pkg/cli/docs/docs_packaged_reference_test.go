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
		"browser or dashboard interaction",
		"primaryResult",
		"invocationReturn",
		"avoids binding a localhost API listener",
		"~/.you-agent-factory/factories",
		"@you%2Fgoal",
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
		"## Goal flow topology",
		"multi-stage flow",
		"plan-goal",
		"check-goal",
		"review-goal",
		"structured-review-goal",
		"goal:init",
		"goal:plan",
		"goal:execute",
		"goal:complete",
		"goal:blocked",
		"goal:needs-human",
		"goal:interrupted",
		"goal:failed",
		"## Inspect-first recovery flow",
		"FactorySession",
		"goal-planner/AGENTS.md",
		"structured-review-goal/AGENTS.md",
		"does not add `/goal/inspect`",
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
	for _, stale := range []string{
		"MODEL_WORKER",
		"MODEL_WORKSTATION",
		"Workstation.workPropagation is absent",
		"does not carry invocationReturn",
		"automatic JSON detection",
	} {
		if strings.Contains(got, stale) {
			t.Fatalf("Markdown(packaged-goal) still contains stale wording %q:\n%s", stale, got)
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

func TestMarkdown_PackagedFusionReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("packaged-fusion")
	if err != nil {
		t.Fatalf("Markdown(packaged-fusion) error = %v", err)
	}

	for _, want := range []string{
		"# Packaged Fusion (`@you/fusion`)",
		"you run --named @you/fusion --help",
		"you run --named @you/fusion \"Draft a release summary\"",
		"~/.you-agent-factory/factories",
		"@you%2Ffusion",
		"`invocationSignature`",
		"`FILE` output contract",
		"`medium`",
		"`you docs config`",
		"`you docs sessions`",
		"`you docs authoring-factories`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(packaged-fusion) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs packaged-fusion`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(packaged-fusion) included wrapper text %q:\n%s", wrapper, got)
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
		"INFERENCE_WORKER",
		"INFERENCE_RUN",
		"does not use agent-loop fields",
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
