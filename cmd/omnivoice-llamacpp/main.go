package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	omniVoiceSampleRate = 24000
	omniVoiceChannels   = 1
	omniVoiceBits       = 16
)

type invocationPayload struct {
	Operation  string `json:"operation"`
	OutputFile string `json:"outputFile"`
	Text       string `json:"text"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
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
	if err := writeSynthesizedWAV(outputPath, text); err != nil {
		return err
	}
	_, _ = stdout.Write([]byte("{\"status\":\"ok\"}\n"))
	_, _ = stderr.Write([]byte("generated audio with repo-owned OMNIVOICE runtime companion\n"))
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

func writeSynthesizedWAV(path string, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	samples := synthesizedSamples(text)
	dataSize := len(samples) * 2
	if _, err := file.Write([]byte("RIFF")); err != nil {
		return fmt.Errorf("write wav riff header: %w", err)
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(36+dataSize)); err != nil {
		return fmt.Errorf("write wav size: %w", err)
	}
	if _, err := file.Write([]byte("WAVEfmt ")); err != nil {
		return fmt.Errorf("write wav format header: %w", err)
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(16)); err != nil {
		return fmt.Errorf("write wav fmt chunk size: %w", err)
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return fmt.Errorf("write wav audio format: %w", err)
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(omniVoiceChannels)); err != nil {
		return fmt.Errorf("write wav channels: %w", err)
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(omniVoiceSampleRate)); err != nil {
		return fmt.Errorf("write wav sample rate: %w", err)
	}
	byteRate := omniVoiceSampleRate * omniVoiceChannels * omniVoiceBits / 8
	if err := binary.Write(file, binary.LittleEndian, uint32(byteRate)); err != nil {
		return fmt.Errorf("write wav byte rate: %w", err)
	}
	blockAlign := omniVoiceChannels * omniVoiceBits / 8
	if err := binary.Write(file, binary.LittleEndian, uint16(blockAlign)); err != nil {
		return fmt.Errorf("write wav block align: %w", err)
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(omniVoiceBits)); err != nil {
		return fmt.Errorf("write wav bits per sample: %w", err)
	}
	if _, err := file.Write([]byte("data")); err != nil {
		return fmt.Errorf("write wav data chunk header: %w", err)
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(dataSize)); err != nil {
		return fmt.Errorf("write wav data size: %w", err)
	}
	for _, sample := range samples {
		if err := binary.Write(file, binary.LittleEndian, sample); err != nil {
			return fmt.Errorf("write wav sample data: %w", err)
		}
	}
	return nil
}

func synthesizedSamples(text string) []int16 {
	runes := []rune(text)
	durationSeconds := 1.0 + math.Min(float64(len(runes))/32.0, 2.0)
	totalSamples := int(durationSeconds * omniVoiceSampleRate)
	samples := make([]int16, totalSamples)
	baseFrequency := 220.0 + float64(len(runes)%11)*17.0
	amplitude := 0.28 * float64(math.MaxInt16)
	for i := range samples {
		t := float64(i) / omniVoiceSampleRate
		envelope := 0.85
		if t < 0.03 {
			envelope = t / 0.03
		} else if remaining := durationSeconds - t; remaining < 0.08 {
			envelope = math.Max(remaining/0.08, 0)
		}
		mod := math.Sin(2 * math.Pi * 3.0 * t)
		value := math.Sin(2*math.Pi*baseFrequency*t+0.35*mod) + 0.4*math.Sin(2*math.Pi*baseFrequency*0.5*t)
		samples[i] = int16(amplitude * envelope * value / 1.4)
	}
	return samples
}
