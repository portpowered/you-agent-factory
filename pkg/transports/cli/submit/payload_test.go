package submit

import (
	"strings"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

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
