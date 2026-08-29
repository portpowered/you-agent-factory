package localai

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"

	localaiproto "github.com/portpowered/infinite-you/tests/functional/internal/support/localai/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// DefaultEmbeddingDimensions is intentionally small so functional tests
	// exercise dimensionality without carrying a model-sized payload.
	DefaultEmbeddingDimensions = 8
	fixtureHealthMessage       = "LOCALAI_FIXTURE_READY"
	fixtureTranscript          = "LOCALAI_FIXTURE_TRANSCRIPT"
	fixtureTranscriptSegment   = "LOCALAI_FIXTURE_SEGMENT"

	// FixtureHealthMessage, FixtureTranscript, and FixtureTranscriptSegment
	// are the stable semantic values returned through the managed fixture
	// adapter. The lower-case aliases above keep package-local protocol tests
	// concise.
	FixtureHealthMessage     = fixtureHealthMessage
	FixtureTranscript        = fixtureTranscript
	FixtureTranscriptSegment = fixtureTranscriptSegment
)

// Mode selects the observable backend behavior for one fixture instance.
type Mode string

const (
	ModeNormal           Mode = "normal"
	ModeUnavailable      Mode = "unavailable"
	ModeProtocolMismatch Mode = "protocol-mismatch"
	ModeMalformed        Mode = "malformed-response"

	// Naming aliases keep failure-focused call sites aligned with the
	// acceptance vocabulary while retaining concise primary constants.
	ModeBackendUnavailable = ModeUnavailable
	ModeMalformedResponse  = ModeMalformed
)

// ErrInvalidMode reports a fixture configuration that is not one of the
// explicitly supported deterministic scenarios.
var ErrInvalidMode = errors.New("invalid LocalAI fixture mode")

// Options configures one fixture instance. It is test configuration only and
// does not alter production model, registry, artifact, or operator state.
type Options struct {
	Mode                Mode
	EmbeddingDimensions int
}

// FixtureOptions is the descriptive alias used by callers that prefer the
// longer name at a shared functional-test boundary.
type FixtureOptions = Options

// Call records the protocol-level request facts needed by conformance tests.
// Values are detached from the protobuf request and preserve repeated-input
// order.
type Call struct {
	Method      string
	Model       string
	Prompt      string
	Text        string
	Destination string
	Images      []string
	Audios      []string
	Videos      []string
}

// Fixture is a lifecycle-owned LocalAI gRPC backend with a dynamically bound
// loopback endpoint. It is safe for concurrent protocol calls and can be
// reused by sibling functional-test packages.
type Fixture struct {
	server     *grpc.Server
	listener   net.Listener
	endpoint   string
	backend    *backendServer
	serveReady chan struct{}
	serveDone  chan error

	closeOnce sync.Once
	closeErr  error
}

// New starts one fixture without coupling it to testing.TB. Most functional
// tests should use Start so cleanup is registered automatically.
func New(options Options) (*Fixture, error) {
	options, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for LocalAI fixture: %w", err)
	}

	fixture := &Fixture{
		listener:   listener,
		endpoint:   listener.Addr().String(),
		serveReady: make(chan struct{}),
		serveDone:  make(chan error, 1),
	}
	fixture.backend = &backendServer{options: options}
	fixture.server = grpc.NewServer()
	localaiproto.RegisterBackendServer(fixture.server, fixture.backend)
	// Keep the fixture's protobuf descriptors in their isolated functional
	// namespace, while also accepting the production pinned service name. The
	// raw production adapter deliberately invokes /backend.Backend/* and the
	// fixture must exercise that exact transport boundary.
	pinnedService := localaiproto.Backend_ServiceDesc
	pinnedService.ServiceName = "backend.Backend"
	fixture.server.RegisterService(&pinnedService, fixture.backend)
	go fixture.serve()
	<-fixture.serveReady
	return fixture, nil
}

