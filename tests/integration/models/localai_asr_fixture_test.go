package models_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	knownASRFixtureFile                   = "localai-asr-known.wav"
	knownASRFixtureMetadataFile           = "localai-asr-known.json"
	knownASRFixtureSchema                 = "localai.asr-known-fixture.v1"
	knownASRFixtureSHA256                 = "eea86018ce1730baaf7f5dd6ec88c1f727dd90203521a9115b489310a248ea05"
	knownASRFixtureMediaType              = "audio/wav"
	knownASRFixtureMaxBytes         int64 = 1 << 20
	knownASRFixtureBytes            int64 = 10340
	knownASRFixtureDataBytes        int64 = 10296
	knownASRFixtureDuration         int64 = 644
	knownASRFixtureSourceRepository       = "https://github.com/Jakobovski/free-spoken-digit-dataset"
	knownASRFixtureSourceRevision         = "2b2c7c40d93a401feccf428247dcd2317431fdd6"
	knownASRFixtureSourcePath             = "recordings/0_jackson_0.wav"
	knownASRFixtureAttribution            = "Free Spoken Digit Dataset contributors; speaker jackson"
	knownASRFixtureLicense                = "CC BY-SA 4.0"
	knownASRFixtureLicenseURL             = "https://creativecommons.org/licenses/by-sa/4.0/"
	knownASRFixtureLanguage               = "en"
	knownASRFixtureTranscript             = "zero"
	knownASRFixtureNormalization          = "lowercase-trim-space-and-terminal-punctuation"
)

type knownASRFixtureMetadata struct {
	Schema     string                     `json:"schema"`
	File       string                     `json:"file"`
	Bytes      int64                      `json:"bytes"`
	SHA256     string                     `json:"sha256"`
	MediaType  string                     `json:"mediaType"`
	WAV        knownASRWAVMetadata        `json:"wav"`
	Transcript knownASRTranscriptMetadata `json:"transcript"`
	Source     knownASRSourceMetadata     `json:"source"`
}

type knownASRWAVMetadata struct {
	AudioFormat   uint16 `json:"audioFormat"`
	Channels      uint16 `json:"channels"`
	SampleRateHz  uint32 `json:"sampleRateHz"`
	BitsPerSample uint16 `json:"bitsPerSample"`
	DataBytes     int64  `json:"dataBytes"`
	DurationMS    int64  `json:"durationMillis"`
}

type knownASRTranscriptMetadata struct {
	Language      string `json:"language"`
	Text          string `json:"text"`
	Normalization string `json:"normalization"`
}

type knownASRSourceMetadata struct {
	Repository  string `json:"repository"`
	Revision    string `json:"revision"`
	Path        string `json:"path"`
	Attribution string `json:"attribution"`
	License     string `json:"license"`
	LicenseURL  string `json:"licenseUrl"`
}

type knownASRWaveDetails struct {
	AudioFormat  uint16
	Channels     uint16
	SampleRateHz uint32
	Bits         uint16
	DataBytes    int64
	DurationMS   int64
}

type knownASRFixtureMismatch struct {
	Field    string
	Expected string
	Observed string
}

func (m *knownASRFixtureMismatch) Error() string {
	return fmt.Sprintf("%s: expected %s, observed %s", m.Field, m.Expected, m.Observed)
}

