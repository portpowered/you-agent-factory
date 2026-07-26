package stdio

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestContextLineReaderReadsBoundedLines(t *testing.T) {
	reader, err := NewContextLineReader(strings.NewReader("one\ntwo\nthree\n"), 2)
	if err != nil {
		t.Fatalf("NewContextLineReader() error = %v", err)
	}
	for _, want := range []string{"one", "two"} {
		got, readErr := reader.ReadLine(context.Background())
		if readErr != nil {
			t.Fatalf("ReadLine() error = %v", readErr)
		}
		if got != want {
			t.Fatalf("ReadLine() = %q, want %q", got, want)
		}
	}
	if _, readErr := reader.ReadLine(context.Background()); !errors.Is(readErr, io.EOF) {
		t.Fatalf("ReadLine() terminal error = %v, want EOF", readErr)
	}
}

func TestContextLineReaderStopsWaitingOnCancellation(t *testing.T) {
	input, output := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = output.Close()
	})
	reader, err := NewContextLineReader(input, 1)
	if err != nil {
		t.Fatalf("NewContextLineReader() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, readErr := reader.ReadLine(ctx); !errors.Is(readErr, context.Canceled) {
		t.Fatalf("ReadLine() error = %v, want context cancellation", readErr)
	}
}
