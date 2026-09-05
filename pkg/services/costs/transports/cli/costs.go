// Package cli adapts the Costs HTTP contract into the `you metrics costs`
// command. It does not read pricing, metrics, or runtime artifacts locally.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

// ReportOperation reads one Costs report without choosing an output format.
// The metrics session command uses this narrow result-valued seam so Costs
// remains the owner of HTTP response mapping, request bounds, and monetary
// semantics.
type ReportOperation func(context.Context, CostsConfig) (generatedclient.CostsReport, error)

// CostsCommandConfig contains invocation-scoped callbacks from the root CLI.
type CostsCommandConfig struct {
	Operation      Operation
	Server         func() string
	JSON           func() bool
	RequestTimeout time.Duration
}

// CostsConfig contains the fully resolved inputs for one cost report request.
type CostsConfig struct {
	Server         string
	SessionID      string
	JSON           bool
	Output         io.Writer
	RequestTimeout time.Duration
}

// NewOperation binds the generated HTTP client factory to the Costs command
// operation. No monetary calculation is performed in this adapter.
func NewOperation(factory ClientFactory) Operation {
	readReport := NewReportOperation(factory)
	return func(ctx context.Context, config CostsConfig) error {
		if err := validateCostsRequest(ctx, config); err != nil {
			return err
		}
		report, err := readReport(ctx, config)
		if err != nil {
			return err
		}
		output, err := renderCostsOutput(report, config.JSON)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(config.Output, output); err != nil {
			return fmt.Errorf("write metrics costs output: %w", err)
		}
		return nil
	}
}

// NewReportOperation binds the generated HTTP client factory to a result-valued
// Costs read. It shares the exact request timeout and typed response handling
// used by the standalone `you metrics costs` command.
func NewReportOperation(factory ClientFactory) ReportOperation {
	return func(ctx context.Context, config CostsConfig) (generatedclient.CostsReport, error) {
		return readReport(factory, ctx, config)
	}
}

func readReport(factory ClientFactory, ctx context.Context, config CostsConfig) (generatedclient.CostsReport, error) {
	if err := validateCostsReportRequest(ctx, config); err != nil {
		return generatedclient.CostsReport{}, err
	}
	if factory == nil {
		return generatedclient.CostsReport{}, fmt.Errorf("build metrics costs client: factory is required")
	}
	client, err := factory(strings.TrimSpace(config.Server))
	if err != nil {
		return generatedclient.CostsReport{}, newCostsError(
			CostsNetworkFailureCode,
			internalErrorFamily,
			fmt.Sprintf("build GET %s client for %s failed; check --server and confirm the Factory API endpoint is valid", metricsCostsEndpoint, safeServerEndpoint(config.Server)),
			err,
		)
	}
	if client == nil {
		return generatedclient.CostsReport{}, newCostsError(
			CostsNetworkFailureCode,
			internalErrorFamily,
			fmt.Sprintf("build GET %s client for %s failed; check --server and confirm the Factory API endpoint is valid", metricsCostsEndpoint, safeServerEndpoint(config.Server)),
			nil,
		)
	}
	requestTimeout := normalizeRequestTimeout(config.RequestTimeout)
	requestContext, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	response, err := client.GetMetricsCostsWithResponse(
		requestContext,
		costsRequestParams(config.SessionID),
	)
	if err != nil {
		return generatedclient.CostsReport{}, newCostsTransportError(config.Server, requestTimeout, err)
	}
	if response == nil || response.JSON200 == nil {
		return generatedclient.CostsReport{}, costsResponseError(response, config.Server)
	}
	return *response.JSON200, nil
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
				Server:         server,
				SessionID:      sessionID,
				JSON:           jsonOutput,
				Output:         cmd.OutOrStdout(),
				RequestTimeout: config.RequestTimeout,
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

func validateCostsReportRequest(ctx context.Context, config CostsConfig) error {
	if ctx == nil {
		return fmt.Errorf("query metrics costs: context is required")
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

func costsResponseError(response *generatedclient.GetMetricsCostsClientResponse, server string) error {
	if response == nil {
		return newCostsError(
			CostsNetworkFailureCode,
			internalErrorFamily,
			fmt.Sprintf("GET %s at %s returned no response; check --server and confirm the Factory API is running", metricsCostsEndpoint, safeServerEndpoint(server)),
			nil,
		)
	}
	for _, candidate := range []*generatedclient.ErrorResponse{
		response.JSON400,
		response.JSON408,
		response.JSON500,
		response.JSON504,
	} {
		if candidate == nil {
			continue
		}
		code := strings.TrimSpace(string(candidate.Code))
		if code == "" {
			code = CostsHTTPFailureCode
		}
		message := strings.TrimSpace(candidate.Message)
		if message == "" {
			message = fmt.Sprintf("server returned HTTP %d", response.StatusCode())
		}
		return newCostsError(
			code,
			candidateFamily(factoryapi.ErrorFamily(candidate.Family)),
			fmt.Sprintf("GET %s at %s returned HTTP %d: %s", metricsCostsEndpoint, safeServerEndpoint(server), response.StatusCode(), message),
			nil,
		)
	}
	if response.StatusCode() == http.StatusNotFound {
		var candidate factoryapi.ErrorResponse
		if err := json.Unmarshal(response.Body, &candidate); err == nil {
			code := strings.TrimSpace(string(candidate.Code))
			message := strings.TrimSpace(candidate.Message)
			if code != "" || message != "" {
				if code == "" {
					code = CostsHTTPFailureCode
				}
				if message == "" {
					message = fmt.Sprintf("server returned HTTP %d", response.StatusCode())
				}
				return newCostsError(
					code,
					candidateFamily(candidate.Family),
					fmt.Sprintf("GET %s at %s returned HTTP %d: %s", metricsCostsEndpoint, safeServerEndpoint(server), response.StatusCode(), message),
					nil,
				)
			}
		}
	}
	if response.StatusCode() != 0 {
		return newCostsError(
			CostsHTTPFailureCode,
			familyForStatus(response.StatusCode()),
			fmt.Sprintf("GET %s at %s returned HTTP %d; retry the command or check the Factory API logs", metricsCostsEndpoint, safeServerEndpoint(server), response.StatusCode()),
			nil,
		)
	}
	return newCostsError(
		CostsHTTPFailureCode,
		internalErrorFamily,
		fmt.Sprintf("GET %s at %s returned no cost report; retry the command or check the Factory API logs", metricsCostsEndpoint, safeServerEndpoint(server)),
		nil,
	)
}
