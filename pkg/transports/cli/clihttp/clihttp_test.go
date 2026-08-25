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

func TestProtocolJSONMethodsDecodeBodiesAndReturnResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, method string
		status       int
		invoke       func(Protocol, any) (Response, error)
	}{
		{name: "post", method: http.MethodPost, status: http.StatusOK, invoke: func(p Protocol, dst any) (Response, error) {
			return p.PostJSON(context.Background(), "http://factory.test/work", strings.NewReader(`{"name":"request"}`), dst)
		}},
		{name: "post created", method: http.MethodPost, status: http.StatusCreated, invoke: func(p Protocol, dst any) (Response, error) {
			return p.PostJSONCreated(context.Background(), "http://factory.test/work", strings.NewReader(`{"name":"request"}`), dst)
		}},
		{name: "put", method: http.MethodPut, status: http.StatusOK, invoke: func(p Protocol, dst any) (Response, error) {
			return p.PutJSON(context.Background(), "http://factory.test/work", strings.NewReader(`{"name":"request"}`), dst)
		}},
		{name: "put created", method: http.MethodPut, status: http.StatusCreated, invoke: func(p Protocol, dst any) (Response, error) {
			return p.PutJSONCreated(context.Background(), "http://factory.test/work", strings.NewReader(`{"name":"request"}`), dst)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &clockSequence{values: []time.Time{time.Unix(3, 0), time.Unix(3, int64(23*time.Millisecond))}}
			protocol, err := NewProtocol(doerFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != test.method {
					t.Fatalf("method = %s, want %s", request.Method, test.method)
				}
				if request.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("Content-Type = %q", request.Header.Get("Content-Type"))
				}
				body, readErr := io.ReadAll(request.Body)
				if readErr != nil {
					t.Fatalf("request body: %v", readErr)
				}
				if string(body) != `{"name":"request"}` {
					t.Fatalf("request body = %q", body)
				}
				return &http.Response{
					StatusCode: test.status,
					Request:    request,
					Body:       io.NopCloser(strings.NewReader(`{"name":"response"}`)),
				}, nil
			}), clock)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Name string `json:"name"`
			}
			result, err := test.invoke(protocol, &decoded)
			if err != nil {
				t.Fatalf("invoke: %v", err)
			}
			if result.HTTP == nil || result.HTTP.StatusCode != test.status {
				t.Fatalf("response = %#v", result.HTTP)
			}
			if result.Duration != 23*time.Millisecond {
				t.Fatalf("duration = %s", result.Duration)
			}
			if decoded.Name != "response" {
				t.Fatalf("decoded response = %#v", decoded)
			}
			_ = result.HTTP.Body.Close()
		})
	}
}

func TestProtocolReturnsUnexpectedStatusWithoutDecoding(t *testing.T) {
	t.Parallel()
	clock := &clockSequence{values: []time.Time{time.Unix(4, 0), time.Unix(4, int64(7*time.Millisecond))}}
	body := &trackedBody{Reader: strings.NewReader(`{"message":"server rejected request"}`)}
	protocol, err := NewProtocol(doerFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusAccepted, Request: request, Body: body}, nil
	}), clock)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Message string `json:"message"`
	}
	result, err := protocol.GetJSON(context.Background(), "http://factory.test/work", &decoded)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if result.HTTP == nil || result.HTTP.StatusCode != http.StatusAccepted {
		t.Fatalf("response = %#v", result.HTTP)
	}
	if result.Duration != 7*time.Millisecond {
		t.Fatalf("duration = %s", result.Duration)
	}
	if decoded.Message != "" {
		t.Fatalf("unexpected status was decoded into %#v", decoded)
	}
	if body.closed {
		t.Fatal("unexpected-status response body was closed before the caller could decode it")
	}
	_ = result.HTTP.Body.Close()
}

