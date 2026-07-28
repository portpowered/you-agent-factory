package factorydefinition_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionmcp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mcp"
)

func TestBind_ValidateContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	validation := &mcpDefinitionsValidationFake{}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) transport error = %v, want typed tool response", err)
	}
	if validation.invoked {
		t.Fatal("fake validation root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_definition.request.canceled",
		false,
	)
	if envelope.Message != "factory definition request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ValidateContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	validation := &blockingDefinitionsValidationFake{entered: entered, enteredOnce: &enteredOnce}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			factorydefinitionmcp.ToolValidate,
			json.RawMessage(minimalValidationFactoryBody),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake validation root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(validate) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(validate) transport error = %v, want typed tool response", callErr)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_definition.request.canceled",
		false,
	)
	if envelope.Message != "factory definition request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ValidateContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	validation := &blockingDefinitionsValidationFake{}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_definition.request.timed_out",
		true,
	)
	if envelope.Message != "factory definition request timed out" {
		t.Fatalf("error.message = %q, want timed out request message; envelope = %#v", envelope.Message, envelope)
	}
}

type blockingDefinitionsValidationFake struct {
	entered       chan struct{}
	enteredOnce   *sync.Once
}

func (fake *blockingDefinitionsValidationFake) ValidateSubmittedDefinition(
	ctx context.Context,
	_ factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	if fake.entered != nil && fake.enteredOnce != nil {
		fake.enteredOnce.Do(func() { close(fake.entered) })
	}
	<-ctx.Done()
	return factorydefinitions.ValidationResult{}, ctx.Err()
}

var _ factorydefinitions.SubmittedDefinitionValidationOperation = (*blockingDefinitionsValidationFake)(nil)
