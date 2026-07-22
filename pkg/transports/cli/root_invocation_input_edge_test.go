package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveFactoryInvocationInput_RequiresProcessTerminalMetadata(t *testing.T) {
	_, err := collectRunInvocationStdin(nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "process terminal metadata is required") {
		t.Fatalf("error = %v, want missing terminal metadata", err)
	}
}

func TestPrepareRunInvocationInputRequiresInjectedWorkRole(t *testing.T) {
	_, err := prepareRunInvocationInput(&cobra.Command{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Work invocation-input preparation is required") {
		t.Fatalf("error = %v, want required Work role", err)
	}
}

func TestResolveFactoryInvocationInput_RequiresProcessStdinForExplicitDash(t *testing.T) {
	_, err := collectRunInvocationStdin([]string{"-"}, nil, func() bool { return true })
	if err == nil || !strings.Contains(err.Error(), "process stdin is required") {
		t.Fatalf("error = %v, want missing stdin", err)
	}
}

func TestCollectRunInvocationStdinPreservesReaderFailure(t *testing.T) {
	want := errors.New("reader failed")
	_, err := collectRunInvocationStdin([]string{"-"}, failingInvocationReader{err: want}, func() bool { return true })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped reader failure", err)
	}
}

type failingInvocationReader struct{ err error }

func (reader failingInvocationReader) Read([]byte) (int, error) { return 0, reader.err }
