package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestServe_PipeSmoke_RealStdioInitializeExchangeAndCleanEOF is this slice's
// single OS/process-boundary functional cell (see PRD acceptance criteria
// for story 002): it proves a real pipe-based initialize exchange and clean
// stdin EOF over genuine os.Pipe stdio, which the table-driven
// strings.Reader/bytes.Buffer cells elsewhere in this file cannot express --
// only a real OS pipe exercises genuine blocking-read and EOF-on-close
// semantics. pkg/wire now constructs this transport's Server through
// provideACPServer and exposes it on the canonical *application.Process
// (see pkg/wire/acp_transport_composition_test.go, which drives real
// session/new and session/set_config_option calls through
// root.BuildProcess); this cell still owns the genuine OS-pipe
// blocking-read/EOF proof because no CLI command yet calls Serve, so no
// tests/functional/ scenario exercises this transport over real OS stdio.
//
// Synchronization is entirely blocking-IO/channel based: the client's
// bufio.Reader.ReadString('\n') call blocks until the server actually
// writes the response, and Serve's outcome is read from a buffered channel.
// The only timer is a single terminal time.After deadline guarding against
// a genuine hang, matching this repository's established stdio smoke-test
// pattern (see pkg/transports/cli/mcp/serve_smoke_test.go); it is not a
// retry or polling loop.
func TestServe_PipeSmoke_RealStdioInitializeExchangeAndCleanEOF(t *testing.T) {
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	server := New(nil, nil, nil, nil)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(context.Background(), stdinRead, stdoutWrite)
	}()

	if _, err := stdinWrite.Write([]byte(initializeLine("1"))); err != nil {
		t.Fatalf("write initialize request: %v", err)
	}

	line, err := bufio.NewReader(stdoutRead).ReadString('\n')
	if err != nil {
		t.Fatalf("read initialize response: %v", err)
	}

	var resp rpcMessage
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(resp.ID) != "1" {
		t.Fatalf("response id = %s, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("response error = %+v, want a successful result", resp.Error)
	}
	assertJSONEqualStrings(t, initializeSuccessResult, string(resp.Result))

	if err := stdinWrite.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil on clean stdin EOF", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after stdin closed")
	}
}
