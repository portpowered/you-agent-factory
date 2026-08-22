package providers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Service is the singular cross-service Providers root authority. Peer packages
// depend on this one named interface for Providers-owned catalog enumeration,
// identity/selection authority, availability/capability facts, and one normalized execution attempt rather
// than Workers provider registry/conductor types or concrete adapter packages.
// Providers owns exactly one native attempt per Execute call; callers own
// selection, retry, throttle, and scheduling policy.
type Service interface {
	// ListProviders returns detached catalog descriptors for every known
	// provider, including availability and capability facts. Unavailable or
	// prerequisite-blocked providers remain listed with their catalog facts.
	ListProviders(context.Context, ListProvidersRequest) (ListProvidersResult, error)
	// GetProvider returns one detached catalog descriptor for a Providers-owned
	// provider identity. Invalid identity fails with ErrInvalidID, unknown
	// identity fails with ErrUnknownProvider, and blocked availability or
	// missing prerequisite facts fail with ErrProviderUnavailable. Static
	// catalog requirements remain selectable but are reported as unverified
	// until a readiness probe supplies current facts.
	GetProvider(context.Context, GetProviderRequest) (GetProviderResult, error)
	// ResolveIdentity canonicalizes one Providers-owned ID or accepted alias
	// without starting execution.
	ResolveIdentity(context.Context, ResolveIdentityRequest) (ResolveIdentityResult, error)
	// ResolveSelection applies Providers-owned selection precedence and returns
	// one canonical provider identity.
	ResolveSelection(context.Context, ResolveSelectionRequest) (ResolveSelectionResult, error)
	// ValidatePrerequisites verifies that one canonical provider is currently
	// selectable.
	ValidatePrerequisites(context.Context, ValidatePrerequisitesRequest) error
	// Execute performs exactly one normalized provider attempt. Invalid request
	// identity fails with ErrInvalidID. Attempt failures return typed
	// Providers-owned errors such as ErrExecuteCancelled, ErrExecuteTimeout, and
	// ErrExecuteFailed that peers can branch on with errors.Is / errors.As.
	// Successful results and typed failures may carry an optional detached
	// SessionRef for the provider session observed during the attempt.
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
	// ControlAttempt requests pause, cancel, or terminate for one identified
	// provider attempt. Invalid provider, attempt, or action input fails with
	// ErrInvalidID or ErrInvalidControlRequest before any outcome is produced.
	// Valid requests return a typed completed or unsupported outcome as a
	// successful result; unsupported is not encoded as an error.
	ControlAttempt(context.Context, ControlAttemptRequest) (ControlAttemptResult, error)
	// Continue resumes the exact provider session identified by Reference.
	// Unsupported continuation is returned as a typed successful outcome and
	// never falls back to Execute.
	Continue(context.Context, ContinueRequest) (ContinueResult, error)
	// ContinueReference validates and routes a detached opaque continuation
	// reference without exposing Provider Session state to callers.
	ContinueReference(context.Context, ContinueReferenceRequest) (ContinueReferenceResult, error)
}

// ContinueReferenceRequest carries the detached continuation vocabulary to
// the Providers root. It is intentionally separate from ContinueRequest so
// Workers and Runtime do not need to construct or inspect a provider-owned
// SessionRef while forwarding an opaque continuation.
type ContinueReferenceRequest struct {
	Reference ContinuationRef
	Attempt   ExecuteRequest
}

// ContinueReferenceResult is the detached result of one opaque continuation
// request. Reference echoes the canonical provider identity and exact session
// kind/id while retaining any caller-supplied external reference.
type ContinueReferenceResult struct {
	Reference ContinuationRef
	Outcome   ContinuationOutcome
	Result    ExecuteResult
}

func firstContinuationIdentity(ref ContinuationRef) string {
	if value := strings.TrimSpace(ref.ProviderSessionID); value != "" {
		return value
	}
	return strings.TrimSpace(ref.ExternalRef)
}

// Clone returns a detached continuation-reference result.
func (result ContinueReferenceResult) Clone() ContinueReferenceResult {
	clone := result
	clone.Reference = result.Reference.Clone()
	clone.Result = result.Result.Clone()
	return clone
}

// String returns a bounded identity useful for diagnostics without exposing
// provider-specific request material.
func (ref ContinuationRef) String() string {
	normalized := ref.Normalize()
	return fmt.Sprintf("%s/%s/%s", normalized.Provider, normalized.Kind, firstContinuationIdentity(normalized))
}

// PriceTableCurrency identifies the currency used by every published rate.
// Providers currently publishes USD only.
const PriceTableCurrencyUSD = "USD"

// PriceClass identifies one optional token subclass rate.
type PriceClass string

const (
	PriceClassCachedInput     PriceClass = "cached-input"
	PriceClassReasoningOutput PriceClass = "reasoning-output"
)

