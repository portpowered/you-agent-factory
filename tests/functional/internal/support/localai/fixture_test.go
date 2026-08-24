package localai

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	localaiproto "github.com/portpowered/infinite-you/tests/functional/internal/support/localai/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestFixtureServesPinnedDeterministicOperations(t *testing.T) {
	fixture := Start(t, Options{EmbeddingDimensions: 5})
	if fixture.Endpoint() == "" {
		t.Fatal("fixture endpoint is empty")
	}
	host, port, err := net.SplitHostPort(fixture.Endpoint())
	if err != nil || host != "127.0.0.1" || port == "7437" {
		t.Fatalf("fixture endpoint = %q, want dynamic loopback address", fixture.Endpoint())
	}

	client := newClient(t, fixture.Endpoint())
	health, err := client.Health(t.Context(), &localaiproto.HealthMessage{})
	if err != nil || string(health.GetMessage()) != fixtureHealthMessage {
		t.Fatalf("Health() = (%q, %v), want deterministic readiness", health.GetMessage(), err)
	}

	images := []string{"image-first", "image-second"}
	prediction, err := client.Predict(t.Context(), &localaiproto.PredictOptions{
		Prompt: "compare",
		Images: images,
		Videos: []string{"video-one"},
	})
	if err != nil {
		t.Fatalf("Predict(): %v", err)
	}
	if got, want := string(prediction.GetMessage()), ExpectedOmniText("compare", images, nil, []string{"video-one"}); got != want {
		t.Fatalf("Predict() text = %q, want %q", got, want)
	}

	embedding, err := client.Embedding(t.Context(), &localaiproto.PredictOptions{Prompt: "embed me"})
	if err != nil {
		t.Fatalf("Embedding(): %v", err)
	}
	if got, want := len(embedding.GetEmbeddings()), 5; got != want {
		t.Fatalf("Embedding() dimensions = %d, want %d", got, want)
	}
	if got, want := embedding.GetEmbeddings()[0], float32(0.1); got != want {
		t.Fatalf("Embedding()[0] = %v, want %v", got, want)
	}

	audioPath := filepath.Join(t.TempDir(), "fixture.wav")
	tts, err := client.TTS(t.Context(), &localaiproto.TTSRequest{Text: "hello", Dst: audioPath})
	if err != nil || !tts.GetSuccess() {
		t.Fatalf("TTS() = (%#v, %v), want success", tts, err)
	}
	audio, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read TTS output: %v", err)
	}
	assertWAV(t, audio)

	transcript, err := client.AudioTranscription(t.Context(), &localaiproto.TranscriptRequest{Dst: "fixture-input.wav"})
	if err != nil {
		t.Fatalf("AudioTranscription(): %v", err)
	}
	if transcript.GetText() != fixtureTranscript || len(transcript.GetSegments()) != 1 || transcript.GetSegments()[0].GetText() != fixtureTranscriptSegment {
		t.Fatalf("AudioTranscription() = %#v, want transcript and segment", transcript)
	}

	statusResponse, err := client.Status(t.Context(), &localaiproto.HealthMessage{})
	if err != nil || statusResponse.GetState() != localaiproto.StatusResponse_READY {
		t.Fatalf("Status() = (%v, %v), want READY", statusResponse.GetState(), err)
	}

	calls := fixture.Calls()
	if len(calls) != 6 {
		t.Fatalf("fixture recorded %d calls, want 6", len(calls))
	}
	if got := calls[1].Images; len(got) != 2 || got[0] != images[0] || got[1] != images[1] {
		t.Fatalf("recorded image order = %#v, want %#v", got, images)
	}
}

func TestFixtureFailureModesReturnStableGRPCErrors(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		code codes.Code
		want string
	}{
		{name: "unavailable", mode: ModeUnavailable, code: codes.Unavailable, want: "localai fixture backend unavailable"},
		{name: "protocol mismatch", mode: ModeProtocolMismatch, code: codes.FailedPrecondition, want: "localai fixture backend protocol mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := Start(t, Options{Mode: test.mode})
			client := newClient(t, fixture.Endpoint())
			_, err := client.Predict(t.Context(), &localaiproto.PredictOptions{Prompt: "failure"})
			if status.Code(err) != test.code || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Predict() error = %v, want %s containing %q", err, test.code, test.want)
			}
		})
	}
}

func TestFixtureMalformedModeReturnsSuccessfulButInvalidPayload(t *testing.T) {
	fixture := Start(t, Options{Mode: ModeMalformed})
	client := newClient(t, fixture.Endpoint())
	embedding, err := client.Embedding(t.Context(), &localaiproto.PredictOptions{Prompt: "malformed"})
	if err != nil {
		t.Fatalf("Embedding() error = %v, want malformed payload without transport error", err)
	}
	if len(embedding.GetEmbeddings()) != 0 {
		t.Fatalf("malformed embedding dimensions = %d, want 0", len(embedding.GetEmbeddings()))
	}
	statusResponse, err := client.Status(t.Context(), &localaiproto.HealthMessage{})
	if err != nil {
		t.Fatalf("Status() error = %v, want malformed payload without transport error", err)
	}
	if statusResponse.GetState() == localaiproto.StatusResponse_READY {
		t.Fatal("malformed Status() reported READY")
	}
}

func TestFixtureCloseIsIdempotentAndStopsEndpoint(t *testing.T) {
	fixture, err := New(Options{})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatalf("first Close(): %v", err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatalf("second Close(): %v", err)
	}
	connection, err := net.Dial("tcp", fixture.Endpoint())
	if err == nil {
		_ = connection.Close()
		t.Fatal("fixture endpoint accepted a connection after Close")
	}
}

func TestNewRejectsInvalidFixtureConfiguration(t *testing.T) {
	if _, err := New(Options{Mode: Mode("unknown")}); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("New(invalid mode) error = %v, want ErrInvalidMode", err)
	}
	if _, err := New(Options{EmbeddingDimensions: -1}); err == nil {
		t.Fatal("New(negative dimensions) succeeded")
	}
}

func newClient(t *testing.T, endpoint string) localaiproto.BackendClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		t.Fatalf("dial fixture: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return localaiproto.NewBackendClient(connection)
}

func assertWAV(t *testing.T, audio []byte) {
	t.Helper()
	if len(audio) <= 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[36:40]) != "data" {
		t.Fatalf("audio = %d bytes, want non-trivial WAV", len(audio))
	}
}
