package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type commandResult struct {
	stdout string
	stderr string
	err    string
}

func TestRunCrossAdapterFixtureHasStableMergedOutput(t *testing.T) {
	standardPath, termsPath := writeCommandFixtures(t,
		[]byte("# Guide\n\nThe service doesn't stop; now.\n"),
		[]byte(cliManifestFixture),
	)

	first := runCommandFixture(t, standardPath, termsPath, "docs/guide.md", "contracts/cli/commands.json")
	second := runCommandFixture(t, standardPath, termsPath, "contracts/cli/commands.json", "docs/guide.md")
	if first != second {
		t.Fatalf("input enumeration order changed command result:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.err == "" || !strings.Contains(first.err, "found 6 blocking finding") {
		t.Fatalf("mixed fixture error = %q, want six blocking findings", first.err)
	}
	if first.stdout != "" {
		t.Fatalf("mixed fixture stdout = %q, want empty stdout", first.stdout)
	}

	standard, err := os.ReadFile(standardPath)
	if err != nil {
		t.Fatalf("read writing standard: %v", err)
	}
	terms, err := os.ReadFile(termsPath)
	if err != nil {
		t.Fatalf("read terminology register: %v", err)
	}
	policy, err := LoadPolicy(standard, terms)
	if err != nil {
		t.Fatalf("LoadPolicy(): %v", err)
	}
	manifest, err := os.ReadFile("contracts/cli/commands.json")
	if err != nil {
		t.Fatalf("read CLI fixture: %v", err)
	}
	markdown, err := os.ReadFile("docs/guide.md")
	if err != nil {
		t.Fatalf("read Markdown fixture: %v", err)
	}
	expectedFindings := append(AnalyzeCLIManifest("contracts/cli/commands.json", manifest, policy), AnalyzeMarkdown("docs/guide.md", markdown, policy)...)
	var expected bytes.Buffer
	if err := RenderFindings(&expected, expectedFindings); err != nil {
		t.Fatalf("RenderFindings(expected): %v", err)
	}
	if first.stderr != expected.String() {
		t.Fatalf("mixed fixture stderr differs from exact merged findings:\nwant=%q\ngot=%q", expected.String(), first.stderr)
	}

	lines := strings.Split(strings.TrimSuffix(first.stderr, "\n"), "\n")
	wantOrder := []string{
		"contracts/cli/commands.json:17:",
		"contracts/cli/commands.json:26:",
		"contracts/cli/commands.json:32:",
		"contracts/cli/commands.json:66:",
		"docs/guide.md:3:13 [B-CONTRACTION]",
		"docs/guide.md:3:25 [B-SEMICOLON]",
	}
	if len(lines) != len(wantOrder) {
		t.Fatalf("mixed fixture rendered %d lines, want %d: %q", len(lines), len(wantOrder), first.stderr)
	}
	for index, prefix := range wantOrder {
		if !strings.HasPrefix(lines[index], prefix) {
			t.Fatalf("rendered finding %d = %q, want prefix %q", index, lines[index], prefix)
		}
	}
}

func TestRunValidCrossAdapterFixtureIsStableReadOnlyAndOffline(t *testing.T) {
	validManifest := strings.ReplaceAll(cliManifestFixture, "The Factory doesn't stop.", "The Factory is ready at https://example.com/api.")
	validManifest = strings.ReplaceAll(validManifest, "Use --server; continue.", "Use --server and continue.")
	validManifest = strings.ReplaceAll(validManifest, "Use the old command; replace it.", "Use the old command and replace it.")
	standardPath, termsPath := writeCommandFixtures(t,
		[]byte("# Guide\n\nUse `CPN; don't` at https://example.com/api.\n"),
		[]byte(validManifest),
	)

	manifestBefore, manifestInfoBefore := snapshotCommandFixture(t, "contracts/cli/commands.json")
	markdownBefore, markdownInfoBefore := snapshotCommandFixture(t, "docs/guide.md")
	previousTransport := http.DefaultTransport
	http.DefaultTransport = commandFailingTransport{}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	first := runCommandFixture(t, standardPath, termsPath, "contracts/cli/commands.json", "docs/guide.md")
	second := runCommandFixture(t, standardPath, termsPath, "contracts/cli/commands.json", "docs/guide.md")
	if first != second {
		t.Fatalf("repeated valid runs changed command result:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.err != "" || first.stdout != "prosecheck passed (2 input(s))\n" || first.stderr != "" {
		t.Fatalf("valid fixture result = %#v, want success with stable stdout", first)
	}

	manifestAfter, manifestInfoAfter := snapshotCommandFixture(t, "contracts/cli/commands.json")
	markdownAfter, markdownInfoAfter := snapshotCommandFixture(t, "docs/guide.md")
	if !bytes.Equal(manifestBefore, manifestAfter) || !bytes.Equal(markdownBefore, markdownAfter) {
		t.Fatal("valid command run changed fixture bytes")
	}
	if manifestInfoBefore != manifestInfoAfter || markdownInfoBefore != markdownInfoAfter {
		t.Fatalf("valid command run changed fixture metadata:\nbefore=%#v\nafter=%#v", []commandFixtureMetadata{manifestInfoBefore, markdownInfoBefore}, []commandFixtureMetadata{manifestInfoAfter, markdownInfoAfter})
	}
}

func writeCommandFixtures(t *testing.T, markdown, manifest []byte) (string, string) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Dir("docs/guide.md"), 0o700); err != nil {
		t.Fatalf("mkdir Markdown fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir("contracts/cli/commands.json"), 0o700); err != nil {
		t.Fatalf("mkdir CLI fixture: %v", err)
	}
	if err := os.WriteFile("docs/guide.md", markdown, 0o600); err != nil {
		t.Fatalf("write Markdown fixture: %v", err)
	}
	if err := os.WriteFile("contracts/cli/commands.json", manifest, 0o600); err != nil {
		t.Fatalf("write CLI fixture: %v", err)
	}
	standardPath, termsPath := repositoryPolicyPaths(t)
	return standardPath, termsPath
}

func runCommandFixture(t *testing.T, standardPath, termsPath string, inputs ...string) commandResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(append([]string{"-standard", standardPath, "-terms", termsPath}, inputs...), &stdout, &stderr)
	result := commandResult{stdout: stdout.String(), stderr: stderr.String()}
	if err != nil {
		result.err = err.Error()
	}
	return result
}

type commandFixtureMetadata struct {
	name    string
	mode    os.FileMode
	size    int64
	modTime time.Time
	isDir   bool
}

func snapshotCommandFixture(t *testing.T, path string) ([]byte, commandFixtureMetadata) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat fixture %s: %v", path, err)
	}
	return data, commandFixtureMetadata{
		name:    info.Name(),
		mode:    info.Mode(),
		size:    info.Size(),
		modTime: info.ModTime(),
		isDir:   info.IsDir(),
	}
}

type commandFailingTransport struct{}

func (commandFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected network access from prosecheck")
}
