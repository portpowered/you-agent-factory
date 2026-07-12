package omnivoice

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

const referenceSampleRate = 24_000

// DecodeReferenceWAV decodes a PCM or IEEE-float WAV, folds it to mono, and
// resamples it to the 24 kHz waveform the OmniVoice ABI requires.
func DecodeReferenceWAV(path string) ([]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read OMNIVOICE reference WAV %q: %w", path, err)
	}
	format, payload, err := parseWAV(data)
	if err != nil {
		return nil, fmt.Errorf("decode OMNIVOICE reference WAV %q: %w", path, err)
	}
	samples, err := decodeSamples(format, payload)
	if err != nil {
		return nil, fmt.Errorf("decode OMNIVOICE reference WAV %q: %w", path, err)
	}
	return resample(samples, format.sampleRate), nil
}

type wavFormat struct {
	audioFormat   uint16
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
}

func parseWAV(data []byte) (wavFormat, []byte, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wavFormat{}, nil, fmt.Errorf("expected RIFF/WAVE input")
	}
	var format wavFormat
	var payload []byte
	for offset := 12; offset+8 <= len(data); {
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		start, end := offset+8, offset+8+size
		if end > len(data) {
			return wavFormat{}, nil, fmt.Errorf("truncated %q WAV chunk", string(data[offset:offset+4]))
		}
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if size < 16 {
				return wavFormat{}, nil, fmt.Errorf("invalid fmt WAV chunk")
			}
			format = wavFormat{binary.LittleEndian.Uint16(data[start : start+2]), binary.LittleEndian.Uint16(data[start+2 : start+4]), binary.LittleEndian.Uint32(data[start+4 : start+8]), binary.LittleEndian.Uint16(data[start+14 : start+16])}
		case "data":
			payload = data[start:end]
		}
		offset = end + size%2
	}
	if format.channels == 0 || format.sampleRate == 0 || len(payload) == 0 {
		return wavFormat{}, nil, fmt.Errorf("WAV must contain non-empty fmt and data chunks")
	}
	return format, payload, nil
}

func decodeSamples(format wavFormat, payload []byte) ([]float32, error) {
	bytesPerSample := int(format.bitsPerSample / 8)
	if bytesPerSample == 0 || len(payload)%(bytesPerSample*int(format.channels)) != 0 {
		return nil, fmt.Errorf("invalid WAV sample layout")
	}
	frames := len(payload) / (bytesPerSample * int(format.channels))
	result := make([]float32, frames)
	for frame := range frames {
		var mixed float64
		for channel := range int(format.channels) {
			offset := (frame*int(format.channels) + channel) * bytesPerSample
			sample, err := decodeSample(format.audioFormat, format.bitsPerSample, payload[offset:offset+bytesPerSample])
			if err != nil {
				return nil, err
			}
			mixed += sample
		}
		result[frame] = float32(mixed / float64(format.channels))
	}
	return result, nil
}

func decodeSample(audioFormat, bits uint16, data []byte) (float64, error) {
	switch audioFormat {
	case 1:
		switch bits {
		case 16:
			return float64(int16(binary.LittleEndian.Uint16(data))) / 32768, nil
		case 24:
			value := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
			if value&0x800000 != 0 {
				value |= ^0xffffff
			}
			return float64(value) / 8388608, nil
		case 32:
			return float64(int32(binary.LittleEndian.Uint32(data))) / 2147483648, nil
		}
	case 3:
		switch bits {
		case 32:
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(data))), nil
		case 64:
			return math.Float64frombits(binary.LittleEndian.Uint64(data)), nil
		}
	}
	return 0, fmt.Errorf("unsupported WAV encoding format=%d bits=%d", audioFormat, bits)
}

func resample(input []float32, sourceRate uint32) []float32 {
	if sourceRate == referenceSampleRate || len(input) == 0 {
		return input
	}
	length := int(math.Round(float64(len(input)) * referenceSampleRate / float64(sourceRate)))
	if length < 1 {
		length = 1
	}
	output := make([]float32, length)
	for index := range output {
		position := float64(index) * float64(sourceRate) / referenceSampleRate
		left := int(position)
		if left >= len(input)-1 {
			output[index] = input[len(input)-1]
			continue
		}
		fraction := float32(position - float64(left))
		output[index] = input[left]*(1-fraction) + input[left+1]*fraction
	}
	return output
}

// WriteWAV writes 16-bit mono PCM WAV output produced by OmniVoice.
func WriteWAV(path string, samples []float32, sampleRate int) error {
	if len(samples) == 0 || sampleRate <= 0 {
		return fmt.Errorf("embedded OMNIVOICE returned invalid audio buffer")
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create embedded OMNIVOICE output %q: %w", path, err)
	}
	defer file.Close()
	dataSize := len(samples) * 2
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))
	if _, err := file.Write(header); err != nil {
		return fmt.Errorf("write embedded OMNIVOICE WAV header: %w", err)
	}
	pcm := make([]byte, dataSize)
	for index, sample := range samples {
		clamped := math.Max(-1, math.Min(1, float64(sample)))
		binary.LittleEndian.PutUint16(pcm[index*2:], uint16(int16(math.Round(clamped*32767))))
	}
	if _, err := file.Write(pcm); err != nil {
		return fmt.Errorf("write embedded OMNIVOICE WAV samples: %w", err)
	}
	return nil
}
