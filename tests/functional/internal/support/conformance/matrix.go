package conformance

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// OperationCatalog is the narrow catalog capability needed to build an
// inventory. models.GenericOperationCatalog satisfies it directly; the
// interface also lets tests prove that a newly declared operation is not
// dropped by a closed operation allowlist.
type OperationCatalog interface {
	GenericOperationContracts() []models.Operation
}

// ContractStatus records whether the matrix knows how to construct a
// semantically meaningful request for a row.
type ContractStatus string

const (
	ContractSupported   ContractStatus = "supported"
	ContractUnsupported ContractStatus = "unsupported-contract"
)

// Classification is the stable row outcome used by accounting and strict
// runs. Expected-unimplemented is an observed, reportable result; it is never
// represented by omitting a row or calling testing.T.Skip.
type Classification string

const (
	ClassificationImplemented           Classification = "implemented"
	ClassificationExpectedUnimplemented Classification = "expected-unimplemented"
	ClassificationUnexpectedFailure     Classification = "unexpected-failure"
)

// Mode controls whether a run only accounts for gaps or also returns a strict
// failure when any declared pair did not implement successfully.
type Mode string

const (
	ModeAccounting Mode = "accounting"
	ModeStrict     Mode = "strict"
)

// Variant names are deliberately stable because they appear in functional
// output and make a newly missing multimodal case easy to identify.
const (
	VariantText          = "text"
	VariantPromptOnly    = "prompt-only"
	VariantSingleImage   = "single-image"
	VariantMultipleImage = "multiple-image"
	VariantAudio         = "audio"
	VariantVideo         = "video"
	VariantDefault       = "default"
)

const (
	fixturePrompt      = "fixture prompt"
	fixtureText        = "fixture text"
	fixtureAudio       = "fixture audio"
	fixtureImageFirst  = "fixture image first"
	fixtureImageSecond = "fixture image second"
	fixtureVideo       = "fixture video"
)

var (
	// ErrStrictConformance reports that a strict run found one or more rows
	// which were not implemented successfully.
	ErrStrictConformance = errors.New("strict conformance run failed")
	// ErrInvalidMode reports a mode outside the accounting/strict vocabulary.
	ErrInvalidMode = errors.New("invalid conformance run mode")
	// ErrNilExecutor reports a run that cannot observe any supported row.
	ErrNilExecutor = errors.New("conformance executor is nil")
)

// UnsupportedContractError is the explicit typed outcome for a catalog
// operation whose shape is known to the Models catalog but not yet covered by
// this matrix's request and semantic contract.
type UnsupportedContractError struct {
	Operation string
	Variant   string
	Message   string
}

func (err *UnsupportedContractError) Error() string {
	if err == nil {
		return ""
	}
	if strings.TrimSpace(err.Message) != "" {
		return err.Message
	}
	return fmt.Sprintf("conformance contract is unsupported for %s/%s", err.Operation, err.Variant)
}

// Row is one catalog-derived operation/variant pair. Operation is cloned from
// the public Models contract. Inputs are detached request templates for this
// variant, preserving repeated-input order.
type Row struct {
	Label          string
	Operation      models.Operation
	Variant        string
	Inputs         []models.InferenceInput
	ContractStatus ContractStatus
	ContractError  *UnsupportedContractError
}

// IsContractSupported reports whether the row has a request shape that this
// matrix can exercise.
func (row Row) IsContractSupported() bool {
	return row.ContractStatus == ContractSupported && row.ContractError == nil
}

// Clone returns a detached row safe for a caller to retain or mutate.
func (row Row) Clone() Row {
	row.Operation = row.Operation.Clone()
	row.Inputs = append([]models.InferenceInput(nil), row.Inputs...)
	if row.ContractError != nil {
		contractError := *row.ContractError
		row.ContractError = &contractError
	}
	return row
}

// Matrix is the ordered inventory generated from one operation catalog.
type Matrix struct {
	Rows []Row
}

// Clone returns a detached matrix.
func (matrix Matrix) Clone() Matrix {
	cloned := Matrix{Rows: make([]Row, len(matrix.Rows))}
	for index, row := range matrix.Rows {
		cloned.Rows[index] = row.Clone()
	}
	return cloned
}

