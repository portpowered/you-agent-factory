package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestMapModelsRootError_ClassifiesMissingFactoryLayoutWithSearchedRoot(t *testing.T) {
	t.Parallel()

	searchedRoot := `C:\workspace\project\factory`
	cause := fmt.Errorf("resolve current factory in %s: %w", searchedRoot, factorydefinitions.ErrFactoryLayoutNotFound)
	mapped := mapModelsRootError(cause)
	if mapped == nil {
		t.Fatal("mapModelsRootError() = nil, want classified failure")
	}
	if !errors.Is(mapped, factorydefinitions.ErrFactoryLayoutNotFound) {
		t.Fatalf("mapped error = %v, want ErrFactoryLayoutNotFound cause", mapped)
	}

	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(mapped, &coded) {
		t.Fatalf("mapped error = %T, want CLI-coded failure", mapped)
	}
	if coded.CLIErrorCode() != modelsFactoryLayoutNotFoundCode ||
		coded.CLIErrorFamily() != factoryapi.ErrorFamilyNotFound ||
		!strings.Contains(coded.CLIErrorMessage(), searchedRoot) {
		t.Fatalf("coded failure = (%q, %q, %q), want not-found with searched root %q", coded.CLIErrorCode(), coded.CLIErrorFamily(), coded.CLIErrorMessage(), searchedRoot)
	}

	var output bytes.Buffer
	if !clidiag.WriteFailure(&output, mapped) {
		t.Fatal("WriteFailure() = false, want one customer-visible diagnostic")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("diagnostic JSON = %q: %v", output.String(), err)
	}
	if response.Code != factoryapi.ErrorResponseCode(modelsFactoryLayoutNotFoundCode) ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		!strings.Contains(response.Message, searchedRoot) {
		t.Fatalf("customer response = %#v, want not-found classification and searched root %q", response, searchedRoot)
	}
}
