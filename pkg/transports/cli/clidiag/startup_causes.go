package clidiag

import (
	"errors"
	"fmt"
	"io"
	"reflect"
)

// CausesRendered includes cause output emitted through a nested writer.
func (writer *DiagnosticWriter) CausesRendered() bool {
	return writer != nil && (writer.causesRendered || causesRendered(writer.output))
}

// MarkCausesRendered propagates cause ownership to the outer process boundary.
func (writer *DiagnosticWriter) MarkCausesRendered() {
	if writer != nil {
		writer.causesRendered = true
		markCausesRendered(writer.output)
	}
}

func causesRendered(output io.Writer) bool {
	marker, ok := output.(interface{ CausesRendered() bool })
	return ok && marker.CausesRendered()
}

func markCausesRendered(output io.Writer) {
	if marker, ok := output.(interface{ MarkCausesRendered() }); ok {
		marker.MarkCausesRendered()
	}
}

// startupFailure selects default cause rendering only for local preparation
// and runtime opening. It preserves the original error's identity and envelope.
type startupFailure struct{ error }

func (failure startupFailure) Unwrap() error { return failure.error }

// WithStartupCause retains the error and selects safe default startup causes.
func WithStartupCause(err error) error {
	if err == nil || HasStartupCause(err) {
		return err
	}
	return startupFailure{err}
}

// HasStartupCause reports whether local startup selected default cause output.
func HasStartupCause(err error) bool {
	_, ok := findCause[startupFailure](err)
	return ok
}

// WriteStartupCauses appends safe causes once, even after a command has already
// emitted its primary diagnostic. Remote, usage and cancellation rendering is
// left to the existing command policy.
func WriteStartupCauses(output io.Writer, err error) bool {
	if output == nil || err == nil || causesRendered(output) || !HasStartupCause(err) {
		return false
	}
	if _, usage := findCause[*UsageError](err); usage {
		return false
	}
	failure, ok := findCause[startupFailure](err)
	if !ok {
		return false
	}
	root := failure.error
	// Coded presentation is already in the primary envelope. Its unwrapped
	// implementation cause supplies the additional diagnostic.
	// Inspect only this node: errors.As would recursively traverse cyclic causes.
	if _, coded := root.(CodedError); coded { //nolint:errorlint // Direct-node presentation selection preserves bounded traversal.
		root = errors.Unwrap(root)
	}
	writeCauses(output, debugCauseChain(root), "")
	return true
}

func writeCauses(output io.Writer, causes []string, prefix string) {
	for index, cause := range causes {
		_, _ = fmt.Fprintf(output, "%scause[%d]=%s\n", prefix, index, cause)
	}
	markCausesRendered(output)
}

const maxDebugCauseDepth = 16

// errorCauses bounds traversal independently of error identity: even custom
// non-comparable errors and cyclic wrappers cannot grow the walk indefinitely.
func errorCauses(err error) ([]error, bool) {
	seen := make(map[error]bool)
	nodes := make([]error, 0, maxDebugCauseDepth)
	pending := []error{err}
	truncated := false
	for visited := 0; len(pending) > 0 && visited < maxDebugCauseDepth; visited++ {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == nil {
			continue
		}
		value := reflect.ValueOf(current)
		if value.Kind() == reflect.Pointer && value.IsNil() {
			continue
		}
		if value.Comparable() {
			if seen[current] {
				continue
			}
			seen[current] = true
		}
		nodes = append(nodes, current)
		switch wrapped := current.(type) { //nolint:errorlint // Unwrap exactly one node; recursive errors.As can loop on cycles.
		case interface{ Unwrap() []error }:
			children := wrapped.Unwrap()
			if len(children) > maxDebugCauseDepth {
				children = children[:maxDebugCauseDepth]
				truncated = true
			}
			for index := len(children) - 1; index >= 0; index-- {
				pending = append(pending, children[index])
			}
		case interface{ Unwrap() error }:
			pending = append(pending, wrapped.Unwrap())
		}
	}
	return nodes, truncated || len(pending) > 0
}

func findCause[T any](err error) (T, bool) {
	nodes, _ := errorCauses(err)
	for _, node := range nodes {
		if found, ok := node.(T); ok {
			return found, true
		}
	}
	var zero T
	return zero, false
}

func debugCauseChain(err error) []string {
	nodes, truncated := errorCauses(err)
	causes := make([]string, 0, len(nodes))
	messages := make(map[string]bool)
	for _, node := range nodes {
		message := sanitizeDebugMessage(node.Error())
		if !messages[message] {
			causes = append(causes, message)
			messages[message] = true
		}
	}
	if truncated {
		causes = append(causes, "<cause chain truncated>")
	}
	return causes
}
