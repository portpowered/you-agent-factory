package startup

import (
	"context"
	"errors"
	"testing"
)

func TestHandlerRunsWithSuppliedContextRequestAndResult(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), lifecycleContextKey{}, "value")
	wantErr := errors.New("lifecycle failed")
	called := 0
	wantRequest := Request{Kind: KindMCPServe, MCP: MCPIntent{ProjectRoot: "/tmp/project"}}
	handler := Handler(func(got context.Context, request Request) error {
		called++
		if got != ctx {
			t.Fatal("Handler did not preserve context identity")
		}
		if request.Kind != wantRequest.Kind || request.MCP.ProjectRoot != wantRequest.MCP.ProjectRoot {
			t.Fatalf("Handler request = %+v, want %+v", request, wantRequest)
		}
		return wantErr
	})

	if err := handler.Handle(ctx, wantRequest); !errors.Is(err, wantErr) {
		t.Fatalf("Handler() error = %v, want %v", err, wantErr)
	}
	if called != 1 {
		t.Fatalf("lifecycle calls = %d, want 1", called)
	}
}

type lifecycleContextKey struct{}
