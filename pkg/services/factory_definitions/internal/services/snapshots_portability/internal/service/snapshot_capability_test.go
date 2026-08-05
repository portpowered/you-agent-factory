package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestSnapshotsCaptureRoundTripUsesStableIdentityAndCompleteLayout(t *testing.T) {
	t.Parallel()

	svc := newRoundTripService(t)
	ctx := context.Background()
	first, err := svc.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{
		FactoryDir: "/factories/alpha",
		Canonical:  canonicalSnapshotInput("hello"),
	})
	if err != nil {
		t.Fatalf("first CaptureFactorySnapshot: %v", err)
	}
	second, err := svc.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{
		FactoryDir: "/factories/alpha",
		Canonical:  equivalentCanonicalSnapshotInput(),
	})
	if err != nil {
		t.Fatalf("second CaptureFactorySnapshot: %v", err)
	}
	if first.Identity == "" || first.Identity != second.Identity {
		t.Fatalf("capture identities = %q, %q; want equal non-empty values", first.Identity, second.Identity)
	}

	changed, err := svc.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{
		FactoryDir: "/factories/alpha",
		Canonical:  canonicalSnapshotInput("changed"),
	})
	if err != nil {
		t.Fatalf("changed CaptureFactorySnapshot: %v", err)
	}
	if changed.Identity == first.Identity {
		t.Fatalf("changed artifact identity = %q, want a distinct identity", changed.Identity)
	}
	var captured map[string]any
	if err := first.Snapshot.Decode(&captured); err != nil {
		t.Fatalf("decode captured snapshot: %v", err)
	}
	if workers, ok := captured["workers"].([]any); !ok || len(workers) != 1 {
		t.Fatalf("captured workers = %#v, want one runtime worker", captured["workers"])
	}
	if workstations, ok := captured["workstations"].([]any); !ok || len(workstations) != 1 {
		t.Fatalf("captured workstations = %#v, want one runtime workstation", captured["workstations"])
	}

	payload, err := json.Marshal(first.Snapshot)
	if err != nil {
		t.Fatalf("marshal captured snapshot: %v", err)
	}
	prepared, err := svc.PrepareFactorySnapshotImport(ctx, factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload})
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if prepared.Identity != first.Identity || prepared.Snapshot == nil {
		t.Fatalf("prepared identity/snapshot = %#v; want detached captured snapshot", prepared)
	}
	if len(prepared.Portable.Assets) != 1 || prepared.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("prepared portable assets = %#v, want README artifact", prepared.Portable.Assets)
	}

	targetDir := t.TempDir()
	stalePath := filepath.Join(targetDir, "stale.txt")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed replaced target: %v", err)
	}
	materialized, err := svc.MaterializeFactorySnapshot(ctx, factorydefinitions.MaterializeFactorySnapshotRequest{
		TargetDir:        targetDir,
		Snapshot:         prepared.Snapshot,
		ExpectedIdentity: first.Identity,
	})
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot: %v", err)
	}
	if materialized.Identity != first.Identity || materialized.Portable.FactoryDir != targetDir {
		t.Fatalf("materialized result = %#v, want target and captured identity", materialized)
	}
	source, err := os.ReadFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read restored Factory source: %v", err)
	}
	if string(source) != string(payload) {
		t.Fatalf("restored Factory source differs from detached snapshot")
	}
	artifact, err := os.ReadFile(filepath.Join(targetDir, "docs", "README.md"))
	if err != nil {
		t.Fatalf("read restored artifact: %v", err)
	}
	if string(artifact) != "hello" {
		t.Fatalf("restored artifact = %q, want hello", artifact)
	}
	if _, err := os.Stat(stalePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale target file stat = %v, want not exist after complete replacement", err)
	}
}