// TestLocalAIRealHarnessKnownASRFixture proves the repository-owned input
// contract without starting the product, a model, a backend, or a network
// client. Later ASR selectors can consume this exact path independently of
// the TTS journeys.
func TestLocalAIRealHarnessKnownASRFixture(t *testing.T) {
	t.Parallel()

	fixturePath := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join(
		"tests", "integration", "models", "testdata", knownASRFixtureFile,
	)))
	metadataPath := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join(
		"tests", "integration", "models", "testdata", knownASRFixtureMetadataFile,
	)))

	details, err := validateKnownASRFixture(fixturePath, metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"LOCALAI-ASR-FIXTURE-EVIDENCE schema=%s bytes=%d sha256=%s format=%d channels=%d sampleRateHz=%d bits=%d dataBytes=%d durationMillis=%d transcript=%q sourceRevision=%s",
		knownASRFixtureSchema, knownASRFixtureBytes, knownASRFixtureSHA256,
		details.AudioFormat, details.Channels, details.SampleRateHz, details.Bits,
		details.DataBytes, details.DurationMS, knownASRFixtureTranscript,
		knownASRFixtureSourceRevision,
	)

	t.Run("mutated-fixture-is-rejected", func(t *testing.T) {
		t.Parallel()
		body, readErr := os.ReadFile(fixturePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		body[len(body)-1] ^= 0xff
		mutatedPath := filepath.Join(t.TempDir(), knownASRFixtureFile)
		if writeErr := os.WriteFile(mutatedPath, body, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}

		_, err := validateKnownASRFixture(mutatedPath, metadataPath)
		assertKnownASRMismatch(t, err, "fixture.sha256", knownASRFixtureSHA256, knownASRSHA256Hex(body))
	})

	t.Run("mutated-metadata-is-rejected", func(t *testing.T) {
		t.Parallel()
		metadataBody, readErr := os.ReadFile(metadataPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var metadata knownASRFixtureMetadata
		if decodeErr := decodeKnownASRJSON(metadataBody, &metadata); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		metadata.Transcript.Text = "one"
		mutatedBody, marshalErr := json.MarshalIndent(metadata, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		mutatedPath := filepath.Join(t.TempDir(), knownASRFixtureMetadataFile)
		if writeErr := os.WriteFile(mutatedPath, append(mutatedBody, '\n'), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}

		_, err := validateKnownASRFixture(fixturePath, mutatedPath)
		assertKnownASRMismatch(t, err, "metadata.transcript.text", `"zero"`, `"one"`)
	})
}

func validateKnownASRFixture(fixturePath, metadataPath string) (knownASRWaveDetails, error) {
	metadataBody, err := os.ReadFile(metadataPath)
	if err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("read ASR fixture metadata: %w", err)
	}
	var metadata knownASRFixtureMetadata
	if err := decodeKnownASRJSON(metadataBody, &metadata); err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("decode ASR fixture metadata: %w", err)
	}
	if mismatch := validateKnownASRMetadata(metadata); mismatch != nil {
		return knownASRWaveDetails{}, mismatch
	}

	info, err := os.Stat(fixturePath)
	if err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("stat ASR fixture: %w", err)
	}
	if info.Size() > knownASRFixtureMaxBytes {
		return knownASRWaveDetails{}, &knownASRFixtureMismatch{
			Field: "fixture.bytes", Expected: fmt.Sprint(knownASRFixtureMaxBytes) + " or less", Observed: fmt.Sprint(info.Size()),
		}
	}
	body, err := os.ReadFile(fixturePath)
	if err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("read ASR fixture: %w", err)
	}
	if mismatch := compareKnownASRValue("fixture.bytes", knownASRFixtureBytes, int64(len(body))); mismatch != nil {
		return knownASRWaveDetails{}, mismatch
	}
	if mismatch := compareKnownASRValue("fixture.sha256", knownASRFixtureSHA256, knownASRSHA256Hex(body)); mismatch != nil {
		return knownASRWaveDetails{}, mismatch
	}

	details, err := parseKnownASRWave(body)
	if err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("parse ASR fixture WAV: %w", err)
	}
	if mismatch := compareKnownASRWave(metadata.WAV, details); mismatch != nil {
		return knownASRWaveDetails{}, mismatch
	}
	return details, nil
}

