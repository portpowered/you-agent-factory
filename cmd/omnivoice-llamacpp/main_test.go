package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunInvokeWritesWAV(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	tokenizerPath := filepath.Join(dir, "tokenizer.gguf")
	outputPath := filepath.Join(dir, "out.wav")
	for _, path := range []string{modelPath, tokenizerPath} {
		if err := os.WriteFile(path, []byte("stub"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(
		[]string{"invoke", "--model", modelPath, "--tokenizer", tokenizerPath, "--output", outputPath},
		bytes.NewBufferString(`{"operation":"TTS","text":"hello world"}`),
		stdout,
		stderr,
	)
	if err != nil {
		t.Fatalf("run invoke: %v", err)
	}
	audio, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output wav: %v", err)
	}
	if len(audio) <= 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		t.Fatalf("wav header = %q / %q, want RIFF/WAVE with body", string(audio[:4]), string(audio[8:12]))
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status":"ok"`)) {
		t.Fatalf("stdout = %q, want status payload", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Fatalf("stderr should include runtime diagnostics")
	}
}

func TestParseInvokeArgsRejectsUnknownFlag(t *testing.T) {
	if _, _, _, err := parseInvokeArgs([]string{"--bad", "value"}); err == nil {
		t.Fatal("expected unknown flag error")
	}
}