func TestSnapshotsIdentityChangesForEveryPortableDefinitionFact(t *testing.T) {
	t.Parallel()

	svc := newRoundTripService(t)
	baseInput := defaultSnapshotIdentityInput()
	base := captureLoadedIdentitySnapshot(t, svc, baseInput)

	tests := []struct {
		name   string
		change func(*snapshotIdentityInput)
	}{
		{
			name: "canonical definition",
			change: func(input *snapshotIdentityInput) {
				input.name = "beta"
			},
		},
		{
			name: "runtime worker binding",
			change: func(input *snapshotIdentityInput) {
				input.workerBody = "changed runtime writer"
			},
		},
		{
			name: "runtime workstation binding",
			change: func(input *snapshotIdentityInput) {
				input.workstationBody = "changed runtime workstation"
			},
		},
		{
			name: "Factory source root",
			change: func(input *snapshotIdentityInput) {
				input.directory = "/factories/beta"
			},
		},
		{
			name: "source directory",
			change: func(input *snapshotIdentityInput) {
				input.sourceDirectory = "/source/beta"
			},
		},
		{
			name: "metadata",
			change: func(input *snapshotIdentityInput) {
				input.metadata = "second"
			},
		},
		{
			name: "bundled artifact content",
			change: func(input *snapshotIdentityInput) {
				input.artifact = "changed"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := baseInput
			test.change(&input)
			changed := captureLoadedIdentitySnapshot(t, svc, input)
			if changed.Identity == "" || changed.Identity == base.Identity {
				t.Fatalf(
					"%s identity = %q, want a distinct non-empty identity from %q",
					test.name,
					changed.Identity,
					base.Identity,
				)
			}
		})
	}
}

func TestSnapshotsRejectIntegrityFailuresBeforeTargetMutation(t *testing.T) {
	t.Parallel()

	svc := newRoundTripService(t)
	ctx := context.Background()
	captured, err := svc.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{
		FactoryDir: "/factories/alpha",
		Canonical:  canonicalSnapshotInput("hello"),
	})
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot: %v", err)
	}
	payload, err := json.Marshal(captured.Snapshot)
	if err != nil {
		t.Fatalf("marshal captured snapshot: %v", err)
	}

	targetDir := t.TempDir()
	previousPath := filepath.Join(targetDir, "previous.txt")
	if err := os.WriteFile(previousPath, []byte("retain"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	_, err = svc.MaterializeFactorySnapshot(ctx, factorydefinitions.MaterializeFactorySnapshotRequest{
		TargetDir:        targetDir,
		Snapshot:         captured.Snapshot,
		ExpectedIdentity: factorydefinitions.SnapshotIdentity("sha256:wrong"),
	})
	assertSnapshotErrorClassification(t, err, factorydefinitions.SnapshotErrorIntegrity)
	assertRetainedTarget(t, previousPath)

	var tampered map[string]any
	if err := json.Unmarshal(payload, &tampered); err != nil {
		t.Fatalf("decode snapshot for tamper: %v", err)
	}
	manifest := tampered["resourceManifest"].(map[string]any)
	files := manifest["bundledFiles"].([]any)
	files[0].(map[string]any)["content"].(map[string]any)["inline"] = "tampered"
	tamperedPayload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatalf("encode tampered snapshot: %v", err)
	}
	_, err = svc.PrepareFactorySnapshotImport(ctx, factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: tamperedPayload})
	assertSnapshotErrorClassification(t, err, factorydefinitions.SnapshotErrorIntegrity)
	assertRetainedTarget(t, previousPath)

	absentTarget := filepath.Join(t.TempDir(), "absent")
	_, err = svc.MaterializeFactorySnapshot(ctx, factorydefinitions.MaterializeFactorySnapshotRequest{
		TargetDir:        absentTarget,
		Snapshot:         captured.Snapshot,
		ExpectedIdentity: factorydefinitions.SnapshotIdentity("sha256:wrong"),
	})
	assertSnapshotErrorClassification(t, err, factorydefinitions.SnapshotErrorIntegrity)
	if _, statErr := os.Stat(absentTarget); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("absent failed target stat = %v, want not exist", statErr)
	}
}

