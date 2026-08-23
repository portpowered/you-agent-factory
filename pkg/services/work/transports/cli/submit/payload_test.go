package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

type countingStdinReader struct {
	reader    io.Reader
	bytesRead int
}

func (r *countingStdinReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += n
	return n, err
}

func TestSubmitBatch_LocalPayloadSizeErrorPreservesCLIDiagnostic(t *testing.T) {
	var out bytes.Buffer
	err := SubmitBatch(workdomain.NewFactoryRequestBatchPreparation(), BatchConfig{
		Context: context.Background(),
		Args:    []string{batchJSONWithPayloadSize("batch-payload-too-large", "oversized", workdomain.MaxWorkPayloadBytes+1)},
		DryRun:  true,
		Server:  "http://127.0.0.1:1",
		Output:  &out,
	})
	if err == nil {
		t.Fatal("SubmitBatch dry-run accepted an oversized Work payload")
	}

	var sizeErr *workdomain.PayloadSizeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("error type = %T, want *work.PayloadSizeError in the cause chain", err)
	}
	for _, marker := range []string{
		`Work "oversized"`,
		"payloadBytes=65537",
		"payloadLimitBytes=65536",
	} {
		if !strings.Contains(err.Error(), marker) {
			t.Fatalf("error missing %q: %v", marker, err)
		}
	}
	var coded interface {
		error
		CLIErrorCode() string
		CLIErrorMessage() string
		CLIErrorFamily() factoryapi.ErrorFamily
	}
	if !errors.As(err, &coded) {
		t.Fatalf("error type = %T, want a family-coded CLI diagnostic", err)
	}
	if coded.CLIErrorCode() != string(factoryapi.ErrorResponseCodeBADREQUEST) ||
		coded.CLIErrorFamily() != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("CLI fields = code %q family %q, want BAD_REQUEST", coded.CLIErrorCode(), coded.CLIErrorFamily())
	}
	if !strings.Contains(coded.CLIErrorMessage(), "payloadBytes=65537") {
		t.Fatalf("CLI message = %q, want measured payload bytes", coded.CLIErrorMessage())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want no dry-run success output", out.String())
	}
}

func batchJSONWithPayloadSize(requestID, workName string, payloadBytes int) string {
	const emptyPayload = `{"text":""}`
	textBytes := payloadBytes - len(emptyPayload)
	if textBytes < 0 {
		panic("payload size must fit the test payload shape")
	}
	payload := `{"text":"` + strings.Repeat("x", textBytes) + `"}`
	if len(payload) != payloadBytes {
		panic(fmt.Sprintf("test payload length = %d, want %d", len(payload), payloadBytes))
	}
	return `{"requestId":"` + requestID + `","type":"FACTORY_REQUEST_BATCH","works":[{"name":"` + workName + `","workTypeName":"task","payload":` + payload + `}]}`
}

func TestReadSubmitPayload_StdinDashReadsMarkdownPayload(t *testing.T) {
	read := workdomain.PayloadFileReader(func(string) ([]byte, error) {
		t.Fatal("file reader should not be called for stdin payload")
		return nil, nil
	})

	payload, raw, payloadType, err := readSubmitPayload(
		read,
		"-",
		strings.NewReader("# Review\n\nFrom stdin."),
	)
	if err != nil {
		t.Fatalf("readSubmitPayload(stdin) error = %v", err)
	}
	if payloadType != "markdown" {
		t.Fatalf("payloadType = %q, want markdown", payloadType)
	}
	if string(raw) != "# Review\n\nFrom stdin." {
		t.Fatalf("raw payload = %q", string(raw))
	}
	if string(payload) != `"# Review\n\nFrom stdin."` {
		t.Fatalf("encoded payload = %s", string(payload))
	}
}

func TestReadSubmitPayload_StdinDashRejectsEmptyInput(t *testing.T) {
	read := workdomain.PayloadFileReader(func(string) ([]byte, error) {
		t.Fatal("file reader should not be called for empty stdin payload")
		return nil, nil
	})

	_, _, _, err := readSubmitPayload(read, "-", strings.NewReader("\n"))
	if err == nil || !strings.Contains(err.Error(), "stdin input is empty") {
		t.Fatalf("readSubmitPayload(empty stdin) error = %v, want empty stdin failure", err)
	}
}