func validateKnownASRMetadata(metadata knownASRFixtureMetadata) *knownASRFixtureMismatch {
	checks := []struct {
		field, expected, observed string
	}{
		{"metadata.schema", knownASRFixtureSchema, metadata.Schema},
		{"metadata.file", knownASRFixtureFile, metadata.File},
		{"metadata.bytes", fmt.Sprint(knownASRFixtureBytes), fmt.Sprint(metadata.Bytes)},
		{"metadata.sha256", knownASRFixtureSHA256, metadata.SHA256},
		{"metadata.mediaType", knownASRFixtureMediaType, metadata.MediaType},
		{"metadata.wav.audioFormat", "1", fmt.Sprint(metadata.WAV.AudioFormat)},
		{"metadata.wav.channels", "1", fmt.Sprint(metadata.WAV.Channels)},
		{"metadata.wav.sampleRateHz", "8000", fmt.Sprint(metadata.WAV.SampleRateHz)},
		{"metadata.wav.bitsPerSample", "16", fmt.Sprint(metadata.WAV.BitsPerSample)},
		{"metadata.wav.dataBytes", fmt.Sprint(knownASRFixtureDataBytes), fmt.Sprint(metadata.WAV.DataBytes)},
		{"metadata.wav.durationMillis", fmt.Sprint(knownASRFixtureDuration), fmt.Sprint(metadata.WAV.DurationMS)},
		{"metadata.transcript.language", knownASRFixtureLanguage, metadata.Transcript.Language},
		{"metadata.transcript.text", knownASRFixtureTranscript, metadata.Transcript.Text},
		{"metadata.transcript.normalization", knownASRFixtureNormalization, metadata.Transcript.Normalization},
		{"metadata.source.repository", knownASRFixtureSourceRepository, metadata.Source.Repository},
		{"metadata.source.revision", knownASRFixtureSourceRevision, metadata.Source.Revision},
		{"metadata.source.path", knownASRFixtureSourcePath, metadata.Source.Path},
		{"metadata.source.attribution", knownASRFixtureAttribution, metadata.Source.Attribution},
		{"metadata.source.license", knownASRFixtureLicense, metadata.Source.License},
		{"metadata.source.licenseUrl", knownASRFixtureLicenseURL, metadata.Source.LicenseURL},
	}
	for _, check := range checks {
		if check.expected != check.observed {
			return &knownASRFixtureMismatch{Field: check.field, Expected: quoteKnownASRValue(check.expected), Observed: quoteKnownASRValue(check.observed)}
		}
	}
	if metadata.Bytes > knownASRFixtureMaxBytes {
		return &knownASRFixtureMismatch{Field: "metadata.bytes", Expected: fmt.Sprint(knownASRFixtureMaxBytes) + " or less", Observed: fmt.Sprint(metadata.Bytes)}
	}
	return nil
}

func compareKnownASRWave(expected knownASRWAVMetadata, observed knownASRWaveDetails) *knownASRFixtureMismatch {
	checks := []struct {
		field, expected, observed string
	}{
		{"wav.audioFormat", fmt.Sprint(expected.AudioFormat), fmt.Sprint(observed.AudioFormat)},
		{"wav.channels", fmt.Sprint(expected.Channels), fmt.Sprint(observed.Channels)},
		{"wav.sampleRateHz", fmt.Sprint(expected.SampleRateHz), fmt.Sprint(observed.SampleRateHz)},
		{"wav.bitsPerSample", fmt.Sprint(expected.BitsPerSample), fmt.Sprint(observed.Bits)},
		{"wav.dataBytes", fmt.Sprint(expected.DataBytes), fmt.Sprint(observed.DataBytes)},
		{"wav.durationMillis", fmt.Sprint(expected.DurationMS), fmt.Sprint(observed.DurationMS)},
	}
	for _, check := range checks {
		if check.expected != check.observed {
			return &knownASRFixtureMismatch{Field: check.field, Expected: check.expected, Observed: check.observed}
		}
	}
	return nil
}

func compareKnownASRValue(field string, expected, observed any) *knownASRFixtureMismatch {
	expectedText, observedText := fmt.Sprint(expected), fmt.Sprint(observed)
	if expectedText == observedText {
		return nil
	}
	return &knownASRFixtureMismatch{Field: field, Expected: expectedText, Observed: observedText}
}

