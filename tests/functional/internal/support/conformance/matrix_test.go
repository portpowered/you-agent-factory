package conformance

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestBuildGenericCatalogExpandsStableRequiredVariants(t *testing.T) {
	matrix := Build(models.GenericOperationCatalog{})
	if len(matrix.Rows) != 7 {
		t.Fatalf("generic matrix rows = %d, want 7", len(matrix.Rows))
	}

	wantLabels := []string{
		"OMNI/prompt-only",
		"OMNI/single-image",
		"OMNI/multiple-image",
		"OMNI/video",
		"EMBED/text",
		"TTS/text",
		"ASR/audio",
	}
	for index, want := range wantLabels {
		row := matrix.Rows[index]
		if row.Label != want {
			t.Fatalf("row[%d] label = %q, want %q", index, row.Label, want)
		}
		if !row.IsContractSupported() {
			t.Fatalf("row[%d] contract = %#v, want supported", index, row.ContractError)
		}
	}

	assertInput(t, matrix.Rows[0].Inputs, []inputExpectation{{"prompt", models.ModalityText, "text/plain", "fixture prompt"}})
	assertInput(t, matrix.Rows[1].Inputs, []inputExpectation{
		{"prompt", models.ModalityText, "text/plain", "fixture prompt"},
		{"image", models.ModalityImage, "image/png", "fixture image first"},
	})
	assertInput(t, matrix.Rows[2].Inputs, []inputExpectation{
		{"prompt", models.ModalityText, "text/plain", "fixture prompt"},
		{"image", models.ModalityImage, "image/png", "fixture image first"},
		{"image", models.ModalityImage, "image/jpeg", "fixture image second"},
	})
	assertInput(t, matrix.Rows[3].Inputs, []inputExpectation{
		{"prompt", models.ModalityText, "text/plain", "fixture prompt"},
		{"video", models.ModalityVideo, "video/mp4", "fixture video"},
	})
	assertInput(t, matrix.Rows[4].Inputs, []inputExpectation{{"text", models.ModalityText, "text/plain", "fixture text"}})
	assertInput(t, matrix.Rows[5].Inputs, []inputExpectation{{"text", models.ModalityText, "text/plain", "fixture text"}})
	assertInput(t, matrix.Rows[6].Inputs, []inputExpectation{{"audio", models.ModalityAudio, "audio/wav", "fixture audio"}})

	if matrix.Rows[2].Inputs[1].Name != matrix.Rows[2].Inputs[2].Name ||
		matrix.Rows[2].Inputs[1].Content == matrix.Rows[2].Inputs[2].Content {
		t.Fatalf("multiple-image inputs = %#v, want two ordered distinct image values", matrix.Rows[2].Inputs)
	}
}

func TestBuildClonesCatalogContractsAndRetainsNewOperationRows(t *testing.T) {
	required := true
	catalog := testCatalog{operations: []models.Operation{
		{
			Name: "DEPTH",
			Inputs: []models.OperationSlot{{
				Name: "document", Modality: models.ModalityBinary, Required: &required,
				ContentTypes: []string{"BINARY"}, MediaTypes: []string{"application/octet-stream"},
			}},
			Outputs: []models.OperationSlot{{Name: "result", Modality: models.ModalityBinary}},
		},
	}}
	matrix := Build(catalog)
	if len(matrix.Rows) != 1 {
		t.Fatalf("new operation rows = %d, want one explicit default row", len(matrix.Rows))
	}
	row := matrix.Rows[0]
	if row.Label != "DEPTH/default" || row.Variant != VariantDefault || row.IsContractSupported() {
		t.Fatalf("new operation row = %#v, want DEPTH/default unsupported contract", row)
	}
	if row.ContractError == nil || !strings.Contains(row.ContractError.Error(), "no conformance contract") {
		t.Fatalf("new operation contract error = %v, want actionable unsupported-contract error", row.ContractError)
	}
	assertInput(t, row.Inputs, []inputExpectation{{"document", models.ModalityBinary, "application/octet-stream", "fixture binary 1"}})

	matrix.Rows[0].Operation.Inputs[0].MediaTypes[0] = "mutated/type"
	matrix.Rows[0].Inputs[0].Content = "mutated content"
	fresh := Build(catalog)
	if fresh.Rows[0].Operation.Inputs[0].MediaTypes[0] != "application/octet-stream" ||
		fresh.Rows[0].Inputs[0].Content != "fixture binary 1" {
		t.Fatalf("catalog-derived row retained caller mutation: %#v", fresh.Rows[0])
	}
}

