package videoaudio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
)

// ExtractVideoAudio returns a mono 16 kHz WAV for the first audio track in a
// video. A video with no audio track returns nil, nil. The caller supplies the
// process runner so decoding remains an explicit external effect.
func ExtractVideoAudio(
	ctx context.Context,
	runner platformprocess.CommandRunner,
	video []byte,
	tempDirectory func() string,
	createTemp localai.TempFileFactory,
	writeFile localai.InputFileWriter,
	readFile func(string) ([]byte, error),
	removeFile localai.InputFileRemover,
) ([]byte, error) {
	if err := validateVideoAudioInput(ctx, runner, video); err != nil {
		return nil, err
	}
	if tempDirectory == nil || createTemp == nil || writeFile == nil || readFile == nil || removeFile == nil {
		return nil, errors.New("video audio extraction requires temporary file staging")
	}
	inputPath, cleanupInput, err := reserveVideoAudioPath(tempDirectory, createTemp, removeFile, ".you-model-video-*")
	if err != nil {
		return nil, err
	}
	defer cleanupInput()
	if err := writeFile(inputPath, video); err != nil {
		return nil, fmt.Errorf("stage video audio input: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hasAudio, err := probeVideoAudio(ctx, runner, inputPath)
	if err != nil || !hasAudio {
		return nil, err
	}
	outputPath, cleanupOutput, err := reserveVideoAudioPath(tempDirectory, createTemp, removeFile, ".you-model-video-audio-*.wav")
	if err != nil {
		return nil, err
	}
	defer cleanupOutput()
	return decodeVideoAudio(ctx, runner, inputPath, outputPath, readFile)
}

func validateVideoAudioInput(ctx context.Context, runner platformprocess.CommandRunner, video []byte) error {
	if ctx == nil {
		return errors.New("video audio extraction requires a context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runner == nil {
		return errors.New("video audio extraction requires a process runner")
	}
	if len(video) == 0 {
		return errors.New("video audio extraction requires nonempty video data")
	}

	return nil
}

func probeVideoAudio(
	ctx context.Context,
	runner platformprocess.CommandRunner,
	inputPath string,
) (bool, error) {
	probe, err := runVideoAudioCommand(ctx, runner, platformprocess.CommandRequest{
		Command: "ffprobe",
		Args: []string{
			"-v", "error", "-show_entries", "stream=codec_type",
			"-of", "json", "-i", inputPath,
		},
	}, "probe")
	if err != nil {
		return false, err
	}
	return parseVideoAudioProbe(probe.Stdout)
}

func parseVideoAudioProbe(payload []byte) (bool, error) {
	var metadata struct {
		Streams *[]struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return false, fmt.Errorf("video audio probe returned invalid metadata: %w", err)
	}
	if metadata.Streams == nil {
		return false, errors.New("video audio probe omitted stream metadata")
	}
	var hasVideo, hasAudio bool
	for _, stream := range *metadata.Streams {
		hasVideo = hasVideo || stream.CodecType == "video"
		hasAudio = hasAudio || stream.CodecType == "audio"
	}
	if !hasVideo {
		return false, errors.New("video audio probe found no video stream")
	}
	if !hasAudio {
		return false, nil
	}
	return true, nil
}

func reserveVideoAudioPath(
	tempDirectory func() string, createTemp localai.TempFileFactory,
	removeFile localai.InputFileRemover, pattern string,
) (string, func(), error) {
	temporary, err := createTemp(tempDirectory(), pattern)
	if err != nil || temporary == nil || strings.TrimSpace(temporary.Name()) == "" {
		if temporary != nil {
			path := temporary.Name()
			_ = temporary.Close()
			if path != "" {
				_ = removeFile(path)
			}
		}
		if err == nil {
			err = errors.New("temporary file factory returned no usable path")
		}
		return "", func() {}, fmt.Errorf("reserve video audio temporary file: %w", err)
	}
	path := temporary.Name()
	cleanup := func() { _ = removeFile(path) }
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close video audio temporary file: %w", err)
	}
	return path, cleanup, nil
}

func decodeVideoAudio(
	ctx context.Context,
	runner platformprocess.CommandRunner,
	inputPath, outputPath string,
	readFile func(string) ([]byte, error),
) ([]byte, error) {
	_, err := runVideoAudioCommand(ctx, runner, platformprocess.CommandRequest{
		Command: "ffmpeg",
		Args: []string{
			"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", inputPath,
			"-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000",
			"-c:a", "pcm_s16le", "-f", "wav", "-rf64", "auto", outputPath,
		},
	}, "decoder")
	if err != nil {
		return nil, err
	}
	wav, err := readFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read decoded video audio: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(wav) < 44 || (!bytes.Equal(wav[:4], []byte("RIFF")) && !bytes.Equal(wav[:4], []byte("RF64"))) ||
		!bytes.Equal(wav[8:12], []byte("WAVE")) {
		return nil, errors.New("video audio decoder returned an invalid WAV")
	}
	return wav, nil
}

func runVideoAudioCommand(
	ctx context.Context,
	runner platformprocess.CommandRunner,
	request platformprocess.CommandRequest,
	name string,
) (platformprocess.CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("video audio %s failed: %w", name, err)
	}
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	if result.ExitCode != 0 {
		return platformprocess.CommandResult{}, fmt.Errorf("video audio %s exited with code %d: %s", name, result.ExitCode, boundedVideoAudioDiagnostic(result.Stderr))
	}
	return result, nil
}

func boundedVideoAudioDiagnostic(stderr []byte) string {
	const maxBytes = 256
	if len(stderr) > maxBytes {
		stderr = stderr[:maxBytes]
	}
	return strings.TrimSpace(string(stderr))
}