// Build expands every operation returned by catalog. The canonical four
// operations produce seven rows: four OMNI variants plus EMBED text, TTS text,
// and ASR audio. Any other declared operation produces a named default row
// with an explicit unsupported-contract error instead of disappearing.
func Build(catalog OperationCatalog) Matrix {
	if catalog == nil {
		return Matrix{}
	}

	rows := make([]Row, 0)
	labels := make(map[string]int)
	for _, operation := range catalog.GenericOperationContracts() {
		for _, row := range expandOperation(operation) {
			row.Label = uniqueLabel(row.Label, labels)
			rows = append(rows, row)
		}
	}
	return Matrix{Rows: rows}
}

func expandOperation(operation models.Operation) []Row {
	operation = operation.Clone()
	name := normalizedOperationName(operation.Name)
	operation.Name = name

	switch name {
	case models.OperationOMNI:
		return []Row{
			buildKnownRow(operation, VariantPromptOnly, []inputRequest{{name: "prompt", modality: models.ModalityText, content: fixturePrompt}}),
			buildKnownRow(operation, VariantSingleImage, []inputRequest{
				{name: "prompt", modality: models.ModalityText, content: fixturePrompt},
				{name: "image", modality: models.ModalityImage, mediaType: "image/png", content: fixtureImageFirst},
			}),
			buildKnownRow(operation, VariantMultipleImage, []inputRequest{
				{name: "prompt", modality: models.ModalityText, content: fixturePrompt},
				{name: "image", modality: models.ModalityImage, mediaType: "image/png", content: fixtureImageFirst},
				{name: "image", modality: models.ModalityImage, mediaType: "image/jpeg", content: fixtureImageSecond},
			}),
			buildKnownRow(operation, VariantVideo, []inputRequest{
				{name: "prompt", modality: models.ModalityText, content: fixturePrompt},
				{name: "video", modality: models.ModalityVideo, mediaType: "video/mp4", content: fixtureVideo},
			}),
		}
	case models.OperationEMBED:
		return []Row{buildKnownRow(operation, VariantText, []inputRequest{{name: "text", modality: models.ModalityText, content: fixtureText}})}
	case models.OperationTTS:
		return []Row{buildKnownRow(operation, VariantText, []inputRequest{{name: "text", modality: models.ModalityText, content: fixtureText}})}
	case models.OperationASR:
		return []Row{buildKnownRow(operation, VariantAudio, []inputRequest{{name: "audio", modality: models.ModalityAudio, mediaType: "audio/wav", content: fixtureAudio}})}
	default:
		return []Row{buildUnsupportedDefaultRow(operation)}
	}
}

type inputRequest struct {
	name      string
	modality  models.Modality
	mediaType string
	content   string
}

type outputRequirement struct {
	name     string
	modality models.Modality
}

func buildKnownRow(operation models.Operation, variant string, requests []inputRequest) Row {
	row := Row{
		Label:          normalizedOperationName(operation.Name) + "/" + variant,
		Operation:      operation.Clone(),
		Variant:        variant,
		ContractStatus: ContractSupported,
	}

	for _, request := range requests {
		slot, ok := operationSlot(operation.Inputs, request.name)
		if !ok {
			return markUnsupported(row, "required input slot %q is absent", request.name)
		}
		if slot.Modality != request.modality {
			return markUnsupported(row, "input slot %q has modality %q, want %q", request.name, slot.Modality, request.modality)
		}
		if request.name == "image" && variant == VariantMultipleImage && !slot.Repeatable {
			return markUnsupported(row, "input slot %q is not repeatable for multiple-image", request.name)
		}
		row.Inputs = append(row.Inputs, inputFromSlot(slot, request))
	}

	if err := validateKnownOutputs(operation, variant); err != nil {
		row.ContractStatus = ContractUnsupported
		row.ContractError = &UnsupportedContractError{
			Operation: operation.Name,
			Variant:   variant,
			Message:   err.Error(),
		}
	}
	return row
}

func buildUnsupportedDefaultRow(operation models.Operation) Row {
	row := Row{
		Label:          normalizedOperationName(operation.Name) + "/" + VariantDefault,
		Operation:      operation.Clone(),
		Variant:        VariantDefault,
		ContractStatus: ContractUnsupported,
		ContractError: &UnsupportedContractError{
			Operation: normalizedOperationName(operation.Name),
			Variant:   VariantDefault,
			Message:   fmt.Sprintf("no conformance contract is declared for catalog operation %q", normalizedOperationName(operation.Name)),
		},
	}

	// Retain detached required-input templates even for an unsupported row.
	// This makes the missing contract actionable and proves the new operation
	// was discovered from its public catalog shape.
	for index, slot := range operation.Inputs {
		if !slotRequired(slot) {
			continue
		}
		row.Inputs = append(row.Inputs, inputFromSlot(slot, inputRequest{
			name:      slot.Name,
			modality:  slot.Modality,
			mediaType: defaultMediaType(slot.Modality, index),
			content:   defaultContent(slot.Modality, index),
		}))
	}
	return row
}