// ErrPriceTableInvalid reports a malformed or insufficient provider-owned
// pricing fact before it can reach valuation.
var ErrPriceTableInvalid = errors.New("provider price table is invalid")

var priceProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
var priceRatePattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// PriceTable is the detached, declarative provider pricing catalog consumed by
// Costs. It is not operator configuration and has no mutation operation.
type PriceTable struct {
	Currency string
	Models   []PriceTableModel
}

// PriceTableModel publishes exact USD rates for one canonical provider/model
// pair. A nil subclass rate means that coverage is unknown. EqualRateClasses
// explicitly records when a subclass uses its standard class rate; equality
// is never inferred from omission.
type PriceTableModel struct {
	Provider                        ID
	Model                           string
	InputPerMillionTokens           string
	OutputPerMillionTokens          string
	CachedInputPerMillionTokens     *string
	ReasoningOutputPerMillionTokens *string
	SourceURL                       string
	AsOfDate                        string
	EqualRateClasses                []PriceClass
}

// PriceTableReaderFunc adapts a pure Providers-owned price-table read to the
// narrow consumer capability used by Costs. The function form keeps pricing
// out of the singular Providers service root while retaining detached reads.
type PriceTableReaderFunc func() (PriceTable, error)

// ReadPriceTable implements the narrow price-table consumer capability.
func (reader PriceTableReaderFunc) ReadPriceTable() (PriceTable, error) {
	if reader == nil {
		return PriceTable{}, fmt.Errorf("read provider price table: reader is required")
	}
	table, err := reader()
	if err != nil {
		return PriceTable{}, err
	}
	return table.Clone(), nil
}

// Clone returns a detached price table.
func (table PriceTable) Clone() PriceTable {
	cloned := table
	if table.Models == nil {
		return cloned
	}
	cloned.Models = make([]PriceTableModel, len(table.Models))
	for index, model := range table.Models {
		cloned.Models[index] = model.Clone()
	}
	return cloned
}

// Clone returns a detached model-price entry.
func (model PriceTableModel) Clone() PriceTableModel {
	cloned := model
	cloned.CachedInputPerMillionTokens = clonePriceRate(model.CachedInputPerMillionTokens)
	cloned.ReasoningOutputPerMillionTokens = clonePriceRate(model.ReasoningOutputPerMillionTokens)
	cloned.EqualRateClasses = append([]PriceClass(nil), model.EqualRateClasses...)
	return cloned
}

// Normalize trims and validates a complete provider-owned table. It also
// canonicalizes provider IDs and produces deterministic equal-rate ordering.
func (table PriceTable) Normalize() (PriceTable, error) {
	if strings.TrimSpace(table.Currency) != PriceTableCurrencyUSD {
		return PriceTable{}, fmt.Errorf("%w: currency %q is unsupported; only %s is accepted", ErrPriceTableInvalid, table.Currency, PriceTableCurrencyUSD)
	}
	normalized := PriceTable{Currency: PriceTableCurrencyUSD, Models: make([]PriceTableModel, len(table.Models))}
	seen := make(map[string]struct{}, len(table.Models))
	for index, model := range table.Models {
		entry, err := normalizePriceTableModel(index, model)
		if err != nil {
			return PriceTable{}, err
		}
		key := string(entry.Provider) + "\x00" + entry.Model
		if _, exists := seen[key]; exists {
			return PriceTable{}, fmt.Errorf("%w: models[%d] duplicates provider/model %q/%q", ErrPriceTableInvalid, index, entry.Provider, entry.Model)
		}
		seen[key] = struct{}{}
		normalized.Models[index] = entry
	}
	return normalized, nil
}

func normalizePriceTableModel(index int, model PriceTableModel) (PriceTableModel, error) {
	provider := ID(strings.ToLower(strings.TrimSpace(model.Provider.String())))
	if !priceProviderPattern.MatchString(string(provider)) {
		return PriceTableModel{}, fmt.Errorf("%w: models[%d].provider %q must be a canonical provider identity", ErrPriceTableInvalid, index, model.Provider)
	}
	modelName := strings.TrimSpace(model.Model)
	if modelName == "" {
		return PriceTableModel{}, fmt.Errorf("%w: models[%d].model must be non-empty", ErrPriceTableInvalid, index)
	}
	input, err := normalizePriceRate(index, "inputPerMillionTokens", model.InputPerMillionTokens, true)
	if err != nil {
		return PriceTableModel{}, err
	}
	output, err := normalizePriceRate(index, "outputPerMillionTokens", model.OutputPerMillionTokens, true)
	if err != nil {
		return PriceTableModel{}, err
	}
	if err := validateProvenance(index, model.SourceURL, model.AsOfDate); err != nil {
		return PriceTableModel{}, err
	}
	normalized := PriceTableModel{
		Provider:               provider,
		Model:                  modelName,
		InputPerMillionTokens:  input,
		OutputPerMillionTokens: output,
		SourceURL:              strings.TrimSpace(model.SourceURL),
		AsOfDate:               strings.TrimSpace(model.AsOfDate),
	}
	if normalized.CachedInputPerMillionTokens, err = normalizeOptionalRate(index, "cachedInputPerMillionTokens", model.CachedInputPerMillionTokens); err != nil {
		return PriceTableModel{}, err
	}
	if normalized.ReasoningOutputPerMillionTokens, err = normalizeOptionalRate(index, "reasoningOutputPerMillionTokens", model.ReasoningOutputPerMillionTokens); err != nil {
		return PriceTableModel{}, err
	}
	normalized.EqualRateClasses, err = normalizeEqualRateClasses(index, model.EqualRateClasses, input, output, normalized.CachedInputPerMillionTokens, normalized.ReasoningOutputPerMillionTokens)
	if err != nil {
		return PriceTableModel{}, err
	}
	return normalized, nil
}