func TestClassifyUsesTypedPublicOutcomes(t *testing.T) {
	matrix := Build(models.GenericOperationCatalog{})
	implemented := Classify(matrix.Rows[0], InvocationOutcome{
		Result: models.GenericInvocationResult{Status: models.ModelInvocationStatusCompleted},
	})
	if implemented.Classification != ClassificationImplemented || implemented.Err != nil {
		t.Fatalf("completed outcome = %#v, want implemented", implemented)
	}

	expected := Classify(matrix.Rows[1], InvocationOutcome{Err: models.ErrUnsupportedOperation})
	if expected.Classification != ClassificationExpectedUnimplemented || !errors.Is(expected.Err, models.ErrUnsupportedOperation) {
		t.Fatalf("unsupported outcome = %#v, want expected-unimplemented", expected)
	}

	legacyExpected := Classify(matrix.Rows[2], InvocationOutcome{Err: &models.InferenceFailure{
		Class: models.InferenceFailureClassUnsupportedOperation, Message: "operation is not implemented",
	}})
	if legacyExpected.Classification != ClassificationExpectedUnimplemented {
		t.Fatalf("typed legacy unsupported outcome = %#v, want expected-unimplemented", legacyExpected)
	}

	unrelated := Classify(matrix.Rows[3], InvocationOutcome{Err: &models.InvocationFailure{
		Class: models.InvocationFailureClassBackendProtocol, Message: "protocol mismatch",
	}})
	if unrelated.Classification != ClassificationUnexpectedFailure || unrelated.Err == nil {
		t.Fatalf("protocol outcome = %#v, want unexpected-failure", unrelated)
	}

	malformedResult := Classify(matrix.Rows[4], InvocationOutcome{})
	if malformedResult.Classification != ClassificationUnexpectedFailure || malformedResult.Err == nil {
		t.Fatalf("empty outcome = %#v, want unexpected-failure", malformedResult)
	}

	unsupportedContract := Classify(matrix.Rows[0].withUnsupportedContractForTest(), InvocationOutcome{})
	if unsupportedContract.Classification != ClassificationExpectedUnimplemented || unsupportedContract.Err == nil {
		t.Fatalf("unsupported contract outcome = %#v, want explicit expected-unimplemented", unsupportedContract)
	}
}

func TestRunAccountingAndStrictModePrintEveryDeclaredRow(t *testing.T) {
	matrix := Build(models.GenericOperationCatalog{})
	executor := func(row Row) (models.GenericInvocationResult, error) {
		if row.Label == "OMNI/prompt-only" {
			return models.GenericInvocationResult{Status: models.ModelInvocationStatusCompleted}, nil
		}
		return models.GenericInvocationResult{}, models.ErrUnsupportedModelOperation
	}

	accounting, err := matrix.Run(executor, ModeAccounting)
	if err != nil {
		t.Fatalf("accounting run error = %v, want nil", err)
	}
	if accounting.ImplementedCount() != 1 || accounting.ExpectedUnimplementedCount() != 6 ||
		accounting.UnexpectedFailureCount() != 0 || accounting.UnimplementedCount() != 6 {
		t.Fatalf("accounting report counts = implemented:%d expected:%d unexpected:%d unimplemented:%d, want 1/6/0/6",
			accounting.ImplementedCount(), accounting.ExpectedUnimplementedCount(), accounting.UnexpectedFailureCount(), accounting.UnimplementedCount())
	}
	var output strings.Builder
	if _, err := accounting.WriteTo(&output); err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}
	printed := output.String()
	for _, row := range matrix.Rows {
		if !strings.Contains(printed, row.Label+" classification=") {
			t.Fatalf("report output = %q, missing row %q", printed, row.Label)
		}
	}
	if !strings.Contains(printed, "implemented=1 unimplemented=6 of 7 declared pairs\n") {
		t.Fatalf("report output = %q, want exact summary line", printed)
	}

	strict, err := matrix.Run(executor, ModeStrict)
	if !errors.Is(err, ErrStrictConformance) {
		t.Fatalf("strict error = %v, want ErrStrictConformance", err)
	}
	if strict.SummaryLine() != accounting.SummaryLine() {
		t.Fatalf("strict summary = %q, accounting summary = %q, want same inventory", strict.SummaryLine(), accounting.SummaryLine())
	}
}

func TestRunRejectsInvalidModeAndNilExecutor(t *testing.T) {
	matrix := Build(models.GenericOperationCatalog{})
	if _, err := matrix.Run(func(Row) (models.GenericInvocationResult, error) { return models.GenericInvocationResult{}, nil }, Mode("unknown")); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("invalid mode error = %v, want ErrInvalidMode", err)
	}
	if _, err := matrix.Run(nil, ModeAccounting); !errors.Is(err, ErrNilExecutor) {
		t.Fatalf("nil executor error = %v, want ErrNilExecutor", err)
	}
}

type inputExpectation struct {
	name     string
	modality models.Modality
	media    string
	content  string
}

func assertInput(t *testing.T, inputs []models.InferenceInput, want []inputExpectation) {
	t.Helper()
	if len(inputs) != len(want) {
		t.Fatalf("inputs = %#v, want %d inputs", inputs, len(want))
	}
	for index, expected := range want {
		got := inputs[index]
		if got.Name != expected.name || got.Modality != expected.modality || got.MediaType != expected.media || got.Content != expected.content {
			t.Fatalf("input[%d] = %#v, want name=%q modality=%q media=%q content=%q", index, got, expected.name, expected.modality, expected.media, expected.content)
		}
	}
}

type testCatalog struct {
	operations []models.Operation
}

func (catalog testCatalog) GenericOperationContracts() []models.Operation {
	return catalog.operations
}

func (row Row) withUnsupportedContractForTest() Row {
	row.ContractStatus = ContractUnsupported
	row.ContractError = &UnsupportedContractError{Operation: row.Operation.Name, Variant: row.Variant, Message: "test unsupported contract"}
	return row
}
