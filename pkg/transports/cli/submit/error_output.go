package submit

import (
	"encoding/json"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const submitErrorBodyReadLimit = 64 << 10

// SubmissionHTTPError is the safe typed failure returned for a non-201 submit
// response. Arbitrary server messages are deliberately excluded because they
// may echo submitted payloads or credentials.
type SubmissionHTTPError struct {
	StatusCode int
	Code       factoryapi.ErrorResponseCode
	Family     factoryapi.ErrorFamily
	operation  string
}

func (e *SubmissionHTTPError) Error() string {
	details := make([]string, 0, 2)
	if e.Code != "" {
		details = append(details, "code="+string(e.Code))
	}
	if e.Family != "" {
		details = append(details, "family="+string(e.Family))
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

func submitFailureError(statusCode int, body []byte) error {
	return submissionFailureError("submission", statusCode, body)
}

func submissionFailureError(operation string, statusCode int, body []byte) error {
	var response struct {
		Code   factoryapi.ErrorResponseCode `json:"code"`
		Family factoryapi.ErrorFamily       `json:"family"`
	}
	_ = json.Unmarshal(body, &response)
	return &SubmissionHTTPError{
		StatusCode: statusCode,
		Code:       safeSubmitErrorCode(response.Code),
		Family:     safeSubmitErrorFamily(response.Family),
		operation:  operation,
	}
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
