package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
)

func TestOpenValidatesComposedServerAndInvocationStreams(t *testing.T) {
	t.Parallel()

	if NewOpener() == nil {
		t.Fatal("NewOpener() returned nil")
	}
	if _, err := Open(nil, strings.NewReader(""), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "server") {
		t.Fatalf("Open(nil server) error = %v, want composed-server validation", err)
	}
	server, err := mcpserver.New(mcpserver.Options{
		ToolOperation: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"result":{}}`), nil
		},
	})
	if err != nil {
		t.Fatalf("mcpserver.New() error = %v", err)
	}
	if _, err := Open(server, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "streams") {
		t.Fatalf("Open(nil input) error = %v, want stream validation", err)
	}
	if _, err := Open(server, strings.NewReader(""), nil); err == nil || !strings.Contains(err.Error(), "streams") {
		t.Fatalf("Open(nil output) error = %v, want stream validation", err)
	}
	opened, err := Open(server, strings.NewReader(""), &bytes.Buffer{})
	if err != nil || opened == nil {
		t.Fatalf("Open(valid) = %v, %v; want session", opened, err)
	}
	var nilSession *session
	if err := nilSession.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("nil session Run() error = %v, want session validation", err)
	}
}
