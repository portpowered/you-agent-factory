package mcpcli

import (
	"io"
	"strings"
	"testing"
)

func TestServeCommand_IsRegistered(t *testing.T) {
	root := NewCommand()
	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("find mcp serve command: %v", err)
	}
	if serveCmd.Use != "serve" {
		t.Fatalf("serve command use = %q, want serve", serveCmd.Use)
	}
}

func TestServeCommand_LongHelpMentionsFactoryPreviewTools(t *testing.T) {
	root := NewCommand()
	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("find mcp serve command: %v", err)
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, want := range []string{"Factory preview", "stdio", "start-preview"} {
		if !strings.Contains(serveCmd.Long, want) {
			t.Fatalf("serve long help missing %q:\n%s", want, serveCmd.Long)
		}
	}
}
