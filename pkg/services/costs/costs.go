// Package costs values canonical runtime usage with Providers-owned prices.
package costs

import (
	"context"
	"fmt"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// PriceTableReader is the Costs-side view of the narrow Providers-owned
// pricing capability. Providers remains the authority for the table and its
// facts; this consumer port keeps Costs independent of the provider service's
// broader identity and execution operations.
type PriceTableReader interface {
	ReadPriceTable() (providers.PriceTable, error)
}

// CostsQuery is the stateless operation over canonical runtime usage rows.
// The request supplies artifact/configuration paths as data; the operation
// retains no request state between calls.
type CostsQuery func(context.Context, QueryRequest) (Report, error)

// Query invokes the operation through its descriptive method form.
func (query CostsQuery) Query(ctx context.Context, request QueryRequest) (Report, error) {
	if query == nil {
		return Report{}, &QueryError{Kind: QueryErrorInvalidInput, Message: "query runtime costs: operation is required"}
	}
	return query(ctx, request)
}

// QueryCosts is an explicit alias for callers that prefer the operation name
// over Query.
func (query CostsQuery) QueryCosts(ctx context.Context, request QueryRequest) (Report, error) {
	return query.Query(ctx, request)
}

// Query is the short compatibility name for the Costs operation type.
type Query = CostsQuery

// QueryRequest identifies the canonical metrics input, the explicit operator
// settings document, and optional scope filters. The settings path is read on
// every query so changes take effect without rebuilding the process.
// FactorySessionID and RuntimeInstanceID are optional filters. A non-empty
// RetainedFactorySessionIDs set is the authoritative session filter supplied
// by Factory Sessions; without it, FactorySessionID remains the direct
// single-identity compatibility filter.
type QueryRequest struct {
	MetricsRoot               string
	OperatorSettingsPath      string
	FactorySessionID          string
	RetainedFactorySessionIDs []string
	RuntimeInstanceID         string
}

// Validate checks request fields that can be validated before either injected
// dependency is called.
func (request QueryRequest) Validate() error {
	if strings.TrimSpace(request.MetricsRoot) == "" {
		return fmt.Errorf("metrics root is required")
	}
	return nil
}

// CostsQueryRequest is the descriptive alias for QueryRequest.
type CostsQueryRequest = QueryRequest

// ScopeKind identifies whether a report covers all sessions or one selected
// Factory Session.
type ScopeKind string

const (
	ScopeAllFactorySessions ScopeKind = "ALL_FACTORY_SESSIONS"
	ScopeFactorySession     ScopeKind = "FACTORY_SESSION"
)

// Scope describes the selection used to produce a report.
type Scope struct {
	Kind             ScopeKind `json:"kind"`
	FactorySessionID string    `json:"factory_session_id,omitempty"`
}

// Status is the truthful valuation state of a report, rollup, or line item.
type Status string

const (
	StatusPriced   Status = "PRICED"
	StatusPartial  Status = "PARTIAL"
	StatusUnpriced Status = "UNPRICED"
	StatusNoUsage  Status = "NO_USAGE"
)

// PriceSource identifies the complete pricing row used for a priced line
// item. An empty value is reserved for unpriced rows.
type PriceSource string

const (
	PriceSourceBuiltIn          PriceSource = "BUILT_IN"
	PriceSourceOperatorSupplied PriceSource = "OPERATOR_SUPPLIED"
)

// Coverage counts encountered and fully priced usage rows and distinct
// provider/model identities. A provider/model pair is priced only when every
// encountered row for that pair is priced in the same scope.
type Coverage struct {
	EncounteredRows           int `json:"encountered_rows"`
	PricedRows                int `json:"priced_rows"`
	UnpricedRows              int `json:"unpriced_rows"`
	EncounteredProviderModels int `json:"encountered_provider_models"`
	PricedProviderModels      int `json:"priced_provider_models"`
	UnpricedProviderModels    int `json:"unpriced_provider_models"`
}

// TokenCounts preserves absent subclass measurements while aggregating all
// observed token counts. An explicit zero is represented by a non-nil pointer.
type TokenCounts struct {
	InputTokens           *int64 `json:"input_tokens,omitempty"`
	OutputTokens          *int64 `json:"output_tokens,omitempty"`
	CachedInputTokens     *int64 `json:"cached_input_tokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens,omitempty"`
}

// TokenTotals is the complete aggregate token shape used by reports and
// rollups. Nullable fields preserve the distinction between an absent source
// measurement and an observed zero while keeping every JSON key present.
// TotalTokens is input plus output and never adds cached-input or
// reasoning-output subclasses a second time.
type TokenTotals struct {
	TotalTokens           *int64 `json:"total_tokens"`
	InputTokens           *int64 `json:"input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	CachedInputTokens     *int64 `json:"cached_input_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
}

// UnpricedPair identifies one provider/model pair whose usage could not be
// valued. Nil identities are explicit unknowns rather than omitted facts.
type UnpricedPair struct {
	Provider      *string `json:"provider"`
	Model         *string `json:"model"`
	DispatchCount int     `json:"dispatch_count"`
}

// LineItem is one canonical usage row and its complete valuation outcome.
// Unpriced rows retain their identity and observed token counts; PricedAmount
// is absent for unpriced rows and is present as "0" for explicitly free usage.
type LineItem struct {
	FactorySessionID string `json:"factory_session_id,omitempty"`
	WorkID           string `json:"work_id,omitempty"`
	DispatchID       string `json:"dispatch_id,omitempty"`
	WorkerSessionID  string `json:"worker_session_id,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	TokenCounts
	Status       Status      `json:"status"`
	PriceSource  PriceSource `json:"price_source,omitempty"`
	PricedAmount *string     `json:"priced_amount,omitempty"`
	Reason       string      `json:"reason,omitempty"`
}

// Rollup is a monetary and usage summary for one Work item, Worker Session,
// or Factory Session key.
type Rollup struct {
	Key string `json:"key"`
	TokenCounts
	Currency              string         `json:"currency"`
	Status                Status         `json:"status"`
	KnownCost             *string        `json:"known_cost"`
	PricedSubtotal        *string        `json:"priced_subtotal,omitempty"` // Deprecated compatibility alias; use KnownCost.
	TokenTotals           TokenTotals    `json:"token_totals"`
	UnpricedDispatchCount int            `json:"unpriced_dispatch_count"`
	UnpricedPairs         []UnpricedPair `json:"unpriced_pairs"`
	Coverage              Coverage       `json:"coverage"`
}

// ProviderModelRollup is a rollup keyed by an exact provider/model identity.
type ProviderModelRollup struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Rollup
}

// Report is the complete deterministic Costs result. All monetary values are
// exact decimal strings in Currency; no floating-point amount is exposed.
type Report struct {
	Scope                 Scope                 `json:"scope"`
	Currency              string                `json:"currency"`
	Status                Status                `json:"status"`
	KnownCost             *string               `json:"known_cost"`
	PricedSubtotal        *string               `json:"priced_subtotal,omitempty"` // Deprecated compatibility alias; use KnownCost.
	TokenTotals           TokenTotals           `json:"token_totals"`
	UnpricedDispatchCount int                   `json:"unpriced_dispatch_count"`
	UnpricedPairs         []UnpricedPair        `json:"unpriced_pairs"`
	Coverage              Coverage              `json:"coverage"`
	LineItems             []LineItem            `json:"line_items"`
	WorkItems             []Rollup              `json:"work_items"`
	WorkerSessions        []Rollup              `json:"worker_sessions"`
	ProviderModels        []ProviderModelRollup `json:"provider_models"`
	FactorySessions       []Rollup              `json:"factory_sessions"`
}

// CostsQueryResult is the descriptive alias for Report.
type CostsQueryResult = Report

// QueryErrorKind classifies failures at the Costs operation boundary.
type QueryErrorKind string

const (
	QueryErrorInvalidInput       QueryErrorKind = "INVALID_INPUT"
	QueryErrorSettingsReadFailed QueryErrorKind = "SETTINGS_READ_FAILED"
	QueryErrorInvalidPriceTable  QueryErrorKind = "INVALID_PRICE_TABLE"
	QueryErrorMetricsFailed      QueryErrorKind = "METRICS_QUERY_FAILED"
	QueryErrorInvalidUsage       QueryErrorKind = "INVALID_USAGE"
)

// QueryError preserves a safe operation-level message and the injected
// dependency failure without exposing configuration or usage payloads.
type QueryError struct {
	Kind    QueryErrorKind
	Message string
	Cause   error
}

func (err *QueryError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message != "" {
		return err.Message
	}
	return string(err.Kind)
}

func (err *QueryError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}
