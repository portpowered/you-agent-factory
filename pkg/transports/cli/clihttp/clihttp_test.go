package clihttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

type clockSequence struct {
	values []time.Time
	index  int
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func (c *clockSequence) Now() time.Time { value := c.values[c.index]; c.index++; return value }

func TestNewProtocolRequiresExactEdges(t *testing.T) {
	t.Parallel()
	clock := &clockSequence{values: []time.Time{time.Unix(1, 0)}}
	doer := doerFunc(func(*http.Request) (*http.Response, error) { return nil, nil })
	if _, err := NewProtocol(nil, clock); err == nil || err.Error() != "HTTP doer is required" {
		t.Fatalf("nil doer error = %v", err)
	}
	if _, err := NewProtocol(doer, nil); err == nil || err.Error() != "HTTP clock is required" {
		t.Fatalf("nil clock error = %v", err)
	}
}

func TestProtocolGetJSONReturnsResponseMetadata(t *testing.T) {
	start := time.Unix(10, 0)
	clock := &clockSequence{values: []time.Time{start, start.Add(37 * time.Millisecond)}}
	protocol, err := NewProtocol(doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatalf("method = %s", request.Method)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sessions":["~default"]}`))}, nil
	}), clock)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Sessions []string `json:"sessions"`
	}
	result, err := protocol.GetJSON(context.Background(), "http://factory.test/factory-sessions", &response)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	defer result.HTTP.Body.Close()
	if result.Duration != 37*time.Millisecond {
		t.Fatalf("duration = %s", result.Duration)
	}
	if len(response.Sessions) != 1 || response.Sessions[0] != "~default" {
		t.Fatalf("response = %#v", response)
	}
}

func TestProtocolJSONMethodsSetBodyHeadersAndExpectedStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, method string
		status       int
		invoke       func(Protocol) (Response, error)
	}{
		{name: "post", method: http.MethodPost, status: http.StatusOK, invoke: func(p Protocol) (Response, error) {
			return p.PostJSON(context.Background(), "http://factory.test", strings.NewReader(`{}`), nil)
		}},
		{name: "post created", method: http.MethodPost, status: http.StatusCreated, invoke: func(p Protocol) (Response, error) {
			return p.PostJSONCreated(context.Background(), "http://factory.test", strings.NewReader(`{}`), nil)
		}},
		{name: "put", method: http.MethodPut, status: http.StatusOK, invoke: func(p Protocol) (Response, error) {
			return p.PutJSON(context.Background(), "http://factory.test", strings.NewReader(`{}`), nil)
		}},
		{name: "put created", method: http.MethodPut, status: http.StatusCreated, invoke: func(p Protocol) (Response, error) {
			return p.PutJSONCreated(context.Background(), "http://factory.test", strings.NewReader(`{}`), nil)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &clockSequence{values: []time.Time{time.Unix(1, 0), time.Unix(1, 1)}}
			protocol, err := NewProtocol(doerFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != test.method {
					t.Fatalf("method = %s", request.Method)
				}
				if got := request.Header.Get("Content-Type"); got != "application/json" {
					t.Fatalf("Content-Type = %q", got)
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(""))}, nil
			}), clock)
			if err != nil {
				t.Fatal(err)
			}
			result, err := test.invoke(protocol)
			if err != nil {
				t.Fatal(err)
			}
			defer result.HTTP.Body.Close()
		})
	}
}

func TestProtocolPreservesDurationOnTransportAndDecodeErrors(t *testing.T) {
	t.Run("transport", func(t *testing.T) {
		clock := &clockSequence{values: []time.Time{time.Unix(1, 0), time.Unix(1, int64(9*time.Millisecond))}}
		protocol, _ := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF }), clock)
		result, err := protocol.GetJSON(context.Background(), "http://factory.test", nil)
		if !errors.Is(err, io.ErrUnexpectedEOF) || result.Duration != 9*time.Millisecond {
			t.Fatalf("result = %#v, err = %v", result, err)
		}
	})
	t.Run("decode", func(t *testing.T) {
		clock := &clockSequence{values: []time.Time{time.Unix(2, 0), time.Unix(2, int64(11*time.Millisecond))}}
		body := &trackedBody{Reader: strings.NewReader("{")}
		protocol, _ := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		}), clock)
		result, err := protocol.GetJSON(context.Background(), "http://factory.test", &struct{}{})
		if err == nil || result.Duration != 11*time.Millisecond {
			t.Fatalf("result = %#v, err = %v", result, err)
		}
		if !body.closed {
			t.Fatal("decode failure did not close response body")
		}
	})
}

func TestProtocolRejectsTrailingJSONValues(t *testing.T) {
	t.Parallel()

	body := &trackedBody{Reader: strings.NewReader(`{"ok":true} {"unexpected":true}`)}
	clock := &clockSequence{values: []time.Time{time.Unix(3, 0), time.Unix(3, int64(time.Millisecond))}}
	protocol, err := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	}), clock)
	if err != nil {
		t.Fatal(err)
	}

	var response struct {
		OK bool `json:"ok"`
	}
	result, err := protocol.GetJSON(context.Background(), "http://factory.test/models/llm", &response)
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("trailing JSON error = %v, want multiple-value parse failure", err)
	}
	if !response.OK || result.Duration != time.Millisecond {
		t.Fatalf("response/duration = %#v/%s, want decoded first value and measured duration", response, result.Duration)
	}
	if !body.closed {
		t.Fatal("trailing JSON failure did not close response body")
	}
}

