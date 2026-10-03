package videoaudio

import (
	"bytes"
	"context"
	"errors"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	"os"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type videoAudioRunner struct {
	requests    []platformprocess.CommandRequest
	inputVideos [][]byte
	results     []platformprocess.CommandResult
	errs        []error
}

func (r *videoAudioRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.requests = append(r.requests, request)
	for index, arg := range request.Args {
		if arg == "-i" && index+1 < len(request.Args) {
			video, err := os.ReadFile(request.Args[index+1])
			if err != nil {
				return platformprocess.CommandResult{}, err
			}
			r.inputVideos = append(r.inputVideos, video)
		}
	}
	index := len(r.requests) - 1
	if index >= len(r.results) {
		return platformprocess.CommandResult{}, errors.New("unexpected command")
	}
	if index < len(r.errs) && r.errs[index] != nil {
		return platformprocess.CommandResult{}, r.errs[index]
	}
	result := r.results[index]
	if request.Command == "ffmpeg" && result.ExitCode == 0 {
		if err := os.WriteFile(request.Args[len(request.Args)-1], result.Stdout, 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		result.Stdout = nil
	}
	return result, nil
}

func testMP4Video() []byte {
	return []byte("\x00\x00\x00\x18ftypisommockvideo")
}

func testAudioWAV() []byte {
	wav := make([]byte, 48)
	copy(wav, "RIFF")
	copy(wav[8:], "WAVEfmt ")
	return wav
}

func TestExtractVideoAudioPassesMP4ThroughProbeAndDecoder(t *testing.T) {
	t.Parallel()
	video, wav := testMP4Video(), testAudioWAV()
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{
		{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"4.0"}}`)},
		{Stdout: wav},
	}}
	got, err := extractTestVideoAudio(t, context.Background(), runner, video)
	if err != nil || !bytes.Equal(got, wav) {
		t.Fatalf("extractTestVideoAudio(t, ) = %q, %v, want WAV", got, err)
	}
	if len(runner.requests) != 2 || runner.requests[0].Command != "ffprobe" || runner.requests[1].Command != "ffmpeg" {
		t.Fatalf("commands = %#v, want probe then decoder", runner.requests)
	}
	for index, request := range runner.requests {
		if len(request.Stdin) != 0 || !bytes.Equal(runner.inputVideos[index], video) {
			t.Fatalf("%s did not read original MP4 from seekable staging", request.Command)
		}
	}
	args := strings.Join(runner.requests[1].Args, " ")
	for _, option := range []string{"-map 0:a:0", "-ac 1", "-ar 16000", "-af aresample=16000:async=1:first_pts=0:min_hard_comp=0.001", "-c:a pcm_s16le", "-f wav", "-rf64 auto"} {
		if !strings.Contains(args, option) {
			t.Errorf("decoder args %q missing %q", args, option)
		}
	}
}

func TestExtractVideoAudioWithoutAudioTrack(t *testing.T) {
	t.Parallel()
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{{
		Stdout: []byte(`{"streams":[{"codec_type":"video"}],"format":{"duration":"4.0"}}`),
	}}}
	got, err := extractTestVideoAudio(t, context.Background(), runner, testMP4Video())
	if err != nil || got != nil || len(runner.requests) != 1 {
		t.Fatalf("no-audio result = %q, %v; requests = %d", got, err, len(runner.requests))
	}
}

func TestExtractVideoAudioRejectsProbeAndDecoderFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		runner  *videoAudioRunner
		message string
	}{
		{"probe failure", &videoAudioRunner{results: []platformprocess.CommandResult{{ExitCode: 1, Stderr: []byte("invalid input")}}}, "probe exited"},
		{"invalid probe", &videoAudioRunner{results: []platformprocess.CommandResult{{Stdout: []byte("garbage")}}}, "invalid metadata"},
		{"missing stream metadata", &videoAudioRunner{results: []platformprocess.CommandResult{{Stdout: []byte(`{"format":{"duration":"4"}}`)}}}, "omitted stream"},
		{"no video stream", &videoAudioRunner{results: []platformprocess.CommandResult{{Stdout: []byte(`{"streams":[{"codec_type":"audio"}],"format":{"duration":"4"}}`)}}}, "no video stream"},
		{"decoder failure", &videoAudioRunner{results: []platformprocess.CommandResult{
			{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"4"}}`)},
			{ExitCode: 1, Stderr: []byte("unsupported codec")},
		}}, "decoder exited"},
		{"invalid decoder output", &videoAudioRunner{results: []platformprocess.CommandResult{
			{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"4"}}`)},
			{Stdout: []byte("not a WAV")},
		}}, "invalid WAV"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := extractTestVideoAudio(t, context.Background(), test.runner, testMP4Video())
			if got != nil || err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("result = %q, %v, want %q error", got, err, test.message)
			}
		})
	}
}

func TestExtractVideoAudioRejectsInvalidInputBeforeStartingProcess(t *testing.T) {
	t.Parallel()
	runner := &videoAudioRunner{}
	for _, video := range [][]byte{nil, {}} {
		if _, err := extractTestVideoAudio(t, context.Background(), runner, video); err == nil {
			t.Errorf("input length %d unexpectedly accepted", len(video))
		}
	}
	if len(runner.requests) != 0 {
		t.Fatalf("started %d commands for invalid input", len(runner.requests))
	}
}

func TestExtractVideoAudioAcceptsOtherVideoContainersAfterProbe(t *testing.T) {
	t.Parallel()
	video := []byte("webm fixture bytes")
	wav := testAudioWAV()
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{
		{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"4"}}`)},
		{Stdout: wav},
	}}
	got, err := extractTestVideoAudio(t, context.Background(), runner, video)
	if err != nil || !bytes.Equal(got, wav) || len(runner.requests) != 2 {
		t.Fatalf("non-MP4 video result = %q, %v; requests = %d", got, err, len(runner.requests))
	}
	if !bytes.Equal(runner.inputVideos[0], video) {
		t.Fatal("non-MP4 video bytes did not reach the probe")
	}
}

