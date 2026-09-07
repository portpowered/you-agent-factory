package localai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"google.golang.org/protobuf/proto"
)

func TestPinnedASRBackendStagesExactAudioAndMapsPinnedFields(t *testing.T) {
	t.Parallel()

	connection := &asrProtocolConnection{}
	connection.response, _ = proto.Marshal(&TranscriptResult{
		Text: "spoken words",
		Segments: []*TranscriptSegment{
			{Id: 0, Start: 0, End: 15_000_000, Text: "spoken words"},
			{Id: 1, Start: 15_000_000, End: 30_000_000, Text: "next words"},
		},
	})
	dialer := &asrProtocolDialer{connection: connection}
	temporary := &asrProtocolTempFile{path: `C:\private\.you-model-asr-123.wav`}
	var createdDirectory, createdPattern string
	var writtenPath string
	var writtenBytes []byte
	var removedPath string
	backend := NewPinnedASRBackend(
		dialer,
		func() string { return `C:\private` },
		func(directory, pattern string) (TempFile, error) {
			createdDirectory, createdPattern = directory, pattern
			return temporary, nil
		},
		func(path string, content []byte) error {
			writtenPath, writtenBytes = path, append([]byte(nil), content...)
			return nil
		},
		func(path string) error {
			removedPath = path
			return nil
		},
	)
	if backend == nil {
		t.Fatal("NewPinnedASRBackend() = nil, want production adapter")
	}

	audio := []byte{0x00, 0xff, 0x10, 0x80, 0x7f}
	response, err := backend(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:45902"),
		models.ASRBackendRequest{
			Audio: audio, MediaType: "audio/wav; charset=binary", Prompt: "meeting hint",
			Parameters: map[string]any{
				"language":                "en",
				"threads":                 json.Number("4"),
				"translate":               true,
				"diarize":                 false,
				"temperature":             json.Number("0.25"),
				"timestamp_granularities": []any{"segment", "word"},
			},
		},
	)
	if err != nil {
		t.Fatalf("ASR backend error = %v", err)
	}
	assertASRProtocolResponse(t, response)
	assertASRStaging(t, temporary, createdDirectory, createdPattern, writtenPath, writtenBytes, removedPath, audio)
	assertASRTransport(t, dialer, connection)
	assertASRRequest(t, connection.request, temporary.path)
}

func assertASRProtocolResponse(t *testing.T, response models.ASRBackendResponse) {
	t.Helper()
	if response.Text != "spoken words" || len(response.Segments) != 2 {
		t.Fatalf("ASR response = %#v, want decoded transcript and timestamp", response)
	}
	first, second := response.Segments[0], response.Segments[1]
	if first.ID != 0 || first.Start != 0 || first.End != 15 || first.Text != "spoken words" ||
		second.ID != 1 || second.Start != 15 || second.End != 30 || second.Text != "next words" {
		t.Fatalf("ASR response segments = %#v, want ordered whole-millisecond segments", response.Segments)
	}
}

func assertASRStaging(t *testing.T, temporary *asrProtocolTempFile, directory, pattern, path string, content []byte, removed string, audio []byte) {
	t.Helper()
	if directory != `C:\private` || pattern != ".you-model-asr-*.wav" {
		t.Fatalf("staging reservation = directory %q pattern %q, want private wav reservation", directory, pattern)
	}
	if temporary.closeCalls != 1 || path != temporary.path || !bytes.Equal(content, audio) || removed != temporary.path {
		t.Fatalf("staging lifecycle = close:%d write:%q bytes:%x remove:%q, want one close/exact bytes/remove", temporary.closeCalls, path, content, removed)
	}
}

func assertASRTransport(t *testing.T, dialer *asrProtocolDialer, connection *asrProtocolConnection) {
	t.Helper()
	if dialer.endpoint != "127.0.0.1:45902" || connection.method != localAITranscriptionMethod || connection.closed != 1 {
		t.Fatalf("transport facts = endpoint:%q method:%q closed:%d, want selected endpoint/AudioTranscription/one close", dialer.endpoint, connection.method, connection.closed)
	}
}

