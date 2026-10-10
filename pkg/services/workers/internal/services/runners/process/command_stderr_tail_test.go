package process

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCommandFailureLogFields_AppendsBoundedStderrTail(t *testing.T) {
	base := []any{"status", "non_zero_exit"}

	if got := commandFailureLogFields(CommandRequest{}, base, CommandResult{}); len(got) != len(base) {
		t.Fatalf("empty stderr must not add fields, got %v", got)
	}

	stderr := strings.Repeat("é", 5000) + "\nfinal error line\n"
	fields := commandFailureLogFields(CommandRequest{}, base, CommandResult{Stderr: []byte(stderr)})
	if len(fields) != len(base)+2 || fields[len(base)] != "stderr_tail" {
		t.Fatalf("expected stderr_tail field, got %v", fields)
	}
	tail, _ := fields[len(base)+1].(string)
	if len(tail) > commandFailureStderrTailBytes {
		t.Fatalf("tail is %d bytes, want <= %d", len(tail), commandFailureStderrTailBytes)
	}
	if !utf8.ValidString(tail) || !strings.HasSuffix(tail, "final error line") {
		t.Fatalf("tail must be valid UTF-8 ending in the last line, got %q", tail)
	}
}
