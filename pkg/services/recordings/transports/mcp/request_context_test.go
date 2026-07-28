package recordingmcp_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

func TestBind_ReadPortableArtifactContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcprecording.Bind(mcprecording.RootDependencies{
		Recordings: fakeRecordingsRoot{invoked: &invoked},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		mcprecording.ToolReadPortableArtifact,
		testReadPortableArtifactInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake recordings root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.request.canceled",
		false,
		"",
	)
	if envelope.Message != "recording request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ReadPortableArtifactContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	fake := fakeRecordingsRoot{
		readPortableArtifact: func(ctx context.Context, _ recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
			enteredOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return recordings.ReadPortableArtifactResult{}, ctx.Err()
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			mcprecording.ToolReadPortableArtifact,
			testReadPortableArtifactInputJSON(),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake recordings root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(read_portable_artifact) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", callErr)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.request.canceled",
		false,
		"",
	)
	if envelope.Message != "recording request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ReadPortableArtifactContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		readPortableArtifact: func(ctx context.Context, _ recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
			<-ctx.Done()
			return recordings.ReadPortableArtifactResult{}, ctx.Err()
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		mcprecording.ToolReadPortableArtifact,
		testReadPortableArtifactInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.request.timed_out",
		true,
		"",
	)
	if envelope.Message != "recording request timed out" {
		t.Fatalf("error.message = %q, want timed out request message; envelope = %#v", envelope.Message, envelope)
	}
}
