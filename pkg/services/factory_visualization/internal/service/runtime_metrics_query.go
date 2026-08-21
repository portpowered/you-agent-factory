package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// metricsQuery is deliberately free of caches and runtime/session state. A
// query reads the supplied artifact root and builds all accumulators locally.
type metricsQuery struct {
	reader RuntimeMetricsReader
	logger logging.Logger
}

// NewRuntimeMetricsQuery constructs the stateless Factory Visualization
// metrics operation from its narrow reader and operation logger ports.
func NewRuntimeMetricsQuery(
	reader RuntimeMetricsReader,
	logger logging.Logger,
) (RuntimeMetricsQuery, error) {
	if reader == nil {
		return nil, errors.New("construct Factory Visualization metrics query: reader is required")
	}
	query := &metricsQuery{reader: reader, logger: logging.EnsureLogger(logger)}
	return RuntimeMetricsQuery(query.QueryRuntimeMetrics), nil
}

func (q *metricsQuery) QueryRuntimeMetrics(
	ctx context.Context,
	request RuntimeMetricsQueryRequest,
) (RuntimeMetricsQueryResult, error) {
	if q == nil || q.reader == nil {
		return RuntimeMetricsQueryResult{}, &RuntimeMetricsQueryError{
			Kind:    RuntimeMetricsQueryInvalidInput,
			Message: "query Factory Runtime metrics: reader is required",
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	root := strings.TrimSpace(request.MetricsRoot)
	if root == "" {
		return RuntimeMetricsQueryResult{}, &RuntimeMetricsQueryError{
			Kind:    RuntimeMetricsQueryInvalidInput,
			Message: "query Factory Runtime metrics: metrics root is required",
		}
	}
	if err := ctx.Err(); err != nil {
		return RuntimeMetricsQueryResult{}, err
	}

	sessionID := strings.TrimSpace(request.SessionID)
	runtimeID := strings.TrimSpace(request.RuntimeInstanceID)
	q.logger.Info(
		"Factory Runtime metrics query started",
		"metrics_root", root,
		"session_id", sessionID,
		"runtime_instance_id", runtimeID,
	)

	records, err := q.reader.Read(ctx, root)
	if err != nil {
		q.logger.Error(
			"Factory Runtime metrics query failed",
			"metrics_root", root,
			"session_id", sessionID,
			"runtime_instance_id", runtimeID,
			"error", err,
		)
		return RuntimeMetricsQueryResult{}, &RuntimeMetricsQueryError{
			Kind:    RuntimeMetricsQueryReadFailed,
			Message: "query Factory Runtime metrics: read artifacts",
			Cause:   err,
		}
	}

	accumulator := newMetricsAccumulator()
	considered := 0
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return RuntimeMetricsQueryResult{}, err
		}
		if !runtimeMetricRecordMatches(record, sessionID, runtimeID) {
			continue
		}
		recognized, err := q.addRecord(accumulator, record, root)
		if err != nil {
			return RuntimeMetricsQueryResult{}, err
		}
		if recognized {
			considered++
		}
	}

	return q.finishQuery(accumulator, root, sessionID, runtimeID, considered)
}

func (q *metricsQuery) addRecord(
	accumulator *metricsAccumulator,
	record RuntimeMetricRecord,
	root string,
) (bool, error) {
	metricName, _ := recordString(record, "metric_name")
	isUsage := isUsageRuntimeMetric(metricName)
	if isUsage {
		if err := accumulator.addUsage(record); err != nil {
			q.logger.Warn(
				"Factory Runtime metrics usage record rejected",
				"metrics_root", root,
				"metric_name", metricName,
				"error", err,
			)
			return false, &RuntimeMetricsQueryError{
				Kind:    RuntimeMetricsQueryInvalidUsage,
				Message: "query Factory Runtime metrics: usage record is invalid",
				Cause:   err,
			}
		}
	}
	return accumulator.add(record) || isUsage, nil
}

