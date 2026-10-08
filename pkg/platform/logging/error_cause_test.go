package logging

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestSafeErrorCauseRetainsJoinedFileFailuresWithoutPayloadText(t *testing.T) {
	t.Parallel()
	primary := &fs.PathError{Op: "read recording", Path: "current-board.json", Err: fs.ErrPermission}
	cleanup := &fs.PathError{Op: "close recording", Path: "successor.json", Err: fs.ErrClosed}
	err := errors.Join(fmt.Errorf("PRIVATE wrapper: %w", primary), cleanup,
		errors.New("PRIVATE payload and token"))
	got := SafeErrorCause(err)
	for _, want := range []string{`read recording "current-board.json": permission denied`,
		`close recording "successor.json": file already closed`} {
		if !strings.Contains(got, want) {
			t.Fatalf("safe cause %q omits %q", got, want)
		}
	}
	if strings.Contains(got, "PRIVATE") {
		t.Fatalf("safe cause exposed arbitrary error text: %q", got)
	}
	if !errors.Is(err, primary) || !errors.Is(err, cleanup) {
		t.Fatal("diagnostic changed cause identity")
	}
}

func TestSafeErrorCauseReportsJSONPositionWithoutDecoderExcerpts(t *testing.T) {
	t.Parallel()
	err := &json.UnmarshalTypeError{Value: "PRIVATE", Field: "PRIVATE", Offset: 42}
	if got := SafeErrorCause(err); got != "JSON type mismatch at byte 42" {
		t.Fatalf("safe cause = %q", got)
	}
	var value any
	errSyntax := json.Unmarshal([]byte(`{"PRIVATE": !}`), &value)
	if got := SafeErrorCause(errSyntax); !strings.HasPrefix(got, "invalid JSON at byte ") || strings.Contains(got, "PRIVATE") {
		t.Fatalf("syntax diagnostic = %q", got)
	}
}

type cyclicDiagnosticError struct{}

func (err *cyclicDiagnosticError) Error() string { return "PRIVATE" }
func (err *cyclicDiagnosticError) Unwrap() error { return err }

func TestSafeErrorCauseBoundsUnknownAndCyclicErrors(t *testing.T) {
	t.Parallel()
	for _, err := range []error{nil, errors.New("PRIVATE"), &cyclicDiagnosticError{}} {
		if got := SafeErrorCause(err); got != "" {
			t.Fatalf("unrecognized error = %q", got)
		}
	}
}
