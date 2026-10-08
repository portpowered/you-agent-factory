package admission_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func listSubmissionSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sessionID, "/work"))
}

func upsertSubmissionSessionWorkRequest(t testing.TB, baseURL, sessionID string, request factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal Work request: %v", err)
	}
	endpoint := support.SessionWorkURL(
		baseURL, sessionID,
		"/work-requests/"+url.PathEscape(request.RequestId),
	)
	httpRequest, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build PUT %s: %v", endpoint, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("PUT %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("PUT %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode PUT %s: %v", endpoint, err)
	}
	return result
}

func submitSubmissionSessionWork(t testing.TB, baseURL, sessionID string, request factoryapi.SubmitWorkRequest) factoryapi.SubmitWorkResponse {
	t.Helper()

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal submit Work request: %v", err)
	}
	endpoint := support.SessionWorkURL(baseURL, sessionID, "/work")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return result
}
