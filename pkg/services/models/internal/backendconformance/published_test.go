//go:build backendconformance

package backendconformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestVerifyPublishedArtifactLocationsHTTPServer(t *testing.T) {
	validBody := publishedTestBody(MinimumPinnedArtifactSizeBytes + 1)
	validDigest := publishedTestDigest(validBody)
	exactFloorBody := publishedTestBody(MinimumPinnedArtifactSizeBytes)
	exactFloorDigest := publishedTestDigest(exactFloorBody)
	belowFloorBody := publishedTestBody(MinimumPinnedArtifactSizeBytes - 1)
	belowFloorDigest := publishedTestDigest(belowFloorBody)
	server := newPublishedArtifactTestServer(t, validBody, exactFloorBody, belowFloorBody)

	t.Run("final 200 with exact body size and digest", func(t *testing.T) {
		if err := verifyPublishedArtifactRequest(t, server, "/success", int64(len(validBody)), validDigest); err != nil {
			t.Fatalf("verification error = %v", err)
		}
	})

	t.Run("follows redirect and uses GET", func(t *testing.T) {
		if err := verifyPublishedArtifactRequest(t, server, "/redirect", int64(len(validBody)), validDigest); err != nil {
			t.Fatalf("verification error = %v", err)
		}
		joined := strings.Join(server.requests(), "|")
		if !strings.Contains(joined, "GET /redirect") || !strings.Contains(joined, "GET /success") {
			t.Fatalf("requests = %q, want GET on redirect and final URL", joined)
		}
		if strings.Contains(joined, "HEAD") {
			t.Fatalf("requests = %q, did not expect HEAD", joined)
		}
	})

	t.Run("does not depend on Content-Length", func(t *testing.T) {
		if err := verifyPublishedArtifactRequest(t, server, "/missing-length", int64(len(validBody)), validDigest); err != nil {
			t.Fatalf("verification error = %v", err)
		}
	})

	for _, testCase := range []struct {
		name       string
		path       string
		size       int64
		digest     string
		wantDetail string
	}{
		{name: "non-200", path: "/not-found", size: int64(len(validBody)), digest: validDigest, wantDetail: "final response status 404"},
		{name: "mismatched size", path: "/wrong-size", size: int64(len(validBody) - 1), digest: validDigest, wantDetail: "measured response body size"},
		{name: "mismatched digest", path: "/wrong-digest", size: int64(len(validBody)), digest: strings.Repeat("0", sha256.Size*2), wantDetail: "SHA-256"},
		{name: "size exactly one MiB", path: "/exact-floor", size: int64(len(exactFloorBody)), digest: exactFloorDigest, wantDetail: "must be strictly greater than"},
		{name: "size below one MiB", path: "/below-floor", size: int64(len(belowFloorBody)), digest: belowFloorDigest, wantDetail: "must be strictly greater than"},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			assertPublishedArtifactRejected(t, server, testCase.path, testCase.size, testCase.digest, testCase.wantDetail)
		})
	}
}

type publishedArtifactTestServer struct {
	server         *httptest.Server
	mu             sync.Mutex
	methods        []string
	validBody      []byte
	exactFloorBody []byte
	belowFloorBody []byte
}

func newPublishedArtifactTestServer(t *testing.T, validBody, exactFloorBody, belowFloorBody []byte) *publishedArtifactTestServer {
	t.Helper()
	testServer := &publishedArtifactTestServer{
		validBody:      validBody,
		exactFloorBody: exactFloorBody,
		belowFloorBody: belowFloorBody,
	}
	testServer.server = httptest.NewServer(http.HandlerFunc(testServer.handle))
	t.Cleanup(testServer.server.Close)
	return testServer
}

func (server *publishedArtifactTestServer) handle(response http.ResponseWriter, request *http.Request) {
	server.mu.Lock()
	server.methods = append(server.methods, request.Method+" "+request.URL.Path)
	server.mu.Unlock()

	switch request.URL.Path {
	case "/success":
		writePublishedTestBody(response, server.validBody)
	case "/redirect":
		http.Redirect(response, request, "/success", http.StatusFound)
	case "/missing-length":
		response.WriteHeader(http.StatusOK)
		if flusher, ok := response.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = response.Write(server.validBody)
	case "/not-found":
		response.WriteHeader(http.StatusNotFound)
	case "/wrong-size", "/wrong-digest":
		writePublishedTestBody(response, server.validBody)
	case "/exact-floor":
		writePublishedTestBody(response, server.exactFloorBody)
	case "/below-floor":
		writePublishedTestBody(response, server.belowFloorBody)
	default:
		response.WriteHeader(http.StatusNotImplemented)
	}
}

