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
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	knownASRFixtureDirectory                   = "tests/fixtures/localai/asr"
	knownASRFixtureFile                        = "localai-asr-known.wav"
	knownASRFixtureManifestFile                = "manifest.json"
	knownASRFixtureSchema                      = "localai.asr-semantic-fixture.v1"
	knownASRFixtureSHA256                      = "eea86018ce1730baaf7f5dd6ec88c1f727dd90203521a9115b489310a248ea05"
	knownASRFixtureMediaType                   = "audio/wav"
	knownASRFixtureMaxBytes              int64 = 1 << 20
	knownASRFixtureBytes                 int64 = 10340
	knownASRFixtureDataBytes             int64 = 10296
	knownASRFixtureDurationMillis              = 643.5
	knownASRFixtureSourceRepository            = "https://github.com/Jakobovski/free-spoken-digit-dataset"
	knownASRFixtureSourceRevision              = "2b2c7c40d93a401feccf428247dcd2317431fdd6"
	knownASRFixtureSourcePath                  = "recordings/0_jackson_0.wav"
	knownASRFixtureAttribution                 = "Free Spoken Digit Dataset contributors; speaker jackson"
	knownASRFixtureLicense                     = "CC BY-SA 4.0"
	knownASRFixtureLicenseURL                  = "https://creativecommons.org/licenses/by-sa/4.0/"
	knownASRFixtureLanguage                    = "en"
	knownASRFixtureRawTranscript               = "Zero."
	knownASRFixtureTranscript                  = "zero"
	knownASRFixtureNormalization               = "lowercase-trim-space-and-terminal-punctuation"
	knownASRFixtureSegmentTimeUnit             = "milliseconds"
	knownASRFixtureSegmentID                   = 0
	knownASRFixtureSegmentStartMillis          = 0.0
	knownASRFixtureSegmentEndMillis            = 500.0
	knownASRFixtureSegmentText                 = " Zero."
	knownASRFixtureSegmentNormalizedText       = "zero"
)

type knownASRFixtureManifest struct {
	Schema     string                     `json:"schema"`
	File       string                     `json:"file"`
	Bytes      int64                      `json:"bytes"`
	SHA256     string                     `json:"sha256"`
	MediaType  string                     `json:"mediaType"`
	WAV        knownASRWAVMetadata        `json:"wav"`
	Transcript knownASRTranscriptMetadata `json:"transcript"`
	Segments   knownASRSegmentsMetadata   `json:"segments"`
	Source     knownASRSourceMetadata     `json:"source"`
}

type knownASRWAVMetadata struct {
	AudioFormat   uint16  `json:"audioFormat"`
	Channels      uint16  `json:"channels"`
	SampleRateHz  uint32  `json:"sampleRateHz"`
	BitsPerSample uint16  `json:"bitsPerSample"`
	DataBytes     int64   `json:"dataBytes"`
	DurationMS    float64 `json:"durationMillis"`
}

type knownASRTranscriptMetadata struct {
	Language      string `json:"language"`
	Raw           string `json:"raw"`
	Normalized    string `json:"normalized"`
	Normalization string `json:"normalization"`
}

type knownASRSegmentsMetadata struct {
	TimeUnit    string                     `json:"timeUnit"`
	Items       []knownASRSegment          `json:"items"`
	Constraints knownASRSegmentConstraints `json:"constraints"`
}

type knownASRSegment struct {
	ID             int     `json:"id"`
	Start          float64 `json:"start"`
	End            float64 `json:"end"`
	Text           string  `json:"text"`
	NormalizedText string  `json:"normalizedText"`
}