func (q *metricsQuery) finishQuery(
	accumulator *metricsAccumulator,
	root string,
	sessionID string,
	runtimeID string,
	considered int,
) (RuntimeMetricsQueryResult, error) {
	result, err := accumulator.result()
	if err != nil {
		q.logger.Warn(
			"Factory Runtime metrics usage rows rejected",
			"metrics_root", root,
			"error", err,
		)
		return RuntimeMetricsQueryResult{}, &RuntimeMetricsQueryError{
			Kind:    RuntimeMetricsQueryInvalidUsage,
			Message: "query Factory Runtime metrics: usage rows are invalid",
			Cause:   err,
		}
	}
	q.logger.Info(
		"Factory Runtime metrics query completed",
		"metrics_root", root,
		"session_id", sessionID,
		"runtime_instance_id", runtimeID,
		"records_considered", considered,
		"workstation_groups", len(result.Workstations),
		"worker_type_groups", len(result.WorkerTypes),
		"provider_groups", len(result.Providers),
		"cost_availability", result.Cost.Availability,
	)
	return result, nil
}

type metricsAccumulator struct {
	totals       metricAggregateBuilder
	workstations map[string]*metricAggregateBuilder
	workerTypes  map[string]*metricAggregateBuilder
	providers    map[string]*metricAggregateBuilder
	usage        map[usageIdentity]*usageBuilder
}

type usageIdentity struct {
	factorySessionID string
	workID           string
	dispatchID       string
	workerSessionID  string
	provider         string
	model            string
}

type usageBuilder struct {
	identity              usageIdentity
	inputTokens           *int64
	outputTokens          *int64
	cachedInputTokens     *int64
	reasoningOutputTokens *int64
}

type metricAggregateBuilder struct {
	inputTokens         float64
	outputTokens        float64
	completedDispatches float64
	failuresByReason    map[string]float64
	dispatchDuration    durationBuilder
	providerDuration    durationBuilder
}

type durationBuilder struct {
	unit    string
	samples []float64
}

func newMetricsAccumulator() *metricsAccumulator {
	return &metricsAccumulator{
		workstations: make(map[string]*metricAggregateBuilder),
		workerTypes:  make(map[string]*metricAggregateBuilder),
		providers:    make(map[string]*metricAggregateBuilder),
		usage:        make(map[usageIdentity]*usageBuilder),
	}
}

func (a *metricsAccumulator) addUsage(record RuntimeMetricRecord) error {
	metricName, ok := recordString(record, "metric_name")
	if !ok || !isUsageRuntimeMetric(metricName) {
		return nil
	}
	tokenCount, err := runtimeMetricTokenCount(record)
	if err != nil {
		return fmt.Errorf("%s: %w", metricName, err)
	}
	identity := usageIdentityFromRecord(record)
	builder := a.usage[identity]
	if builder == nil {
		builder = &usageBuilder{identity: identity}
		a.usage[identity] = builder
	}
	return builder.add(metricName, tokenCount)
}

func (builder *usageBuilder) add(metricName string, value int64) error {
	var target **int64
	switch metricName {
	case factoryruntime.RuntimeProviderInputTokens:
		target = &builder.inputTokens
	case factoryruntime.RuntimeProviderOutputTokens:
		target = &builder.outputTokens
	case factoryruntime.RuntimeProviderCachedInputTokens:
		target = &builder.cachedInputTokens
	case factoryruntime.RuntimeProviderReasoningOutputTokens:
		target = &builder.reasoningOutputTokens
	default:
		return fmt.Errorf("unsupported usage metric")
	}
	if *target == nil {
		valueCopy := value
		*target = &valueCopy
		return nil
	}
	if value > math.MaxInt64-**target {
		return fmt.Errorf("token count overflows int64")
	}
	**target += value
	return nil
}

func usageIdentityFromRecord(record RuntimeMetricRecord) usageIdentity {
	return usageIdentity{
		factorySessionID: recordStringValue(record, "session_id"),
		workID:           recordStringValue(record, "work_id"),
		dispatchID:       recordStringValue(record, "dispatch_id"),
		workerSessionID:  recordStringValue(record, "worker_session_id"),
		provider:         recordStringValue(record, "provider"),
		model:            recordStringValue(record, "model"),
	}
}

func recordStringValue(record RuntimeMetricRecord, key string) string {
	value, _ := recordString(record, key)
	return strings.TrimSpace(value)
}

func runtimeMetricTokenCount(record RuntimeMetricRecord) (int64, error) {
	value, ok := recordFloat(record, "value")
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("value must be a finite number")
	}
	if value < 0 || math.Trunc(value) != value {
		return 0, fmt.Errorf("value must be a non-negative integer")
	}
	if value >= float64(uint64(1)<<63) {
		return 0, fmt.Errorf("value exceeds int64 range")
	}
	return int64(value), nil
}