func TestSnapshotsClassifyMissingMalformedAndUnsafeInputs(t *testing.T) {
	t.Parallel()

	svc := newRoundTripService(t)
	_, err := svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{},
	)
	assertSnapshotErrorClassification(t, err, factorydefinitions.SnapshotErrorMissing)

	_, err = svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte("{")},
	)
	assertSnapshotErrorClassification(t, err, factorydefinitions.SnapshotErrorMalformed)

	_, err = svc.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: "/factories/alpha",
			Canonical:  canonicalSnapshotInput(""),
		},
	)
	assertSnapshotErrorClassification(t, err, factorydefinitions.SnapshotErrorMissing)

	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "alpha"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	_, err = svc.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "../outside",
			Snapshot:  snapshot,
		},
	)
	assertSnapshotErrorClassification(t, err, factorydefinitions.SnapshotErrorUnsafe)
}

func TestSnapshotsCaptureLoadedDefinitionDetachesMetadata(t *testing.T) {
	t.Parallel()

	svc := newRoundTripService(t)
	metadata := map[string]string{"caller": "first"}
	source := stubLoadedSource{
		dir: "/factories/alpha",
		cfg: &factorydefinitions.FactoryConfig{
			Name: "alpha",
			ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
				BundledFiles: []factorydefinitions.BundledFileConfig{{
					Type:       factorydefinitions.BundledFileTypeDoc,
					TargetPath: "factory/docs/README.md",
					Content: factorydefinitions.BundledFileContentConfig{
						Encoding: factorydefinitions.BundledFileEncodingUTF8,
						Inline:   "hello",
					},
				}},
			},
		},
	}
	captured, err := svc.CaptureLoadedFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureLoadedFactorySnapshotRequest{
			Source:          source,
			SourceDirectory: "/source/alpha",
			Metadata:        metadata,
		},
	)
	if err != nil {
		t.Fatalf("CaptureLoadedFactorySnapshot: %v", err)
	}
	metadata["caller"] = "mutated"
	source.cfg.ResourceManifest.BundledFiles[0].Content.Inline = "mutated"
	var object map[string]any
	if err := captured.Snapshot.Decode(&object); err != nil {
		t.Fatalf("decode captured loaded snapshot: %v", err)
	}
	if object["sourceDirectory"] != "/source/alpha" || object["metadata"].(map[string]any)["caller"] != "first" {
		t.Fatalf("captured loaded snapshot = %#v, want detached source facts", object)
	}
	payload, err := json.Marshal(captured.Snapshot)
	if err != nil {
		t.Fatalf("marshal captured loaded snapshot: %v", err)
	}
	prepared, err := svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport(detached loaded snapshot): %v", err)
	}
	targetDir := t.TempDir()
	if _, err := svc.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: targetDir,
			Snapshot:  prepared.Snapshot,
		},
	); err != nil {
		t.Fatalf("MaterializeFactorySnapshot(detached loaded snapshot): %v", err)
	}
	artifact, err := os.ReadFile(filepath.Join(targetDir, "docs", "README.md"))
	if err != nil {
		t.Fatalf("read detached loaded artifact: %v", err)
	}
	if string(artifact) != "hello" {
		t.Fatalf("detached loaded artifact = %q, want original content", artifact)
	}
}

func canonicalSnapshotInput(content string) []byte {
	return []byte(`{"name":"alpha","workers":[{"name":"writer","type":"codex","body":"write"}],"workstations":[{"name":"review","type":"LOGICAL_MOVE","body":"review"}],"resourceManifest":{"bundledFiles":[{"type":"DOC","targetPath":"factory/docs/README.md","content":{"encoding":"utf-8","inline":"` + content + `"}}]}}`)
}

type snapshotIdentitySource struct {
	stubLoadedSource
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
}

