package factorysessions

import (
	"errors"
	"fmt"
	"io"

	"go.uber.org/zap"
)

// ProviderIdentityResolver resolves one authored provider selection through
// the immutable process registry without exposing a second service interface.
type ProviderIdentityResolver func(string) (string, error)

// ExecutionRuntimeOpeningRequest carries invocation-edge roots required to
// open a runtime-backed durable execution service without ambient discovery.
type ExecutionRuntimeOpeningRequest struct {
	ProjectRoot      string
	SystemConfigHome string
}

// StdioOpeningRequest carries only invocation-edge values into the Factory
// Sessions-owned stdio opening policy.
type StdioOpeningRequest struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	SystemConfigHome   string
	Input              io.Reader
	Output             io.Writer
}

// DirectJavaScriptRunRequest carries customer-edge values for one raw
// JavaScript workflow invocation. Source resolution and execution policy stay
// behind DirectJavaScriptRunOperation.
type DirectJavaScriptRunRequest struct {
	SourcePath          string
	MockWorkersEnabled  bool
	JSONOutput          bool
	Output              io.Writer
	Host                *RuntimeHostRequest
	RuntimeHostObserver RuntimeHostObserver
	Logger              *zap.Logger
}

// --- merged from opening_contract.go ---

// Opening/binding root slice freezes ForRuntime binding vocabulary on the
// singular Service. Peers bind already-constructed process-scoped capabilities
// through plain request/result/error contracts without downcasting or importing
// nested opening, assembly, or runtime-opening interfaces:
//
//   - Binding request: OpeningBindingRequest (ForRuntime / RuntimeBinding)
//   - Binding result: OpeningBindingResult carrying a usable root Service view
//   - Typed failure: *OpeningBindingError / ErrOpeningBindingInvalid
//
// Characterization requires binding to stay inert: no listeners or runtimes are
// started while constructing the bound Service view.

// OpeningBindingRequest is the plain root runtime-binding input accepted by
// Service.ForRuntime. Process-scoped collaborators remain on the injected
// Service; this value carries only per-runtime selections such as Clock.
type OpeningBindingRequest = RuntimeBinding

// OpeningBindingResult is the plain root success shape for one ForRuntime
// binding. Service is the usable aggregate session authority view; peers must
// not expect nested opening interfaces to be bundled here.
type OpeningBindingResult struct {
	Service Service
}

// OpeningBindingError is the typed construction/opening failure published on
// the opening/binding root slice. Peers match it with errors.As and may also
// use errors.Is against ErrOpeningBindingInvalid.
type OpeningBindingError struct {
	Field   string
	Message string
}

func (err *OpeningBindingError) Error() string {
	if err == nil {
		return "opening binding error"
	}
	if err.Field == "" {
		return err.Message
	}
	if err.Message == "" {
		return fmt.Sprintf("opening binding failed for %s", err.Field)
	}
	return fmt.Sprintf("%s: %s", err.Field, err.Message)
}

func (err *OpeningBindingError) Is(target error) bool {
	return target == ErrOpeningBindingInvalid
}

// ErrOpeningBindingInvalid reports that required ForRuntime binding inputs were
// missing or otherwise rejected during inert construction/opening.
var ErrOpeningBindingInvalid = errors.New("factory session opening binding is invalid")