func (a *metricsAccumulator) add(record RuntimeMetricRecord) bool {
	metricName, ok := recordString(record, "metric_name")
	if !ok || !isSupportedRuntimeMetric(metricName) {
		return false
	}
	value, ok := recordFloat(record, "value")
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	reason, _ := recordString(record, "reason")
	if metricName == factoryruntime.RuntimeProviderFailed && strings.TrimSpace(reason) == "" {
		return false
	}
	unit, _ := recordString(record, "unit")
	apply := func(builder *metricAggregateBuilder) {
		applyMetric(builder, metricName, value, strings.TrimSpace(unit), strings.TrimSpace(reason))
	}
	apply(&a.totals)
	addLabeledAggregate(a.workstations, record, "workstation", apply)
	addLabeledAggregate(a.workerTypes, record, "worker_type", apply)
	addLabeledAggregate(a.providers, record, "provider", apply)
	return true
}

func addLabeledAggregate(
	groups map[string]*metricAggregateBuilder,
	record RuntimeMetricRecord,
	label string,
	apply func(*metricAggregateBuilder),
) {
	key, ok := recordString(record, label)
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return
	}
	builder := groups[key]
	if builder == nil {
		builder = &metricAggregateBuilder{}
		groups[key] = builder
	}
	apply(builder)
}

func applyMetric(
	builder *metricAggregateBuilder,
	metricName string,
	value float64,
	unit string,
	reason string,
) {
	switch metricName {
	case factoryruntime.RuntimeProviderInputTokens:
		builder.inputTokens += value
	case factoryruntime.RuntimeProviderOutputTokens:
		builder.outputTokens += value
	case factoryruntime.RuntimeDispatchComplete:
		builder.completedDispatches += value
	case factoryruntime.RuntimeProviderFailed:
		if builder.failuresByReason == nil {
			builder.failuresByReason = make(map[string]float64)
		}
		builder.failuresByReason[reason] += value
	case factoryruntime.RuntimeDispatchDuration:
		builder.dispatchDuration.add(value, unit)
	case factoryruntime.RuntimeProviderDuration:
		builder.providerDuration.add(value, unit)
	}
}

func isSupportedRuntimeMetric(name string) bool {
	switch name {
	case factoryruntime.RuntimeProviderInputTokens,
		factoryruntime.RuntimeProviderOutputTokens,
		factoryruntime.RuntimeDispatchComplete,
		factoryruntime.RuntimeProviderFailed,
		factoryruntime.RuntimeDispatchDuration,
		factoryruntime.RuntimeProviderDuration:
		return true
	default:
		return false
	}
}

func isUsageRuntimeMetric(name string) bool {
	switch name {
	case factoryruntime.RuntimeProviderInputTokens,
		factoryruntime.RuntimeProviderOutputTokens,
		factoryruntime.RuntimeProviderCachedInputTokens,
		factoryruntime.RuntimeProviderReasoningOutputTokens:
		return true
	default:
		return false
	}
}

func (d *durationBuilder) add(value float64, unit string) {
	if d.unit == "" {
		d.unit = unit
	}
	d.samples = append(d.samples, value)
}

func (a *metricsAccumulator) result() (RuntimeMetricsQueryResult, error) {
	usageRows, err := sortedUsageRows(a.usage)
	if err != nil {
		return RuntimeMetricsQueryResult{}, err
	}
	return RuntimeMetricsQueryResult{
		Cost:         RuntimeMetricsCost{Availability: RuntimeMetricsCostUnavailable},
		Totals:       a.totals.result(),
		Workstations: sortedBreakdowns(a.workstations),
		WorkerTypes:  sortedBreakdowns(a.workerTypes),
		Providers:    sortedBreakdowns(a.providers),
		UsageRows:    usageRows,
	}, nil
}