func TestProtocolRejectsInvalidURLWithTypedMetadata(t *testing.T) {
	clock := &clockSequence{values: []time.Time{time.Unix(5, 0)}}
	protocol, err := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid URL reached the HTTP doer")
		return nil, nil
	}), clock)
	if err != nil {
		t.Fatal(err)
	}
	const invalidURL = "://not-a-url"
	_, gotErr := protocol.GetJSON(context.Background(), invalidURL, nil)
	var httpErr *HTTPError
	if !errors.As(gotErr, &httpErr) {
		t.Fatalf("error = %T %v, want *HTTPError", gotErr, gotErr)
	}
	if httpErr.CLIHTTPMethod() != http.MethodGet || httpErr.CLIHTTPURL() != invalidURL || httpErr.CLIHTTPStatus() != 0 {
		t.Fatalf("HTTP metadata = (%q, %q, %d)", httpErr.CLIHTTPMethod(), httpErr.CLIHTTPURL(), httpErr.CLIHTTPStatus())
	}
	if !strings.Contains(httpErr.Error(), "build request") {
		t.Fatalf("error = %q, want request construction context", httpErr.Error())
	}
}

func TestProtocolTransportFailureUsesResponseRequestMetadata(t *testing.T) {
	t.Parallel()
	request, err := http.NewRequest(http.MethodPost, "https://factory.test/original", nil)
	if err != nil {
		t.Fatal(err)
	}
	responseRequest, err := http.NewRequest(http.MethodPut, "https://factory.test/actual?token=secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("connection reset")
	clock := &clockSequence{values: []time.Time{time.Unix(6, 0), time.Unix(6, int64(13*time.Millisecond))}}
	response := &http.Response{
		StatusCode: http.StatusBadGateway,
		Request:    responseRequest,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	protocol, err := NewProtocol(doerFunc(func(*http.Request) (*http.Response, error) {
		return response, cause
	}), clock)
	if err != nil {
		t.Fatal(err)
	}
	result, gotErr := protocol.Execute(request)
	if result.HTTP != response || result.Duration != 13*time.Millisecond {
		t.Fatalf("result = %#v", result)
	}
	if !errors.Is(gotErr, cause) {
		t.Fatalf("error = %v, want cause %v", gotErr, cause)
	}
	var metadata interface {
		CLIHTTPMethod() string
		CLIHTTPURL() string
		CLIHTTPStatus() int
	}
	if !errors.As(gotErr, &metadata) {
		t.Fatalf("error = %T, want HTTP metadata", gotErr)
	}
	if metadata.CLIHTTPMethod() != http.MethodPut || metadata.CLIHTTPURL() != responseRequest.URL.String() || metadata.CLIHTTPStatus() != http.StatusBadGateway {
		t.Fatalf("HTTP metadata = (%q, %q, %d)", metadata.CLIHTTPMethod(), metadata.CLIHTTPURL(), metadata.CLIHTTPStatus())
	}
	_ = response.Body.Close()
}

func TestDecodeAPIErrorRejectsMissingOrMalformedBodies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		response *http.Response
	}{
		{name: "nil response"},
		{name: "nil body", response: &http.Response{}},
		{name: "malformed JSON", response: &http.Response{Body: io.NopCloser(strings.NewReader("{"))}},
		{name: "message-less JSON", response: &http.Response{Body: io.NopCloser(strings.NewReader(`{"code":"NOT_FOUND"}`))}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded, ok := DecodeAPIError(test.response)
			if ok {
				t.Fatalf("decoded = %#v, want rejection", decoded)
			}
			if test.response != nil && test.response.Body != nil {
				_ = test.response.Body.Close()
			}
		})
	}
}

func TestAPIErrorNilAccessors(t *testing.T) {
	t.Parallel()
	var nilError *APIError
	if nilError.Error() != "" {
		t.Fatalf("nil APIError error = %q", nilError.Error())
	}
	if nilError.Unwrap() != nil {
		t.Fatal("nil APIError unwrap was non-nil")
	}
	if nilError.CLIErrorCode() != "" {
		t.Fatalf("nil APIError code = %q", nilError.CLIErrorCode())
	}
	if nilError.CLIErrorFamily() != "" {
		t.Fatalf("nil APIError family = %q", nilError.CLIErrorFamily())
	}
	if nilError.CLIErrorMessage() != "" {
		t.Fatalf("nil APIError message = %q", nilError.CLIErrorMessage())
	}
	if nilError.CLIHTTPMethod() != "" {
		t.Fatalf("nil APIError method = %q", nilError.CLIHTTPMethod())
	}
	if nilError.CLIHTTPURL() != "" {
		t.Fatalf("nil APIError URL = %q", nilError.CLIHTTPURL())
	}
	if nilError.CLIHTTPStatus() != 0 {
		t.Fatalf("nil APIError status = %d", nilError.CLIHTTPStatus())
	}
	if got := nilError.CLIErrorResponse(); got != (factoryapi.ErrorResponse{}) {
		t.Fatalf("nil API response = %#v", got)
	}
}

