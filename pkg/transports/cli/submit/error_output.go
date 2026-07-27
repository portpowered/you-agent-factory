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
	WorkID     string
}

func (e *SubmissionHTTPError) Error() string {
	details := make([]string, 0, 3)
	if e.Code != "" {
		details = append(details, "code="+string(e.Code))
	}
	if e.Family != "" {
		details = append(details, "family="+string(e.Family))
	}
	if e.WorkID != "" {
		details = append(details, "workId="+e.WorkID)
	}
	if len(details) == 0 {
		return fmt.Sprintf("submission failed (%d)", e.StatusCode)
	}
	return fmt.Sprintf("submission failed (%d): %s", e.StatusCode, strings.Join(details, " "))
}

func submitFailureError(statusCode int, body []byte) error {
	var response struct {
		Code   factoryapi.ErrorResponseCode `json:"code"`
		Family factoryapi.ErrorFamily       `json:"family"`
		WorkID string                       `json:"workId"`
	}
	_ = json.Unmarshal(body, &response)
	return &SubmissionHTTPError{
		StatusCode: statusCode,
		Code:       safeSubmitErrorCode(response.Code),
		Family:     safeSubmitErrorFamily(response.Family),
		WorkID:     safeSubmitWorkID(response.WorkID),
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

func safeSubmitWorkID(value string) string {
	const maxWorkIDLength = 128
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > maxWorkIDLength {
		return ""
	}
	for _, character := range trimmed {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("-._~", character) {
			continue
		}
		return ""
	}
	return trimmed
}
