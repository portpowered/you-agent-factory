package videoaudio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

const (
	maxVideoAudioInputBytes  = 8 << 20
	maxVideoAudioOutputBytes = 4 << 20
	videoAudioBytesPerSecond = 16000 * 2 // mono 16 kHz PCM s16le
	videoAudioWAVHeaderBytes = 4096      // room for WAV metadata beyond PCM
	videoAudioTimeout        = 2 * time.Minute
)

// ExtractVideoAudio returns a mono 16 kHz WAV for the first audio track in a
// video. A video with no audio track returns nil, nil. The caller supplies the
// process runner so decoding remains an explicit external effect.
func ExtractVideoAudio(
	ctx context.Context,
	runner platformprocess.CommandRunner,
	video []byte,
) ([]byte, error) {
	if err := validateVideoAudioInput(ctx, runner, video); err != nil {
		return nil, err
	}
	decodeCtx, cancel := context.WithTimeout(ctx, videoAudioTimeout)
	defer cancel()
	duration, hasAudio, err := probeVideoAudio(decodeCtx, runner, video)
	if err != nil || !hasAudio {
		return nil, err
	}
	if err := validateVideoAudioDuration(duration); err != nil {
		return nil, err
	}
	return decodeVideoAudio(decodeCtx, runner, video)
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
	if len(video) > maxVideoAudioInputBytes {
		return fmt.Errorf("video exceeds the %d-byte audio extraction limit", maxVideoAudioInputBytes)
	}
	return nil
}

func probeVideoAudio(
	ctx context.Context,
	runner platformprocess.CommandRunner,
	video []byte,
) (float64, bool, error) {
	probe, err := runVideoAudioCommand(ctx, runner, platformprocess.CommandRequest{
		Command: "ffprobe",
		Args: []string{
			"-v", "error", "-show_entries", "stream=codec_type:format=duration",
			"-of", "json", "-i", "pipe:0",
		},
		Stdin: video,
	}, "probe")
	if err != nil {
		return 0, false, err
	}
	return parseVideoAudioProbe(probe.Stdout)
}

func parseVideoAudioProbe(payload []byte) (float64, bool, error) {
	var metadata struct {
		Streams *[]struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return 0, false, fmt.Errorf("video audio probe returned invalid metadata: %w", err)
	}
	if metadata.Streams == nil {
		return 0, false, errors.New("video audio probe omitted stream metadata")
	}
	var hasVideo, hasAudio bool
	for _, stream := range *metadata.Streams {
		hasVideo = hasVideo || stream.CodecType == "video"
		hasAudio = hasAudio || stream.CodecType == "audio"
	}
	if !hasVideo {
		return 0, false, errors.New("video audio probe found no video stream")
	}
	if !hasAudio {
		return 0, false, nil
	}
	duration, err := strconv.ParseFloat(metadata.Format.Duration, 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 {
		return 0, false, errors.New("video audio probe returned an invalid duration")
	}
	return duration, true, nil
}

func validateVideoAudioDuration(duration float64) error {
	// The decoder's 4 MiB stdout guard must fit every PCM sample plus a WAV
	// header. Reject a clip that would otherwise be truncated by ffmpeg -fs.
	if duration*videoAudioBytesPerSecond+videoAudioWAVHeaderBytes > maxVideoAudioOutputBytes {
		return fmt.Errorf("video audio duration exceeds the %d-byte decoded WAV limit", maxVideoAudioOutputBytes)
	}
	return nil
}

func decodeVideoAudio(
	ctx context.Context,
	runner platformprocess.CommandRunner,
	video []byte,
) ([]byte, error) {
	decoded, err := runVideoAudioCommand(ctx, runner, platformprocess.CommandRequest{
		Command: "ffmpeg",
		Args: []string{
			"-nostdin", "-hide_banner", "-loglevel", "error", "-i", "pipe:0",
			"-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000",
			"-c:a", "pcm_s16le", "-f", "wav", "-fs", strconv.Itoa(maxVideoAudioOutputBytes), "pipe:1",
		},
		Stdin: video,
	}, "decoder")
	if err != nil {
		return nil, err
	}
	if len(decoded.Stdout) >= maxVideoAudioOutputBytes {
		return nil, fmt.Errorf("decoded video audio exceeds the %d-byte limit", maxVideoAudioOutputBytes)
	}
	if len(decoded.Stdout) < 44 || !bytes.Equal(decoded.Stdout[:4], []byte("RIFF")) ||
		!bytes.Equal(decoded.Stdout[8:12], []byte("WAVE")) {
		return nil, errors.New("video audio decoder returned an invalid WAV")
	}
	return decoded.Stdout, nil
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