func sortedUsageRows(groups map[usageIdentity]*usageBuilder) ([]RuntimeMetricsUsageRow, error) {
	identities := make([]usageIdentity, 0, len(groups))
	for identity := range groups {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(left, right int) bool {
		return usageIdentityLess(identities[left], identities[right])
	})
	rows := make([]RuntimeMetricsUsageRow, 0, len(identities))
	for _, identity := range identities {
		row, err := groups[identity].result()
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func usageIdentityLess(left, right usageIdentity) bool {
	for _, values := range [][2]string{
		{left.factorySessionID, right.factorySessionID},
		{left.workID, right.workID},
		{left.dispatchID, right.dispatchID},
		{left.workerSessionID, right.workerSessionID},
		{left.provider, right.provider},
		{left.model, right.model},
	} {
		if values[0] == values[1] {
			continue
		}
		return values[0] < values[1]
	}
	return false
}

func (builder *usageBuilder) result() (RuntimeMetricsUsageRow, error) {
	if builder.cachedInputTokens != nil && builder.inputTokens != nil && *builder.cachedInputTokens > *builder.inputTokens {
		return RuntimeMetricsUsageRow{}, fmt.Errorf("cached input tokens exceed input tokens for dispatch %q", builder.identity.dispatchID)
	}
	if builder.reasoningOutputTokens != nil && builder.outputTokens != nil && *builder.reasoningOutputTokens > *builder.outputTokens {
		return RuntimeMetricsUsageRow{}, fmt.Errorf("reasoning output tokens exceed output tokens for dispatch %q", builder.identity.dispatchID)
	}
	return RuntimeMetricsUsageRow{
		FactorySessionID:      builder.identity.factorySessionID,
		WorkID:                builder.identity.workID,
		DispatchID:            builder.identity.dispatchID,
		WorkerSessionID:       builder.identity.workerSessionID,
		Provider:              builder.identity.provider,
		Model:                 builder.identity.model,
		InputTokens:           cloneInt64Pointer(builder.inputTokens),
		OutputTokens:          cloneInt64Pointer(builder.outputTokens),
		CachedInputTokens:     cloneInt64Pointer(builder.cachedInputTokens),
		ReasoningOutputTokens: cloneInt64Pointer(builder.reasoningOutputTokens),
	}, nil
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func sortedBreakdowns(groups map[string]*metricAggregateBuilder) []RuntimeMetricsBreakdown {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	breakdowns := make([]RuntimeMetricsBreakdown, 0, len(keys))
	for _, key := range keys {
		breakdowns = append(breakdowns, RuntimeMetricsBreakdown{
			Key:       key,
			Aggregate: groups[key].result(),
		})
	}
	return breakdowns
}

func (a *metricAggregateBuilder) result() RuntimeMetricsAggregate {
	return RuntimeMetricsAggregate{
		InputTokens:         a.inputTokens,
		OutputTokens:        a.outputTokens,
		CompletedDispatches: a.completedDispatches,
		FailuresByReason:    cloneFloatMap(a.failuresByReason),
		DispatchDuration:    a.dispatchDuration.result(),
		ProviderDuration:    a.providerDuration.result(),
	}
}

func (d *durationBuilder) result() *RuntimeMetricsDuration {
	if len(d.samples) == 0 {
		return nil
	}
	samples := append([]float64(nil), d.samples...)
	sort.Float64s(samples)
	return &RuntimeMetricsDuration{
		Unit:    d.unit,
		Samples: len(samples),
		P50:     nearestRankPercentile(samples, 0.50),
		P95:     nearestRankPercentile(samples, 0.95),
	}
}

func nearestRankPercentile(samples []float64, percentile float64) *float64 {
	if len(samples) == 0 {
		return nil
	}
	rank := int(math.Ceil(percentile * float64(len(samples))))
	if rank < 1 {
		rank = 1
	}
	value := samples[rank-1]
	return &value
}

func cloneFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]float64, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func runtimeMetricRecordMatches(record RuntimeMetricRecord, sessionID, runtimeID string) bool {
	if sessionID != "" {
		value, ok := recordString(record, "session_id")
		if !ok || value != sessionID {
			return false
		}
	}
	if runtimeID != "" {
		value, ok := recordString(record, "runtime_instance_id")
		if !ok || value != runtimeID {
			return false
		}
	}
	return true
}

func recordString(record RuntimeMetricRecord, key string) (string, bool) {
	value, ok := record[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func recordFloat(record RuntimeMetricRecord, key string) (float64, bool) {
	value, ok := record[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	default:
		return 0, false
	}
}