func assertASRRequest(t *testing.T, request *TranscriptRequest, path string) {
	t.Helper()
	if request == nil {
		t.Fatal("pinned ASR request = nil, want decoded request")
	}
	if request.GetDst() != path || request.GetPrompt() != "meeting hint" || request.GetLanguage() != "en" ||
		request.GetThreads() != 4 || !request.GetTranslate() || request.GetDiarize() || request.GetTemperature() != 0.25 ||
		!equalStrings(request.GetTimestampGranularities(), []string{"segment", "word"}) {
		t.Fatalf("pinned ASR request = %s, want exact staged path/prompt/parameters", request.String())
	}
}

func TestTranscriptRequestUsesSafeDefaultThreads(t *testing.T) {
	t.Parallel()

	request, err := transcriptRequest("temp/asr-input.wav", models.ASRBackendRequest{Prompt: "hint"})
	if err != nil {
		t.Fatalf("transcriptRequest() error = %v", err)
	}
	if request.GetDst() != "temp/asr-input.wav" || request.GetPrompt() != "hint" || request.GetThreads() != localAIASRDefaultThreads {
		t.Fatalf("transcript request = %s, want staged path, prompt, and default threads %d", request.String(), localAIASRDefaultThreads)
	}
}

func TestPinnedASRBackendCleansStagedInputOnFailureAndRecovers(t *testing.T) {
	t.Parallel()

	connection := &asrProtocolConnection{err: errors.New("backend transport detail")}
	connection.response, _ = proto.Marshal(&TranscriptResult{
		Text:     "recovered",
		Segments: []*TranscriptSegment{{Id: 1, Start: 10_000_000, End: 20_000_000, Text: "recovered"}},
	})
	dialer := &asrProtocolDialer{connection: connection}
	var files []*asrProtocolTempFile
	var removed []string
	backend := NewPinnedASRBackend(
		dialer,
		func() string { return "temp" },
		func(string, string) (TempFile, error) {
			file := &asrProtocolTempFile{path: "temp/asr-input"}
			files = append(files, file)
			return file, nil
		},
		func(string, []byte) error { return nil },
		func(path string) error {
			removed = append(removed, path)
			return nil
		},
	)
	request := models.ASRBackendRequest{Audio: []byte("audio"), MediaType: "audio/wav"}
	_, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45903"), request)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol || failure.Operation != models.OperationASR {
		t.Fatalf("failed ASR invocation = %v, failure = %#v, want typed ASR backend-protocol failure", err, failure)
	}
	if len(removed) != 1 || len(files) != 1 || files[0].closeCalls != 1 || connection.closed != 1 {
		t.Fatalf("failed cleanup = files:%d closes:%d removes:%d connections:%d, want one each", len(files), files[0].closeCalls, len(removed), connection.closed)
	}

	connection.err = nil
	response, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45903"), request)
	if err != nil || response.Text != "recovered" {
		t.Fatalf("recovery invocation = response:%#v error:%v, want generated response", response, err)
	}
	if len(removed) != 2 || len(files) != 2 || files[1].closeCalls != 1 || connection.closed != 2 {
		t.Fatalf("recovery cleanup = files:%d closes:%d removes:%d connections:%d, want second clean lifecycle", len(files), files[1].closeCalls, len(removed), connection.closed)
	}
}

