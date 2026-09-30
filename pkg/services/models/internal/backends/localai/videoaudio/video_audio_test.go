package videoaudio

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type videoAudioRunner struct {
	requests []platformprocess.CommandRequest
	results  []platformprocess.CommandResult
	errs     []error
}

func (r *videoAudioRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.requests = append(r.requests, request)
	index := len(r.requests) - 1
	if index >= len(r.results) {
		return platformprocess.CommandResult{}, errors.New("unexpected command")
	}
	if index < len(r.errs) && r.errs[index] != nil {
		return platformprocess.CommandResult{}, r.errs[index]
	}
	return r.results[index], nil
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
	got, err := ExtractVideoAudio(context.Background(), runner, video)
	if err != nil || !bytes.Equal(got, wav) {
		t.Fatalf("ExtractVideoAudio() = %q, %v, want WAV", got, err)
	}
	if len(runner.requests) != 2 || runner.requests[0].Command != "ffprobe" || runner.requests[1].Command != "ffmpeg" {
		t.Fatalf("commands = %#v, want probe then decoder", runner.requests)
	}
	for _, request := range runner.requests {
		if !bytes.Equal(request.Stdin, video) {
			t.Fatalf("%s stdin differs from original MP4", request.Command)
		}
	}
	args := strings.Join(runner.requests[1].Args, " ")
	for _, option := range []string{"-map 0:a:0", "-ac 1", "-ar 16000", "-c:a pcm_s16le", "-f wav"} {
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
	got, err := ExtractVideoAudio(context.Background(), runner, testMP4Video())
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
		{"oversized decoded audio", &videoAudioRunner{results: []platformprocess.CommandResult{{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"135"}}`)}}}, "duration"},
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
			got, err := ExtractVideoAudio(context.Background(), test.runner, testMP4Video())
			if got != nil || err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("result = %q, %v, want %q error", got, err, test.message)
			}
		})
	}
}

func TestExtractVideoAudioRejectsInvalidInputBeforeStartingProcess(t *testing.T) {
	t.Parallel()
	runner := &videoAudioRunner{}
	oversized := make([]byte, maxVideoAudioInputBytes+1)
	copy(oversized, testMP4Video())
	for _, video := range [][]byte{nil, {}, oversized} {
		if _, err := ExtractVideoAudio(context.Background(), runner, video); err == nil {
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
	got, err := ExtractVideoAudio(context.Background(), runner, video)
	if err != nil || !bytes.Equal(got, wav) || len(runner.requests) != 2 {
		t.Fatalf("non-MP4 video result = %q, %v; requests = %d", got, err, len(runner.requests))
	}
	if !bytes.Equal(runner.requests[0].Stdin, video) {
		t.Fatal("non-MP4 video bytes did not reach the probe")
	}
}

func TestExtractVideoAudioRejectsInvalidNonemptyVideoAfterProbe(t *testing.T) {
	t.Parallel()
	runner := &videoAudioRunner{results: []platformprocess.CommandResult{{
		ExitCode: 1, Stderr: []byte("invalid data found when processing input"),
	}}}
	got, err := ExtractVideoAudio(context.Background(), runner, []byte("invalid video"))
	if got != nil || err == nil || !strings.Contains(err.Error(), "probe exited") || len(runner.requests) != 1 {
		t.Fatalf("invalid video result = %q, %v; requests = %d", got, err, len(runner.requests))
	}
}
