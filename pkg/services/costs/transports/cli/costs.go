// Package cli adapts the Costs HTTP contract into the `you metrics costs`
// command. It does not read pricing, metrics, or runtime artifacts locally.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/spf13/cobra"
)

// Client is the generated response-aware HTTP client capability used by the
// command. Keeping this interface narrow makes transport behavior testable
// without moving cost policy into the CLI.
type Client interface {
	GetMetricsCostsWithResponse(
		context.Context,
		*generatedclient.GetMetricsCostsParams,
		...generatedclient.RequestEditorFn,
	) (*generatedclient.GetMetricsCostsClientResponse, error)
}

// ClientFactory constructs one generated client for the selected server.
// Construction belongs to Wire; the CLI only receives this capability.
type ClientFactory func(string) (Client, error)

// Operation is the injected command operation for one cost report request.
type Operation func(context.Context, CostsConfig) error

// CostsCommandConfig contains invocation-scoped callbacks from the root CLI.
type CostsCommandConfig struct {
	Operation Operation
	Server    func() string
	JSON      func() bool
}

// CostsConfig contains the fully resolved inputs for one cost report request.
type CostsConfig struct {
	Server    string
	SessionID string
	JSON      bool
	Output    io.Writer
}

// NewOperation binds the generated HTTP client factory to the Costs command
// operation. No monetary calculation is performed in this adapter.
func NewOperation(factory ClientFactory) Operation {
	return func(ctx context.Context, config CostsConfig) error {
		if err := validateCostsRequest(ctx, config); err != nil {
			return err
		}
		if factory == nil {
			return fmt.Errorf("build metrics costs client: factory is required")
		}
		client, err := factory(strings.TrimSpace(config.Server))
		if err != nil {
			return fmt.Errorf("build metrics costs client: %w", err)
		}
		if client == nil {
			return fmt.Errorf("build metrics costs client: client is required")
		}
		response, err := client.GetMetricsCostsWithResponse(
			ctx,
			costsRequestParams(config.SessionID),
		)
		if err != nil {
			return fmt.Errorf("query metrics costs: %w", err)
		}
		if response == nil || response.JSON200 == nil {
			return costsResponseError(response)
		}
		output, err := renderCostsOutput(*response.JSON200, config.JSON)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(config.Output, output); err != nil {
			return fmt.Errorf("write metrics costs output: %w", err)
		}
		return nil
	}
}

// NewCostsCommand builds the `costs` child of `you metrics`.
func NewCostsCommand(config CostsCommandConfig) *cobra.Command {
	sessionID := ""
	command := &cobra.Command{
		Use:          "costs",
		Short:        "Inspect exact runtime cost rollups",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			server := ""
			if config.Server != nil {
				server = config.Server()
			}
			jsonOutput := false
			if config.JSON != nil {
				jsonOutput = config.JSON()
			}
			return RunCosts(cmd.Context(), config.Operation, CostsConfig{
				Server:    server,
				SessionID: sessionID,
				JSON:      jsonOutput,
				Output:    cmd.OutOrStdout(),
			})
		},
	}
	command.Flags().StringVar(&sessionID, "session", "", "limit costs to one Factory Session")
	return command
}

// RunCosts invokes the injected operation after validating the command
// boundary. The operation renders into a complete buffer before writing, so a
// failed route cannot leave partial success output on stdout.
func RunCosts(ctx context.Context, operation Operation, config CostsConfig) error {
	if err := validateCostsRequest(ctx, config); err != nil {
		return err
	}
	if operation == nil {
		return fmt.Errorf("query metrics costs: operation is required")
	}
	return operation(ctx, config)
}

func validateCostsRequest(ctx context.Context, config CostsConfig) error {
	if ctx == nil {
		return fmt.Errorf("query metrics costs: context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("render metrics costs: output writer is required")
	}
	if strings.TrimSpace(config.Server) == "" {
		return fmt.Errorf("query metrics costs: server is required")
	}
	return nil
}

func costsRequestParams(sessionID string) *generatedclient.GetMetricsCostsParams {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return &generatedclient.GetMetricsCostsParams{SessionId: &sessionID}
}

func costsResponseError(response *generatedclient.GetMetricsCostsClientResponse) error {
	if response == nil {
		return fmt.Errorf("query metrics costs: empty server response")
	}
	if response.JSON400 != nil && strings.TrimSpace(response.JSON400.Message) != "" {
		return fmt.Errorf("query metrics costs: %s", response.JSON400.Message)
	}
	if response.JSON500 != nil && strings.TrimSpace(response.JSON500.Message) != "" {
		return fmt.Errorf("query metrics costs: %s", response.JSON500.Message)
	}
	if response.StatusCode() != 0 {
		return fmt.Errorf("query metrics costs: server returned HTTP %d", response.StatusCode())
	}
	return fmt.Errorf("query metrics costs: response did not contain a cost report")
}
