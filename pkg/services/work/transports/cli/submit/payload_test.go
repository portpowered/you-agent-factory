package submit

import (
	"errors"
	"io"
	"strings"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

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
