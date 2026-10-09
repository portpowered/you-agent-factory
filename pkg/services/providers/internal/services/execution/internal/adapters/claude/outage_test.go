package claude

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

func TestProviderOutageClassification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, message string
		kind          providers.ExecuteFailureKind
	}{
		{"captured 503", `unexpected status 503 Service Unavailable: {"detail":"Unable to verify Daybreak Blue access. Please try again."}`, providers.ExecuteFailureKindDependency},
		{"500 collision", "unexpected status 500 Internal Server Error: access forbidden; invalid request; authentication unavailable; please try again", providers.ExecuteFailureKindDependency},
		{"502 collision", "unexpected status 502 Bad Gateway: access forbidden; invalid request; authentication unavailable; please try again", providers.ExecuteFailureKindDependency},
		{"504 collision", "unexpected status 504 Gateway Timeout: access forbidden; invalid request; authentication unavailable; please try again", providers.ExecuteFailureKindDependency},
		{"503 arguments", "unexpected status 503: bad request; invalid argument", providers.ExecuteFailureKindDependency},
		{"400", "unexpected status 400 Bad Request: invalid request; please try again", providers.ExecuteFailureKindInvalidRequest},
		{"401", "unexpected status 401 Unauthorized: please try again", providers.ExecuteFailureKindAuthentication},
		{"403", "HTTP 403 Forbidden: please try again", providers.ExecuteFailureKindAuthentication},
		{"429", "unexpected status 429 Too Many Requests: please try again", providers.ExecuteFailureKindThrottled},
		{"auth prose", "authentication failed; please try again", providers.ExecuteFailureKindAuthentication},
		{"timeout prose", "request timed out; please try again", providers.ExecuteFailureKindTimeout},
		{"unavailable", "service unavailable; please try again", providers.ExecuteFailureKindDependency},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, message := range []string{tc.message, strings.ToUpper(tc.message)} {
				result := providerservice.CommandResult{ExitCode: 1, Stderr: []byte(message)}
				assertOutageKind(t, exitFailureFromCommandResult(result), tc.kind)
			}
			resultSubtype := "api_error"
			if tc.name == "auth prose" {
				resultSubtype = "authentication_error"
			}
			if tc.name == "timeout prose" {
				resultSubtype = ""
			}
			assertOutageKind(t, classifyResultFailure(nativeEnvelope{Subtype: resultSubtype, Result: tc.message}), tc.kind)
			if tc.name == "timeout prose" {
				return
			}
			nativeType := resultSubtype
			raw, err := json.Marshal(map[string]any{"type": "error", "error": map[string]string{"type": nativeType, "message": tc.message}})
			if err != nil {
				t.Fatal(err)
			}
			result := providerservice.CommandResult{ExitCode: 1, Stdout: raw}
			assertOutageKind(t, exitFailureFromCommandResult(result), tc.kind)
			if tc.kind == providers.ExecuteFailureKindDependency && strings.Contains(tc.message, "unexpected status") {
				for _, nativeType := range []string{"authentication_error", "invalid_request_error", ""} {
					assertOutageKind(t, classifyResultFailure(nativeEnvelope{Subtype: nativeType, Result: tc.message}), tc.kind)
					failure := classifyAPIErrorRecord(apiErrorRecord{Type: nativeType, Message: tc.message})
					assertOutageKind(t, failure, tc.kind)
				}
			}
		})
	}
}

func TestProviderOutageStatusEvidenceBoundaries(t *testing.T) {
	t.Parallel()
	result := providerservice.CommandResult{ExitCode: 1, Stdout: []byte("authentication failed: invalid request"), Stderr: []byte("unexpected status 503 Service Unavailable")}
	assertOutageKind(t, exitFailureFromCommandResult(result), providers.ExecuteFailureKindDependency)
	result = providerservice.CommandResult{ExitCode: 1, Stderr: []byte("unexpected status 5039: invalid request")}
	assertOutageKind(t, exitFailureFromCommandResult(result), providers.ExecuteFailureKindInvalidRequest)

	result = providerservice.CommandResult{ExitCode: 1, Stderr: []byte(`unexpected status 400 Bad Request: {"type":"api_error","message":"please try again"}`)}
	assertOutageKind(t, exitFailureFromCommandResult(result), providers.ExecuteFailureKindInvalidRequest)

	result = providerservice.CommandResult{ExitCode: 1, Stdout: []byte("request identifier 503"), Stderr: []byte("invalid request")}
	assertOutageKind(t, exitFailureFromCommandResult(result), providers.ExecuteFailureKindInvalidRequest)
}

func assertOutageKind(t *testing.T, err error, want providers.ExecuteFailureKind) {
	t.Helper()
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != want {
		t.Fatalf("failure = %#v (%v), want %s", failure, err, want)
	}
}
