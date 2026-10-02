package service

import (
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mattn/go-shellwords"
)

// parseACPCommand preserves Windows executable paths while retaining the
// existing shell-style parsing for portable ACP launch commands.
func parseACPCommand(command string) ([]string, error) {
	first := strings.TrimSpace(command)
	first = strings.TrimPrefix(first, `"`)
	if len(first) >= 3 && isASCIIAlpha(first[0]) && first[1] == ':' && (first[2] == '\\' || first[2] == '/') {
		command = strings.ReplaceAll(command, `\`, `\\`)
	}
	return shellwords.Parse(command)
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

// acpStdio keeps the parent response reader independent of exec.Cmd.Wait.
// StdoutPipe would be closed by Wait before the SDK finishes reading buffered
// peer output. The service owns these pipe handles until the peer is retired.
type acpStdio struct {
	requests  io.WriteCloser
	responses io.ReadCloser
	childEnds []*os.File
}

// attachACPStdio installs a fresh parent-owned stdio channel on cmd. It must
// run before cmd.Start, and detach must run once the start succeeded.
func attachACPStdio(cmd *exec.Cmd) (*acpStdio, error) {
	childStdin, requests, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	responses, childStdout, err := os.Pipe()
	if err != nil {
		_ = childStdin.Close()
		_ = requests.Close()
		return nil, err
	}
	cmd.Stdin, cmd.Stdout = childStdin, childStdout
	return &acpStdio{requests: requests, responses: responses, childEnds: []*os.File{childStdin, childStdout}}, nil
}

// detach releases this process's copies of the child's pipe ends after a
// successful start. The child owns its own handles, so closing them here
// leaves the channel's lifetime bounded by the peer and lets the response
// reader observe a clean end of stream instead of a closed local handle.
func (stdio *acpStdio) detach() {
	for _, file := range stdio.childEnds {
		_ = file.Close()
	}
	stdio.childEnds = nil
}

// close releases every end this service still owns, including the child's
// ends when the peer never started.
func (stdio *acpStdio) close() {
	stdio.detach()
	_ = stdio.requests.Close()
	_ = stdio.responses.Close()
}
