package localai

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestOmniPredictOptionsLeavesTokensUnboundedByDefault(t *testing.T) {
	t.Parallel()

	options, err := predictOptions(PredictRequest{
		Prompt:     "hello",
		Parameters: []models.OperationParameter{{Name: "temperature", Value: 0.2}},
	})
	if err != nil {
		t.Fatalf("predictOptions() error = %v", err)
	}
	if options.Tokens != 0 {
		t.Fatalf("Tokens = %d, want unbounded default 0", options.Tokens)
	}
	if options.Metadata["temperature"] != "0.2" {
		t.Fatalf("Metadata = %#v, want temperature passthrough", options.Metadata)
	}
	if _, ok := options.Metadata["max_tokens"]; ok {
		t.Fatalf("Metadata = %#v, want no max_tokens entry", options.Metadata)
	}
}

func TestOmniPredictOptionsMapsExplicitMaxTokens(t *testing.T) {
	t.Parallel()

	for _, value := range []any{int(128), int32(64), int64(256), float64(512), json.Number("1024")} {
		options, err := predictOptions(PredictRequest{
			Prompt: "hello",
			Parameters: []models.OperationParameter{
				{Name: "temperature", Value: 0.2},
				{Name: "max_tokens", Value: value},
			},
		})
		if err != nil {
			t.Fatalf("predictOptions(max_tokens=%v) error = %v", value, err)
		}
		var want int32
		switch raw := value.(type) {
		case int:
			want = int32(raw)
		case int32:
			want = raw
		case int64:
			want = int32(raw)
		case float64:
			want = int32(raw)
		case json.Number:
			want = 1024
		}
		if options.Tokens != want {
			t.Fatalf("predictOptions(max_tokens=%v) Tokens = %d, want %d", value, options.Tokens, want)
		}
		if _, ok := options.Metadata["max_tokens"]; ok {
			t.Fatalf("predictOptions(max_tokens=%v) Metadata = %#v, want max_tokens excluded", value, options.Metadata)
		}
		if options.Metadata["temperature"] != "0.2" {
			t.Fatalf("predictOptions(max_tokens=%v) Metadata = %#v, want temperature passthrough", value, options.Metadata)
		}
	}
}

func TestOmniPredictOptionsAcceptsInt32UpperBound(t *testing.T) {
	t.Parallel()

	options, err := predictOptions(PredictRequest{
		Parameters: []models.OperationParameter{{Name: "max_tokens", Value: math.MaxInt32}},
	})
	if err != nil {
		t.Fatalf("predictOptions() error = %v", err)
	}
	if options.Tokens != math.MaxInt32 {
		t.Fatalf("Tokens = %d, want %d", options.Tokens, int32(math.MaxInt32))
	}
}

func TestOmniPredictOptionsRejectsInvalidMaxTokens(t *testing.T) {
	t.Parallel()

	invalid := []any{
		0, -1, -100,
		int64(0), int64(-5),
		float64(1.5), float32(2.5),
		"100", "128", true, nil,
		map[string]any{"n": 1}, []any{1},
		float64(math.NaN()), float64(math.Inf(1)),
		float64(math.MaxInt32) + 1,
		int64(math.MaxInt32) + 1,
		uint64(math.MaxInt32) + 1,
		json.Number("1.5"), json.Number("abc"), json.Number("0"),
	}
	for _, value := range invalid {
		_, err := predictOptions(PredictRequest{
			Prompt:     "hello",
			Parameters: []models.OperationParameter{{Name: "max_tokens", Value: value}},
		})
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) ||
			failure.Class != models.InvocationFailureClassInvalidParameter ||
			failure.Operation != models.OperationOMNI ||
			failure.Parameter != "max_tokens" {
			t.Fatalf("predictOptions(max_tokens=%#v) error = %v, failure = %#v, want typed OMNI/max_tokens InvalidParameter", value, err, failure)
		}
	}
}

func TestOmniPredictSurfacesInvalidMaxTokensWithoutProtocolMask(t *testing.T) {
	t.Parallel()

	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: &recordingGRPCConnection{}})
	_, err := client.Predict(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
		PredictRequest{
			Prompt:     "hello",
			Parameters: []models.OperationParameter{{Name: "max_tokens", Value: 0}},
		},
	)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) ||
		failure.Class != models.InvocationFailureClassInvalidParameter ||
		failure.Operation != models.OperationOMNI ||
		failure.Parameter != "max_tokens" {
		t.Fatalf("Predict() error = %v, failure = %#v, want typed OMNI/max_tokens InvalidParameter", err, failure)
	}
}

func TestEmbeddingKeepsMaxTokensAsMetadata(t *testing.T) {
	t.Parallel()

	options, err := embeddingOptions(models.EmbeddingBackendRequest{
		Text:       "query",
		Parameters: map[string]any{"max_tokens": 128, "normalize": true},
	})
	if err != nil {
		t.Fatalf("embeddingOptions() error = %v", err)
	}
	if options.Tokens != 0 {
		t.Fatalf("Tokens = %d, want embedding default 0", options.Tokens)
	}
	if options.Metadata["max_tokens"] != "128" || options.Metadata["normalize"] != "true" {
		t.Fatalf("Metadata = %#v, want legacy max_tokens passthrough", options.Metadata)
	}
}