func TestExtractVideoAudioRejectsInvalidNonemptyVideoAfterProbe(t *testing.T) {
	t.Parallel()
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{{
		ExitCode: 1, Stderr: []byte("invalid data found when processing input"),
	}}}
	got, err := extractTestVideoAudio(t, context.Background(), runner, []byte("invalid video"))
	if got != nil || err == nil || !strings.Contains(err.Error(), "probe exited") || len(runner.requests) != 1 {
		t.Fatalf("invalid video result = %q, %v; requests = %d", got, err, len(runner.requests))
	}
}

func extractTestVideoAudio(t *testing.T, ctx context.Context, runner platformprocess.CommandRunner, video []byte) ([]byte, error) {
	t.Helper()
	directory := t.TempDir()
	result, err := ExtractVideoAudio(ctx, runner, video, func() string { return directory },
		func(directory, pattern string) (localai.TempFile, error) { return os.CreateTemp(directory, pattern) },
		func(path string, content []byte) error { return os.WriteFile(path, content, 0o600) },
		os.ReadFile, os.Remove)
	entries, readErr := os.ReadDir(directory)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("temporary media staging leaked: entries=%v error=%v", entries, readErr)
	}
	return result, err
}

func TestExtractVideoAudioPreservesLargeInputAndCompleteLongAudio(t *testing.T) {
	t.Parallel()
	video := make([]byte, 9<<20)
	copy(video, testMP4Video())
	wav := make([]byte, 5<<20)
	copy(wav, testAudioWAV())
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{
		{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"1800"}}`)},
		{Stdout: wav},
	}}
	got, err := extractTestVideoAudio(t, t.Context(), runner, video)
	if err != nil || !bytes.Equal(got, wav) || !bytes.Equal(runner.inputVideos[1], video) {
		t.Fatalf("large/long video was rejected or truncated: bytes=%d error=%v", len(got), err)
	}
	args := strings.Join(runner.requests[1].Args, " ")
	if strings.Contains(args, "-fs ") || strings.Contains(args, "pipe:") {
		t.Fatalf("decoder args limit or pipe the seekable media: %s", args)
	}
}

func TestExtractVideoAudioUsesCallerCancellationAndCleansStaging(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	runner := &cancelingVideoAudioRunner{cancel: cancel}
	_, err := extractTestVideoAudio(t, ctx, runner, testMP4Video())
	if !errors.Is(err, context.Canceled) || runner.calls != 1 {
		t.Fatalf("cancellation = %v, commands=%d", err, runner.calls)
	}
}

type cancelingVideoAudioRunner struct {
	cancel context.CancelFunc
	calls  int
}

func (runner *cancelingVideoAudioRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls++
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return platformprocess.CommandResult{}, errors.New("media helper introduced a deadline")
	}
	runner.cancel()
	return platformprocess.CommandResult{}, ctx.Err()
}

func TestExtractVideoAudioAcceptsUnknownContainerDurationAndRF64Audio(t *testing.T) {
	t.Parallel()
	wav := testAudioWAV()
	copy(wav, "RF64")
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{
		{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}]}`)},
		{Stdout: wav},
	}}
	got, err := extractTestVideoAudio(t, t.Context(), runner, testMP4Video())
	if err != nil || !bytes.Equal(got, wav) {
		t.Fatalf("RF64 extraction with unknown container duration = %d bytes, %v", len(got), err)
	}
}

func TestNormalizeAudioDecodesWithoutRequiringVideoStream(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	wav := testAudioWAV()
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{{Stdout: wav}}}
	got, err := NormalizeAudio(t.Context(), runner, []byte("compressed audio"), func() string { return directory },
		func(directory, pattern string) (localai.TempFile, error) { return os.CreateTemp(directory, pattern) },
		func(path string, content []byte) error { return os.WriteFile(path, content, 0o600) }, os.ReadFile, os.Remove)
	if err != nil || !bytes.Equal(got, wav) || len(runner.requests) != 1 || runner.requests[0].Command != "ffmpeg" {
		t.Fatalf("normalization=%dbytes,%v commands=%v", len(got), err, runner.requests)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging leaked: %v,%v", entries, err)
	}
}
