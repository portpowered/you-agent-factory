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

func TestContextLineReaderReadsFinalLineWithoutNewline(t *testing.T) {
	reader, err := NewContextLineReader(strings.NewReader("final line"), 1)
	if err != nil {
		t.Fatalf("NewContextLineReader() error = %v", err)
	}

	got, err := reader.ReadLine(context.Background())
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	if got != "final line" {
		t.Fatalf("ReadLine() = %q, want %q", got, "final line")
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

func TestContextLineReaderRejectsInvalidConstructionAndReads(t *testing.T) {
	if _, err := NewContextLineReader(nil, 1); err == nil {
		t.Fatal("NewContextLineReader(nil) error = nil")
	}
	if _, err := NewContextLineReader(strings.NewReader("line"), 0); err == nil {
		t.Fatal("NewContextLineReader(limit=0) error = nil")
	}
	var reader *ContextLineReader
	if _, err := reader.ReadLine(context.Background()); err == nil {
		t.Fatal("nil ReadLine() error = nil")
	}
	valid, err := NewContextLineReader(strings.NewReader("line"), 1)
	if err != nil {
		t.Fatalf("NewContextLineReader() error = %v", err)
	}
	if _, err := valid.ReadLine(nil); err == nil {
		t.Fatal("ReadLine(nil) error = nil")
	}
}
