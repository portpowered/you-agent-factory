package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsMarkdownFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(filepath.Join(docsDir, "nested"), 0o755); err != nil {
		t.Fatalf("create nested docs directory: %v", err)
	}

	writeMarkdownTestFile(t, filepath.Join(root, "README.md"), []byte("# Readme\n"))
	writeMarkdownTestFile(t, filepath.Join(docsDir, "guide.md"), []byte("# Guide\n\n```go\nfmt.Println()\n```\n"))
	writeMarkdownTestFile(t, filepath.Join(docsDir, "nested", "notes.MD"), []byte("# Notes\n"))
	writeMarkdownTestFile(t, filepath.Join(docsDir, "ignored.txt"), []byte("not markdown and no newline"))

	if err := run([]string{filepath.Join(root, "README.md"), docsDir}); err != nil {
		t.Fatalf("lint valid file and directory: %v", err)
	}
}

func TestRunRejectsMalformedMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{name: "invalid UTF-8", content: []byte{0xff, '\n'}, want: "content is not valid UTF-8"},
		{name: "missing final newline", content: []byte("# Heading"), want: "missing final newline"},
		{name: "unclosed backtick fence", content: []byte("```go\ncontent\n"), want: "unclosed ``` code fence"},
		{name: "unclosed tilde fence", content: []byte("~~~text\ncontent\n"), want: "unclosed ~~~ code fence"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docsDir := t.TempDir()
			path := filepath.Join(docsDir, "invalid.md")
			writeMarkdownTestFile(t, path, tt.content)

			err := run([]string{docsDir})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("run error = %v, want failure containing %q", err, tt.want)
			}
		})
	}
}

func TestRunReportsInputFailures(t *testing.T) {
	if err := run(nil); err == nil || err.Error() != "usage: markdown-linter <file-or-directory> [...]" {
		t.Fatalf("run without paths error = %v, want usage error", err)
	}

	missing := filepath.Join(t.TempDir(), "missing.md")
	if err := run([]string{missing}); err == nil || !strings.Contains(err.Error(), "inspect "+missing) {
		t.Fatalf("run missing path error = %v, want inspect failure", err)
	}

	failures := lintFile(missing)
	if len(failures) != 1 || !strings.Contains(failures[0], missing+": read:") {
		t.Fatalf("lint missing file failures = %v, want read failure", failures)
	}
}

func TestRunReportsScannerFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized-line.md")
	content := []byte(strings.Repeat("x", bufio.MaxScanTokenSize+1) + "\n")
	writeMarkdownTestFile(t, path, content)

	err := run([]string{path})
	if err == nil || !strings.Contains(err.Error(), ": scan: bufio.Scanner:") {
		t.Fatalf("run oversized line error = %v, want scanner failure", err)
	}
}

func writeMarkdownTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write markdown test file %s: %v", path, err)
	}
}
