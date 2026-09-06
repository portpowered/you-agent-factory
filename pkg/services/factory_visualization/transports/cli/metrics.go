package cli

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"
	"time"

	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/spf13/cobra"
)

const (
	metricsGroupByWorkstation = factoryvisualization.RuntimeMetricsGroupByWorkstation
	metricsGroupByWorker      = factoryvisualization.RuntimeMetricsGroupByWorker
	metricsGroupByProvider    = factoryvisualization.RuntimeMetricsGroupByProvider
)

// MetricsCommandConfig contains the process-scoped inputs for the metrics
// command. Production composition supplies the generated HTTP operation;
// focused local callers may still inject the public query compatibility path.
type MetricsCommandConfig struct {
	Operation Operation
	// SessionEvents is the server-owned retained Factory Event replay seam used
	// by the remote session report. It is deliberately narrower than the run
	// package's transport contract so this service-owned CLI remains reusable.
	SessionEvents   SessionEventOperation
	Server          func() string
	Query           factoryvisualization.RuntimeMetricsQuery
	HomeDir         func() (string, error)
	JSON            func() bool
	Verbose         func() bool
	Costs           *cobra.Command
	CostReport      CostReportOperation
	CostHumanReport CostHumanReportOperation
}

// MetricsConfig contains the resolved inputs for one metrics query and output
// rendering operation.
type MetricsConfig struct {
	Server          string
	GroupBy         string
	SessionID       string
	JSON            bool
	Output          io.Writer
	Query           factoryvisualization.RuntimeMetricsQuery
	HomeDir         func() (string, error)
	Diagnostics     io.Writer
	Verbose         bool
	CostReport      CostReportOperation
	CostHumanReport CostHumanReportOperation

	// SessionReport selects the remote, event-backed one-session report. The
	// field is internal to the CLI composition boundary; callers should use
	// NewMetricsCommand's `metrics session` subcommand.
	SessionReport     bool
	SessionEvents     SessionEventOperation
	SessionLens       string
	SessionByWorker   bool
	SessionByDispatch bool
}

// NewMetricsCommand builds the public `you metrics` command.
func NewMetricsCommand(config MetricsCommandConfig) *cobra.Command {
	groupBy := metricsGroupByWorkstation
	sessionID := ""
	sessionLens := ""
	sessionByWorker := false
	sessionByDispatch := false
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
	sessionCommand := &cobra.Command{
		Use:          "session <session-id>",
		Short:        "Inspect one remote Factory Session metrics report",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput := false
			if config.JSON != nil {
				jsonOutput = config.JSON()
			}
			metricsConfig := MetricsConfig{
				Server:            "",
				GroupBy:           metricsGroupByWorkstation,
				SessionID:         args[0],
				JSON:              jsonOutput,
				Output:            cmd.OutOrStdout(),
				Diagnostics:       cmd.ErrOrStderr(),
				Verbose:           config.Verbose != nil && config.Verbose(),
				SessionReport:     true,
				SessionEvents:     config.SessionEvents,
				CostReport:        config.CostReport,
				CostHumanReport:   config.CostHumanReport,
				SessionLens:       sessionLens,
				SessionByWorker:   sessionByWorker,
				SessionByDispatch: sessionByDispatch,
			}
			if config.Server != nil {
				metricsConfig.Server = config.Server()
			}
			return RunMetricsOperation(cmd.Context(), config.Operation, metricsConfig)
		},
	}
	sessionCommand.Flags().StringVar(&sessionLens, "lens", "", "metrics lens")
	sessionCommand.Flags().BoolVar(&sessionByWorker, "by-worker", false, "group session attempts by worker")
	sessionCommand.Flags().BoolVar(&sessionByDispatch, "by-dispatch", false, "show session attempts by dispatch")
	command.AddCommand(sessionCommand)
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
		GroupBy:     groupBy,
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

