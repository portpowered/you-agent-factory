package cli

import (
	"errors"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type asrCLICodedError interface {
	error
	CLIErrorCode() string
	CLIErrorFamily() factoryapi.ErrorFamily
	CLIErrorMessage() string
}

func TestModelsCLIMapsTypedASRMalformedFailureToSpecificMessage(t *testing.T) {
	t.Parallel()

	failure := &modelinference.InvocationFailure{
		Class:     modelinference.InvocationFailureClassMalformedResponse,
		Operation: modelinference.OperationASR,
		Slot:      "transcript",
		Message:   "ASR backend response is missing the transcript",
		Cause:     modelinference.ErrInferenceFailed,
	}
	mapped, ok := mapModelsInvocationError(failure)
	if !ok {
		t.Fatal("mapModelsInvocationError() ok = false, want true for typed ASR failure")
	}
	var coded asrCLICodedError
	if !errors.As(mapped, &coded) {
		t.Fatalf("mapped error %v does not expose CLI diagnostics", mapped)
	}
	if coded.CLIErrorCode() != modelsMalformedResponseCode || coded.CLIErrorFamily() != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("CLI fields = (%q, %q), want (%q, internal)", coded.CLIErrorCode(), coded.CLIErrorFamily(), modelsMalformedResponseCode)
	}
	if coded.CLIErrorMessage() != failure.Message || !strings.Contains(coded.CLIErrorMessage(), "missing the transcript") {
		t.Fatalf("CLIErrorMessage() = %q, want specific transcript diagnostic %q", coded.CLIErrorMessage(), failure.Message)
	}
	if !errors.Is(mapped, modelinference.ErrInferenceFailed) {
		t.Fatalf("mapped error = %v, want ErrInferenceFailed preserved in cause chain", mapped)
	}
}
