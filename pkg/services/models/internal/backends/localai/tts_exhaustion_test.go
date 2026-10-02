package localai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestTTSNativeExhaustionMapsOnlyExactDiagnosticAndKeepsCause(t *testing.T) {
	t.Parallel()
	const diagnostic = "qwen3-tts: synthesis failed: pipeline_tts_synthesize: generation exhausted max_new_tokens=2048 without EOS"
	for _, tc := range []struct {
		name, message, want string
	}{
		{"native exhaustion", diagnostic, "TTS generation limit reached without EOS"},
		{"unrelated error", "private path and token", "TTS backend request failed"},
		{"suffix detail", diagnostic + " private path", "TTS backend request failed"},
		{"zero limit", strings.Replace(diagnostic, "2048", "0", 1), "TTS backend request failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			private := status.Error(codes.Unknown, tc.message)
			err := ttsTransportFailure(t.Context(), "TTS backend request failed", private)
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol ||
				failure.Message != tc.want || !errors.Is(err, private) || !errors.Is(err, models.ErrInferenceFailed) ||
				strings.Contains(err.Error(), "private") {
				t.Fatalf("failure = %v (%#v), want safe exact mapping and preserved cause", err, failure)
			}
		})
	}
}

func TestTTSExhaustionRejectsProtocolResultAndCancellationTakesPrecedence(t *testing.T) {
	t.Parallel()
	const diagnostic = "qwen3-tts: synthesis failed: pipeline_tts_synthesize: generation exhausted max_new_tokens=1 without EOS"
	payload, err := proto.Marshal(&Result{Success: false, Message: diagnostic})
	if err != nil {
		t.Fatal(err)
	}
	connection := &ttsProtocolConnection{response: payload}
	err = invokeTTSProtocol(t.Context(), connection, &TTSRequest{Text: "hello"})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Message != "TTS generation limit reached without EOS" {
		t.Fatalf("protocol failure = %v, want classified rejection before audio consumption", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ttsTransportFailure(ctx, "failed", errors.New(diagnostic)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled exhaustion = %v, want cancellation", err)
	}
}
