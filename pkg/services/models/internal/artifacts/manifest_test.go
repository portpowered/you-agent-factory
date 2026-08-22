package artifacts_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
)

const fixtureProtocolRevision = "ad62c6df07ae1169eb14411a565a689cd996b19c"

func TestDecodeSelectsDetachedDescriptorAndVerifiesFixtureBytes(t *testing.T) {
	t.Parallel()

	manifest := decodeFixtureManifest(t)
	descriptor := selectFixtureDescriptor(t, manifest)
	assertFixtureDescriptor(t, descriptor)
	verifyFixtureDescriptorBytes(t, descriptor)
	assertDescriptorIsDetached(t, manifest, descriptor)
}

func decodeFixtureManifest(t *testing.T) artifacts.Manifest {
	t.Helper()
	manifest, err := artifacts.DecodeManifest(fixtureManifest(t))
	if err != nil {
		t.Fatalf("DecodeManifest: %v", err)
	}
	if manifest.ArtifactCount() != 1 {
		t.Fatalf("ArtifactCount = %d, want one fixture entry", manifest.ArtifactCount())
	}
	return manifest
}

func selectFixtureDescriptor(t *testing.T, manifest artifacts.Manifest) artifacts.ArtifactDescriptor {
	t.Helper()
	descriptor, err := manifest.Select(fixtureSelectionRequest())
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	return descriptor
}

func fixtureSelectionRequest() artifacts.SelectionRequest {
	return artifacts.SelectionRequest{
		Backend:          "localai-vibevoice",
		OperatingSystem:  "linux",
		Architecture:     "amd64",
		ProtocolRevision: fixtureProtocolRevision,
		Accelerator:      "cpu",
	}
}

func assertFixtureDescriptor(t *testing.T, descriptor artifacts.ArtifactDescriptor) {
	t.Helper()
	if descriptor.ID != "localai-vibevoice/linux-amd64" || descriptor.Backend.ID != "localai-vibevoice" {
		t.Fatalf("descriptor identity = %#v, want detached backend identity", descriptor)
	}
	if descriptor.Source.Commit != "b224c96db6f4b87306a33a808650bfce63b12588" || descriptor.Protocol.Revision != fixtureProtocolRevision {
		t.Fatalf("descriptor provenance = %#v, want pinned source and protocol", descriptor)
	}
	if descriptor.Publication.PinFingerprint != strings.Repeat("a", 64) {
		t.Fatalf("descriptor publication = %#v, want immutable fixture identity", descriptor.Publication)
	}
	if descriptor.Target.OperatingSystem != "linux" || descriptor.Target.Architecture != "amd64" || descriptor.Artifact.SizeBytes != 22 {
		t.Fatalf("descriptor compatibility = %#v, want linux amd64 fixture facts", descriptor)
	}
	if descriptor.Artifact.SHA256 != "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172" {
		t.Fatalf("descriptor sha256 = %q, want fixture checksum", descriptor.Artifact.SHA256)
	}
}

func verifyFixtureDescriptorBytes(t *testing.T, descriptor artifacts.ArtifactDescriptor) {
	t.Helper()
	if err := descriptor.VerifyBytes([]byte("pinned-backend-fixture")); err != nil {
		t.Fatalf("VerifyBytes: %v", err)
	}
}

func assertDescriptorIsDetached(t *testing.T, manifest artifacts.Manifest, descriptor artifacts.ArtifactDescriptor) {
	t.Helper()
	descriptor.Target.Accelerators[0] = "mutated"
	next, err := manifest.Select(fixtureSelectionRequest())
	if err != nil {
		t.Fatalf("Select after descriptor mutation: %v", err)
	}
	if next.Target.Accelerators[0] != "cpu" {
		t.Fatalf("manifest state changed through descriptor: %#v", next.Target.Accelerators)
	}
}

