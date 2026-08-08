package prompting

import (
	"errors"
	"strings"
	"testing"
)

type promptSourceFileSystem struct {
	data []byte
	err  error
}

func (f promptSourceFileSystem) ReadFile(string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]byte(nil), f.data...), nil
}

func TestResolveAuthoredPromptSourceReadsCurrentBodyAndTemplate(t *testing.T) {
	const path = "factory/workers/worker/AGENTS.md"
	body, err := ResolveAuthoredPromptSource(promptSourceFileSystem{data: []byte("---\ntype: MODEL\n---\nold body")}, path, true)
	if err != nil {
		t.Fatalf("ResolveAuthoredPromptSource(body) error = %v", err)
	}
	if body != "old body" {
		t.Fatalf("body = %q, want old body", body)
	}

	template, err := ResolveAuthoredPromptSource(
		promptSourceFileSystem{data: []byte("template with frontmatter-looking text")},
		"factory/workstations/review/prompt.md",
		false,
	)
	if err != nil {
		t.Fatalf("ResolveAuthoredPromptSource(template) error = %v", err)
	}
	if template != "template with frontmatter-looking text" {
		t.Fatalf("template = %q", template)
	}
}

func TestResolveAuthoredPromptSourceFailsWithoutStaleFallback(t *testing.T) {
	const path = "factory/workers/worker/AGENTS.md"
	_, err := ResolveAuthoredPromptSource(
		promptSourceFileSystem{err: errors.New("permission denied")},
		path,
		true,
	)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error = %v, want source path and filesystem error", err)
	}
}

func TestResolveAuthoredPromptSourceRejectsMalformedBodyFrontmatter(t *testing.T) {
	_, err := ResolveAuthoredPromptSource(
		promptSourceFileSystem{data: []byte("---\ntype: MODEL\nbody without close")},
		"factory/workers/worker/AGENTS.md",
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "missing closing frontmatter delimiter") {
		t.Fatalf("error = %v, want malformed frontmatter failure", err)
	}
}
