//go:build backendconformance || functionallong

package backendconformance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// PublishedArtifactRequestTimeout bounds each GET request made by the live
// publication verifier. The low-frequency workflow supplies an HTTP client
// with the same bound; the request context keeps the contract true for test
// clients and transports as well.
const PublishedArtifactRequestTimeout = 30 * time.Second

// PublishedArtifact is the detached set of manifest facts needed to verify one
// immutable release asset without exposing the manifest's private wire shape.
type PublishedArtifact struct {
	BackendID string
	TargetID  string
	Location  string
	SizeBytes int64
	SHA256    string
}

// HTTPDoer is the small external-effect boundary used by the live verifier.
// The production cell supplies an *http.Client; deterministic tests can supply
// a local server or a response fixture.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// PublishedArtifactFailure describes one backend/target-specific live failure.
// The verifier streams response bodies without retaining or including them in
// diagnostics.
type PublishedArtifactFailure struct {
	Artifact PublishedArtifact
	Detail   string
}

// PublishedArtifactValidationError aggregates live publication failures in a
// stable order so scheduled-run diagnostics remain easy to compare.
type PublishedArtifactValidationError struct {
	Failures []PublishedArtifactFailure
}

func (err *PublishedArtifactValidationError) Error() string {
	if err == nil || len(err.Failures) == 0 {
		return ""
	}

	failures := append([]PublishedArtifactFailure(nil), err.Failures...)
	sort.SliceStable(failures, func(left, right int) bool {
		leftArtifact, rightArtifact := failures[left].Artifact, failures[right].Artifact
		if leftArtifact.BackendID != rightArtifact.BackendID {
			return leftArtifact.BackendID < rightArtifact.BackendID
		}
		if leftArtifact.TargetID != rightArtifact.TargetID {
			return leftArtifact.TargetID < rightArtifact.TargetID
		}
		return leftArtifact.Location < rightArtifact.Location
	})

	var builder strings.Builder
	builder.WriteString("published backend artifact conformance failed:")
	for _, failure := range failures {
		fmt.Fprintf(&builder, "\n- backend %q target %q URL %q: %s",
			failure.Artifact.BackendID, failure.Artifact.TargetID,
			diagnosticURL(failure.Artifact.Location), failure.Detail)
	}
	return builder.String()
}

// VerifyPublishedArtifactLocations sends one bounded GET request per pinned
// artifact. The response body, rather than a mutable or optional response
// header, is the source of truth for measured size and SHA-256. The supplied
// client follows redirects using its normal policy.
func VerifyPublishedArtifactLocations(ctx context.Context, client HTTPDoer, entries []PublishedArtifact) error {
	if ctx == nil {
		return fmt.Errorf("published backend artifact verification requires a context")
	}
	if client == nil {
		return fmt.Errorf("published backend artifact verification requires an HTTP client")
	}

	failures := make([]PublishedArtifactFailure, 0)
	for _, entry := range entries {
		if failure, ok := verifyPublishedArtifact(ctx, client, entry); ok {
			failures = append(failures, failure)
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return &PublishedArtifactValidationError{Failures: failures}
}

func verifyPublishedArtifact(ctx context.Context, client HTTPDoer, entry PublishedArtifact) (PublishedArtifactFailure, bool) {
	requestContext, cancel := context.WithTimeout(ctx, PublishedArtifactRequestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, entry.Location, nil)
	if err != nil {
		// Keep the raw request-construction error out of diagnostics because it
		// can echo an unsafe URL or userinfo supplied by a custom test adapter.
		return publishedFailure(entry, "cannot create GET request for the manifest location"), true
	}

	response, err := client.Do(request)
	if err != nil {
		return publishedFailure(entry, transportFailureDetail(requestContext, err)), true
	}
	if response == nil {
		return publishedFailure(entry, "transport failure: HTTP client returned no response"), true
	}
	if response.Body == nil {
		return publishedFailure(entry, "final response omitted an artifact body"), true
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return publishedFailure(entry, fmt.Sprintf("final response status %d; expected HTTP 200", response.StatusCode)), true
	}

	hash := sha256.New()
	observedLength, err := io.Copy(hash, response.Body)
	if err != nil {
		return publishedFailure(entry, responseBodyFailureDetail(requestContext, err)), true
	}
	if observedLength <= MinimumPinnedArtifactSizeBytes {
		return publishedFailure(entry, fmt.Sprintf("measured response body %d bytes must be strictly greater than %d bytes (1 MiB)", observedLength, MinimumPinnedArtifactSizeBytes)), true
	}
	if observedLength != entry.SizeBytes {
		return publishedFailure(entry, fmt.Sprintf("measured response body size %d bytes does not equal expected %d bytes", observedLength, entry.SizeBytes)), true
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if actualSHA256 != entry.SHA256 {
		return publishedFailure(entry, fmt.Sprintf("measured response body SHA-256 %s does not equal expected %s", actualSHA256, entry.SHA256)), true
	}
	return PublishedArtifactFailure{}, false
}

func transportFailureDetail(requestContext context.Context, err error) string {
	switch {
	case errors.Is(requestContext.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return "transport failure: request exceeded its bounded timeout"
	case errors.Is(requestContext.Err(), context.Canceled), errors.Is(err, context.Canceled):
		return "transport failure: request was canceled"
	default:
		// Do not copy transport error strings into scheduled diagnostics: some
		// clients include request URLs, credentials, or response details there.
		return "transport failure: GET request could not be completed"
	}
}

func responseBodyFailureDetail(requestContext context.Context, err error) string {
	switch {
	case errors.Is(requestContext.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return "response body read exceeded its bounded timeout"
	case errors.Is(requestContext.Err(), context.Canceled), errors.Is(err, context.Canceled):
		return "response body read was canceled"
	default:
		return "response body could not be read"
	}
}

func publishedFailure(entry PublishedArtifact, detail string) PublishedArtifactFailure {
	return PublishedArtifactFailure{Artifact: entry, Detail: detail}
}

func diagnosticURL(location string) string {
	parsed, err := url.Parse(location)
	if err != nil {
		return "<invalid URL>"
	}
	parsed.User = nil
	return parsed.String()
}