func TestTranscriptResponseRejectsMalformedTimestampsAtomically(t *testing.T) {
	t.Parallel()

	valid := func(start, end int64) *TranscriptResult {
		return &TranscriptResult{
			Text:     "transcript",
			Segments: []*TranscriptSegment{{Id: 0, Start: start, End: end, Text: "transcript"}},
		}
	}
	tests := []struct {
		name     string
		response *TranscriptResult
	}{
		{name: "nil segment", response: &TranscriptResult{Text: "transcript", Segments: []*TranscriptSegment{nil}}},
		{name: "negative start", response: valid(-1, 1_000_000)},
		{name: "negative end", response: valid(0, -1)},
		{name: "minimum int64", response: valid(math.MinInt64, 1_000_000)},
		{name: "maximum int64", response: valid(0, math.MaxInt64)},
		{name: "non-integral start", response: valid(1, 1_000_000)},
		{name: "non-integral end", response: valid(0, 1_000_001)},
		{
			name: "invalid later segment does not publish an earlier segment",
			response: &TranscriptResult{
				Text: "transcript",
				Segments: []*TranscriptSegment{
					{Id: 0, Start: 0, End: 1_000_000, Text: "first"},
					{Id: 1, Start: 1_000_001, End: 2_000_000, Text: "second"},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped, err := transcriptResponse(test.response)
			if err == nil {
				t.Fatalf("transcriptResponse() error = nil, mapped = %#v; want malformed response", mapped)
			}
			if mapped.Text != "" || mapped.Segments != nil || mapped.Artifacts != nil {
				t.Fatalf("transcriptResponse() mapped partial response = %#v, want empty response", mapped)
			}
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse ||
				failure.Operation != models.OperationASR || failure.Slot != "segments" ||
				!errors.Is(err, models.ErrInferenceFailed) {
				t.Fatalf("transcriptResponse() error = %v, failure = %#v, want typed malformed ASR failure", err, failure)
			}
		})
	}
}

func TestTranscriptResponsePreservesNilBackendResponse(t *testing.T) {
	t.Parallel()

	mapped, err := transcriptResponse(nil)
	if err != nil || mapped.Text != "" || mapped.Segments != nil || mapped.Artifacts != nil {
		t.Fatalf("transcriptResponse(nil) = response:%#v error:%v, want empty response without error", mapped, err)
	}
}

func TestPinnedASRBackendPropagatesMalformedTimestampFailureAtomically(t *testing.T) {
	t.Parallel()

	connection := &asrProtocolConnection{}
	var err error
	connection.response, err = proto.Marshal(&TranscriptResult{
		Text:     "transcript",
		Segments: []*TranscriptSegment{{Id: 0, Start: 0, End: 1, Text: "transcript"}},
	})
	if err != nil {
		t.Fatalf("marshal malformed response: %v", err)
	}
	temporary := &asrProtocolTempFile{path: "temp/asr-malformed"}
	removed := 0
	backend := NewPinnedASRBackend(
		&asrProtocolDialer{connection: connection},
		func() string { return "temp" },
		func(string, string) (TempFile, error) { return temporary, nil },
		func(string, []byte) error { return nil },
		func(string) error {
			removed++
			return nil
		},
	)

	response, err := backend(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:45912"),
		models.ASRBackendRequest{Audio: []byte("audio"), MediaType: "audio/wav"},
	)
	if response.Text != "" || response.Segments != nil || response.Artifacts != nil {
		t.Fatalf("malformed ASR response = %#v, want no partial response", response)
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse ||
		failure.Operation != models.OperationASR || failure.Slot != "segments" ||
		!errors.Is(err, models.ErrInferenceFailed) {
		t.Fatalf("malformed ASR error = %v, failure = %#v, want typed failure", err, failure)
	}
	if temporary.closeCalls != 1 || removed != 1 || connection.closed != 1 {
		t.Fatalf("malformed ASR cleanup = close:%d remove:%d connection:%d, want one each", temporary.closeCalls, removed, connection.closed)
	}
}

func TestPinnedASRBackendHonorsCancellationAndStagingErrors(t *testing.T) {
	t.Parallel()

	t.Run("pre-cancelled", func(t *testing.T) {
		connection := &asrProtocolConnection{}
		calls := 0
		backend := NewPinnedASRBackend(
			&asrProtocolDialer{connection: connection},
			func() string { return "temp" },
			func(string, string) (TempFile, error) {
				calls++
				return &asrProtocolTempFile{path: "temp/asr-input"}, nil
			},
			func(string, []byte) error { return nil },
			func(string) error { return nil },
		)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := backend(ctx, models.ASRBackendRequest{Audio: []byte("audio"), MediaType: "audio/wav"})
		if !errors.Is(err, context.Canceled) || calls != 0 || connection.closed != 0 {
			t.Fatalf("pre-cancelled ASR = error:%v staged:%d closed:%d, want cancellation before effects", err, calls, connection.closed)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		connection := &asrProtocolConnection{waitForContext: true, started: make(chan struct{})}
		temporary := &asrProtocolTempFile{path: "temp/asr-timeout"}
		backend := NewPinnedASRBackend(
			&asrProtocolDialer{connection: connection},
			func() string { return "temp" },
			func(string, string) (TempFile, error) { return temporary, nil },
			func(string, []byte) error { return nil },
			func(string) error { return nil },
		)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := connection.started
		resultCh := make(chan error, 1)
		go func() {
			_, err := backend(WithInvocationEndpoint(ctx, "127.0.0.1:45905"), models.ASRBackendRequest{Audio: []byte("audio"), MediaType: "audio/wav"})
			resultCh <- err
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("ASR transport did not start")
		}
		err := <-resultCh
		if !errors.Is(err, context.DeadlineExceeded) || temporary.closeCalls != 1 || connection.closed != 1 {
			t.Fatalf("timed ASR = error:%v close:%d connection close:%d, want deadline and cleanup", err, temporary.closeCalls, connection.closed)
		}
	})

	t.Run("unwritable input", func(t *testing.T) {
		connection := &asrProtocolConnection{}
		temporary := &asrProtocolTempFile{path: "temp/asr-unwritable"}
		backend := NewPinnedASRBackend(
			&asrProtocolDialer{connection: connection},
			func() string { return "temp" },
			func(string, string) (TempFile, error) { return temporary, nil },
			func(string, []byte) error { return errors.New("disk detail") },
			func(string) error { return nil },
		)
		_, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45904"), models.ASRBackendRequest{Audio: []byte("audio"), MediaType: "audio/wav"})
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol || connection.method != "" || temporary.closeCalls != 1 {
			t.Fatalf("unwritable ASR = error:%v failure:%#v method:%q close:%d, want typed failure before RPC and cleanup", err, failure, connection.method, temporary.closeCalls)
		}
	})
}

type asrProtocolDialer struct {
	connection *asrProtocolConnection
	endpoint   string
}

func (dialer *asrProtocolDialer) Dial(_ context.Context, endpoint string) (platformgrpc.Connection, error) {
	dialer.endpoint = endpoint
	return dialer.connection, nil
}

type asrProtocolConnection struct {
	method         string
	request        *TranscriptRequest
	response       []byte
	err            error
	waitForContext bool
	started        chan struct{}
	closed         int
}

func (connection *asrProtocolConnection) Invoke(ctx context.Context, method string, payload []byte) ([]byte, error) {
	connection.method = method
	if method == localAITranscriptionMethod {
		request := &TranscriptRequest{}
		if err := proto.Unmarshal(payload, request); err != nil {
			return nil, err
		}
		connection.request = request
	}
	if connection.started != nil {
		close(connection.started)
		connection.started = nil
	}
	if connection.waitForContext {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if connection.err != nil {
		return nil, connection.err
	}
	return append([]byte(nil), connection.response...), nil
}

func (connection *asrProtocolConnection) Close() error {
	connection.closed++
	return nil
}

type asrProtocolTempFile struct {
	path       string
	closeErr   error
	closeCalls int
}

func (file *asrProtocolTempFile) Name() string { return file.path }

func (file *asrProtocolTempFile) Close() error {
	file.closeCalls++
	return file.closeErr
}