type knownASRSegmentConstraints struct {
	Finite       bool    `json:"finite"`
	Monotonic    bool    `json:"monotonic"`
	MinimumStart float64 `json:"minimumStart"`
	MaximumEnd   float64 `json:"maximumEnd"`
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
	DurationMS   float64
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

	fixturePath, manifestPath := knownASRFixturePaths(t)

	details, err := validateKnownASRFixture(fixturePath, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"LOCALAI-ASR-FIXTURE-EVIDENCE schema=%s bytes=%d sha256=%s mediaType=%s format=%d channels=%d sampleRateHz=%d bits=%d dataBytes=%d durationMillis=%g rawTranscript=%q normalizedTranscript=%q segmentStart=%g segmentEnd=%g sourceRevision=%s license=%s",
		knownASRFixtureSchema, knownASRFixtureBytes, knownASRFixtureSHA256,
		knownASRFixtureMediaType, details.AudioFormat, details.Channels,
		details.SampleRateHz, details.Bits, details.DataBytes, details.DurationMS,
		knownASRFixtureRawTranscript, knownASRFixtureTranscript,
		knownASRFixtureSegmentStartMillis, knownASRFixtureSegmentEndMillis,
		knownASRFixtureSourceRevision, knownASRFixtureLicense,
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

		_, err := validateKnownASRFixture(mutatedPath, manifestPath)
		assertKnownASRMismatch(t, err, "fixture.sha256", knownASRFixtureSHA256, knownASRSHA256Hex(body))
	})

	t.Run("mutated-transcript-semantics-are-rejected", func(t *testing.T) {
		t.Parallel()
		mutatedPath := writeKnownASRManifestMutation(t, manifestPath, func(manifest *knownASRFixtureManifest) {
			manifest.Transcript.Normalized = "one"
		})
		_, err := validateKnownASRFixture(fixturePath, mutatedPath)
		assertKnownASRMismatch(t, err, "metadata.transcript.normalized", `"zero"`, `"one"`)
	})

	t.Run("mutated-segment-semantics-are-rejected", func(t *testing.T) {
		t.Parallel()
		mutatedPath := writeKnownASRManifestMutation(t, manifestPath, func(manifest *knownASRFixtureManifest) {
			manifest.Segments.Items[0].End = knownASRFixtureDurationMillis + 1
		})
		_, err := validateKnownASRFixture(fixturePath, mutatedPath)
		assertKnownASRMismatch(t, err, "segments.items[0].end", `"500"`, `"644.5"`)
	})

	t.Run("mutated-provenance-is-rejected", func(t *testing.T) {
		t.Parallel()
		mutatedPath := writeKnownASRManifestMutation(t, manifestPath, func(manifest *knownASRFixtureManifest) {
			manifest.Source.License = "proprietary"
		})
		_, err := validateKnownASRFixture(fixturePath, mutatedPath)
		assertKnownASRMismatch(t, err, "metadata.source.license", `"CC BY-SA 4.0"`, `"proprietary"`)
	})
}

func knownASRFixturePaths(t testing.TB) (string, string) {
	t.Helper()
	directory := testutil.MustRepoPath(t, knownASRFixtureDirectory)
	return filepath.Join(directory, knownASRFixtureFile), filepath.Join(directory, knownASRFixtureManifestFile)
}

func writeKnownASRManifestMutation(t *testing.T, manifestPath string, mutate func(*knownASRFixtureManifest)) string {
	t.Helper()
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read ASR fixture manifest: %v", err)
	}
	var manifest knownASRFixtureManifest
	if err := decodeKnownASRJSON(body, &manifest); err != nil {
		t.Fatalf("decode ASR fixture manifest: %v", err)
	}
	mutate(&manifest)
	mutatedBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal mutated ASR fixture manifest: %v", err)
	}
	mutatedPath := filepath.Join(t.TempDir(), knownASRFixtureManifestFile)
	if err := os.WriteFile(mutatedPath, append(mutatedBody, '\n'), 0o600); err != nil {
		t.Fatalf("write mutated ASR fixture manifest: %v", err)
	}
	return mutatedPath
}

func validateKnownASRFixture(fixturePath, manifestPath string) (knownASRWaveDetails, error) {
	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("read ASR fixture manifest: %w", err)
	}
	var manifest knownASRFixtureManifest
	if err := decodeKnownASRJSON(manifestBody, &manifest); err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("decode ASR fixture manifest: %w", err)
	}
	if mismatch := validateKnownASRManifest(manifest); mismatch != nil {
		return knownASRWaveDetails{}, mismatch
	}

	info, err := os.Stat(fixturePath)
	if err != nil {
		return knownASRWaveDetails{}, fmt.Errorf("stat ASR fixture: %w", err)
	}
	if !info.Mode().IsRegular() {
		return knownASRWaveDetails{}, fmt.Errorf("ASR fixture is not a regular file: %s", fixturePath)
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
	if mismatch := compareKnownASRWave(manifest.WAV, details); mismatch != nil {
		return knownASRWaveDetails{}, mismatch
	}
	return details, nil
}

func validateKnownASRManifest(manifest knownASRFixtureManifest) *knownASRFixtureMismatch {
	checks := []struct {
		field, expected, observed string
	}{
		{"metadata.schema", knownASRFixtureSchema, manifest.Schema},
		{"metadata.file", knownASRFixtureFile, manifest.File},
		{"metadata.bytes", fmt.Sprint(knownASRFixtureBytes), fmt.Sprint(manifest.Bytes)},
		{"metadata.sha256", knownASRFixtureSHA256, manifest.SHA256},
		{"metadata.mediaType", knownASRFixtureMediaType, manifest.MediaType},
		{"metadata.transcript.language", knownASRFixtureLanguage, manifest.Transcript.Language},
		{"metadata.transcript.raw", knownASRFixtureRawTranscript, manifest.Transcript.Raw},
		{"metadata.transcript.normalized", knownASRFixtureTranscript, manifest.Transcript.Normalized},
		{"metadata.transcript.normalization", knownASRFixtureNormalization, manifest.Transcript.Normalization},
		{"metadata.segments.timeUnit", knownASRFixtureSegmentTimeUnit, manifest.Segments.TimeUnit},
		{"metadata.source.repository", knownASRFixtureSourceRepository, manifest.Source.Repository},
		{"metadata.source.revision", knownASRFixtureSourceRevision, manifest.Source.Revision},
		{"metadata.source.path", knownASRFixtureSourcePath, manifest.Source.Path},
		{"metadata.source.attribution", knownASRFixtureAttribution, manifest.Source.Attribution},
		{"metadata.source.license", knownASRFixtureLicense, manifest.Source.License},
		{"metadata.source.licenseUrl", knownASRFixtureLicenseURL, manifest.Source.LicenseURL},
	}
	for _, check := range checks {
		if check.expected != check.observed {
			return &knownASRFixtureMismatch{Field: check.field, Expected: quoteKnownASRValue(check.expected), Observed: quoteKnownASRValue(check.observed)}
		}
	}
	if manifest.Bytes > knownASRFixtureMaxBytes {
		return &knownASRFixtureMismatch{Field: "metadata.bytes", Expected: fmt.Sprint(knownASRFixtureMaxBytes) + " or less", Observed: fmt.Sprint(manifest.Bytes)}
	}
	if mismatch := compareKnownASRFloat("metadata.wav.durationMillis", knownASRFixtureDurationMillis, manifest.WAV.DurationMS); mismatch != nil {
		return mismatch
	}
	if mismatch := validateKnownASRSegments(manifest.Segments); mismatch != nil {
		return mismatch
	}
	return nil
}

