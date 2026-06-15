package cli

import (
	"io"
	"strings"
	"testing"
)

func TestMCPServeCommand_IsRegistered(t *testing.T) {
	root := NewRootCommand()
	mcpCmd, _, err := root.Find([]string{"mcp"})
	if err != nil {
		t.Fatalf("find mcp command: %v", err)
	}
	serveCmd, _, err := mcpCmd.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("find mcp serve command: %v", err)
	}
	if serveCmd.Use != "serve" {
		t.Fatalf("serve command use = %q, want serve", serveCmd.Use)
	}
}

func TestMCPServeCommand_LongHelpMentionsFactoryPreviewTools(t *testing.T) {
	root := NewRootCommand()
	mcpCmd, _, err := root.Find([]string{"mcp", "serve"})
	if err != nil {
		t.Fatalf("find mcp serve command: %v", err)
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, want := range []string{"Factory preview", "stdio", "start-preview"} {
		if !strings.Contains(mcpCmd.Long, want) {
			t.Fatalf("serve long help missing %q:\n%s", want, mcpCmd.Long)
		}
	}
}
