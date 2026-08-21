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
// command. The command reads through the public Factory Visualization query;
// it does not inspect runtime metric artifacts itself.
type MetricsCommandConfig struct {
	Query   factoryvisualization.RuntimeMetricsQuery
	HomeDir func() (string, error)
}

// MetricsConfig contains the resolved inputs for one metrics query and human
// rendering operation.
type MetricsConfig struct {
	GroupBy string
	Output  io.Writer
	Query   factoryvisualization.RuntimeMetricsQuery
	HomeDir func() (string, error)
}

// NewMetricsCommand builds the public `you metrics` command.
func NewMetricsCommand(config MetricsCommandConfig) *cobra.Command {
	groupBy := metricsGroupByWorkstation
	command := &cobra.Command{
		Use:          "metrics",
		Short:        "Inspect recorded Factory Runtime metrics",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunMetrics(cmd.Context(), MetricsConfig{
				GroupBy: groupBy,
				Output:  cmd.OutOrStdout(),
				Query:   config.Query,
				HomeDir: config.HomeDir,
			})
		},
	}
	command.Flags().StringVar(&groupBy, "group-by", metricsGroupByWorkstation,
		"group metrics by workstation, worker, or provider")
	return command
}

// RunMetrics queries and renders one human-readable metrics result. It builds
// the complete output before writing so a query failure cannot leave a partial
// success report on stdout.
func RunMetrics(ctx context.Context, config MetricsConfig) error {
	if err := validateMetricsConfig(ctx, config); err != nil {
		return err
	}
	groupBy, err := normalizeMetricsGroupBy(config.GroupBy)
	if err != nil {
		return err
	}
	homeDir, err := config.HomeDir()
	if err != nil {
		return fmt.Errorf("resolve metrics home directory: %w", err)
	}
	if strings.TrimSpace(homeDir) == "" {
		return fmt.Errorf("resolve metrics home directory: path is empty")
	}
	result, err := config.Query.QueryRuntimeMetrics(ctx, factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: platformmetrics.RuntimeMetricsRoot(homeDir),
	})
	if err != nil {
		return fmt.Errorf("query Factory Runtime metrics: %w", err)
	}
	output := renderHumanMetrics(groupBy, result)
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
		return fmt.Errorf("query Factory Runtime metrics: query operation is required")
	}
	if config.HomeDir == nil {
		return fmt.Errorf("resolve metrics home directory: resolver is required")
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
		return "", fmt.Errorf(
			"invalid --group-by %q: choose workstation, worker, or provider",
			value,
		)
	}
}
