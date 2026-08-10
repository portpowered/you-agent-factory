package work

import (
	"bytes"
	"context"
	"errors"
	pathpkg "path"
	"strings"
	"text/template"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/stateaccessquery"
)

const (
	DefaultListMaxResults = 50

	FilterStateName    = "state.name"
	FilterStateType    = "state.type"
	FilterName         = "name"
	FilterWorkTypeName = "workTypeName"
	FilterTraceID      = "traceId"
	FilterTerminal     = "terminal"
	FilterNonTerminal  = "nonTerminal"

	SortByStateType = "state.type"

	StateTypeInitial    = "INITIAL"
	StateTypeProcessing = "PROCESSING"
	StateTypeTerminal   = "TERMINAL"
	StateTypeFailed     = "FAILED"
)

// ErrWorkNotFound is the typed state-access failure returned when list/get /
// move-and-read cannot resolve the requested Work. Peers branch with errors.Is.
var ErrWorkNotFound = errors.New("Work not found")

// ReadModel is the detached customer-facing Work projection returned by the
// root Service state-access slice (ListWork, GetWork, MoveWorkAndRead). It
// deliberately contains no token, place, marking, or topology fields and no
// Factory Runtime or peer implementation types.
type ReadModel struct {
	CursorID                 string
	Name                     string
	WorkID                   string
	WorkTypeName             string
	State                    *State
	ChainingTraceDepth       int
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	TraceID                  string
	Content                  []WorkContentPart
	Tags                     map[string]string
	Relations                []ReadRelation
	StopSummary              *StopSummary
	ExpectedArtifacts        []ExpectedArtifactReadModel
}

// ExpectedArtifactDeclaration is the detached, compiled form of one authored
// expected-artifact contract. Runtime topology and replay projections use this
// type so Work never imports Factory Definition implementation contracts.
type ExpectedArtifactDeclaration struct {
	Name     string
	Pattern  string
	NonEmpty bool
}

// ExpectedArtifactVerificationStatus describes the latest recorded
// verification observation for one expected artifact declaration.
type ExpectedArtifactVerificationStatus string

const (
	ExpectedArtifactVerificationPending   ExpectedArtifactVerificationStatus = "PENDING"
	ExpectedArtifactVerificationSatisfied ExpectedArtifactVerificationStatus = "SATISFIED"
	ExpectedArtifactVerificationFailed    ExpectedArtifactVerificationStatus = "FAILED"
)

// ExpectedArtifactVerificationReason identifies why an expected artifact was
// not satisfied. Values are intentionally aligned with the canonical failure
// event vocabulary while remaining owned by the Work read contract.
type ExpectedArtifactVerificationReason string

const (
	ExpectedArtifactVerificationReasonMissing ExpectedArtifactVerificationReason = "MISSING"
	ExpectedArtifactVerificationReasonEmpty   ExpectedArtifactVerificationReason = "EMPTY"
)

// ExpectedArtifactReadModel is the safe per-Work artifact projection exposed
// by list and get reads. Pattern is always workspace-relative or one of the
// redacted diagnostic placeholders; it never contains a host workspace path.
type ExpectedArtifactReadModel struct {
	Name         string                              `json:"name"`
	Pattern      string                              `json:"pattern"`
	NonEmpty     bool                                `json:"nonEmpty"`
	Verification ExpectedArtifactVerificationStatus  `json:"verification"`
	Reason       *ExpectedArtifactVerificationReason `json:"reason,omitempty"`
}

// ExpectedArtifactInput is the stable template data needed to render an
// authored artifact pattern from recorded Work inputs. It deliberately omits
// filesystem and host-runtime details.
type ExpectedArtifactInput struct {
	Name       string
	WorkID     string
	WorkTypeID string
	DataType   string
	TraceID    string
	ParentID   string
	Project    string
	Tags       map[string]string
	Payload    string
}

// ExpectedArtifactTemplateContext carries the non-host context exposed to
// artifact templates. Replay callers normally leave these values empty.
type ExpectedArtifactTemplateContext struct {
	// Project is the stable project value available to artifact templates.
	Project string `json:"project,omitempty"`
	// SessionID is the stable Factory Session value available to artifact
	// templates. Host paths, environment variables, and Factory docs are not
	// part of the artifact template vocabulary.
	SessionID string `json:"sessionId,omitempty"`
}

