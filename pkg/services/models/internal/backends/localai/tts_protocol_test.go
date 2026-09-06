package localai

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestPinnedTTSProtocolUsesConfirmedWireFields(t *testing.T) {
	t.Parallel()

	fields := (&TTSRequest{}).ProtoReflect().Descriptor().Fields()
	for name, wantNumber := range map[string]int{"text": 1, "model": 2, "dst": 3, "voice": 4, "language": 5, "instructions": 6, "params": 7} {
		field := fields.ByName(protoreflect.Name(name))
		if field == nil || int(field.Number()) != wantNumber {
			t.Fatalf("TTSRequest field %q = %v, want wire number %d", name, field, wantNumber)
		}
	}
	replyAudio := (&Reply{}).ProtoReflect().Descriptor().Fields().ByName("audio")
	if replyAudio == nil || int(replyAudio.Number()) != 6 {
		t.Fatalf("Reply.audio field = %v, want wire number 6", replyAudio)
	}
}

func TestPinnedTTSBackendMapsPrivateRequestAndCleansOutput(t *testing.T) {
	t.Parallel()

	connection := &ttsProtocolConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true, Message: "private backend detail"})
	dialer := &ttsProtocolDialer{connection: connection}
	temporary := &ttsProtocolTempFile{path: `C:\private\.you-model-tts-123.wav`}
	audio := ttsProtocolWAV()
	var createdDirectory, createdPattern, inspectedPath, readPath, removedPath string
	backend := NewPinnedTTSBackend(
		dialer,
		func() string { return `C:\private` },
		func(directory, pattern string) (TempFile, error) {
			createdDirectory, createdPattern = directory, pattern
			return temporary, nil
		},
		func(path string) (os.FileInfo, error) {
			inspectedPath = path
			return ttsProtocolFileInfo{size: int64(len(audio))}, nil
		},
		func(path string) ([]byte, error) {
			readPath = path
			return append([]byte(nil), audio...), nil
		},
		func(path string) error {
			removedPath = path
			return nil
		},
	)
	if backend == nil {
		t.Fatal("NewPinnedTTSBackend() = nil, want complete private adapter")
	}
	response, err := backend(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:45906"),
		codecs.TTSRequest{
			Text: "hello", Model: "tts",
			Voice:      "voice-bytes",
			Parameters: map[string]any{"language": "en", "instructions": "speak clearly"},
		},
	)
	assertTTSProtocolResponse(t, err, response, audio)
	assertTTSProtocolOutputLifecycle(t, temporary, createdDirectory, createdPattern, inspectedPath, readPath, removedPath)
	assertTTSProtocolTransport(t, connection, dialer)
	assertTTSProtocolRequest(t, &connection.request, temporary.path)
}

func assertTTSProtocolResponse(t *testing.T, err error, response codecs.TTSResponse, audio []byte) {
	t.Helper()
	if err != nil {
		t.Fatalf("TTS backend error = %v", err)
	}
	if string(response.Audio) != string(audio) || response.MediaType != "audio/wav" {
		t.Fatalf("TTS response = %#v, want detached WAV response", response)
	}
}

func assertTTSProtocolOutputLifecycle(
	t *testing.T,
	temporary *ttsProtocolTempFile,
	createdDirectory, createdPattern, inspectedPath, readPath, removedPath string,
) {
	t.Helper()
	if createdDirectory != `C:\private` || createdPattern != ttsOutputFilePattern || inspectedPath != temporary.path || readPath != temporary.path || removedPath != temporary.path {
		t.Fatalf("private output lifecycle = directory:%q pattern:%q inspect:%q read:%q remove:%q, want exact private destination", createdDirectory, createdPattern, inspectedPath, readPath, removedPath)
	}
	if temporary.closeCalls != 1 {
		t.Fatalf("temporary close calls = %d, want one", temporary.closeCalls)
	}
}

