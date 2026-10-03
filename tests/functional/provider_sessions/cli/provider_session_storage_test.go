package cli_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// Route faults only by the selected immutable ID, so peer session reads use
// the same real temporary storage without a process-global fault switch.
type providerSessionReadFiles struct{}

func (providerSessionReadFiles) Open(path string) (io.ReadCloser, error) {
	if filepath.Base(path) == "rollout-session_fixture_codex_open_fault_read.jsonl" {
		return nil, errors.New("private-storage-fault: /private/operator/session-store")
	}
	return os.Open(path)
}

func (providerSessionReadFiles) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func addProviderSessionReadRoutes(t *testing.T, home string, routes map[string]platformprocess.CommandResult, codexStdout []byte) {
	t.Helper()
	for _, test := range providerSessionReadCases {
		stdout := bytesReplaceAll(codexStdout, workerSessionsCodexSuccessID, test.id)
		switch {
		case strings.Contains(test.name, "truncated"):
			writeCodexRollout(t, home, test.id, []byte(
				"{\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"retained fixture answer\"}}\n"+
					"{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"content\":[{\"text\":\"partial"))
		case !strings.Contains(test.name, "missing"):
			writeCodexRollout(t, home, test.id, []byte("{}\n"))
		}
		routes[test.id] = platformprocess.CommandResult{Stdout: stdout}
	}
}
