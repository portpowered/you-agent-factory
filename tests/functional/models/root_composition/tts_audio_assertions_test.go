package root_composition_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"
)

func ttsDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func assertSemanticTTSAudio(t *testing.T, audio []byte, label string) {
	t.Helper()
	const (
		wavHeaderSize  = 44
		pcmFormat      = 1
		monoChannels   = 1
		bitsPerSample  = 16
		minimumSamples = 1
	)
	if len(audio) < wavHeaderSize {
		t.Fatalf("%s audio length = %d, want at least %d-byte WAV header", label, len(audio), wavHeaderSize)
	}
	if string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		t.Fatalf("%s audio has invalid RIFF/WAVE PCM chunk markers", label)
	}
	if got := uint64(binary.LittleEndian.Uint32(audio[4:8])) + 8; got != uint64(len(audio)) {
		t.Fatalf("%s RIFF size = %d, want payload length %d", label, got, len(audio))
	}
	if got := binary.LittleEndian.Uint32(audio[16:20]); got != 16 {
		t.Fatalf("%s fmt chunk size = %d, want PCM fmt size 16", label, got)
	}
	if got := binary.LittleEndian.Uint16(audio[20:22]); got != pcmFormat {
		t.Fatalf("%s audio format = %d, want PCM format %d", label, got, pcmFormat)
	}
	channels := binary.LittleEndian.Uint16(audio[22:24])
	if channels != monoChannels {
		t.Fatalf("%s channels = %d, want %d", label, channels, monoChannels)
	}
	sampleRate := binary.LittleEndian.Uint32(audio[24:28])
	byteRate := binary.LittleEndian.Uint32(audio[28:32])
	blockAlign := binary.LittleEndian.Uint16(audio[32:34])
	bits := binary.LittleEndian.Uint16(audio[34:36])
	if sampleRate == 0 || blockAlign == 0 || bits != bitsPerSample {
		t.Fatalf("%s PCM format = sampleRate:%d blockAlign:%d bits:%d, want nonzero/16-bit PCM", label, sampleRate, blockAlign, bits)
	}
	if want := sampleRate * uint32(blockAlign); byteRate != want {
		t.Fatalf("%s byte rate = %d, want %d from sample rate and block alignment", label, byteRate, want)
	}
	dataSize := binary.LittleEndian.Uint32(audio[40:44])
	if uint64(dataSize)+wavHeaderSize != uint64(len(audio)) || dataSize < minimumSamples*uint32(blockAlign) || dataSize%uint32(blockAlign) != 0 {
		t.Fatalf("%s data chunk size = %d, want aligned nonempty payload for %d-byte samples", label, dataSize, blockAlign)
	}
	duration := time.Second * time.Duration(dataSize/uint32(blockAlign)) / time.Duration(sampleRate)
	if duration <= 0 {
		t.Fatalf("%s duration = %s, want nonzero duration", label, duration)
	}
	t.Logf("semantic audio baseline label=%s mediaType=audio/wav codec=PCM channels=%d sampleRate=%d bits=%d bytes=%d sha256=%s duration=%s", label, channels, sampleRate, bits, len(audio), ttsDigest(audio), duration)
}