func assertTTSProtocolTransport(t *testing.T, connection *ttsProtocolConnection, dialer *ttsProtocolDialer) {
	t.Helper()
	if connection.closed != 1 || dialer.endpoint != "127.0.0.1:45906" || connection.method != localAITTSMethod {
		t.Fatalf("transport lifecycle = connectionClose:%d endpoint:%q method:%q, want one close and TTS method", connection.closed, dialer.endpoint, connection.method)
	}
}

func assertTTSProtocolRequest(t *testing.T, request *TTSRequest, destination string) {
	t.Helper()
	if request.GetText() != "hello" || request.GetModel() != "tts" || request.GetVoice() != "voice-bytes" || request.GetDst() != destination || request.GetLanguage() != "en" || request.GetInstructions() != "speak clearly" || len(request.GetParams()) != 0 {
		t.Fatalf("private TTS request = %s, want exact text/model/voice/destination/confirmed fields", request.String())
	}
}

func TestPinnedTTSBackendFailureIsAtomicAndRecovers(t *testing.T) {
	t.Parallel()

	connection := &ttsProtocolConnection{}
	dialer := &ttsProtocolDialer{connection: connection}
	files := []*ttsProtocolTempFile{}
	removed := []string{}
	audio := ttsProtocolWAV()
	readCalls := 0
	backend := NewPinnedTTSBackend(
		dialer,
		func() string { return "temp" },
		func(string, string) (TempFile, error) {
			file := &ttsProtocolTempFile{path: "temp/tts-output.wav"}
			files = append(files, file)
			return file, nil
		},
		func(string) (os.FileInfo, error) { return ttsProtocolFileInfo{size: int64(len(audio))}, nil },
		func(string) ([]byte, error) {
			readCalls++
			return audio, nil
		},
		func(path string) error {
			removed = append(removed, path)
			return nil
		},
	)
	request := codecs.TTSRequest{Text: "recover", Model: "tts"}
	connection.response, _ = proto.Marshal(&Result{Success: false, Message: "secret backend path"})
	_, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45907"), request)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol || containsAny(err.Error(), "secret backend path") {
		t.Fatalf("failed TTS = %v failure:%#v, want safe typed protocol failure", err, failure)
	}
	if readCalls != 0 || len(removed) != 1 || files[0].closeCalls != 1 || connection.closed != 1 {
		t.Fatalf("failed cleanup = reads:%d removes:%d tempClose:%d connectionClose:%d, want atomic cleanup without read", readCalls, len(removed), files[0].closeCalls, connection.closed)
	}

	connection.response, _ = proto.Marshal(&Result{Success: true})
	response, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45907"), request)
	if err != nil || string(response.Audio) != string(audio) {
		t.Fatalf("recovery TTS = response:%#v error:%v, want valid audio", response, err)
	}
	if len(removed) != 2 || len(files) != 2 || files[1].closeCalls != 1 || connection.closed != 2 || readCalls != 1 {
		t.Fatalf("recovery cleanup = files:%d removes:%d tempClose:%d connectionClose:%d reads:%d, want second isolated lifecycle", len(files), len(removed), files[1].closeCalls, connection.closed, readCalls)
	}
}

