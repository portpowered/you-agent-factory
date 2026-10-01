package codecs_test

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
)

// A small RF64 fixture exercises the 64-bit length representation without
// allocating a multi-gigabyte sample; ffmpeg selects it for large decoded WAVs.
func TestASRCodecRF64UsesDecodedSampleDuration(t *testing.T) {
	t.Parallel()
	codec := codecs.NewASRCodec()
	for _, end := range []int64{1000, 1001} {
		response := codecs.ASRResponse{Text: "hello", Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: end, Text: "hello"}}}
		outputs, err := codec.DecodeResponseValueWithinAudio(response, durationTestRF64())
		if end == 1000 {
			if err != nil || len(outputs) != 2 {
				t.Fatalf("RF64 transcript = %#v/%v", outputs, err)
			}
		} else {
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse || len(outputs) != 0 {
				t.Fatalf("RF64 over-duration response = %#v/%v, want atomic malformed response", outputs, err)
			}
		}
	}
}

func durationTestRF64() []byte {
	riff := make([]byte, 44+48000)
	copy(riff, durationTestWAV()[:44])
	binary.LittleEndian.PutUint32(riff[40:44], 48000)
	audio := make([]byte, len(riff)+36)
	copy(audio[:12], riff[:12])
	copy(audio[:4], "RF64")
	binary.LittleEndian.PutUint32(audio[4:8], ^uint32(0))
	copy(audio[12:16], "ds64")
	binary.LittleEndian.PutUint32(audio[16:20], 28)
	binary.LittleEndian.PutUint64(audio[20:28], uint64(len(audio)-8))
	binary.LittleEndian.PutUint64(audio[28:36], 48000)
	binary.LittleEndian.PutUint64(audio[36:44], 24000)
	copy(audio[48:], riff[12:])
	binary.LittleEndian.PutUint32(audio[76:80], ^uint32(0))
	return audio
}
