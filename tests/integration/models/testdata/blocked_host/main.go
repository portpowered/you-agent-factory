package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
)

const (
	healthMethod    = "/backend.Backend/Health"
	loadModelMethod = "/backend.Backend/LoadModel"
)

type event struct {
	Kind         string `json:"kind"`
	Method       string `json:"method,omitempty"`
	PID          int    `json:"pid"`
	Endpoint     string `json:"endpoint"`
	AtUTC        string `json:"at_utc"`
	RequestBytes int    `json:"request_bytes,omitempty"`
	HasDeadline  bool   `json:"has_deadline"`
	DeadlineUTC  string `json:"deadline_utc,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run() error {
	endpoint := strings.TrimSpace(argumentValue(os.Args[1:], "--fixture-endpoint"))
	eventsPath := strings.TrimSpace(argumentValue(os.Args[1:], "--events-file"))
	readyPath := strings.TrimSpace(argumentValue(os.Args[1:], "--ready-file"))
	if endpoint == "" || eventsPath == "" || readyPath == "" {
		return errors.New("fixture endpoint, events file, and ready file are required")
	}
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return fmt.Errorf("listen on fixture endpoint: %w", err)
	}
	defer listener.Close()

	server := grpcgo.NewServer(
		grpcgo.ForceServerCodec(readinessRawCodec{}),
		grpcgo.UnknownServiceHandler(func(_ any, stream grpcgo.ServerStream) error {
			method, _ := grpcgo.MethodFromServerStream(stream)
			request := []byte(nil)
			if err := stream.RecvMsg(&request); err != nil {
				return status.Errorf(codes.InvalidArgument, "read request: %v", err)
			}
			deadline, hasDeadline := stream.Context().Deadline()
			deadlineUTC := ""
			if hasDeadline {
				deadlineUTC = deadline.UTC().Format(time.RFC3339Nano)
			}
			switch method {
			case healthMethod:
				if err := appendEvent(eventsPath, event{
					Kind: "HEALTH_SUCCEEDED", Method: method, PID: os.Getpid(), Endpoint: endpoint,
					AtUTC: time.Now().UTC().Format(time.RFC3339Nano), RequestBytes: len(request),
					HasDeadline: hasDeadline, DeadlineUTC: deadlineUTC,
				}); err != nil {
					return status.Errorf(codes.Internal, "record Health: %v", err)
				}
				return stream.SendMsg([]byte{})
			case loadModelMethod:
				if err := appendEvent(eventsPath, event{
					Kind: "LOADMODEL_BLOCKED", Method: method, PID: os.Getpid(), Endpoint: endpoint,
					AtUTC: time.Now().UTC().Format(time.RFC3339Nano), RequestBytes: len(request),
					HasDeadline: hasDeadline, DeadlineUTC: deadlineUTC,
				}); err != nil {
					return status.Errorf(codes.Internal, "record LoadModel: %v", err)
				}
				<-stream.Context().Done()
				_ = appendEvent(eventsPath, event{
					Kind: "LOADMODEL_CANCELED", Method: method, PID: os.Getpid(), Endpoint: endpoint,
					AtUTC: time.Now().UTC().Format(time.RFC3339Nano), RequestBytes: len(request),
				})
				return status.Error(codes.Canceled, "controlled LoadModel cancellation")
			default:
				return status.Errorf(codes.Unimplemented, "unexpected LocalAI method %s", method)
			}
		}),
	)
	go func() { _ = server.Serve(listener) }()
	ready, err := json.Marshal(event{
		Kind: "LISTENING", PID: os.Getpid(), Endpoint: endpoint,
		AtUTC: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("marshal ready event: %w", err)
	}
	if err := os.WriteFile(readyPath, append(ready, '\n'), 0o600); err != nil {
		server.Stop()
		return fmt.Errorf("write ready event: %w", err)
	}
	select {}
}

func argumentValue(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func appendEvent(path string, value event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open event file: %w", err)
	}
	defer file.Close()
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}

type readinessRawCodec struct{}

var _ encoding.Codec = readinessRawCodec{}

func (readinessRawCodec) Name() string { return "proto" }

func (readinessRawCodec) Marshal(value any) ([]byte, error) {
	bytes, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("readiness raw codec received %T, want []byte", value)
	}
	return bytes, nil
}

func (readinessRawCodec) Unmarshal(data []byte, value any) error {
	bytes, ok := value.(*[]byte)
	if !ok {
		return fmt.Errorf("readiness raw codec received %T, want *[]byte", value)
	}
	*bytes = append((*bytes)[:0], data...)
	return nil
}
