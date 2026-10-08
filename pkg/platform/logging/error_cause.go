package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"syscall"
)

// SafeErrorCause renders recognized diagnostic fields without calling Error on
// arbitrary application errors. Wrappers retain their original identity; this
// is only a presentation view. Joined causes are visited separately so cleanup
// failures do not disappear behind a primary error's diagnostic.
func SafeErrorCause(err error) string {
	return safeErrorCause(err, 0)
}

// Limit traversal even when an external error supplies a cyclic Unwrap chain.
const maximumErrorCauseDepth = 32

func safeErrorCause(err error, depth int) string {
	if err == nil || depth >= maximumErrorCauseDepth {
		return ""
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var causes []string
		for _, cause := range joined.Unwrap() {
			if message := safeErrorCause(cause, depth+1); message != "" && !slices.Contains(causes, message) {
				causes = append(causes, message)
			}
		}
		return strings.Join(causes, "; ")
	}
	message := safeErrorFields(err)
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		cause := safeErrorCause(wrapped.Unwrap(), depth+1)
		if message == "" {
			return cause
		}
		if cause != "" && cause != message {
			return message + ": " + cause
		}
	}
	return message
}

func safeErrorFields(err error) string {
	switch value := err.(type) {
	case interface{ CLIErrorMessage() string }:
		return value.CLIErrorMessage()
	case interface{ InvocationErrorMessage() string }:
		return value.InvocationErrorMessage()
	case *fs.PathError:
		return fmt.Sprintf("%s %q", value.Op, value.Path)
	case *json.SyntaxError:
		return fmt.Sprintf("invalid JSON at byte %d", value.Offset)
	case *json.UnmarshalTypeError:
		// Value and Field may contain recording input; only position is safe.
		return fmt.Sprintf("JSON type mismatch at byte %d", value.Offset)
	case syscall.Errno:
		return value.Error()
	}
	// Match identity, never arbitrary Error text or a wrapper's Is method.
	switch err {
	case context.Canceled, context.DeadlineExceeded, io.EOF, io.ErrUnexpectedEOF,
		fs.ErrPermission, fs.ErrNotExist, fs.ErrExist, fs.ErrInvalid, fs.ErrClosed:
		return err.Error()
	default:
		return ""
	}
}