func TestPinnedTTSBackendRejectsMalformedResultAndAudioBounds(t *testing.T) {
	t.Parallel()

	success, err := proto.Marshal(&Result{Success: true})
	if err != nil {
		t.Fatalf("marshal success result: %v", err)
	}
	cases := []struct {
		name         string
		response     []byte
		fileSize     int64
		fileContents []byte
		wantInspect  int
		wantReads    int
	}{
		{name: "malformed result", response: []byte{0xff}},
		{name: "empty output", response: success, fileSize: 0, wantInspect: 1},
		{name: "non-wav output", response: success, fileSize: int64(len("not audio")), fileContents: []byte("not audio"), wantInspect: 1, wantReads: 1},
		{name: "oversized output", response: success, fileSize: codecs.MaxTTSAudioBytes + 1, wantInspect: 1},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			connection := &ttsProtocolConnection{response: test.response}
			temporary := &ttsProtocolTempFile{path: `C:\private\tts-output.wav`}
			inspectCalls, readCalls, removeCalls := 0, 0, 0
			backend := NewPinnedTTSBackend(
				&ttsProtocolDialer{connection: connection},
				func() string { return `C:\private` },
				func(string, string) (TempFile, error) { return temporary, nil },
				func(string) (os.FileInfo, error) {
					inspectCalls++
					return ttsProtocolFileInfo{size: test.fileSize}, nil
				},
				func(string) ([]byte, error) {
					readCalls++
					return append([]byte(nil), test.fileContents...), nil
				},
				func(string) error {
					removeCalls++
					return nil
				},
			)
			_, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45910"), codecs.TTSRequest{Text: "hello"})
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
				t.Fatalf("TTS = %v failure:%#v, want malformed typed failure", err, failure)
			}
			if containsAny(err.Error(), temporary.path) || inspectCalls != test.wantInspect || readCalls != test.wantReads || removeCalls != 1 || temporary.closeCalls != 1 || connection.closed != 1 {
				t.Fatalf("bounded failure lifecycle = inspect:%d reads:%d removes:%d tempClose:%d connectionClose:%d error:%v, want no partial output and one cleanup", inspectCalls, readCalls, removeCalls, temporary.closeCalls, connection.closed, err)
			}
		})
	}
}

func TestPinnedTTSBackendPreservesCancellationAndReadinessClassification(t *testing.T) {
	t.Parallel()

	t.Run("pre-cancelled", func(t *testing.T) {
		t.Parallel()
		calls := 0
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		backend := NewPinnedTTSBackend(
			&ttsProtocolDialer{connection: &ttsProtocolConnection{}},
			func() string { return "temp" },
			func(string, string) (TempFile, error) {
				calls++
				return &ttsProtocolTempFile{path: "temp/out.wav"}, nil
			},
			func(string) (os.FileInfo, error) { return ttsProtocolFileInfo{size: 1}, nil },
			func(string) ([]byte, error) { return nil, nil },
			func(string) error { return nil },
		)
		_, err := backend(ctx, codecs.TTSRequest{Text: "hello"})
		if !errors.Is(err, context.Canceled) || calls != 0 {
			t.Fatalf("pre-cancelled TTS = error:%v createCalls:%d, want cancellation before effects", err, calls)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		backend := NewPinnedTTSBackend(
			ttsUnavailableDialer{},
			func() string { return "temp" },
			func(string, string) (TempFile, error) { return &ttsProtocolTempFile{path: "temp/unavailable.wav"}, nil },
			func(string) (os.FileInfo, error) { return ttsProtocolFileInfo{size: 1}, nil },
			func(string) ([]byte, error) { return nil, nil },
			func(string) error { return nil },
		)
		_, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45908"), codecs.TTSRequest{Text: "hello"})
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendReadiness || !errors.Is(err, models.ErrUnavailable) {
			t.Fatalf("unavailable TTS = %v failure:%#v, want readiness classification", err, failure)
		}
	})

	t.Run("incompatible protocol", func(t *testing.T) {
		t.Parallel()
		backend := NewPinnedTTSBackend(
			&ttsProtocolDialer{err: status.Error(codes.FailedPrecondition, "private protocol detail")},
			func() string { return "temp" },
			func(string, string) (TempFile, error) {
				return &ttsProtocolTempFile{path: "temp/incompatible.wav"}, nil
			},
			func(string) (os.FileInfo, error) { return ttsProtocolFileInfo{size: 1}, nil },
			func(string) ([]byte, error) { return nil, nil },
			func(string) error { return nil },
		)
		_, err := backend(WithInvocationEndpoint(context.Background(), "127.0.0.1:45911"), codecs.TTSRequest{Text: "hello"})
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol || containsAny(err.Error(), "private protocol detail") {
			t.Fatalf("incompatible TTS = %v failure:%#v, want safe protocol classification", err, failure)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		t.Parallel()
		connection := &ttsProtocolConnection{waitForContext: true, started: make(chan struct{})}
		temporary := &ttsProtocolTempFile{path: "temp/deadline.wav"}
		backend := NewPinnedTTSBackend(
			&ttsProtocolDialer{connection: connection},
			func() string { return "temp" },
			func(string, string) (TempFile, error) { return temporary, nil },
			func(string) (os.FileInfo, error) { return ttsProtocolFileInfo{size: 1}, nil },
			func(string) ([]byte, error) { return nil, nil },
			func(string) error { return nil },
		)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, err := backend(WithInvocationEndpoint(ctx, "127.0.0.1:45909"), codecs.TTSRequest{Text: "hello"})
			result <- err
		}()
		select {
		case <-connection.started:
			cancel()
		case <-time.After(time.Second):
			t.Fatal("TTS transport did not start")
		}
		if err := <-result; !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline TTS = %v, want context cancellation", err)
		}
		if temporary.closeCalls != 1 || connection.closed != 1 {
			t.Fatalf("deadline cleanup = tempClose:%d connectionClose:%d, want one each", temporary.closeCalls, connection.closed)
		}
	})
}

