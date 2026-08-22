package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/spf13/cobra"
)

const (
	metricsGroupByWorkstation = "workstation"
	metricsGroupByWorker      = "worker"
	metricsGroupByProvider    = "provider"
)

// MetricsCommandConfig contains the process-scoped inputs for the metrics
// command. Production composition supplies the generated HTTP operation;
// focused local callers may still inject the public query compatibility path.
type MetricsCommandConfig struct {
	Operation Operation
	Server    func() string
	Query     factoryvisualization.RuntimeMetricsQuery
	HomeDir   func() (string, error)
	JSON      func() bool
	Costs     *cobra.Command
}

// MetricsConfig contains the resolved inputs for one metrics query and output
// rendering operation.
type MetricsConfig struct {
	Server    string
	GroupBy   string
	SessionID string
	JSON      bool
	Output    io.Writer
	Query     factoryvisualization.RuntimeMetricsQuery
	HomeDir   func() (string, error)
}

// NewMetricsCommand builds the public `you metrics` command.
func NewMetricsCommand(config MetricsCommandConfig) *cobra.Command {
	groupBy := metricsGroupByWorkstation
	sessionID := ""
	command := &cobra.Command{
		Use:          "metrics",
		Short:        "Inspect recorded Factory Runtime metrics",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput := false
			if config.JSON != nil {
				jsonOutput = config.JSON()
			}
			metricsConfig := MetricsConfig{
				Server:    "",
				GroupBy:   groupBy,
				SessionID: sessionID,
				JSON:      jsonOutput,
				Output:    cmd.OutOrStdout(),
				Query:     config.Query,
				HomeDir:   config.HomeDir,
			}
			if config.Server != nil {
				metricsConfig.Server = config.Server()
			}
			if config.Operation != nil {
				return RunMetricsOperation(cmd.Context(), config.Operation, metricsConfig)
			}
			return RunMetrics(cmd.Context(), metricsConfig)
		},
	}
	command.Flags().StringVar(&groupBy, "group-by", metricsGroupByWorkstation,
		"group metrics by workstation, worker, or provider")
	command.Flags().StringVar(&sessionID, "session", "",
		"limit metrics to one Factory Session")
	if config.Costs != nil {
		command.AddCommand(config.Costs)
	}
	return command
}

// RunMetrics queries and renders one metrics result. It builds the complete
// output before writing so a query or encoding failure cannot leave a partial
// success report on stdout.
func RunMetrics(ctx context.Context, config MetricsConfig) error {
	if err := validateMetricsConfig(ctx, config); err != nil {
		return err
	}
	groupBy, err := normalizeMetricsGroupBy(config.GroupBy)
	if err != nil {
		return err
	}
	sessionID := strings.TrimSpace(config.SessionID)
	homeDir, err := config.HomeDir()
	if err != nil {
		return newMetricsError(
			MetricsHomeDirectoryFailedCode,
			"resolve metrics home directory: home directory could not be resolved; set HOME or USERPROFILE",
			err,
		)
	}
	if strings.TrimSpace(homeDir) == "" {
		return newMetricsError(
			MetricsHomeDirectoryFailedCode,
			"resolve metrics home directory: resolver returned an empty path; set HOME or USERPROFILE",
			nil,
		)
	}
	result, err := config.Query.QueryRuntimeMetrics(ctx, factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: platformmetrics.RuntimeMetricsRoot(homeDir),
		SessionID:   sessionID,
	})
	if err != nil {
		return newMetricsQueryError(err)
	}
	output, err := renderMetricsOutput(groupBy, sessionID, config.JSON, result)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(config.Output, output); err != nil {
		return fmt.Errorf("write metrics output: %w", err)
	}
	return nil
}

func validateMetricsConfig(ctx context.Context, config MetricsConfig) error {
	if ctx == nil {
		return fmt.Errorf("query Factory Runtime metrics: context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("render metrics: output writer is required")
	}
	if config.Query == nil {
		return newMetricsError(
			MetricsQueryFailedCode,
			"query Factory Runtime metrics: query operation is required",
			nil,
		)
	}
	if config.HomeDir == nil {
		return newMetricsError(
			MetricsHomeDirectoryFailedCode,
			"resolve metrics home directory: resolver is required; set HOME or USERPROFILE",
			nil,
		)
	}
	return nil
}

func normalizeMetricsGroupBy(value string) (string, error) {
	groupBy := strings.ToLower(strings.TrimSpace(value))
	if groupBy == "" {
		return metricsGroupByWorkstation, nil
	}
	switch groupBy {
	case metricsGroupByWorkstation, metricsGroupByWorker, metricsGroupByProvider:
		return groupBy, nil
	default:
		return "", newMetricsError(
			MetricsInvalidGroupByCode,
			fmt.Sprintf(
				"invalid --group-by %q: choose workstation, worker, or provider",
				value,
			),
			nil,
		)
	}
}