func TestVerifyBytesRejectsTamperedFixtureBytes(t *testing.T) {
	t.Parallel()

	manifest, err := artifacts.Decode(fixtureManifest(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	descriptor, err := manifest.Select(artifacts.SelectionRequest{
		Backend: "localai-vibevoice", OperatingSystem: "linux", Architecture: "amd64",
		ProtocolRevision: fixtureProtocolRevision, Accelerator: "cpu",
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	err = artifacts.VerifyBytes(descriptor, []byte("tampered-backend-bytes"))
	if !errors.Is(err, artifacts.ErrIntegrityMismatch) {
		t.Fatalf("VerifyBytes error = %v, want ErrIntegrityMismatch", err)
	}
	var failure *artifacts.Failure
	if !errors.As(err, &failure) || failure.Kind != artifacts.FailureIntegrityMismatch {
		t.Fatalf("VerifyBytes error = %#v, want typed integrity failure", err)
	}
}

func TestManifestAcceptsEveryRegisteredArtifactBackend(t *testing.T) {
	t.Parallel()

	backends := []struct {
		id         string
		repository string
		path       string
	}{
		{id: "localai-llamacpp", repository: "https://github.com/ggerganov/llama.cpp", path: "backend/cpp/llama-cpp"},
		{id: "localai-whisper", repository: "https://github.com/ggml-org/whisper.cpp", path: "backend/go/whisper"},
		{id: "localai-vibevoice", repository: "https://github.com/mudler/vibevoice.cpp", path: "backend/go/vibevoice-cpp"},
	}
	for _, backend := range backends {
		backend := backend
		t.Run(backend.id, func(t *testing.T) {
			data := mutatedFixture(t, func(document map[string]any) {
				publication := document["publication"].(map[string]any)
				publicationSource := publication["source"].(map[string]any)
				publicationSource["repository"] = backend.repository

				entry := document["artifacts"].([]any)[0].(map[string]any)
				manifestBackend := entry["backend"].(map[string]any)
				manifestBackend["id"] = backend.id
				manifestBackend["source"].(map[string]any)["repository"] = backend.repository
				entrySource := entry["source"].(map[string]any)
				entrySource["repository"] = backend.repository
				entrySource["path"] = backend.path
			})

			manifest, err := artifacts.Decode(data)
			if err != nil {
				t.Fatalf("Decode(%q): %v", backend.id, err)
			}
			descriptor, err := manifest.Select(artifacts.SelectionRequest{
				Backend: backend.id, OperatingSystem: "linux", Architecture: "amd64",
				ProtocolRevision: fixtureProtocolRevision, Accelerator: "cpu",
			})
			if err != nil {
				t.Fatalf("Select(%q): %v", backend.id, err)
			}
			if descriptor.Backend.ID != backend.id || descriptor.Source.Repository != backend.repository || descriptor.Source.Path != backend.path {
				t.Fatalf("descriptor = %#v, want registered artifact facts", descriptor)
			}
		})
	}
}

func TestDecodeRejectsMalformedManifestFacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   error
		kind   artifacts.FailureKind
	}{
		{name: "invalid json", mutate: nil, want: artifacts.ErrManifestMalformed, kind: artifacts.FailureMalformedManifest},
		{name: "invalid digest", mutate: mutateArtifact(func(archive map[string]any) { archive["sha256"] = "ABC" }), want: artifacts.ErrInvalidDigest, kind: artifacts.FailureInvalidDigest},
		{name: "invalid size", mutate: mutateArtifact(func(archive map[string]any) { archive["sizeBytes"] = 0 }), want: artifacts.ErrInvalidSize, kind: artifacts.FailureInvalidSize},
		{name: "unsafe location", mutate: mutateArtifact(func(archive map[string]any) { archive["location"] = "file:///tmp/backend.tar.gz" }), want: artifacts.ErrUnsafeLocation, kind: artifacts.FailureUnsafeLocation},
		{name: "unknown backend", mutate: mutateBackend(func(backend map[string]any) { backend["id"] = "localai-unknown" }), want: artifacts.ErrUnknownBackend, kind: artifacts.FailureUnknownBackend},
		{name: "unsupported platform", mutate: mutateTarget(func(target map[string]any) { target["id"] = "freebsd-amd64"; target["operatingSystem"] = "freebsd" }), want: artifacts.ErrUnsupportedPlatform, kind: artifacts.FailureUnsupportedPlatform},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			var data []byte
			if testCase.name == "invalid json" {
				data = []byte("{not-json")
			} else {
				data = mutatedFixture(t, testCase.mutate)
			}
			_, err := artifacts.Decode(data)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Decode error = %v, want %v", err, testCase.want)
			}
			var failure *artifacts.Failure
			if !errors.As(err, &failure) || failure.Kind != testCase.kind {
				t.Fatalf("Decode error = %#v, want typed %s failure", err, testCase.kind)
			}
		})
	}
}

