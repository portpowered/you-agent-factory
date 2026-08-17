package models

// InferenceFailureClass is the stable customer-facing inference failure
// category. It classifies why one model operation could not produce a result,
// and outward adapters branch on it to choose a customer-facing status.
type InferenceFailureClass string

const (
	InferenceFailureClassMissingModel         InferenceFailureClass = "missing_model"
	InferenceFailureClassLoadingModel         InferenceFailureClass = "loading_model"
	InferenceFailureClassUnsupportedOperation InferenceFailureClass = "unsupported_operation"
	InferenceFailureClassTimeout              InferenceFailureClass = "timeout"
	InferenceFailureClassRuntimeFailure       InferenceFailureClass = "runtime_failure"
)

// InferenceFailure is the detached, customer-safe outcome of inference failure
// classification. It is the classified counterpart of TargetError: TargetError
// retains identity around an unclassified cause, and InferenceFailure carries
// the same identity plus the settled category and public message.
//
// The value lives with the Models inference vocabulary (Request, Result,
// ErrMissing, ErrLoading, ErrUnsupported, TargetError) rather than with the
// service that performs the classification, so that every producer and every
// outward adapter can name one type without depending on each other.
type InferenceFailure struct {
	Class      InferenceFailureClass
	Message    string
	ModelName  string
	WorkerName string
	Operation  string
	Cause      error
}

func (e *InferenceFailure) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *InferenceFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