func validateKnownASRSegments(segments knownASRSegmentsMetadata) *knownASRFixtureMismatch {
	checks := []struct {
		field, expected, observed string
	}{
		{"metadata.segments.constraints.finite", "true", fmt.Sprint(segments.Constraints.Finite)},
		{"metadata.segments.constraints.monotonic", "true", fmt.Sprint(segments.Constraints.Monotonic)},
		{"metadata.segments.constraints.minimumStart", fmt.Sprint(knownASRFixtureSegmentStartMillis), fmt.Sprint(segments.Constraints.MinimumStart)},
		{"metadata.segments.constraints.maximumEnd", fmt.Sprint(knownASRFixtureDurationMillis), fmt.Sprint(segments.Constraints.MaximumEnd)},
	}
	for _, check := range checks {
		if check.expected != check.observed {
			return &knownASRFixtureMismatch{Field: check.field, Expected: check.expected, Observed: check.observed}
		}
	}
	if len(segments.Items) != 1 {
		return &knownASRFixtureMismatch{Field: "metadata.segments.items.length", Expected: "1", Observed: fmt.Sprint(len(segments.Items))}
	}
	segment := segments.Items[0]
	checks = []struct {
		field, expected, observed string
	}{
		{"segments.items[0].id", fmt.Sprint(knownASRFixtureSegmentID), fmt.Sprint(segment.ID)},
		{"segments.items[0].start", fmt.Sprint(knownASRFixtureSegmentStartMillis), fmt.Sprint(segment.Start)},
		{"segments.items[0].end", fmt.Sprint(knownASRFixtureSegmentEndMillis), fmt.Sprint(segment.End)},
		{"segments.items[0].text", knownASRFixtureSegmentText, segment.Text},
		{"segments.items[0].normalizedText", knownASRFixtureSegmentNormalizedText, segment.NormalizedText},
	}
	for _, check := range checks {
		if check.expected != check.observed {
			return &knownASRFixtureMismatch{Field: check.field, Expected: quoteKnownASRValue(check.expected), Observed: quoteKnownASRValue(check.observed)}
		}
	}
	if !finiteMonotonicKnownASRSegments(segments.Items, segments.Constraints) {
		return &knownASRFixtureMismatch{Field: "metadata.segments.items", Expected: "finite monotonic bounds", Observed: "segment bounds violate constraints"}
	}
	return nil
}

func finiteMonotonicKnownASRSegments(items []knownASRSegment, constraints knownASRSegmentConstraints) bool {
	previousID := -1
	previousStart, previousEnd := constraints.MinimumStart, constraints.MinimumStart
	for _, item := range items {
		if item.ID <= previousID || !finiteKnownASRFloat(item.Start) || !finiteKnownASRFloat(item.End) ||
			item.Start < constraints.MinimumStart || item.End > constraints.MaximumEnd || item.End <= item.Start ||
			item.Start < previousStart || item.End < previousEnd || item.NormalizedText == "" {
			return false
		}
		previousID, previousStart, previousEnd = item.ID, item.Start, item.End
	}
	return true
}

func finiteKnownASRFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func compareKnownASRFloat(field string, expected, observed float64) *knownASRFixtureMismatch {
	if expected == observed {
		return nil
	}
	return &knownASRFixtureMismatch{Field: field, Expected: fmt.Sprint(expected), Observed: fmt.Sprint(observed)}
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
			if uint64(blockAlign) != uint64(details.Channels)*uint64(details.Bits)/8 {
				return knownASRWaveDetails{}, errors.New("fmt chunk block alignment does not match PCM fields")
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
	details.DurationMS = float64(frames) * 1000 / float64(details.SampleRateHz)
	if !finiteKnownASRFloat(details.DurationMS) {
		return knownASRWaveDetails{}, errors.New("WAV duration is not finite")
	}
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