const (
	defaultExpectedArtifactProject = "default-project"
	defaultExpectedArtifactSession = "~default"
)

// CloneExpectedArtifactTemplateContext returns a detached copy of the stable
// context recorded with an artifact-bearing dispatch.
func CloneExpectedArtifactTemplateContext(
	context *ExpectedArtifactTemplateContext,
) *ExpectedArtifactTemplateContext {
	if context == nil {
		return nil
	}
	clone := *context
	return &clone
}

// ExpectedArtifactVerificationEntry is one recorded unmet declaration.
type ExpectedArtifactVerificationEntry struct {
	// DeclarationIndex is the one-based position in the normalized declaration
	// list. It disambiguates declarations that intentionally share a name.
	DeclarationIndex int
	Name             string
	Pattern          string
	Reason           ExpectedArtifactVerificationReason
}

// ExpectedArtifactObservation describes the durable verification fact for a
// relevant dispatch. A verified observation with no entries means every
// declaration was satisfied; entries identify only unmet declarations.
type ExpectedArtifactObservation struct {
	Verified bool
	Entries  []ExpectedArtifactVerificationEntry
}

// ExpectedArtifactReadModelProjector combines effective declarations and a
// recorded verification observation into a deterministic Work read model. It
// performs no filesystem access, so replay remains stable after workspace
// contents change.
type ExpectedArtifactReadModelProjector struct{}

// Project combines effective declarations and a recorded verification
// observation into a deterministic Work read model.
func (ExpectedArtifactReadModelProjector) Project(
	workTypeDeclarations []ExpectedArtifactDeclaration,
	workstationDeclarations []ExpectedArtifactDeclaration,
	inputs []ExpectedArtifactInput,
	observation ExpectedArtifactObservation,
	templateContexts ...ExpectedArtifactTemplateContext,
) []ExpectedArtifactReadModel {
	declarations := normalizeExpectedArtifactDeclarations(
		workTypeDeclarations,
		workstationDeclarations,
	)
	if len(declarations) == 0 {
		return nil
	}

	templateContext := expectedArtifactTemplateContext(inputs, templateContexts...)
	readModels := make([]ExpectedArtifactReadModel, 0, len(declarations))
	for declarationIndex, declaration := range declarations {
		pattern, renderErr := renderExpectedArtifactPattern(declaration.Pattern, inputs, templateContext)
		if renderErr != nil {
			pattern = "<unrenderable>"
		} else if safe, ok := safeExpectedArtifactPattern(pattern); ok {
			pattern = safe
		} else {
			pattern = "<invalid>"
		}

		readModel := ExpectedArtifactReadModel{
			Name:         declaration.Name,
			Pattern:      pattern,
			NonEmpty:     declaration.NonEmpty,
			Verification: ExpectedArtifactVerificationPending,
		}
		if entry, ok := expectedArtifactFailureEntry(declarationIndex, declaration.Name, pattern, observation.Entries); ok {
			readModel.Verification = ExpectedArtifactVerificationFailed
			readModel.Reason = expectedArtifactReasonPtr(entry.Reason)
			if entry.Pattern != "" {
				readModel.Pattern = entry.Pattern
			}
		} else if observation.Verified {
			readModel.Verification = ExpectedArtifactVerificationSatisfied
		}
		readModels = append(readModels, readModel)
	}
	return readModels
}

func normalizeExpectedArtifactDeclarations(groups ...[]ExpectedArtifactDeclaration) []ExpectedArtifactDeclaration {
	var normalized []ExpectedArtifactDeclaration
	seen := make(map[ExpectedArtifactDeclaration]struct{})
	for _, group := range groups {
		for _, declaration := range group {
			if _, ok := seen[declaration]; ok {
				continue
			}
			seen[declaration] = struct{}{}
			normalized = append(normalized, declaration)
		}
	}
	return normalized
}

