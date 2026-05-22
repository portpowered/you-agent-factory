package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultOmniVoiceTTSCommand = "omnivoice-tts"
	omniVoiceTTSCommandEnv     = "OMNIVOICE_TTS_COMMAND"
	omniVoiceLanguageEnv       = "OMNIVOICE_TTS_LANGUAGE"
	defaultOmniVoiceLanguage   = "English"
)

type invocationPayload struct {
	Operation  string `json:"operation"`
	OutputFile string `json:"outputFile"`
	Text       string `json:"text"`
}

type commandExecutor interface {
	Run(command string, args []string, stdin []byte) ([]byte, []byte, error)
}

type execCommandExecutor struct{}

func (execCommandExecutor) Run(command string, args []string, stdin []byte) ([]byte, []byte, error) {
	cmd := exec.Command(command, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return runWithExecutor(args, stdin, stdout, stderr, execCommandExecutor{})
}

func runWithExecutor(args []string, stdin io.Reader, stdout, stderr io.Writer, executor commandExecutor) error {
	if len(args) == 0 {
		return errors.New("usage: omnivoice-llamacpp invoke --model <path> --tokenizer <path> --output <wav>")
	}
	if args[0] != "invoke" {
		return fmt.Errorf("unsupported subcommand %q", args[0])
	}
	modelPath, tokenizerPath, outputPath, err := parseInvokeArgs(args[1:])
	if err != nil {
		return err
	}
	if err := requireFile(modelPath, "model"); err != nil {
		return err
	}
	if err := requireFile(tokenizerPath, "tokenizer"); err != nil {
		return err
	}
	payload, err := decodeInvocationPayload(stdin)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Operation), "TTS") {
		return fmt.Errorf("unsupported operation %q", payload.Operation)
	}
	text := strings.TrimSpace(payload.Text)
	if text == "" {
		return errors.New("invocation payload requires non-empty text")
	}
	if payloadPath := strings.TrimSpace(payload.OutputFile); payloadPath != "" {
		outputPath = payloadPath
	}
	backendCommand, err := resolveBackendCommand()
	if err != nil {
		return err
	}
	if err := invokeRealRuntime(executor, backendCommand, modelPath, tokenizerPath, outputPath, text, stderr); err != nil {
		return err
	}
	_, _ = stdout.Write([]byte("{\"content\":[{\"type\":\"AUDIO\",\"slot\":\"audio\",\"file\":\"" + jsonEscape(outputPath) + "\",\"contentType\":\"audio/wav\"}]}\n"))
	_, _ = stderr.Write([]byte("generated audio with real OMNIVOICE backend via omnivoice-tts\n"))
	return nil
}

func parseInvokeArgs(args []string) (string, string, string, error) {
	var modelPath string
	var tokenizerPath string
	var outputPath string
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return "", "", "", fmt.Errorf("flag %q requires a value", args[i])
		}
		switch args[i] {
		case "--model":
			modelPath = args[i+1]
		case "--tokenizer":
			tokenizerPath = args[i+1]
		case "--output":
			outputPath = args[i+1]
		default:
			return "", "", "", fmt.Errorf("unsupported flag %q", args[i])
		}
		i++
	}
	if strings.TrimSpace(modelPath) == "" {
		return "", "", "", errors.New("missing required --model path")
	}
	if strings.TrimSpace(tokenizerPath) == "" {
		return "", "", "", errors.New("missing required --tokenizer path")
	}
	if strings.TrimSpace(outputPath) == "" {
		return "", "", "", errors.New("missing required --output path")
	}
	return modelPath, tokenizerPath, outputPath, nil
}

func requireFile(path string, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect %s file %q: %w", label, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s path %q is a directory", label, path)
	}
	return nil
}

func decodeInvocationPayload(stdin io.Reader) (invocationPayload, error) {
	var payload invocationPayload
	data, err := io.ReadAll(stdin)
	if err != nil {
		return payload, fmt.Errorf("read invocation payload: %w", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf("decode invocation payload: %w", err)
	}
	return payload, nil
}

func resolveBackendCommand() (string, error) {
	if command := strings.TrimSpace(os.Getenv(omniVoiceTTSCommandEnv)); command != "" {
		resolved, err := resolveExecutable(command)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", omniVoiceTTSCommandEnv, err)
		}
		return resolved, nil
	}

	if currentExecutable, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(currentExecutable), backendExecutableName())
		if info, statErr := os.Stat(sibling); statErr == nil && !info.IsDir() {
			return sibling, nil
		}
	}

	resolved, err := resolveExecutable(defaultOmniVoiceTTSCommand)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", defaultOmniVoiceTTSCommand, err)
	}
	return resolved, nil
}

func resolveExecutable(command string) (string, error) {
	if strings.ContainsAny(command, `/\`) {
		info, err := os.Stat(command)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("%q is a directory", command)
		}
		return command, nil
	}
	return exec.LookPath(command)
}

func backendExecutableName() string {
	if runtime.GOOS == "windows" {
		return defaultOmniVoiceTTSCommand + ".exe"
	}
	return defaultOmniVoiceTTSCommand
}

func invokeRealRuntime(executor commandExecutor, backendCommand string, modelPath string, tokenizerPath string, outputPath string, text string, stderr io.Writer) error {
	requestStdin := []byte(text)
	args := []string{
		"--model", modelPath,
		"--codec", tokenizerPath,
		"--lang", stringsTrimSpaceOrDefault(os.Getenv(omniVoiceLanguageEnv), defaultOmniVoiceLanguage),
		"-o", outputPath,
	}
	backendStdout, backendStderr, err := executor.Run(backendCommand, args, requestStdin)
	if len(backendStdout) > 0 {
		_, _ = stderr.Write(backendStdout)
		if !bytes.HasSuffix(backendStdout, []byte("\n")) {
			_, _ = stderr.Write([]byte("\n"))
		}
	}
	if len(backendStderr) > 0 {
		_, _ = stderr.Write(backendStderr)
		if !bytes.HasSuffix(backendStderr, []byte("\n")) {
			_, _ = stderr.Write([]byte("\n"))
		}
	}
	if err != nil {
		return fmt.Errorf("run omnivoice-tts backend %q: %w", backendCommand, err)
	}
	return nil
}

func stringsTrimSpaceOrDefault(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func jsonEscape(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return strings.Trim(string(encoded), "\"")
}