func TestProtocolRejectsNilSuccessfulResponse(t *testing.T) {
	clock := &clockSequence{values: []time.Time{time.Unix(1, 0), time.Unix(1, 1)}}
	protocol, _ := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) { return nil, nil }), clock)
	if _, err := protocol.GetJSON(context.Background(), "http://factory.test", nil); err == nil || err.Error() != "HTTP doer returned a nil response" {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeAPIError(t *testing.T) {
	response := &http.Response{Body: io.NopCloser(strings.NewReader(`{"message":"invalid session"}`))}
	decoded, ok := DecodeAPIError(response)
	if !ok || decoded.Message != "invalid session" {
		t.Fatalf("decoded = %#v, ok = %t", decoded, ok)
	}
}

func TestAPIErrorPreservesServerFieldsAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("request context")
	response := factoryapi.ErrorResponse{
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		Family:  factoryapi.ErrorFamilyNotFound,
		Message: "server says missing",
	}
	err := NewAPIError(http.StatusNotFound, response, "work \"missing\" not found: server says missing", cause)
	if got := err.Error(); got != "work \"missing\" not found: server says missing" {
		t.Fatalf("APIError.Error() = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("APIError did not preserve its cause")
	}
	if err.CLIErrorCode() != "NOT_FOUND" || err.CLIErrorFamily() != factoryapi.ErrorFamilyNotFound || err.CLIErrorMessage() != "server says missing" {
		t.Fatalf("CLI fields = code %q family %q message %q", err.CLIErrorCode(), err.CLIErrorFamily(), err.CLIErrorMessage())
	}
	if got := err.CLIErrorResponse(); got != response {
		t.Fatalf("CLI response = %#v, want %#v", got, response)
	}
}

func TestAPIErrorFromResponsePreservesHTTPDebugMetadata(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequest(http.MethodGet, "https://factory.test/factory-sessions/missing?token=secret", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response := &http.Response{StatusCode: http.StatusNotFound, Request: request}
	apiError := NewAPIErrorFromResponse(
		response,
		factoryapi.ErrorResponse{Code: factoryapi.ErrorResponseCodeNOTFOUND, Family: factoryapi.ErrorFamilyNotFound, Message: "missing"},
		"session missing",
		nil,
	)
	if apiError.CLIHTTPMethod() != http.MethodGet ||
		apiError.CLIHTTPURL() != request.URL.String() ||
		apiError.CLIHTTPStatus() != http.StatusNotFound {
		t.Fatalf("HTTP metadata = (%q, %q, %d), want request metadata", apiError.CLIHTTPMethod(), apiError.CLIHTTPURL(), apiError.CLIHTTPStatus())
	}
}

func TestProtocolTransportFailurePreservesHTTPDebugMetadata(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection refused")
	clock := &clockSequence{values: []time.Time{time.Unix(1, 0), time.Unix(1, 1)}}
	protocol, err := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, cause
	}), clock)
	if err != nil {
		t.Fatal(err)
	}
	_, gotErr := protocol.GetJSON(context.Background(), "https://factory.test/factory-sessions?token=secret", nil)
	if !errors.Is(gotErr, cause) {
		t.Fatalf("transport error = %v, want cause %v", gotErr, cause)
	}
	var metadata interface {
		CLIHTTPMethod() string
		CLIHTTPURL() string
		CLIHTTPStatus() int
	}
	if !errors.As(gotErr, &metadata) {
		t.Fatalf("transport error = %T, want HTTP metadata", gotErr)
	}
	if metadata.CLIHTTPMethod() != http.MethodGet ||
		metadata.CLIHTTPURL() != "https://factory.test/factory-sessions?token=secret" ||
		metadata.CLIHTTPStatus() != 0 {
		t.Fatalf("transport HTTP metadata = (%q, %q, %d)", metadata.CLIHTTPMethod(), metadata.CLIHTTPURL(), metadata.CLIHTTPStatus())
	}
}

func TestProtocolDecodeFailurePreservesHTTPDebugMetadata(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequest(http.MethodGet, "https://factory.test/factory-sessions?token=secret", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	clock := &clockSequence{values: []time.Time{time.Unix(1, 0), time.Unix(1, 1)}}
	protocol, err := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Request:    request,
			Body:       io.NopCloser(strings.NewReader("{")),
		}, nil
	}), clock)
	if err != nil {
		t.Fatal(err)
	}
	_, gotErr := protocol.GetJSON(context.Background(), request.URL.String(), &struct{}{})
	if gotErr == nil {
		t.Fatal("decode failure = nil, want error")
	}
	var metadata interface {
		CLIHTTPMethod() string
		CLIHTTPURL() string
		CLIHTTPStatus() int
	}
	if !errors.As(gotErr, &metadata) {
		t.Fatalf("decode failure = %T, want HTTP metadata", gotErr)
	}
	if metadata.CLIHTTPMethod() != http.MethodGet ||
		metadata.CLIHTTPURL() != request.URL.String() ||
		metadata.CLIHTTPStatus() != http.StatusOK {
		t.Fatalf("decode HTTP metadata = (%q, %q, %d)", metadata.CLIHTTPMethod(), metadata.CLIHTTPURL(), metadata.CLIHTTPStatus())
	}
}