func TestReadSubmitPayload_StdinDashReadsJSONPayload(t *testing.T) {
	read := workdomain.PayloadFileReader(func(string) ([]byte, error) {
		t.Fatal("file reader should not be called for stdin JSON payload")
		return nil, nil
	})

	rawJSON := `{"title":"stdin task","prompt":"from pipe"}`
	payload, raw, payloadType, err := readSubmitPayload(read, "-", strings.NewReader(rawJSON))
	if err != nil {
		t.Fatalf("readSubmitPayload(stdin json) error = %v", err)
	}
	if payloadType != "json" {
		t.Fatalf("payloadType = %q, want json", payloadType)
	}
	if string(raw) != rawJSON {
		t.Fatalf("raw payload = %q", string(raw))
	}
	if string(payload) != rawJSON {
		t.Fatalf("encoded payload = %s, want raw JSON passthrough", string(payload))
	}
}

func TestReadSubmitPayload_FileJSONRejectsInvalidContent(t *testing.T) {
	read := workdomain.PayloadFileReader(func(string) ([]byte, error) {
		return []byte(`{"title":`), nil
	})

	_, _, _, err := readSubmitPayload(read, "work.json", strings.NewReader("ignored"))
	if err == nil || !strings.Contains(err.Error(), "payload file is not valid JSON: work.json") {
		t.Fatalf("readSubmitPayload(invalid file json) error = %v, want invalid JSON failure", err)
	}
}

func TestReadSubmitPayload_RejectsNilReader(t *testing.T) {
	_, _, _, err := readSubmitPayload(nil, "-", strings.NewReader("payload"))
	if err == nil || !strings.Contains(err.Error(), "payload file reader is required") {
		t.Fatalf("readSubmitPayload(nil reader) error = %v, want reader required failure", err)
	}
}

func TestReadSubmitStdinPayload_RejectsNilStdin(t *testing.T) {
	_, err := readSubmitStdinPayload(nil)
	if err == nil || !strings.Contains(err.Error(), "process stdin reader is required") {
		t.Fatalf("readSubmitStdinPayload(nil) error = %v, want stdin reader required failure", err)
	}
}

func TestReadSubmitStdinPayload_PropagatesReadError(t *testing.T) {
	readErr := errors.New("stdin read interrupted")
	_, err := readSubmitStdinPayload(errReader{err: readErr})
	if err == nil || !strings.Contains(err.Error(), "read payload stdin") {
		t.Fatalf("readSubmitStdinPayload(read error) error = %v, want wrapped read failure", err)
	}
	if !errors.Is(err, readErr) {
		t.Fatalf("readSubmitStdinPayload(read error) = %v, want wrapped %v", err, readErr)
	}
}

func TestReadSubmitStdinPayloadAcceptsInclusiveByteLimit(t *testing.T) {
	t.Parallel()

	const emptyJSONPayload = `{"text":""}`
	rawJSON := `{"text":"` + strings.Repeat("x", maxSubmitPayloadStdinBytes-len(emptyJSONPayload)) + `"}`
	reader := &countingStdinReader{
		reader: bytes.NewReader([]byte(rawJSON)),
	}
	_, data, payloadType, err := readSubmitPayload(
		func(string) ([]byte, error) { t.Fatal("file reader called for stdin"); return nil, nil },
		"-",
		reader,
	)
	if err != nil {
		t.Fatalf("readSubmitPayload(exact limit): %v", err)
	}
	if payloadType != "json" {
		t.Fatalf("payload type = %q, want json", payloadType)
	}
	if len(data) != maxSubmitPayloadStdinBytes || !json.Valid(data) {
		t.Fatalf("payload bytes = %d, want %d", len(data), maxSubmitPayloadStdinBytes)
	}
	if reader.bytesRead > maxSubmitPayloadStdinBytes+1 {
		t.Fatalf("bytes read = %d, want <= %d", reader.bytesRead, maxSubmitPayloadStdinBytes+1)
	}
}

