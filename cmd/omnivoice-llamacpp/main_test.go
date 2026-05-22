package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fakeCommandExecutor struct {
	stdout []byte
	stderr []byte
	err    error
	run    func(command string, args []string, stdin []byte)
}

func (f fakeCommandExecutor) Run(command string, args []string, stdin []byte) ([]byte, []byte, error) {
	if f.run != nil {
		f.run(command, args, stdin)
	}
	return f.stdout, f.stderr, f.err
}

func TestRunInvokeCallsRealBackendAndReturnsAudioContent(t *testing.T) {
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
	var gotCommand string
	var gotArgs []string
	var gotStdin []byte
	executor := fakeCommandExecutor{
		run: func(command string, args []string, stdin []byte) {
			gotCommand = command
			gotArgs = append([]string(nil), args...)
			gotStdin = append([]byte(nil), stdin...)
			if err := os.WriteFile(outputPath, []byte("RIFFstubWAVEpayload0000000000000000000000000000000000000000"), 0o644); err != nil {
				t.Fatalf("write output wav: %v", err)
			}
		},
		stderr: []byte("real backend diagnostics"),
	}
	t.Setenv(omniVoiceTTSCommandEnv, filepath.Join(dir, backendExecutableName()))
	if err := os.WriteFile(os.Getenv(omniVoiceTTSCommandEnv), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write fake backend binary: %v", err)
	}

	err := runWithExecutor(
		[]string{"invoke", "--model", modelPath, "--tokenizer", tokenizerPath, "--output", outputPath},
		bytes.NewBufferString(`{"operation":"TTS","text":"hello world"}`),
		stdout,
		stderr,
		executor,
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
	if !bytes.Contains(stdout.Bytes(), []byte(`"type":"AUDIO"`)) || !bytes.Contains(stdout.Bytes(), []byte(outputPath)) {
		t.Fatalf("stdout = %q, want audio content payload", stdout.String())
	}
	if gotCommand != os.Getenv(omniVoiceTTSCommandEnv) {
		t.Fatalf("backend command = %q, want %q", gotCommand, os.Getenv(omniVoiceTTSCommandEnv))
	}
	if want := []string{"--model", modelPath, "--codec", tokenizerPath, "--lang", defaultOmniVoiceLanguage, "-o", outputPath}; strings.Join(gotArgs, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("backend args = %#v, want %#v", gotArgs, want)
	}
	if string(gotStdin) != "hello world" {
		t.Fatalf("backend stdin = %q, want %q", string(gotStdin), "hello world")
	}
	if !strings.Contains(stderr.String(), "real backend diagnostics") {
		t.Fatalf("stderr = %q, want backend diagnostics", stderr.String())
	}
}

func TestParseInvokeArgsRejectsUnknownFlag(t *testing.T) {
	if _, _, _, err := parseInvokeArgs([]string{"--bad", "value"}); err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func TestResolveBackendCommandFallsBackToPATH(t *testing.T) {
	dir := t.TempDir()
	commandPath := buildFakeOmniVoiceTTS(t, dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(omniVoiceTTSCommandEnv, "")

	got, err := resolveBackendCommand()
	if err != nil {
		t.Fatalf("resolveBackendCommand: %v", err)
	}
	if got != commandPath {
		t.Fatalf("backend command = %q, want %q", got, commandPath)
	}
}

func TestRunInvokeReturnsBackendFailure(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	tokenizerPath := filepath.Join(dir, "tokenizer.gguf")
	for _, path := range []string{modelPath, tokenizerPath} {
		if err := os.WriteFile(path, []byte("stub"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	t.Setenv(omniVoiceTTSCommandEnv, filepath.Join(dir, backendExecutableName()))
	if err := os.WriteFile(os.Getenv(omniVoiceTTSCommandEnv), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write fake backend binary: %v", err)
	}
	err := runWithExecutor(
		[]string{"invoke", "--model", modelPath, "--tokenizer", tokenizerPath, "--output", filepath.Join(dir, "out.wav")},
		bytes.NewBufferString(`{"operation":"TTS","text":"hello world"}`),
		&bytes.Buffer{},
		&bytes.Buffer{},
		fakeCommandExecutor{err: errors.New("boom")},
	)
	if err == nil || !strings.Contains(err.Error(), "run omnivoice-tts backend") {
		t.Fatalf("run error = %v, want backend failure", err)
	}
}

func buildFakeOmniVoiceTTS(t *testing.T, dir string) string {
	t.Helper()
	programPath := filepath.Join(dir, "main.go")
	binaryPath := filepath.Join(dir, backendExecutableName())
	source := `package main
import (
	"os"
)
func main() {
	args := os.Args[1:]
	output := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" && i+1 < len(args) {
			output = args[i+1]
			i++
		}
	}
	if output == "" {
		panic("missing -o")
	}
	if err := os.WriteFile(output, []byte("RIFFstubWAVEpayload0000000000000000000000000000000000000000"), 0o644); err != nil {
		panic(err)
	}
}
`
	if err := os.WriteFile(programPath, []byte(source), 0o644); err != nil {
		t.Fatalf("write backend source: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", binaryPath, programPath)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake omnivoice backend: %v\n%s", err, string(output))
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0o755); err != nil {
			t.Fatalf("chmod backend binary: %v", err)
		}
	}
	return binaryPath
}
