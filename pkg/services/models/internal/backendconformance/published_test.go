//go:build backendconformance

package backendconformance

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestVerifyPublishedArtifactLocationsHTTPServer(t *testing.T) {
	var (
		mu      sync.Mutex
		methods []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		methods = append(methods, request.Method+" "+request.URL.Path)
		mu.Unlock()

		switch request.URL.Path {
		case "/success":
			response.Header().Set("Content-Length", "123")
			response.WriteHeader(http.StatusOK)
		case "/redirect":
			http.Redirect(response, request, "/success", http.StatusFound)
		case "/not-found":
			response.WriteHeader(http.StatusNotFound)
		case "/missing-length":
			response.Header().Set("Transfer-Encoding", "chunked")
			response.WriteHeader(http.StatusOK)
		case "/mismatched-length":
			response.Header().Set("Content-Length", "124")
			response.WriteHeader(http.StatusOK)
		default:
			response.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(server.Close)

	entry := func(path string) PublishedArtifact {
		return PublishedArtifact{
			BackendID: "localai-test",
			TargetID:  "linux-amd64",
			Location:  server.URL + path,
			SizeBytes: 123,
		}
	}

	t.Run("final 200 with exact length", func(t *testing.T) {
		if err := VerifyPublishedArtifactLocations(context.Background(), server.Client(), []PublishedArtifact{entry("/success")}); err != nil {
			t.Fatalf("verification error = %v", err)
		}
	})

	t.Run("follows redirect and keeps HEAD method", func(t *testing.T) {
		if err := VerifyPublishedArtifactLocations(context.Background(), server.Client(), []PublishedArtifact{entry("/redirect")}); err != nil {
			t.Fatalf("verification error = %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		joined := strings.Join(methods, "|")
		if !strings.Contains(joined, "HEAD /redirect") || !strings.Contains(joined, "HEAD /success") {
			t.Fatalf("requests = %q, want HEAD on redirect and final URL", joined)
		}
	})

	for _, testCase := range []struct {
		name       string
		path       string
		wantDetail string
	}{
		{name: "non-200", path: "/not-found", wantDetail: "final response status 404"},
		{name: "missing length", path: "/missing-length", wantDetail: "omitted Content-Length"},
		{name: "mismatched length", path: "/mismatched-length", wantDetail: "Content-Length 124 bytes does not equal expected 123 bytes"},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			err := VerifyPublishedArtifactLocations(context.Background(), server.Client(), []PublishedArtifact{entry(testCase.path)})
			if err == nil {
				t.Fatal("verification error = nil, want failure")
			}
			message := err.Error()
			for _, expected := range []string{"localai-test", "linux-amd64", server.URL + testCase.path, testCase.wantDetail} {
				if !strings.Contains(message, expected) {
					t.Fatalf("verification error = %q, want %q", message, expected)
				}
			}
		})
	}
}

func TestVerifyPublishedArtifactLocationsRejectsMalformedContentLength(t *testing.T) {
	client := staticHTTPDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Length": []string{"not-a-number"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	err := VerifyPublishedArtifactLocations(context.Background(), client, []PublishedArtifact{{
		BackendID: "localai-test", TargetID: "linux-amd64", Location: "https://example.test/backend.tar.gz", SizeBytes: 123,
	}})
	if err == nil || !strings.Contains(err.Error(), "malformed Content-Length") {
		t.Fatalf("verification error = %v, want malformed Content-Length diagnostic", err)
	}
}

func TestVerifyPublishedArtifactLocationsClosesResponseBody(t *testing.T) {
	body := &trackingReadCloser{}
	client := staticHTTPDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Length": []string{"123"}},
			Body:       body,
		}, nil
	})

	err := VerifyPublishedArtifactLocations(context.Background(), client, []PublishedArtifact{{
		BackendID: "localai-test", TargetID: "linux-amd64", Location: "https://example.test/backend.tar.gz", SizeBytes: 123,
	}})
	if err != nil {
		t.Fatalf("verification error = %v", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func TestVerifyPublishedArtifactLocationsAddsBoundedRequestContext(t *testing.T) {
	client := staticHTTPDoer(func(request *http.Request) (*http.Response, error) {
		if _, ok := request.Context().Deadline(); !ok {
			t.Fatal("request context has no deadline")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Length": []string{"123"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	if err := VerifyPublishedArtifactLocations(context.Background(), client, []PublishedArtifact{{
		BackendID: "localai-test", TargetID: "linux-amd64", Location: "https://example.test/backend.tar.gz", SizeBytes: 123,
	}}); err != nil {
		t.Fatalf("verification error = %v", err)
	}
}

type staticHTTPDoer func(*http.Request) (*http.Response, error)

func (doer staticHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type trackingReadCloser struct {
	closed bool
}

func (body *trackingReadCloser) Read([]byte) (int, error) { return 0, io.EOF }

func (body *trackingReadCloser) Close() error {
	body.closed = true
	return nil
}