func (server *publishedArtifactTestServer) client() *http.Client {
	return server.server.Client()
}

func (server *publishedArtifactTestServer) entry(path string, size int64, digest string) PublishedArtifact {
	return PublishedArtifact{
		BackendID: "localai-test",
		TargetID:  "linux-amd64",
		Location:  server.server.URL + path,
		SizeBytes: size,
		SHA256:    digest,
	}
}

func (server *publishedArtifactTestServer) requests() []string {
	server.mu.Lock()
	defer server.mu.Unlock()
	return append([]string(nil), server.methods...)
}

func verifyPublishedArtifactRequest(t *testing.T, server *publishedArtifactTestServer, path string, size int64, digest string) error {
	t.Helper()
	return VerifyPublishedArtifactLocations(context.Background(), server.client(), []PublishedArtifact{server.entry(path, size, digest)})
}

func assertPublishedArtifactRejected(t *testing.T, server *publishedArtifactTestServer, path string, size int64, digest, wantDetail string) {
	t.Helper()
	err := verifyPublishedArtifactRequest(t, server, path, size, digest)
	if err == nil {
		t.Fatal("verification error = nil, want failure")
	}
	message := err.Error()
	for _, expected := range []string{"localai-test", "linux-amd64", server.server.URL + path, wantDetail} {
		if !strings.Contains(message, expected) {
			t.Fatalf("verification error = %q, want %q", message, expected)
		}
	}
}

func TestVerifyPublishedArtifactLocationsRejectsMissingResponseBody(t *testing.T) {
	client := staticHTTPDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	err := VerifyPublishedArtifactLocations(context.Background(), client, []PublishedArtifact{{
		BackendID: "localai-test", TargetID: "linux-amd64", Location: "https://example.test/backend.tar.gz",
		SizeBytes: MinimumPinnedArtifactSizeBytes + 1, SHA256: strings.Repeat("0", sha256.Size*2),
	}})
	if err == nil || !strings.Contains(err.Error(), "omitted an artifact body") {
		t.Fatalf("verification error = %v, want missing-body diagnostic", err)
	}
}

func TestVerifyPublishedArtifactLocationsClosesResponseBody(t *testing.T) {
	bodyBytes := publishedTestBody(MinimumPinnedArtifactSizeBytes + 1)
	body := &trackingReadCloser{Reader: bytes.NewReader(bodyBytes)}
	client := staticHTTPDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})

	err := VerifyPublishedArtifactLocations(context.Background(), client, []PublishedArtifact{{
		BackendID: "localai-test", TargetID: "linux-amd64", Location: "https://example.test/backend.tar.gz",
		SizeBytes: int64(len(bodyBytes)), SHA256: publishedTestDigest(bodyBytes),
	}})
	if err != nil {
		t.Fatalf("verification error = %v", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func TestVerifyPublishedArtifactLocationsAddsBoundedRequestContext(t *testing.T) {
	bodyBytes := publishedTestBody(MinimumPinnedArtifactSizeBytes + 1)
	client := staticHTTPDoer(func(request *http.Request) (*http.Response, error) {
		if _, ok := request.Context().Deadline(); !ok {
			t.Fatal("request context has no deadline")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(bodyBytes))}, nil
	})

	if err := VerifyPublishedArtifactLocations(context.Background(), client, []PublishedArtifact{{
		BackendID: "localai-test", TargetID: "linux-amd64", Location: "https://example.test/backend.tar.gz",
		SizeBytes: int64(len(bodyBytes)), SHA256: publishedTestDigest(bodyBytes),
	}}); err != nil {
		t.Fatalf("verification error = %v", err)
	}
}

func publishedTestBody(size int64) []byte {
	return bytes.Repeat([]byte{'L'}, int(size))
}

func publishedTestDigest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func writePublishedTestBody(response http.ResponseWriter, body []byte) {
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(body)
}

type staticHTTPDoer func(*http.Request) (*http.Response, error)

func (doer staticHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (body *trackingReadCloser) Close() error {
	body.closed = true
	return nil
}