// Start starts a fixture and attaches it to the caller's test cleanup. The
// variadic form permits both Start(t) and Start(t, Options{...}) at call sites.
func Start(t testing.TB, options ...Options) *Fixture {
	t.Helper()
	var option Options
	if len(options) > 1 {
		t.Fatalf("Start accepts at most one Options value")
	}
	if len(options) == 1 {
		option = options[0]
	}
	fixture, err := New(option)
	if err != nil {
		t.Fatalf("start LocalAI fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(); err != nil {
			t.Errorf("close LocalAI fixture: %v", err)
		}
	})
	return fixture
}

func normalizeOptions(options Options) (Options, error) {
	if options.Mode == "" {
		options.Mode = ModeNormal
	}
	switch options.Mode {
	case ModeNormal, ModeUnavailable, ModeProtocolMismatch, ModeMalformed:
	default:
		return Options{}, fmt.Errorf("%w: %q", ErrInvalidMode, options.Mode)
	}
	if options.EmbeddingDimensions == 0 {
		options.EmbeddingDimensions = DefaultEmbeddingDimensions
	}
	if options.EmbeddingDimensions < 1 {
		return Options{}, fmt.Errorf("embedding dimensions must be positive: %d", options.EmbeddingDimensions)
	}
	return options, nil
}

func (fixture *Fixture) serve() {
	close(fixture.serveReady)
	err := fixture.server.Serve(fixture.listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		err = nil
	}
	fixture.serveDone <- err
}

// Endpoint returns the dynamically bound host:port accepted by a gRPC dialer.
func (fixture *Fixture) Endpoint() string {
	if fixture == nil {
		return ""
	}
	return fixture.endpoint
}

// Close stops the server, closes active connections, and waits for Serve to
// release its listener. It is idempotent and performs no timed polling.
func (fixture *Fixture) Close() error {
	if fixture == nil {
		return nil
	}
	fixture.closeOnce.Do(func() {
		fixture.server.Stop()
		fixture.closeErr = <-fixture.serveDone
	})
	return fixture.closeErr
}

// Calls returns a detached snapshot of protocol calls in arrival order.
func (fixture *Fixture) Calls() []Call {
	if fixture == nil || fixture.backend == nil {
		return nil
	}
	return fixture.backend.callsSnapshot()
}

// ExpectedOmniText returns the exact deterministic text emitted by Predict.
// It deliberately includes indexed repeated media so callers can assert that
// order was preserved across the managed backend boundary.
func ExpectedOmniText(prompt string, images, audios, videos []string) string {
	parts := []string{"LOCALAI_FIXTURE_OMNI", "prompt=" + prompt}
	for index, image := range images {
		parts = append(parts, fmt.Sprintf("image[%d]=%s", index, image))
	}
	for index, audio := range audios {
		parts = append(parts, fmt.Sprintf("audio[%d]=%s", index, audio))
	}
	for index, video := range videos {
		parts = append(parts, fmt.Sprintf("video[%d]=%s", index, video))
	}
	return strings.Join(parts, "|")
}

// EmbeddingValues returns the deterministic vector used by Embedding.
func EmbeddingValues(dimensions int) []float32 {
	if dimensions < 1 {
		return nil
	}
	values := make([]float32, dimensions)
	for index := range values {
		values[index] = float32(index+1) / 10
	}
	return values
}

// AudioBytes returns a detached, valid PCM WAV payload used by TTSStream and
// written by TTS. The payload is intentionally non-trivial but model-free.
func AudioBytes() []byte {
	const (
		// Keep the deterministic header and samples UTF-8-safe because the
		// generic HTTP contract carries inline content as a JSON string.
		sampleRate = 8192
		channels   = 1
		bits       = 16
		samples    = 160
	)
	dataSize := samples * channels * bits / 8
	data := make([]byte, dataSize)
	for index := 0; index < samples; index++ {
		amplitude := int16(64 + index%32)
		binary.LittleEndian.PutUint16(data[index*2:], uint16(amplitude))
	}
	result := make([]byte, 44+len(data))
	copy(result[0:4], "RIFF")
	binary.LittleEndian.PutUint32(result[4:8], uint32(36+len(data)))
	copy(result[8:12], "WAVE")
	copy(result[12:16], "fmt ")
	binary.LittleEndian.PutUint32(result[16:20], 16)
	binary.LittleEndian.PutUint16(result[20:22], 1)
	binary.LittleEndian.PutUint16(result[22:24], channels)
	binary.LittleEndian.PutUint32(result[24:28], sampleRate)
	binary.LittleEndian.PutUint32(result[28:32], sampleRate*channels*bits/8)
	binary.LittleEndian.PutUint16(result[32:34], channels*bits/8)
	binary.LittleEndian.PutUint16(result[34:36], bits)
	copy(result[36:40], "data")
	binary.LittleEndian.PutUint32(result[40:44], uint32(len(data)))
	copy(result[44:], data)
	return result
}

