package clihttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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