func (reducer *metricsSessionReducer) document() metricsSessionDocument {
	states := reducer.sortedAttempts()
	assignMetricsSessionAttemptNumbers(states)
	queueValues := make([]int64, 0, len(states))
	executionValues := make([]int64, 0, len(states))
	outcomes := metricsSessionOutcomeCounts{}
	incomplete := metricsSessionIncomplete{}
	attempts := make([]metricsSessionAttemptDocument, 0, len(states))
	retries := 0
	for _, state := range states {
		queueDuration := metricsSessionQueueDuration(state)
		executionDuration := metricsSessionExecutionDuration(state)
		if queueDuration != nil {
			queueValues = append(queueValues, *queueDuration)
		}
		if executionDuration != nil {
			executionValues = append(executionValues, *executionDuration)
		}
		if state.attempt > 1 {
			retries++
		}
		if state.terminal {
			incrementMetricsSessionOutcome(&outcomes, state.outcome)
			if state.outcome == "" {
				incomplete.MissingOutcome++
			}
		} else if state.status == "QUEUED" {
			incomplete.Queued++
		} else if state.status == "RUNNING" {
			incomplete.Running++
		}
		attempts = append(attempts, metricsSessionAttemptDocumentFor(state, queueDuration, executionDuration))
	}
	return metricsSessionDocument{
		FactorySessionID:          reducer.sessionID,
		AsOf:                      reducer.asOf,
		Status:                    reducer.documentStatus(),
		Units:                     metricsSessionUnits{Duration: "milliseconds", Counts: "count"},
		ElapsedWallTimeMillis:     metricsSessionElapsed(reducer.startedAt, reducer.completedAt, reducer.asOf),
		DistinctWorkItems:         len(reducer.workIDs),
		DispatchAttempts:          len(states),
		WorkerSessions:            len(reducer.workerSessions),
		MaxConcurrentExecutions:   metricsSessionMaxConcurrency(states, reducer.asOf),
		SummedExecutionTimeMillis: sumMetricsSessionDurations(executionValues),
		SummedQueueTimeMillis:     sumMetricsSessionDurations(queueValues),
		Retries:                   retries,
		AttemptOutcomes:           outcomes,
		Incomplete:                incomplete,
		QueueDuration:             metricsSessionDurationFor(queueValues, len(states)),
		ExecutionDuration:         metricsSessionDurationFor(executionValues, len(states)),
		Attempts:                  attempts,
	}
}

func (reducer *metricsSessionReducer) documentStatus() string {
	if status := strings.TrimSpace(reducer.status); status != "" {
		return strings.ToUpper(status)
	}
	if reducer.startedAt != nil {
		return "RUNNING"
	}
	return "UNKNOWN"
}

func (reducer *metricsSessionReducer) sortedAttempts() []*metricsSessionAttemptState {
	states := make([]*metricsSessionAttemptState, 0, len(reducer.attempts))
	for _, state := range reducer.attempts {
		states = append(states, state)
	}
	sort.SliceStable(states, func(i, j int) bool {
		if states[i].dispatchID != states[j].dispatchID {
			if states[i].dispatchID == "" {
				return false
			}
			if states[j].dispatchID == "" {
				return true
			}
			return states[i].dispatchID < states[j].dispatchID
		}
		if states[i].firstEventIndex != states[j].firstEventIndex {
			return states[i].firstEventIndex < states[j].firstEventIndex
		}
		return states[i].key < states[j].key
	})
	return states
}

func assignMetricsSessionAttemptNumbers(states []*metricsSessionAttemptState) {
	byDispatchID := make(map[string]*metricsSessionAttemptState, len(states))
	for _, state := range states {
		if state.dispatchID != "" {
			byDispatchID[state.dispatchID] = state
		}
	}
	for _, state := range states {
		state.attempt = resolveMetricsSessionAttemptNumber(state, byDispatchID, make(map[string]bool))
	}
}

func resolveMetricsSessionAttemptNumber(
	state *metricsSessionAttemptState,
	byDispatchID map[string]*metricsSessionAttemptState,
	visiting map[string]bool,
) int {
	if state.attempt > 0 {
		return state.attempt
	}
	if state.retryOfDispatchID == "" {
		state.attempt = 1
		return state.attempt
	}
	if visiting[state.key] {
		state.attempt = 1
		return state.attempt
	}
	visiting[state.key] = true
	parent := byDispatchID[state.retryOfDispatchID]
	if parent == nil {
		state.attempt = 2
	} else {
		state.attempt = resolveMetricsSessionAttemptNumber(parent, byDispatchID, visiting) + 1
	}
	delete(visiting, state.key)
	return state.attempt
}