type backendServer struct {
	localaiproto.UnimplementedBackendServer
	options Options

	callsMu sync.Mutex
	calls   []Call
}

var _ localaiproto.BackendServer = (*backendServer)(nil)

func (backend *backendServer) callsSnapshot() []Call {
	backend.callsMu.Lock()
	defer backend.callsMu.Unlock()
	calls := make([]Call, len(backend.calls))
	for index, call := range backend.calls {
		calls[index] = call
		calls[index].Images = append([]string(nil), call.Images...)
		calls[index].Audios = append([]string(nil), call.Audios...)
		calls[index].Videos = append([]string(nil), call.Videos...)
	}
	return calls
}

func (backend *backendServer) begin(ctx context.Context, call Call, failMode bool) error {
	backend.callsMu.Lock()
	backend.calls = append(backend.calls, call)
	backend.callsMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !failMode {
		return nil
	}
	switch backend.options.Mode {
	case ModeUnavailable:
		return status.Error(codes.Unavailable, "localai fixture backend unavailable")
	case ModeProtocolMismatch:
		return status.Error(codes.FailedPrecondition, "localai fixture backend protocol mismatch")
	default:
		return nil
	}
}

func (backend *backendServer) Health(ctx context.Context, _ *localaiproto.HealthMessage) (*localaiproto.Reply, error) {
	if err := backend.begin(ctx, Call{Method: "Health"}, false); err != nil {
		return nil, err
	}
	return &localaiproto.Reply{Message: []byte(fixtureHealthMessage)}, nil
}

func (backend *backendServer) Free(ctx context.Context, _ *localaiproto.HealthMessage) (*localaiproto.Result, error) {
	if err := backend.begin(ctx, Call{Method: "Free"}, true); err != nil {
		return nil, err
	}
	return &localaiproto.Result{Message: "fixture freed", Success: backend.options.Mode != ModeMalformed}, nil
}

func (backend *backendServer) LoadModel(ctx context.Context, request *localaiproto.ModelOptions) (*localaiproto.Result, error) {
	call := Call{Method: "LoadModel"}
	if request != nil {
		call.Model = request.GetModel()
	}
	if err := backend.begin(ctx, call, true); err != nil {
		return nil, err
	}
	if backend.options.Mode == ModeMalformed {
		return &localaiproto.Result{}, nil
	}
	return &localaiproto.Result{Message: "fixture model loaded", Success: true}, nil
}

func (backend *backendServer) Status(ctx context.Context, _ *localaiproto.HealthMessage) (*localaiproto.StatusResponse, error) {
	if err := backend.begin(ctx, Call{Method: "Status"}, true); err != nil {
		return nil, err
	}
	if backend.options.Mode == ModeMalformed {
		return &localaiproto.StatusResponse{}, nil
	}
	return &localaiproto.StatusResponse{State: localaiproto.StatusResponse_READY}, nil
}

func (backend *backendServer) Predict(ctx context.Context, request *localaiproto.PredictOptions) (*localaiproto.Reply, error) {
	if request == nil {
		request = &localaiproto.PredictOptions{}
	}
	call := Call{
		Method: "Predict",
		Prompt: request.GetPrompt(),
		Images: append([]string(nil), request.GetImages()...),
		Audios: append([]string(nil), request.GetAudios()...),
		Videos: append([]string(nil), request.GetVideos()...),
	}
	if err := backend.begin(ctx, call, true); err != nil {
		return nil, err
	}
	if backend.options.Mode == ModeMalformed {
		return &localaiproto.Reply{}, nil
	}
	text := ExpectedOmniText(call.Prompt, call.Images, call.Audios, call.Videos)
	return &localaiproto.Reply{Message: []byte(text), Tokens: int32(len(strings.Fields(text)))}, nil
}