func validateKnownOutputs(operation models.Operation, variant string) error {
	want := []outputRequirement{}
	switch normalizedOperationName(operation.Name) {
	case models.OperationOMNI:
		want = append(want, outputRequirement{name: "text", modality: models.ModalityText})
	case models.OperationEMBED:
		want = append(want, outputRequirement{name: "embedding", modality: models.ModalityJSON})
	case models.OperationTTS:
		want = append(want, outputRequirement{name: "audio", modality: models.ModalityAudio})
	case models.OperationASR:
		want = append(want,
			outputRequirement{name: "transcript", modality: models.ModalityText},
			outputRequirement{name: "segments", modality: models.ModalityJSON},
		)
	default:
		return fmt.Errorf("no output contract is declared for catalog operation %q/%s", operation.Name, variant)
	}
	for _, expected := range want {
		slot, ok := operationSlot(operation.Outputs, expected.name)
		if !ok {
			return fmt.Errorf("required output slot %q is absent for %s/%s", expected.name, operation.Name, variant)
		}
		if slot.Modality != expected.modality {
			return fmt.Errorf("output slot %q has modality %q, want %q for %s/%s", expected.name, slot.Modality, expected.modality, operation.Name, variant)
		}
	}
	return nil
}

func markUnsupported(row Row, format string, values ...any) Row {
	row.ContractStatus = ContractUnsupported
	row.ContractError = &UnsupportedContractError{
		Operation: row.Operation.Name,
		Variant:   row.Variant,
		Message:   fmt.Sprintf(format, values...),
	}
	return row
}

func inputFromSlot(slot models.OperationSlot, request inputRequest) models.InferenceInput {
	contentType := firstValue(slot.ContentTypes, string(slot.Modality))
	mediaType := request.mediaType
	if mediaType == "" {
		mediaType = defaultMediaType(slot.Modality, 0)
	}
	content := request.content
	if content == "" {
		content = defaultContent(slot.Modality, 0)
	}
	return models.InferenceInput{
		Name:        slot.Name,
		Modality:    slot.Modality,
		ContentType: contentType,
		MediaType:   mediaType,
		Content:     content,
	}
}

func operationSlot(slots []models.OperationSlot, name string) (models.OperationSlot, bool) {
	for _, slot := range slots {
		if slot.Name == name {
			return slot, true
		}
	}
	return models.OperationSlot{}, false
}

func slotRequired(slot models.OperationSlot) bool {
	return slot.Required != nil && *slot.Required
}

func defaultMediaType(modality models.Modality, _ int) string {
	switch modality {
	case models.ModalityText:
		return "text/plain"
	case models.ModalityImage:
		return "image/png"
	case models.ModalityAudio:
		return "audio/wav"
	case models.ModalityVideo:
		return "video/mp4"
	case models.ModalityJSON:
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

func defaultContent(modality models.Modality, index int) string {
	return fmt.Sprintf("fixture %s %d", strings.ToLower(string(modality)), index+1)
}

func firstValue(values []string, fallback string) string {
	if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return values[0]
	}
	return fallback
}

func normalizedOperationName(name string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return "UNNAMED_OPERATION"
	}
	return name
}

func uniqueLabel(label string, seen map[string]int) string {
	seen[label]++
	if seen[label] == 1 {
		return label
	}
	return fmt.Sprintf("%s#%d", label, seen[label])
}

// InvocationOutcome is the public result/error pair observed for one row.
// Callers should pass the Models result and typed public error returned by a
// real surface; this package does not inspect private runtime state.
type InvocationOutcome struct {
	Result models.GenericInvocationResult
	Err    error
}

// RowResult records the classification for one matrix row.
type RowResult struct {
	Row            Row
	Classification Classification
	Err            error
}

