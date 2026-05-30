package submit

import (
	"encoding/json"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const submitErrorBodyPreviewLimit = 200

func submitFailureError(statusCode int, body []byte) error {
	summary := submitFailureSummary(statusCode, body)
	workID := submitErrorWorkID(body)
	if workID == "" {
		return fmt.Errorf("%s", summary)
	}
	return fmt.Errorf("%s workId=%s", summary, workID)
}

func submitFailureSummary(statusCode int, body []byte) string {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && strings.TrimSpace(errResp.Message) != "" {
		return fmt.Sprintf("submission failed (%d): %s", statusCode, strings.TrimSpace(errResp.Message))
	}
	preview := strings.TrimSpace(string(body))
	if preview == "" {
		return fmt.Sprintf("submission failed (%d)", statusCode)
	}
	if len(preview) > submitErrorBodyPreviewLimit {
		preview = preview[:submitErrorBodyPreviewLimit] + "..."
	}
	return fmt.Sprintf("submission failed (%d): %s", statusCode, preview)
}

func submitErrorWorkID(body []byte) string {
	var partial struct {
		WorkID string `json:"workId"`
	}
	if json.Unmarshal(body, &partial) != nil {
		return ""
	}
	return strings.TrimSpace(partial.WorkID)
}