func (backend *backendServer) PredictStream(request *localaiproto.PredictOptions, stream grpc.ServerStreamingServer[localaiproto.Reply]) error {
	response, err := backend.Predict(stream.Context(), request)
	if err != nil {
		return err
	}
	return stream.Send(response)
}

func (backend *backendServer) Embedding(ctx context.Context, request *localaiproto.PredictOptions) (*localaiproto.EmbeddingResult, error) {
	call := Call{Method: "Embedding"}
	if request != nil {
		call.Prompt = request.GetPrompt()
	}
	if err := backend.begin(ctx, call, true); err != nil {
		return nil, err
	}
	if backend.options.Mode == ModeMalformed {
		return &localaiproto.EmbeddingResult{}, nil
	}
	return &localaiproto.EmbeddingResult{Embeddings: EmbeddingValues(backend.options.EmbeddingDimensions)}, nil
}

func (backend *backendServer) TTS(ctx context.Context, request *localaiproto.TTSRequest) (*localaiproto.Result, error) {
	if request == nil {
		request = &localaiproto.TTSRequest{}
	}
	call := Call{Method: "TTS", Model: request.GetModel(), Text: request.GetText(), Destination: request.GetDst()}
	if err := backend.begin(ctx, call, true); err != nil {
		return nil, err
	}
	if backend.options.Mode == ModeMalformed {
		return &localaiproto.Result{}, nil
	}
	if destination := request.GetDst(); destination != "" {
		if err := os.WriteFile(destination, AudioBytes(), 0o644); err != nil {
			return nil, status.Error(codes.Internal, "localai fixture could not write audio output")
		}
	}
	return &localaiproto.Result{Message: "fixture audio written", Success: true}, nil
}

func (backend *backendServer) TTSStream(request *localaiproto.TTSRequest, stream grpc.ServerStreamingServer[localaiproto.Reply]) error {
	if request == nil {
		request = &localaiproto.TTSRequest{}
	}
	call := Call{Method: "TTSStream", Model: request.GetModel(), Text: request.GetText(), Destination: request.GetDst()}
	if err := backend.begin(stream.Context(), call, true); err != nil {
		return err
	}
	response := &localaiproto.Reply{}
	if backend.options.Mode != ModeMalformed {
		response.Audio = AudioBytes()
		response.Message = []byte("LOCALAI_FIXTURE_AUDIO")
	}
	return stream.Send(response)
}

func (backend *backendServer) AudioTranscription(ctx context.Context, request *localaiproto.TranscriptRequest) (*localaiproto.TranscriptResult, error) {
	call := Call{Method: "AudioTranscription"}
	if request != nil {
		call.Destination = request.GetDst()
		call.Prompt = request.GetPrompt()
	}
	if err := backend.begin(ctx, call, true); err != nil {
		return nil, err
	}
	if backend.options.Mode == ModeMalformed {
		return &localaiproto.TranscriptResult{}, nil
	}
	return fixtureTranscriptResult(), nil
}

func (backend *backendServer) AudioTranscriptionStream(request *localaiproto.TranscriptRequest, stream grpc.ServerStreamingServer[localaiproto.TranscriptStreamResponse]) error {
	response, err := backend.AudioTranscription(stream.Context(), request)
	if err != nil {
		return err
	}
	return stream.Send(&localaiproto.TranscriptStreamResponse{Delta: response.GetText(), FinalResult: response})
}

func fixtureTranscriptResult() *localaiproto.TranscriptResult {
	return &localaiproto.TranscriptResult{
		Text:     fixtureTranscript,
		Language: "en",
		Duration: 1.5,
		Segments: []*localaiproto.TranscriptSegment{{
			Id: 0, Start: 0, End: 1500, Text: fixtureTranscriptSegment,
		}},
	}
}