// Classify maps a typed public invocation outcome to one stable row state.
func Classify(row Row, outcome InvocationOutcome) RowResult {
	if !row.IsContractSupported() {
		return RowResult{
			Row:            row.Clone(),
			Classification: ClassificationExpectedUnimplemented,
			Err:            row.ContractError,
		}
	}
	if outcome.Err == nil && outcome.Result.Status == models.ModelInvocationStatusCompleted {
		return RowResult{Row: row.Clone(), Classification: ClassificationImplemented}
	}
	if isExpectedUnimplemented(outcome.Err) {
		return RowResult{
			Row:            row.Clone(),
			Classification: ClassificationExpectedUnimplemented,
			Err:            outcome.Err,
		}
	}
	if outcome.Err == nil {
		outcome.Err = errors.New("invocation did not return COMPLETED status")
	}
	return RowResult{
		Row:            row.Clone(),
		Classification: ClassificationUnexpectedFailure,
		Err:            outcome.Err,
	}
}

func isExpectedUnimplemented(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, models.ErrUnsupportedOperation) || errors.Is(err, models.ErrUnsupportedModelOperation) {
		return true
	}
	var inferenceFailure *models.InferenceFailure
	if errors.As(err, &inferenceFailure) && inferenceFailure != nil {
		return inferenceFailure.Class == models.InferenceFailureClassUnsupportedOperation
	}
	return false
}

// Executor observes one supported row through the caller-selected public
// surface(s). A later parity runner can combine HTTP and CLI observations
// before returning the one result/error pair required here.
type Executor func(Row) (models.GenericInvocationResult, error)

// Report is the complete, ordered output of a matrix run.
type Report struct {
	Results []RowResult
}

// Run executes every supported row and records every unsupported-contract row
// explicitly. Accounting mode returns the report even when gaps exist;
// strict mode returns the same report plus ErrStrictConformance.
func (matrix Matrix) Run(executor Executor, mode Mode) (Report, error) {
	if mode != ModeAccounting && mode != ModeStrict {
		return Report{}, fmt.Errorf("%w: %q", ErrInvalidMode, mode)
	}
	if executor == nil {
		return Report{}, ErrNilExecutor
	}

	report := Report{Results: make([]RowResult, 0, len(matrix.Rows))}
	for _, row := range matrix.Rows {
		if !row.IsContractSupported() {
			report.Results = append(report.Results, Classify(row, InvocationOutcome{}))
			continue
		}
		result, err := executor(row)
		report.Results = append(report.Results, Classify(row, InvocationOutcome{Result: result, Err: err}))
	}
	if mode == ModeStrict {
		return report, report.StrictError()
	}
	return report, nil
}

// ImplementedCount returns the number of rows with a completed public result.
func (report Report) ImplementedCount() int {
	count := 0
	for _, result := range report.Results {
		if result.Classification == ClassificationImplemented {
			count++
		}
	}
	return count
}

// ExpectedUnimplementedCount returns rows that reached a typed expected gap
// or an explicitly unsupported matrix contract.
func (report Report) ExpectedUnimplementedCount() int {
	count := 0
	for _, result := range report.Results {
		if result.Classification == ClassificationExpectedUnimplemented {
			count++
		}
	}
	return count
}

// UnexpectedFailureCount returns rows that failed without the expected
// unsupported-operation classification.
func (report Report) UnexpectedFailureCount() int {
	count := 0
	for _, result := range report.Results {
		if result.Classification == ClassificationUnexpectedFailure {
			count++
		}
	}
	return count
}

// UnimplementedCount returns every row that did not implement successfully.
// It includes unexpected failures so the summary reconciles exactly to the
// total declared-pair count while row labels retain the finer classification.
func (report Report) UnimplementedCount() int {
	return len(report.Results) - report.ImplementedCount()
}

// SummaryLine returns the stable inventory summary required by functional
// output consumers.
func (report Report) SummaryLine() string {
	return fmt.Sprintf(
		"implemented=%d unimplemented=%d of %d declared pairs",
		report.ImplementedCount(), report.UnimplementedCount(), len(report.Results),
	)
}

// StrictError returns nil for a fully implemented report and a stable wrapped
// error otherwise. The report remains available to print the complete gap
// inventory even when this error is returned.
func (report Report) StrictError() error {
	if report.UnimplementedCount() == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrStrictConformance, report.SummaryLine())
}

// WriteTo emits one stable labelled row followed by the exact summary line.
// Its signature follows io.WriterTo so callers can use it with standard
// streaming helpers without a package-specific adapter.
func (report Report) WriteTo(writer io.Writer) (int64, error) {
	var written int64
	for _, result := range report.Results {
		count, err := fmt.Fprintf(writer, "%s classification=%s\n", result.Row.Label, result.Classification)
		written += int64(count)
		if err != nil {
			return written, err
		}
	}
	count, err := fmt.Fprintln(writer, report.SummaryLine())
	written += int64(count)
	return written, err
}