func TestReadSubmitPayload_StdinTextAcceptsExactCompactPayloadLimit(t *testing.T) {
	t.Parallel()

	// json.Marshal escapes the newline, so this source is smaller than the
	// source cap while its compact JSON representation is exactly the Work
	// admission limit.
	rawText := strings.Repeat("x", maxSubmitPayloadStdinBytes-4) + "\n"
	reader := &countingStdinReader{reader: strings.NewReader(rawText)}
	payload, raw, payloadType, err := readSubmitPayload(
		func(string) ([]byte, error) { t.Fatal("file reader called for stdin"); return nil, nil },
		"-",
		reader,
	)
	if err != nil {
		t.Fatalf("readSubmitPayload(exact encoded text limit): %v", err)
	}
	if payloadType != "markdown" {
		t.Fatalf("payload type = %q, want markdown", payloadType)
	}
	if string(raw) != rawText {
		t.Fatalf("raw payload changed: got %q", raw)
	}
	if len(payload) != maxSubmitPayloadStdinBytes || !json.Valid(payload) {
		t.Fatalf("encoded payload bytes = %d, want valid JSON at %d bytes", len(payload), maxSubmitPayloadStdinBytes)
	}
	if reader.bytesRead != len(rawText) {
		t.Fatalf("bytes read = %d, want %d", reader.bytesRead, len(rawText))
	}
}

func TestReadSubmitPayload_StdinTextRejectsEscapedOverLimitPayload(t *testing.T) {
	t.Parallel()

	rawText := strings.Repeat("\"", maxSubmitPayloadStdinBytes/2)
	reader := &countingStdinReader{reader: strings.NewReader(rawText)}
	_, _, _, err := readSubmitPayload(
		func(string) ([]byte, error) { t.Fatal("file reader called for stdin"); return nil, nil },
		"-",
		reader,
	)
	if err == nil {
		t.Fatal("readSubmitPayload(escaped over-limit text) succeeded")
	}
	for _, want := range []string{
		"payload stdin compact JSON exceeds the 65536-byte Work payload limit",
		"use a payload file for larger input",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
	if reader.bytesRead != len(rawText) {
		t.Fatalf("bytes read = %d, want %d source bytes", reader.bytesRead, len(rawText))
	}
}

func TestReadSubmitStdinPayloadRejectsOverflowAfterOneSentinelByte(t *testing.T) {
	t.Parallel()

	reader := &countingStdinReader{
		reader: bytes.NewReader(bytes.Repeat([]byte("x"), maxSubmitPayloadStdinBytes+1)),
	}
	data, err := readSubmitStdinPayload(reader)
	if err == nil {
		t.Fatal("readSubmitStdinPayload(overflow): want limit error")
	}
	if data != nil {
		t.Fatalf("overflow data retained = %d bytes, want nil", len(data))
	}
	for _, want := range []string{
		fmt.Sprintf("payload stdin exceeds the %d-byte limit", maxSubmitPayloadStdinBytes),
		"use a payload file for larger input",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
	if reader.bytesRead != maxSubmitPayloadStdinBytes+1 {
		t.Fatalf("bytes read = %d, want exactly %d", reader.bytesRead, maxSubmitPayloadStdinBytes+1)
	}
}

func TestClassifySubmitPayloadBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "json object", raw: `{"title":"task"}`, want: "json"},
		{name: "markdown prose", raw: "# Heading\n\nBody", want: "markdown"},
		{name: "json array is markdown", raw: `["not","object-root"]`, want: "markdown"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifySubmitPayloadBytes([]byte(tc.raw)); got != tc.want {
				t.Fatalf("classifySubmitPayloadBytes(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestReadSubmitPayload_StdinDashTrimsWhitespacePath(t *testing.T) {
	read := workdomain.PayloadFileReader(func(string) ([]byte, error) {
		t.Fatal("file reader should not be called for whitespace stdin path")
		return nil, nil
	})

	_, _, payloadType, err := readSubmitPayload(read, "  -  ", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("readSubmitPayload(trimmed stdin path) error = %v", err)
	}
	if payloadType != "markdown" {
		t.Fatalf("payloadType = %q, want markdown", payloadType)
	}
}

func TestReadSubmitPayload_FileReadError(t *testing.T) {
	readErr := errors.New("payload file missing")
	read := workdomain.PayloadFileReader(func(string) ([]byte, error) {
		return nil, readErr
	})

	_, _, _, err := readSubmitPayload(read, "missing.md", io.NopCloser(strings.NewReader("ignored")))
	if err == nil || !errors.Is(err, readErr) {
		t.Fatalf("readSubmitPayload(file error) error = %v, want %v", err, readErr)
	}
}
