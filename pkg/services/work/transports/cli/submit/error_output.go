package submit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const submitErrorBodyReadLimit = 64 << 10

const submitDiagnosticMaxLength = 4 << 10

// SubmissionHTTPError is the safe typed failure returned for a non-201 submit
// response. Arbitrary server messages are excluded; only bounded Work request
// admission diagnostics with the canonical prefix are retained.
type SubmissionHTTPError struct {
	StatusCode int
	Code       factoryapi.ErrorResponseCode
	Family     factoryapi.ErrorFamily
	Message    string
	operation  string
}

// CLIErrorFamily returns the safe public response family when the server
// supplied one. The central CLI renderer uses it only for the public envelope;
// arbitrary response fields remain filtered by submissionFailureError.
func (e *SubmissionHTTPError) CLIErrorFamily() factoryapi.ErrorFamily {
	if e == nil {
		return ""
	}
	return e.Family
}

func (e *SubmissionHTTPError) CLIErrorCode() string {
	if e == nil || e.Code == "" {
		return "SUBMISSION_FAILED"
	}
	return string(e.Code)
}

func (e *SubmissionHTTPError) CLIErrorMessage() string {
	if e == nil {
		return "submission failed"
	}
	return e.Error()
}

func (e *SubmissionHTTPError) Error() string {
	details := make([]string, 0, 2)
	if e.Code != "" {
		details = append(details, "code="+string(e.Code))
	}
	if e.Family != "" {
		details = append(details, "family="+string(e.Family))
	}
	if e.Message != "" {
		details = append(details, "message="+e.Message)
	}
	operation := e.operation
	if operation == "" {
		operation = "submission"
	}
	if len(details) == 0 {
		return fmt.Sprintf("%s failed (%d)", operation, e.StatusCode)
	}
	return fmt.Sprintf("%s failed (%d): %s", operation, e.StatusCode, strings.Join(details, " "))
}

// batchPayloadSizeError preserves the Work-owned size diagnostic for local
// --dry-run validation. It exposes only the safe Work identity and measured
// limit, while retaining the original error for callers that need its type.
type batchPayloadSizeError struct {
	label      string
	diagnostic string
	cause      error
}

func newBatchPayloadSizeError(label string, cause *work.PayloadSizeError, wrapped error) error {
	if cause == nil {
		return wrapped
	}
	return &batchPayloadSizeError{
		label:      label,
		diagnostic: cause.Error(),
		cause:      wrapped,
	}
}

func (e *batchPayloadSizeError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("parse %s: %s", e.label, e.diagnostic)
}

func (e *batchPayloadSizeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *batchPayloadSizeError) CLIErrorCode() string {
	if e == nil {
		return ""
	}
	return string(factoryapi.ErrorResponseCodeBADREQUEST)
}

func (e *batchPayloadSizeError) CLIErrorMessage() string {
	if e == nil {
		return ""
	}
	return e.diagnostic
}

func (e *batchPayloadSizeError) CLIErrorFamily() factoryapi.ErrorFamily {
	if e == nil {
		return ""
	}
	return factoryapi.ErrorFamilyBadRequest
}

func submitFailureError(statusCode int, body []byte) error {
	return submissionFailureError("submission", statusCode, body)
}

func submissionFailureError(operation string, statusCode int, body []byte) error {
	var response struct {
		Message string                       `json:"message"`
		Code    factoryapi.ErrorResponseCode `json:"code"`
		Family  factoryapi.ErrorFamily       `json:"family"`
	}
	_ = json.Unmarshal(body, &response)
	return &SubmissionHTTPError{
		StatusCode: statusCode,
		Code:       safeSubmitErrorCode(response.Code),
		Family:     safeSubmitErrorFamily(response.Family),
		Message:    safeSubmitErrorMessage(response.Message),
		operation:  operation,
	}
}

func safeSubmitErrorMessage(value string) string {
	message := strings.TrimSpace(value)
	if !strings.HasPrefix(message, "work_request:") {
		return ""
	}
	if len(message) > submitDiagnosticMaxLength {
		return message[:submitDiagnosticMaxLength]
	}
	return message
}

func safeSubmitErrorFamily(value factoryapi.ErrorFamily) factoryapi.ErrorFamily {
	allowed := []factoryapi.ErrorFamily{
		factoryapi.ErrorFamilyBadRequest,
		factoryapi.ErrorFamilyConflict,
		factoryapi.ErrorFamilyGone,
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorFamilyNotFound,
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return ""
}

func safeSubmitErrorCode(value factoryapi.ErrorResponseCode) factoryapi.ErrorResponseCode {
	allowed := []factoryapi.ErrorResponseCode{
		factoryapi.ErrorResponseCodeBADREQUEST,
		factoryapi.ErrorResponseCodeEXECUTIONREQUESTIDCONFLICT,
		factoryapi.ErrorResponseCodeFACTORYALREADYEXISTS,
		factoryapi.ErrorResponseCodeFACTORYNOTIDLE,
		factoryapi.ErrorResponseCodeFACTORYSESSIONCONFIGLOADFAILED,
		factoryapi.ErrorResponseCodeFACTORYSESSIONCONTROLREQUESTALREADYAPPLIED,
		factoryapi.ErrorResponseCodeINTERNALERROR,
		factoryapi.ErrorResponseCodeINVALIDFACTORY,
		factoryapi.ErrorResponseCodeINVALIDFACTORYNAME,
		factoryapi.ErrorResponseCodeINVALIDRESPONSEEVENTCURSOR,
		factoryapi.ErrorResponseCodeINVALIDRESPONSEEVENTFILTER,
		factoryapi.ErrorResponseCodeMOVEWORKREQUESTALREADYAPPLIED,
		factoryapi.ErrorResponseCodeNOTFOUND,
		factoryapi.ErrorResponseCodeRESPONSEEVENTSESSIONNOTFOUND,
		factoryapi.ErrorResponseCodeRESPONSEEVENTSTREAMEXPIRED,
		factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return ""
}
