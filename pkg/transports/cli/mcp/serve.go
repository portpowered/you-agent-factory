package mcpcli

import (
	"fmt"
	"net/http"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	mapping "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/spf13/cobra"
)

const (
	projectRootInputID = "you.server.mcp.flag.project-root"
)

// MCPIntent is the stdio serve intent delivered to the injected initializer.
type MCPIntent = startupcli.MCPIntent

// StdioHandler initializes MCP stdio serving for a resolved intent.
type StdioHandler = startupcli.StdioHandler

// ServeBinding supplies the injected lifecycle operations used by the MCP
// resolved-input adapter.
type ServeBinding struct {
	InitializeStdio StdioHandler
	LookupEnv       func(string) (string, bool)
}

// ResolvedServeHandler maps canonical stable-ID inputs into the already
// injected MCP stdio initializer.
func ResolvedServeHandler(
	binding ServeBinding,
) func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return func(cmd *cobra.Command, inputs, inherited resolvedinput.Inputs) error {
		projectRoot, err := inputs.String(projectRootInputID)
		if err != nil {
			return fmt.Errorf("read MCP project root input: %w", err)
		}
		serverURL, err := inherited.String("you.flag.server")
		if err != nil {
			return fmt.Errorf("read MCP server input: %w", err)
		}
		if binding.InitializeStdio == nil {
			return fmt.Errorf("MCP stdio initializer is required")
		}
		intent := MCPIntent{
			ProjectRoot: projectRoot,
			ServerURL:   serverURL,
			Stdin:       cmd.InOrStdin(),
			Stdout:      cmd.OutOrStdout(),
		}
		if binding.LookupEnv != nil {
			headers := make(http.Header)
			if id, present := binding.LookupEnv("YOU_WORKER_SESSION_ID"); present {
				headers.Set("X-You-Worker-Session-Id", id)
			}
			if token, present := binding.LookupEnv("YOU_WORKER_SESSION_TOKEN"); present {
				headers.Set("Authorization", "Bearer "+token)
			}
			caller, err := mapping.WorkerSessionCallerFromHeaders(headers)
			if err != nil {
				return &callerCredentialError{cause: err}
			}
			if caller != nil {
				intent.WorkerSessionID, intent.WorkerSessionToken = caller.WorkerSessionID, caller.Token
			}
		}
		return binding.InitializeStdio(cmd.Context(), intent)
	}
}

// The command diagnostic boundary recognizes these fixed fields without
// exposing the rejected environment values or converting refusal to INTERNAL.
type callerCredentialError struct{ cause error }

func (e *callerCredentialError) Error() string {
	return e.InvocationErrorCode() + ": " + e.InvocationErrorMessage()
}

func (e *callerCredentialError) Unwrap() error { return e.cause }

func (*callerCredentialError) InvocationErrorCode() string { return "WORKER_SESSION_CALLER_INVALID" }

func (*callerCredentialError) InvocationErrorMessage() string {
	return "Worker Session caller credentials are invalid"
}