func parseKnownASRWave(body []byte) (knownASRWaveDetails, error) {
	if len(body) < 12 || string(body[:4]) != "RIFF" || string(body[8:12]) != "WAVE" {
		return knownASRWaveDetails{}, errors.New("missing RIFF/WAVE header")
	}
	if uint64(binary.LittleEndian.Uint32(body[4:8]))+8 != uint64(len(body)) {
		return knownASRWaveDetails{}, errors.New("RIFF size does not match file size")
	}

	var details knownASRWaveDetails
	var haveFormat, haveData bool
	offset := 12
	for offset+8 <= len(body) {
		chunkID := string(body[offset : offset+4])
		chunkBytes := uint64(binary.LittleEndian.Uint32(body[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := uint64(chunkStart) + chunkBytes
		if chunkEnd > uint64(len(body)) {
			return knownASRWaveDetails{}, fmt.Errorf("%s chunk exceeds file", chunkID)
		}
		if chunkID == "fmt " {
			if chunkBytes < 16 {
				return knownASRWaveDetails{}, errors.New("fmt chunk is shorter than PCM header")
			}
			details.AudioFormat = binary.LittleEndian.Uint16(body[chunkStart : chunkStart+2])
			details.Channels = binary.LittleEndian.Uint16(body[chunkStart+2 : chunkStart+4])
			details.SampleRateHz = binary.LittleEndian.Uint32(body[chunkStart+4 : chunkStart+8])
			byteRate := binary.LittleEndian.Uint32(body[chunkStart+8 : chunkStart+12])
			blockAlign := binary.LittleEndian.Uint16(body[chunkStart+12 : chunkStart+14])
			details.Bits = binary.LittleEndian.Uint16(body[chunkStart+14 : chunkStart+16])
			if blockAlign == 0 || byteRate == 0 {
				return knownASRWaveDetails{}, errors.New("fmt chunk has empty byte rate or block alignment")
			}
			if uint64(byteRate) != uint64(details.SampleRateHz)*uint64(blockAlign) {
				return knownASRWaveDetails{}, errors.New("fmt chunk byte rate does not match PCM fields")
			}
			haveFormat = true
		}
		if chunkID == "data" {
			details.DataBytes = int64(chunkBytes)
			haveData = true
			if chunkEnd != uint64(len(body)) {
				return knownASRWaveDetails{}, errors.New("data chunk is not the complete file payload")
			}
		}
		offset = int(chunkEnd)
		if chunkBytes%2 != 0 {
			offset++
		}
	}
	if !haveFormat || !haveData {
		return knownASRWaveDetails{}, errors.New("WAV is missing fmt or data chunk")
	}
	if details.AudioFormat != 1 || details.Channels == 0 || details.SampleRateHz == 0 || details.Bits != 16 {
		return knownASRWaveDetails{}, errors.New("WAV is not PCM with a valid channel/rate/bit depth")
	}
	blockAlign := uint64(details.Channels) * uint64(details.Bits) / 8
	if blockAlign == 0 || uint64(details.DataBytes)%blockAlign != 0 {
		return knownASRWaveDetails{}, errors.New("WAV data is not aligned to PCM frames")
	}
	frames := uint64(details.DataBytes) / blockAlign
	details.DurationMS = int64((frames*1000 + uint64(details.SampleRateHz)/2) / uint64(details.SampleRateHz))
	return details, nil
}

func decodeKnownASRJSON(body []byte, destination any) error {
	if len(body) > 32<<10 {
		return errors.New("metadata exceeds bounded reader limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("metadata contains trailing JSON")
		}
		return err
	}
	return nil
}

func assertKnownASRMismatch(t *testing.T, err error, field, expected, observed string) {
	t.Helper()
	if err == nil {
		t.Fatalf("validation unexpectedly passed; want mismatch %s", field)
	}
	var mismatch *knownASRFixtureMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("validation error = %v, want bounded mismatch for %s", err, field)
	}
	if mismatch.Field != field || mismatch.Expected != expected || mismatch.Observed != observed {
		t.Fatalf("validation mismatch = %#v, want field=%q expected=%q observed=%q", mismatch, field, expected, observed)
	}
}

func quoteKnownASRValue(value string) string {
	return fmt.Sprintf("%q", value)
}

func knownASRSHA256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
