package omnivoice

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeReferenceWAV_ResamplesStereoPCM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.wav")
	if err := os.WriteFile(path, testWAV(48_000, 2, []int16{32767, 32767, 0, 0, -32768, -32768, 0, 0}), 0o644); err != nil {
		t.Fatal(err)
	}
	samples, err := DecodeReferenceWAV(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[0] < 0.99 || samples[1] > -0.99 {
		t.Fatalf("samples = %#v", samples)
	}
}

func TestDecodeReferenceWAV_RejectsNonWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.wav")
	if err := os.WriteFile(path, []byte("not a wav"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReferenceWAV(path); err == nil {
		t.Fatal("expected malformed WAV error")
	}
}

func testWAV(sampleRate uint32, channels uint16, samples []int16) []byte {
	dataSize := len(samples) * 2
	result := make([]byte, 44+dataSize)
	copy(result[0:4], "RIFF")
	binary.LittleEndian.PutUint32(result[4:8], uint32(36+dataSize))
	copy(result[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(result[16:20], 16)
	binary.LittleEndian.PutUint16(result[20:22], 1)
	binary.LittleEndian.PutUint16(result[22:24], channels)
	binary.LittleEndian.PutUint32(result[24:28], sampleRate)
	binary.LittleEndian.PutUint32(result[28:32], sampleRate*uint32(channels)*2)
	binary.LittleEndian.PutUint16(result[32:34], channels*2)
	binary.LittleEndian.PutUint16(result[34:36], 16)
	copy(result[36:40], "data")
	binary.LittleEndian.PutUint32(result[40:44], uint32(dataSize))
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(result[44+index*2:], uint16(sample))
	}
	return result
}