func TestSelectReturnsTypedCapabilityFailures(t *testing.T) {
	t.Parallel()

	manifest, err := artifacts.Decode(fixtureManifest(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	base := artifacts.SelectionRequest{
		Backend: "localai-vibevoice", OperatingSystem: "linux", Architecture: "amd64",
		ProtocolRevision: fixtureProtocolRevision, Accelerator: "cpu",
	}
	tests := []struct {
		name    string
		request artifacts.SelectionRequest
		want    error
		kind    artifacts.FailureKind
	}{
		{name: "unknown backend", request: withRequest(base, func(request *artifacts.SelectionRequest) { request.Backend = "localai-unknown" }), want: artifacts.ErrUnknownBackend, kind: artifacts.FailureUnknownBackend},
		{name: "unsupported platform", request: withRequest(base, func(request *artifacts.SelectionRequest) { request.OperatingSystem = "freebsd" }), want: artifacts.ErrUnsupportedPlatform, kind: artifacts.FailureUnsupportedPlatform},
		{name: "incompatible accelerator", request: withRequest(base, func(request *artifacts.SelectionRequest) { request.Accelerator = "metal" }), want: artifacts.ErrIncompatibleAccelerator, kind: artifacts.FailureIncompatibleAccelerator},
		{name: "incompatible protocol", request: withRequest(base, func(request *artifacts.SelectionRequest) { request.ProtocolRevision = strings.Repeat("0", 40) }), want: artifacts.ErrIncompatibleProtocol, kind: artifacts.FailureIncompatibleProtocol},
		{name: "missing match", request: withRequest(base, func(request *artifacts.SelectionRequest) { request.Backend = "localai-whisper" }), want: artifacts.ErrMissingMatch, kind: artifacts.FailureMissingMatch},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			_, err := manifest.Select(testCase.request)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Select error = %v, want %v", err, testCase.want)
			}
			var failure *artifacts.Failure
			if !errors.As(err, &failure) || failure.Kind != testCase.kind {
				t.Fatalf("Select error = %#v, want typed %s failure", err, testCase.kind)
			}
		})
	}
}

func TestSelectRejectsDuplicateCompatibleEntries(t *testing.T) {
	t.Parallel()

	data := mutatedFixture(t, func(document map[string]any) {
		entries := document["artifacts"].([]any)
		original := entries[0].(map[string]any)
		duplicate := cloneMap(original)
		duplicate["id"] = "localai-vibevoice/linux-amd64-duplicate"
		archive := cloneMap(original["artifact"].(map[string]any))
		archive["name"] = "localai-vibevoice-duplicate.tar.gz"
		archive["location"] = "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-fixture/localai-vibevoice-duplicate.tar.gz"
		duplicate["artifact"] = archive
		document["artifacts"] = append(entries, duplicate)
	})
	manifest, err := artifacts.Decode(data)
	if err != nil {
		t.Fatalf("Decode duplicate fixture: %v", err)
	}
	_, err = manifest.Select(artifacts.SelectionRequest{
		Backend: "localai-vibevoice", OperatingSystem: "linux", Architecture: "amd64",
		ProtocolRevision: fixtureProtocolRevision, Accelerator: "cpu",
	})
	if !errors.Is(err, artifacts.ErrDuplicateMatch) {
		t.Fatalf("Select duplicate error = %v, want ErrDuplicateMatch", err)
	}
	var failure *artifacts.Failure
	if !errors.As(err, &failure) || failure.Kind != artifacts.FailureDuplicateMatch {
		t.Fatalf("Select duplicate error = %#v, want typed duplicate failure", err)
	}
}

func fixtureManifest(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "pinned-manifest.json"))
	if err != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}
	return data
}

func mutatedFixture(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(fixtureManifest(t), &document); err != nil {
		t.Fatalf("decode fixture for mutation: %v", err)
	}
	if mutate != nil {
		mutate(document)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode mutated fixture: %v", err)
	}
	return data
}

func mutateArtifact(mutate func(map[string]any)) func(map[string]any) {
	return func(document map[string]any) {
		entries := document["artifacts"].([]any)
		mutate(entries[0].(map[string]any)["artifact"].(map[string]any))
	}
}

func mutateBackend(mutate func(map[string]any)) func(map[string]any) {
	return func(document map[string]any) {
		entries := document["artifacts"].([]any)
		mutate(entries[0].(map[string]any)["backend"].(map[string]any))
	}
}

func mutateTarget(mutate func(map[string]any)) func(map[string]any) {
	return func(document map[string]any) {
		entries := document["artifacts"].([]any)
		mutate(entries[0].(map[string]any)["target"].(map[string]any))
	}
}

func cloneMap(original map[string]any) map[string]any {
	clone := make(map[string]any, len(original))
	for key, value := range original {
		clone[key] = value
	}
	return clone
}

func withRequest(base artifacts.SelectionRequest, mutate func(*artifacts.SelectionRequest)) artifacts.SelectionRequest {
	request := base
	mutate(&request)
	return request
}