func safeExpectedArtifactPattern(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	portable := strings.ReplaceAll(trimmed, `\`, "/")
	if pathpkg.IsAbs(portable) || strings.HasPrefix(portable, "/") {
		return "", false
	}
	if len(portable) >= 2 && portable[1] == ':' && isASCIIAlpha(portable[0]) {
		return "", false
	}
	for _, segment := range strings.Split(portable, "/") {
		if segment == ".." {
			return "", false
		}
	}
	if _, err := pathpkg.Match(portable, ""); err != nil {
		return "", false
	}
	return portable, true
}

func expectedArtifactTemplateContext(
	inputs []ExpectedArtifactInput,
	templateContexts ...ExpectedArtifactTemplateContext,
) ExpectedArtifactTemplateContext {
	var context ExpectedArtifactTemplateContext
	if len(templateContexts) > 0 {
		context = templateContexts[0]
	}
	if strings.TrimSpace(context.Project) == "" || context.Project == defaultExpectedArtifactProject {
		for _, input := range inputs {
			if project := strings.TrimSpace(input.Project); project != "" {
				context.Project = project
				break
			}
		}
	}
	if strings.TrimSpace(context.Project) == "" {
		context.Project = defaultExpectedArtifactProject
	}
	if strings.TrimSpace(context.SessionID) == "" {
		context.SessionID = defaultExpectedArtifactSession
	}
	return context
}

func renderExpectedArtifactPattern(
	pattern string,
	inputs []ExpectedArtifactInput,
	templateContext ExpectedArtifactTemplateContext,
) (string, error) {
	parsed, err := template.New("expected_artifact").Option("missingkey=error").Parse(pattern)
	if err != nil {
		return "", err
	}
	data := struct {
		Inputs  []ExpectedArtifactInput
		Context ExpectedArtifactTemplateContext
	}{
		Inputs:  inputs,
		Context: templateContext,
	}
	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func expectedArtifactFailureEntry(
	declarationIndex int,
	name string,
	pattern string,
	entries []ExpectedArtifactVerificationEntry,
) (ExpectedArtifactVerificationEntry, bool) {
	declarationNumber := declarationIndex + 1
	for _, entry := range entries {
		if entry.DeclarationIndex == declarationNumber {
			return entry, true
		}
	}
	for _, entry := range entries {
		if entry.Name == name && entry.Pattern == pattern {
			return entry, true
		}
	}
	matchingNames := 0
	for _, entry := range entries {
		if entry.Name == name {
			matchingNames++
		}
	}
	if matchingNames == 1 {
		for _, entry := range entries {
			if entry.Name == name {
				return entry, true
			}
		}
	}
	return ExpectedArtifactVerificationEntry{}, false
}

func expectedArtifactReasonPtr(reason ExpectedArtifactVerificationReason) *ExpectedArtifactVerificationReason {
	if reason == "" {
		return nil
	}
	return &reason
}

func isASCIIAlpha(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

type ReadRelation struct {
	Type           RelationType
	SourceWorkName string
	TargetWorkName string
	TargetWorkID   string
	RequiredState  string
}

// ListResult is the plain Work-owned state-access list contract. Peers consume
// detached ReadModel projections and pagination facts without importing Work
// implementation packages.
type ListResult struct {
	Results    []ReadModel
	MaxResults int
	NextToken  string
	Counts     *ListCountSummary
}

// ListCountSummary describes the complete filtered selection before page
// slicing. It is present only when requested through ListOptions.Counts.
type ListCountSummary struct {
	Total int
}

// ReadSnapshot is the detached runtime observation consumed only by the Work
// owner. Factory Sessions adapts its live runtime into this contract, so Work
// never imports Factory Runtime and transports never observe engine types.
type ReadSnapshot struct{ Items []ReadModel }

// StopSummary is the Work-owned detached copy of the stopped-state context
// needed by a Work read. Factory Sessions remains the policy owner that
// derives it; Work prevents that owner's runtime values leaking to transports.
type StopSummary struct {
	SessionID                string
	StopKind                 string
	SessionLifecycleStatus   *string
	WorkID                   *string
	WorkName                 *string
	WorkTypeName             *string
	WorkState                *string
	LatestDispatch           *StopDispatchSummary
	LatestResultSummary      *string
	SuggestedRecoverySurface *string
	SuggestedRecoveryAction  *string
}

type StopDispatchSummary struct {
	DispatchID      string
	Status          string
	DispatchKind    string
	WorkstationName *string
	FailureDetail   *StopFailureDetail
}

type StopFailureDetail struct{ Reason, Message string }

// State is the query-owned projection of a Work state.
type State struct {
	Name string
	Type string
}

// ListOptions is the plain Work-owned state-access list request contract used by
// Service.ListWork. Filters, ordering, and pagination stay transport-independent.
type ListOptions struct {
	StateName    string
	StateType    string
	Name         string
	WorkTypeName string
	TraceID      string
	Terminal     bool
	NonTerminal  bool
	SortBy       string
	MaxResults   int
	NextToken    string
	Counts       bool
}

// PreparedListRequest is the detached, validated value returned to transport
// adapters. It contains no Work implementation or runtime reference.
type PreparedListRequest struct {
	Options       ListOptions
	FilterSummary string
}

// ListRequestPreparation is the exact Work-owned policy role used by
// transports before representing a Work list request on a protocol.
type ListRequestPreparation interface {
	PrepareListRequest(context.Context, ListOptions) (PreparedListRequest, error)
}

// ValidationError identifies the query field that failed validation. Boundary
// adapters can use Field to present transport-specific field names.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ListQuery is a validated Work-list query. Its options are immutable through
// the exported API.
type ListQuery struct {
	query stateaccessquery.ListQuery
}

// Options returns the normalized query values.
func (q ListQuery) Options() ListOptions {
	opts := q.query.Options()
	return ListOptions{
		StateName:    opts.StateName,
		StateType:    opts.StateType,
		Name:         opts.Name,
		WorkTypeName: opts.WorkTypeName,
		TraceID:      opts.TraceID,
		Terminal:     opts.Terminal,
		NonTerminal:  opts.NonTerminal,
		SortBy:       opts.SortBy,
		MaxResults:   opts.MaxResults,
		NextToken:    opts.NextToken,
		Counts:       opts.Counts,
	}
}

// FilterSummary returns the active filter and sort keys in canonical order.
func (q ListQuery) FilterSummary() string {
	return q.query.FilterSummary()
}

// NewListRequestPreparation constructs the pure Work list preparation role.
// Wire supplies this role to transport adapters; transports never select it.
func NewListRequestPreparation() ListRequestPreparation {
	return listRequestPreparationAdapter{}
}

type listRequestPreparationAdapter struct{}

func (listRequestPreparationAdapter) PrepareListRequest(
	ctx context.Context,
	options ListOptions,
) (PreparedListRequest, error) {
	prepared, err := stateaccessquery.NewListRequestPreparation().PrepareListRequest(
		ctx,
		listOptionsToQuery(options),
	)
	if err != nil {
		return PreparedListRequest{}, mapQueryValidationError(err)
	}
	return PreparedListRequest{
		Options:       listOptionsFromQuery(prepared.Options),
		FilterSummary: prepared.FilterSummary,
	}, nil
}

// NormalizeList validates options and returns their canonical values and
// active-filter summary.
func NormalizeList(options ListOptions) (ListQuery, error) {
	query, err := stateaccessquery.NormalizeList(listOptionsToQuery(options))
	if err != nil {
		return ListQuery{}, mapQueryValidationError(err)
	}
	return ListQuery{query: query}, nil
}

// ValidWorkStateType reports whether stateType is an allowed Work-list state
// type filter.
func ValidWorkStateType(stateType string) bool {
	return stateaccessquery.ValidWorkStateType(stateType)
}

func listOptionsToQuery(options ListOptions) stateaccessquery.ListOptions {
	return stateaccessquery.ListOptions{
		StateName:    options.StateName,
		StateType:    options.StateType,
		Name:         options.Name,
		WorkTypeName: options.WorkTypeName,
		TraceID:      options.TraceID,
		Terminal:     options.Terminal,
		NonTerminal:  options.NonTerminal,
		SortBy:       options.SortBy,
		MaxResults:   options.MaxResults,
		NextToken:    options.NextToken,
		Counts:       options.Counts,
	}
}

func listOptionsFromQuery(options stateaccessquery.ListOptions) ListOptions {
	return ListOptions{
		StateName:    options.StateName,
		StateType:    options.StateType,
		Name:         options.Name,
		WorkTypeName: options.WorkTypeName,
		TraceID:      options.TraceID,
		Terminal:     options.Terminal,
		NonTerminal:  options.NonTerminal,
		SortBy:       options.SortBy,
		MaxResults:   options.MaxResults,
		NextToken:    options.NextToken,
		Counts:       options.Counts,
	}
}

func mapQueryValidationError(err error) error {
	if err == nil {
		return nil
	}
	var validation *stateaccessquery.ValidationError
	if errors.As(err, &validation) {
		return &ValidationError{Field: validation.Field, Message: validation.Message}
	}
	return err
}