type snapshotIdentityInput struct {
	name            string
	directory       string
	sourceDirectory string
	artifact        string
	workerBody      string
	workstationBody string
	metadata        string
}

func defaultSnapshotIdentityInput() snapshotIdentityInput {
	return snapshotIdentityInput{
		name:            "alpha",
		directory:       "/factories/alpha",
		sourceDirectory: "/source/alpha",
		artifact:        "hello",
		workerBody:      "runtime writer",
		workstationBody: "runtime workstation",
		metadata:        "first",
	}
}

func (s snapshotIdentitySource) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	worker, ok := s.workers[name]
	return worker, ok
}

func (s snapshotIdentitySource) Workstation(name string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
}

func newSnapshotIdentitySource(
	name string,
	directory string,
	artifact string,
	workerBody string,
	workstationBody string,
) snapshotIdentitySource {
	return snapshotIdentitySource{
		stubLoadedSource: stubLoadedSource{
			dir: directory,
			cfg: &factorydefinitions.FactoryConfig{
				Name: name,
				Workers: []factorydefinitions.FactoryWorkerConfig{{
					Name: "writer",
					Type: "codex",
					Body: "authored worker",
				}},
				Workstations: []factorydefinitions.FactoryWorkstationConfig{{
					Name: "review",
					Type: "LOGICAL_MOVE",
					Body: "authored workstation",
				}},
				ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
					BundledFiles: []factorydefinitions.BundledFileConfig{{
						Type:       factorydefinitions.BundledFileTypeDoc,
						TargetPath: "factory/docs/README.md",
						Content: factorydefinitions.BundledFileContentConfig{
							Encoding: factorydefinitions.BundledFileEncodingUTF8,
							Inline:   artifact,
						},
					}},
				},
			},
		},
		workers: map[string]*factorydefinitions.FactoryWorkerConfig{
			"writer": {
				Name: "writer",
				Type: "codex",
				Body: workerBody,
			},
		},
		workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{
			"review": {
				Name: "review",
				Type: "LOGICAL_MOVE",
				Body: workstationBody,
			},
		},
	}
}

func captureLoadedIdentitySnapshot(
	t *testing.T,
	svc factorydefinitions.Snapshots,
	input snapshotIdentityInput,
) factorydefinitions.CaptureFactorySnapshotResult {
	t.Helper()
	result, err := svc.CaptureLoadedFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureLoadedFactorySnapshotRequest{
			Source: newSnapshotIdentitySource(
				input.name,
				input.directory,
				input.artifact,
				input.workerBody,
				input.workstationBody,
			),
			SourceDirectory: input.sourceDirectory,
			Metadata:        map[string]string{"caller": input.metadata},
		},
	)
	if err != nil {
		t.Fatalf("CaptureLoadedFactorySnapshot: %v", err)
	}
	if result.Identity == "" {
		t.Fatal("CaptureLoadedFactorySnapshot returned an empty identity")
	}
	return result
}

func equivalentCanonicalSnapshotInput() []byte {
	return []byte(`{
		"workstations":[{"body":"review","type":"LOGICAL_MOVE","name":"review"}],
		"resourceManifest":{"bundledFiles":[{"content":{"inline":"hello","encoding":"utf-8"},"targetPath":"factory/docs/README.md","type":"DOC"}]},
		"name":"alpha",
		"workers":[{"body":"write","name":"writer","type":"codex"}]
	}`)
}

func assertSnapshotErrorClassification(
	t *testing.T,
	err error,
	want factorydefinitions.SnapshotErrorClassification,
) {
	t.Helper()
	var typed *factorydefinitions.SnapshotInputError
	if !errors.As(err, &typed) || typed.Classification != want {
		t.Fatalf("snapshot error = %v, want %q classification", err, want)
	}
}

func assertRetainedTarget(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preserved target: %v", err)
	}
	if string(content) != "retain" {
		t.Fatalf("preserved target = %q, want retain", content)
	}
}