func normalizePriceRate(index int, field, value string, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return "", fmt.Errorf("%w: models[%d].%s is required", ErrPriceTableInvalid, index, field)
		}
		return "", nil
	}
	if !priceRatePattern.MatchString(value) {
		return "", fmt.Errorf("%w: models[%d].%s %q must be a non-negative decimal string", ErrPriceTableInvalid, index, field, value)
	}
	return value, nil
}

func normalizeOptionalRate(index int, field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized, err := normalizePriceRate(index, field, *value, false)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func validateProvenance(index int, sourceURL, asOfDate string) error {
	sourceURL = strings.TrimSpace(sourceURL)
	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("%w: models[%d].sourceURL must be an authoritative http(s) URL", ErrPriceTableInvalid, index)
	}
	asOfDate = strings.TrimSpace(asOfDate)
	parsedDate, err := time.Parse("2006-01-02", asOfDate)
	if err != nil || parsedDate.Format("2006-01-02") != asOfDate {
		return fmt.Errorf("%w: models[%d].asOfDate must be an ISO-8601 date", ErrPriceTableInvalid, index)
	}
	return nil
}

func normalizeEqualRateClasses(index int, classes []PriceClass, input, output string, cached, reasoning *string) ([]PriceClass, error) {
	seen := make(map[PriceClass]struct{}, len(classes))
	for _, class := range classes {
		class, err := normalizeEqualRateClass(index, class, input, output, cached, reasoning)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[class]; exists {
			return nil, fmt.Errorf("%w: models[%d].equalRateClasses duplicates %q", ErrPriceTableInvalid, index, class)
		}
		seen[class] = struct{}{}
	}
	if err := validateImplicitEqualRateClasses(index, classes, input, output, cached, reasoning); err != nil {
		return nil, err
	}
	result := make([]PriceClass, 0, len(seen))
	for class := range seen {
		result = append(result, class)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func normalizeEqualRateClass(index int, class PriceClass, input, output string, cached, reasoning *string) (PriceClass, error) {
	class = PriceClass(strings.ToLower(strings.TrimSpace(string(class))))
	if class != PriceClassCachedInput && class != PriceClassReasoningOutput {
		return "", fmt.Errorf("%w: models[%d].equalRateClasses contains unsupported class %q", ErrPriceTableInvalid, index, class)
	}
	switch class {
	case PriceClassCachedInput:
		if cached == nil || *cached != input {
			return "", fmt.Errorf("%w: models[%d] cached-input equality declaration does not match its input rate", ErrPriceTableInvalid, index)
		}
	case PriceClassReasoningOutput:
		if reasoning == nil || *reasoning != output {
			return "", fmt.Errorf("%w: models[%d] reasoning-output equality declaration does not match its output rate", ErrPriceTableInvalid, index)
		}
	}
	return class, nil
}

func validateImplicitEqualRateClasses(index int, classes []PriceClass, input, output string, cached, reasoning *string) error {
	if cached != nil && *cached == input && !containsPriceClass(classes, PriceClassCachedInput) {
		return fmt.Errorf("%w: models[%d] equal cached-input rate must be explicitly declared", ErrPriceTableInvalid, index)
	}
	if reasoning != nil && *reasoning == output && !containsPriceClass(classes, PriceClassReasoningOutput) {
		return fmt.Errorf("%w: models[%d] equal reasoning-output rate must be explicitly declared", ErrPriceTableInvalid, index)
	}
	return nil
}

func containsPriceClass(classes []PriceClass, want PriceClass) bool {
	for _, class := range classes {
		if PriceClass(strings.ToLower(strings.TrimSpace(string(class)))) == want {
			return true
		}
	}
	return false
}

func clonePriceRate(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
