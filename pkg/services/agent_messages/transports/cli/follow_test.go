package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFollowObservationsAndTypedGap(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	frame := ": keepalive\r\n\r\nevent: message\r\ndata: {\r\ndata: \"recordId\":\"tx-1\",\"sequence\":1,\"kind\":\"SENT\",\"message\":" + messageJSON + "}\r\n\r\n" +
		"event: error\ndata: {\"family\":\"projection\",\"code\":\"MESSAGE_STREAM_GAP\",\"message\":\"Pull the durable inbox\"}\n\n"
	body := &trackedBody{Reader: strings.NewReader(frame)}
	standard := protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
		t.Fatal("follow used the finite request protocol")
		return clihttp.Response{}, nil
	}}
	streaming := protocolStub{execute: func(request *http.Request) (clihttp.Response, error) {
		if request.Header.Get("Accept") != "text/event-stream" || request.URL.Query().Get("follow") != "true" ||
			request.URL.Query().Get("toMe") != "true" || request.Header.Get("Authorization") == "" {
			t.Fatal("tail lost selection, filters or caller")
		}
		return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}}}, nil
	}}
	err := NewAdapter(standard, streaming, nil).List(ListConfig{Config: workerConfig(&output), ToMe: true, Follow: true})
	var typed *clihttp.APIError
	if !errors.As(err, &typed) || typed.Response.Code != factoryapi.ErrorResponseCodeMESSAGESTREAMGAP || !body.closed {
		t.Fatal("tail lost gap or response cleanup")
	}
	var observation factoryapi.AgentMessageObservation
	if json.Unmarshal(output.Bytes(), &observation) != nil || observation.RecordId != "tx-1" || observation.Message.MessageId != "msg-1" {
		t.Fatal("source-native NDJSON representation changed")
	}
}

func TestFollowFailureBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		media string
		frame string
	}{
		{name: "EOF", media: "text/event-stream", frame: ": keepalive\n\n"},
		{name: "wrong media", media: "application/json", frame: "{}"},
		{name: "malformed message", media: "text/event-stream", frame: "event: message\ndata: planted-secret\n\n"},
		{name: "incomplete observation", media: "text/event-stream", frame: "event: message\ndata: {}\n\n"},
		{name: "malformed error", media: "text/event-stream", frame: "event: error\ndata: planted-secret\n\n"},
		{name: "oversized line", media: "text/event-stream", frame: "data: " + strings.Repeat("x", maxStreamFrameBytes+1) + "\n\n"},
		{name: "oversized multiline", media: "text/event-stream", frame: strings.Repeat("data: "+strings.Repeat("x", 1024)+"\n", 130) + "\n"},
		{name: "unterminated frame", media: "text/event-stream", frame: "event: message\ndata: {}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			body := &trackedBody{Reader: strings.NewReader(test.frame)}
			protocol := protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
				return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {test.media}}}}, nil
			}}
			err := NewAdapter(protocol, protocol, nil).List(ListConfig{Config: workerConfig(&output), Follow: true})
			if !errors.Is(err, agentmessages.ErrStreamUnavailable) || output.Len() != 0 || !body.closed {
				t.Fatal("stream failure lost bounded safe refusal or cleanup")
			}
		})
	}
}

func TestFollowCancellationAndOutputFailure(t *testing.T) {
	t.Parallel()
	for _, cancel := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel", false: "writer"}[cancel], func(t *testing.T) {
			t.Parallel()
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			body := &trackedBody{Reader: strings.NewReader("event: message\ndata: {\"recordId\":\"tx-1\",\"sequence\":1,\"kind\":\"SENT\",\"message\":" + messageJSON + "}\n\n")}
			protocol := protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
				if cancel {
					stop()
				}
				return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}}, nil
			}}
			config := workerConfig(failingWriter{})
			config.Context = ctx
			err := NewAdapter(protocol, protocol, nil).List(ListConfig{Config: config, Follow: true})
			want := io.ErrClosedPipe
			if cancel {
				want = context.Canceled
			}
			if !errors.Is(err, want) || !body.closed {
				t.Fatal("tail lost cancellation/writer cause or leaked body")
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