func TestAPIErrorMessageOnlyFallback(t *testing.T) {
	t.Parallel()
	messageOnly := NewAPIError(0, factoryapi.ErrorResponse{Message: " server message "}, " ", nil)
	if messageOnly.Error() != "server message" || messageOnly.CLIErrorMessage() != "server message" {
		t.Fatalf("message-only error = %q / %q", messageOnly.Error(), messageOnly.CLIErrorMessage())
	}
}

func TestAPIErrorStatusOnlyFallback(t *testing.T) {
	t.Parallel()
	statusOnly := NewAPIError(http.StatusServiceUnavailable, factoryapi.ErrorResponse{}, "", nil)
	if statusOnly.Error() != "HTTP request failed (503)" {
		t.Fatalf("status-only error = %q", statusOnly.Error())
	}
}

func TestAPIErrorEmptyFallback(t *testing.T) {
	t.Parallel()
	empty := NewAPIErrorFromResponse(nil, factoryapi.ErrorResponse{}, "", nil)
	if empty.Error() != "HTTP request failed" || empty.CLIHTTPMethod() != "" || empty.CLIHTTPURL() != "" || empty.CLIHTTPStatus() != 0 {
		t.Fatalf("empty error = %#v", empty)
	}
}

func TestHTTPErrorNilAccessors(t *testing.T) {
	t.Parallel()
	var nilError *HTTPError
	if nilError.Error() != "" {
		t.Fatalf("nil HTTPError error = %q", nilError.Error())
	}
	if nilError.Unwrap() != nil {
		t.Fatal("nil HTTPError unwrap was non-nil")
	}
	if nilError.CLIHTTPMethod() != "" {
		t.Fatalf("nil HTTPError method = %q", nilError.CLIHTTPMethod())
	}
	if nilError.CLIHTTPURL() != "" {
		t.Fatalf("nil HTTPError URL = %q", nilError.CLIHTTPURL())
	}
	if nilError.CLIHTTPStatus() != 0 {
		t.Fatalf("nil HTTPError status = %d", nilError.CLIHTTPStatus())
	}
}

func TestHTTPErrorEmptyAndNilConstructors(t *testing.T) {
	t.Parallel()
	if (&HTTPError{}).Error() != "HTTP request failed" {
		t.Fatal("empty HTTPError did not use the safe fallback")
	}
	if NewHTTPError(http.MethodGet, "http://factory.test", 0, nil) != nil {
		t.Fatal("nil NewHTTPError cause manufactured an error")
	}
	if NewHTTPErrorFromResponse(nil, nil) != nil {
		t.Fatal("nil response cause manufactured an error")
	}
	if NewHTTPErrorFromRequest(nil, nil) != nil {
		t.Fatal("nil request cause manufactured an error")
	}
	if WithHTTPResponse(nil, nil) != nil {
		t.Fatal("nil response wrapper manufactured an error")
	}
}

func TestHTTPErrorMetadataWrappers(t *testing.T) {
	t.Parallel()
	request, err := http.NewRequest(http.MethodDelete, "https://factory.test/work", nil)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("server unavailable")
	response := &http.Response{StatusCode: http.StatusServiceUnavailable, Request: request}
	wrapped := NewHTTPErrorFromResponse(response, cause)
	if !errors.Is(wrapped, cause) || wrapped.(interface{ CLIHTTPMethod() string }).CLIHTTPMethod() != http.MethodDelete {
		t.Fatalf("wrapped response error = %v", wrapped)
	}
	if WithHTTPResponse(response, wrapped) != wrapped {
		t.Fatal("WithHTTPResponse replaced existing HTTP metadata")
	}
	withMetadata := WithHTTPResponse(response, cause)
	var metadata interface {
		CLIHTTPMethod() string
		CLIHTTPURL() string
		CLIHTTPStatus() int
	}
	if !errors.As(withMetadata, &metadata) || metadata.CLIHTTPMethod() != http.MethodDelete || metadata.CLIHTTPURL() != request.URL.String() || metadata.CLIHTTPStatus() != http.StatusServiceUnavailable {
		t.Fatalf("wrapped metadata = %T (%v)", withMetadata, withMetadata)
	}
}