func containsAny(value string, sentinels ...string) bool {
	for _, sentinel := range sentinels {
		if strings.Contains(value, sentinel) {
			return true
		}
	}
	return false
}

type ttsProtocolDialer struct {
	connection *ttsProtocolConnection
	endpoint   string
	err        error
}

func (dialer *ttsProtocolDialer) Dial(_ context.Context, endpoint string) (platformgrpc.Connection, error) {
	dialer.endpoint = endpoint
	if dialer.err != nil {
		return nil, dialer.err
	}
	return dialer.connection, nil
}

type ttsUnavailableDialer struct{}

func (ttsUnavailableDialer) Dial(context.Context, string) (platformgrpc.Connection, error) {
	return nil, models.ErrUnavailable
}

type ttsProtocolConnection struct {
	method         string
	request        TTSRequest
	response       []byte
	waitForContext bool
	started        chan struct{}
	invokes        int
	closed         int
}

func (connection *ttsProtocolConnection) Invoke(ctx context.Context, method string, payload []byte) ([]byte, error) {
	connection.invokes++
	connection.method = method
	if method == localAITTSMethod {
		if err := proto.Unmarshal(payload, &connection.request); err != nil {
			return nil, err
		}
	}
	if connection.started != nil {
		close(connection.started)
	}
	if connection.waitForContext {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return append([]byte(nil), connection.response...), nil
}

func (connection *ttsProtocolConnection) Close() error {
	connection.closed++
	return nil
}

type ttsProtocolTempFile struct {
	path       string
	closeCalls int
}

func (file *ttsProtocolTempFile) Name() string { return file.path }

func (file *ttsProtocolTempFile) Close() error {
	file.closeCalls++
	return nil
}

type ttsProtocolFileInfo struct{ size int64 }

func (info ttsProtocolFileInfo) Name() string       { return "tts.wav" }
func (info ttsProtocolFileInfo) Size() int64        { return info.size }
func (info ttsProtocolFileInfo) Mode() os.FileMode  { return 0 }
func (info ttsProtocolFileInfo) ModTime() time.Time { return time.Time{} }
func (info ttsProtocolFileInfo) IsDir() bool        { return false }
func (info ttsProtocolFileInfo) Sys() any           { return nil }

func ttsProtocolWAV() []byte {
	audio := make([]byte, 46)
	copy(audio[0:4], "RIFF")
	binary.LittleEndian.PutUint32(audio[4:8], uint32(len(audio)-8))
	copy(audio[8:12], "WAVE")
	copy(audio[12:16], "fmt ")
	binary.LittleEndian.PutUint32(audio[16:20], 16)
	binary.LittleEndian.PutUint16(audio[20:22], 1)
	binary.LittleEndian.PutUint16(audio[22:24], 1)
	binary.LittleEndian.PutUint32(audio[24:28], 24000)
	binary.LittleEndian.PutUint32(audio[28:32], 48000)
	binary.LittleEndian.PutUint16(audio[32:34], 2)
	binary.LittleEndian.PutUint16(audio[34:36], 16)
	copy(audio[36:40], "data")
	binary.LittleEndian.PutUint32(audio[40:44], 2)
	audio[44] = 0x01
	audio[45] = 0x02
	return audio
}
