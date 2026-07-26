package stdio

import (
	"bufio"
	"context"
	"fmt"
	"io"
)

// ContextLineReader reads a bounded number of lines while allowing callers to
// stop waiting when their invocation context is canceled. The underlying read
// may remain blocked until the process stream closes; process lifecycle owns
// that stream.
type ContextLineReader struct {
	lines <-chan lineResult
}

type lineResult struct {
	value string
	err   error
}

// NewContextLineReader starts a bounded read over an invocation-local stream.
func NewContextLineReader(input io.Reader, maxLines int) (*ContextLineReader, error) {
	if input == nil {
		return nil, fmt.Errorf("line input is required")
	}
	if maxLines <= 0 {
		return nil, fmt.Errorf("line limit must be positive")
	}
	lines := make(chan lineResult, maxLines+1)
	go scanLines(input, maxLines, lines)
	return &ContextLineReader{lines: lines}, nil
}

// ReadLine returns the next line, EOF, a read failure, or context cancellation.
func (reader *ContextLineReader) ReadLine(ctx context.Context) (string, error) {
	if reader == nil {
		return "", fmt.Errorf("context line reader is required")
	}
	if ctx == nil {
		return "", fmt.Errorf("line read context is required")
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case line, ok := <-reader.lines:
		if !ok {
			return "", io.EOF
		}
		return line.value, line.err
	}
}

func scanLines(input io.Reader, maxLines int, lines chan<- lineResult) {
	defer close(lines)
	scanner := bufio.NewScanner(input)
	for range maxLines {
		if !scanner.Scan() {
			break
		}
		lines <- lineResult{value: scanner.Text()}
	}
	if err := scanner.Err(); err != nil {
		lines <- lineResult{err: err}
	}
}
