package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesCanonicalProjectionToStdout(t *testing.T) {
	t.Parallel()

	cfg := commandFixture(t)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := run(cfg, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty", stderr)
	}
	output := stdout.String()
	for _, want := range []string{`"stableId": "cli/you.one"`, `"stableId": "rest/getOne"`, `"stableId": "mcp/mcp.tool.one"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("run() output does not contain %q:\n%s", want, output)
		}
	}
}

func TestRunWritesRequestedFile(t *testing.T) {
	t.Parallel()

	cfg := commandFixture(t)
	cfg.outputPath = filepath.Join(t.TempDir(), "projection.json")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := run(cfg, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout)
	}
	data, err := os.ReadFile(cfg.outputPath)
	if err != nil {
		t.Fatalf("read projection: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) || !strings.Contains(string(data), `"stableId": "rest/getOne"`) {
		t.Fatalf("projection bytes = %q", data)
	}
	if want := "wrote 3 components"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr, want)
	}
}

func TestRunWritesReviewedManifest(t *testing.T) {
	t.Parallel()

	cfg := commandFixture(t)
	cfg.manifest = true
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := run(cfg, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	for _, want := range []string{`"formatVersion": "functional-scenario-manifest/v1"`, `"status": "missing"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("manifest does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func commandFixture(t *testing.T) config {
	t.Helper()
	root := t.TempDir()
	cliPath := filepath.Join(root, "cli.json")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	mcpPath := filepath.Join(root, "mcp.json")
	writeFixture(t, cliPath, `{"commands":{"one":{"id":"you.one","path":"you one","runnable":true}}}`)
	writeFixture(t, openAPIPath, `openapi: 3.0.3
info: {title: test, version: 1.0.0}
paths:
  /one:
    get:
      operationId: getOne
      responses:
        '200': {description: ok}
`)
	writeFixture(t, mcpPath, `{"tools":{"one":{"id":"mcp.tool.one","name":"one"}}}`)
	return config{cliPath: cliPath, openAPIPath: openAPIPath, mcpPath: mcpPath, outputPath: "-"}
}

func writeFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