func metricsSessionAttemptDocumentFor(
	state *metricsSessionAttemptState,
	queueDuration *int64,
	executionDuration *int64,
) metricsSessionAttemptDocument {
	workIDs := make([]string, 0, len(state.workIDs))
	for workID := range state.workIDs {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	return metricsSessionAttemptDocument{
		DispatchID:              optionalMetricsSessionString(state.dispatchID),
		WorkID:                  optionalMetricsSessionSingle(workIDs),
		WorkIDs:                 workIDs,
		WorkerSessionID:         optionalMetricsSessionWorkerID(state.workerSessionIDs),
		Worker:                  state.identityValue(state.worker, state.workerConflict),
		Provider:                state.identityValue(state.provider, state.providerConflict),
		Model:                   state.identityValue(state.model, state.modelConflict),
		Workstation:             state.identityValue(state.workstation, state.workstationConflict),
		Attempt:                 state.attempt,
		RetryOfDispatchID:       optionalMetricsSessionString(state.retryOfDispatchID),
		Status:                  metricsSessionAttemptStatus(state),
		Outcome:                 optionalMetricsSessionString(state.outcome),
		QueueDurationMillis:     queueDuration,
		ExecutionDurationMillis: executionDuration,
	}
}

func metricsSessionAttemptStatus(state *metricsSessionAttemptState) string {
	if state.outcome != "" {
		return state.outcome
	}
	if state.status != "" {
		return strings.ToUpper(state.status)
	}
	return "UNKNOWN"
}

func optionalMetricsSessionString(value string) *string {
	if value = strings.TrimSpace(value); value == "" {
		return nil
	}
	return &value
}

func optionalMetricsSessionSingle(values []string) *string {
	if len(values) != 1 {
		return nil
	}
	return &values[0]
}

func optionalMetricsSessionWorkerID(values map[string]struct{}) *string {
	if len(values) != 1 {
		return nil
	}
	for value := range values {
		value := value
		return &value
	}
	return nil
}

func metricsSessionQueueDuration(state *metricsSessionAttemptState) *int64 {
	return metricsSessionTimeDifference(state.queuedAt, state.startedAt)
}

func metricsSessionExecutionDuration(state *metricsSessionAttemptState) *int64 {
	if !state.terminal {
		return nil
	}
	if duration := metricsSessionTimeDifference(state.startedAt, state.terminalAt); duration != nil {
		return duration
	}
	if state.executionDuration == nil || *state.executionDuration < 0 {
		return nil
	}
	value := *state.executionDuration
	return &value
}

func metricsSessionTimeDifference(start, end *time.Time) *int64 {
	if start == nil || end == nil || end.Before(*start) {
		return nil
	}
	value := end.Sub(*start).Milliseconds()
	return &value
}

func metricsSessionElapsed(start, completed, asOf *time.Time) *int64 {
	if start == nil {
		return nil
	}
	end := completed
	if end == nil {
		end = asOf
	}
	return metricsSessionTimeDifference(start, end)
}

type metricsSessionDecimalValue struct {
	coefficient *big.Int
	scale       int
}

func metricsSessionExactCost(items []generatedclient.CostsLineItem) *string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		if strings.ToUpper(strings.TrimSpace(string(item.Status))) != "PRICED" || item.PricedAmount == nil {
			continue
		}
		values = append(values, *item.PricedAmount)
	}
	return sumMetricsSessionDecimalStrings(values)
}

func sumMetricsSessionDecimalStrings(values []string) *string {
	if len(values) == 0 {
		return nil
	}
	parsed, maxScale, ok := parseMetricsSessionDecimals(values)
	if !ok {
		return nil
	}
	total := new(big.Int)
	for _, value := range parsed {
		coefficient := new(big.Int).Set(value.coefficient)
		if power := maxScale - value.scale; power > 0 {
			coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(power)), nil))
		}
		total.Add(total, coefficient)
	}
	return formatMetricsSessionDecimal(total, maxScale)
}

func parseMetricsSessionDecimals(values []string) ([]metricsSessionDecimalValue, int, bool) {
	parsed := make([]metricsSessionDecimalValue, 0, len(values))
	maxScale := 0
	for _, raw := range values {
		value, ok := parseMetricsSessionDecimal(raw)
		if !ok {
			return nil, 0, false
		}
		parsed = append(parsed, value)
		if value.scale > maxScale {
			maxScale = value.scale
		}
	}
	return parsed, maxScale, true
}

func parseMetricsSessionDecimal(raw string) (metricsSessionDecimalValue, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return metricsSessionDecimalValue{}, false
	}
	sign := ""
	if value[0] == '-' || value[0] == '+' {
		sign, value = value[:1], value[1:]
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || !metricsSessionDecimalDigits(parts[0]) {
		return metricsSessionDecimalValue{}, false
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if !metricsSessionDecimalDigits(fraction) {
		return metricsSessionDecimalValue{}, false
	}
	coefficient, ok := new(big.Int).SetString(sign+parts[0]+fraction, 10)
	if !ok {
		return metricsSessionDecimalValue{}, false
	}
	return metricsSessionDecimalValue{coefficient: coefficient, scale: len(fraction)}, true
}

func metricsSessionDecimalDigits(value string) bool {
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func formatMetricsSessionDecimal(total *big.Int, scale int) *string {
	negative := total.Sign() < 0
	if negative {
		total.Abs(total)
	}
	digits := total.String()
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		position := len(digits) - scale
		digits = digits[:position] + "." + digits[position:]
		digits = strings.TrimRight(digits, "0")
		digits = strings.TrimRight(digits, ".")
	}
	if digits == "" {
		digits = "0"
	}
	if negative && digits != "0" {
		digits = "-" + digits
	}
	return &digits
}
