package commanddispatch_test

import (
	"errors"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
)

func TestStartFailureNamesAnOversizedCommandLine(t *testing.T) {
	t.Parallel()

	failure, ok := commanddispatch.StartFailure(&platformprocess.CommandStartError{
		Command:           "claude",
		ArgsCount:         9,
		CommandLineLength: 32932,
		CommandLineLimit:  platformprocess.WindowsCommandLineLimit,
		Cause:             errors.New("The filename or extension is too long."),
	})
	if !ok {
		t.Fatal("StartFailure() ok = false, want a declared failure for an unstarted process")
	}
	if failure.Kind != providers.ExecuteFailureKindMisconfigured {
		t.Fatalf("failure kind = %q, want %q", failure.Kind, providers.ExecuteFailureKindMisconfigured)
	}
	if failure.Diagnostics == nil || failure.Diagnostics.Metadata["work-failure-type"] != "command_line_too_long" {
		t.Fatalf("failure diagnostics = %#v, want work-failure-type command_line_too_long", failure.Diagnostics)
	}
	for _, want := range []string{"32932", "32767"} {
		if !strings.Contains(failure.Message, want) {
			t.Fatalf("failure message = %q, want it to report %q", failure.Message, want)
		}
	}
}

func TestStartFailureClassifiesOtherSpawnFailuresAsMisconfigured(t *testing.T) {
	t.Parallel()

	failure, ok := commanddispatch.StartFailure(&platformprocess.CommandStartError{
		Command:           "claude",
		ArgsCount:         3,
		CommandLineLength: 120,
		CommandLineLimit:  platformprocess.WindowsCommandLineLimit,
		Cause:             errors.New("executable file not found in %PATH%"),
	})
	if !ok {
		t.Fatal("StartFailure() ok = false, want a declared failure for an unstarted process")
	}
	if failure.Diagnostics == nil || failure.Diagnostics.Metadata["work-failure-type"] != "misconfigured" {
		t.Fatalf("failure diagnostics = %#v, want work-failure-type misconfigured", failure.Diagnostics)
	}
}

func TestStartFailureIgnoresErrorsThatAreNotSpawnFailures(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{nil, errors.New("provider exited with status 1")} {
		if _, ok := commanddispatch.StartFailure(cause); ok {
			t.Fatalf("StartFailure(%v) ok = true, want false for a process that did start", cause)
		}
	}
}

func TestStartFailureUnwrapsThroughWrappingErrors(t *testing.T) {
	t.Parallel()

	wrapped := errWrapper{cause: &platformprocess.CommandStartError{
		Command:           "claude",
		CommandLineLength: 40000,
		CommandLineLimit:  platformprocess.WindowsCommandLineLimit,
		Cause:             errors.New("boom"),
	}}
	failure, ok := commanddispatch.StartFailure(wrapped)
	if !ok {
		t.Fatal("StartFailure() ok = false, want the wrapped spawn failure to be recognized")
	}
	if failure.Diagnostics.Metadata["work-failure-type"] != "command_line_too_long" {
		t.Fatalf("failure diagnostics = %#v, want work-failure-type command_line_too_long", failure.Diagnostics)
	}
}

type errWrapper struct{ cause error }

func (e errWrapper) Error() string { return "wrapped: " + e.cause.Error() }
func (e errWrapper) Unwrap() error { return e.cause }
