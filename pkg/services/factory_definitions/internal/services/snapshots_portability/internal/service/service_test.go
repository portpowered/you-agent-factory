package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilityservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/service"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
)

type stubLoadedSource struct {
	dir string
	cfg *factorydefinitions.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                               { return s.dir }
func (s stubLoadedSource) RuntimeBaseDir() string                           { return "" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                         {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*workerconfig.Config) error) error { return nil }
func (s stubLoadedSource) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*workerconfig.Config, bool) { return nil, false }

func stubLoadCanonical(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	var cfg factorydefinitions.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factorydefinitions.ErrInvalidNamedFactory
	}
	return stubLoadedSource{dir: "/factories/example", cfg: &cfg}, nil
}

func stubPreparePortable(
	_ string,
	factoryConfig *factorydefinitions.FactoryConfig,
	_ bool,
) (*factorydefinitions.FactoryConfig, error) {
	return factoryConfig, nil
}

func newCaptureService(t *testing.T) snapshotsportability.Service {
	t.Helper()
	return newSnapshotService(t, stubDecodeSnapshot)
}

func stubDecodeSnapshot(payload []byte) (*factorydefinitions.FactorySnapshot, error) {
	return factorydefinitions.NewFactorySnapshot(json.RawMessage(payload))
}

func newSnapshotService(
	t *testing.T,
	decode factorydefinitions.FactorySnapshotJSONDecoder,
) snapshotsportability.Service {
	t.Helper()
	svc := snapshotsportabilityservice.New(
		stubLoadCanonical,
		func(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
			return factorydefinitions.NewFactorySnapshot(map[string]any{"name": "captured"})
		},
		stubPreparePortable,
		decode,
		func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
			return nil, nil
		},
		func(string, *factorydefinitions.FactoryConfig) error { return nil },
	)
	if svc == nil {
		t.Fatal("component rejected complete test fixture")
	}
	return svc
}

func testSnapshotPayload() []byte {
	return []byte(`{
		"name": "alpha",
		"factoryDirectory": "/factories/alpha",
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)
}

// Capture owns orchestration; encoding, hashing and file effects are proved at
// their component boundaries and through public session/export scenarios.
func TestCaptureFactorySnapshotUsesPreparedSourceAndReturnsCapturedIdentity(t *testing.T) {
	t.Parallel()
	loadedConfig := &factorydefinitions.FactoryConfig{Name: "loaded"}
	preparedConfig := &factorydefinitions.FactoryConfig{Name: "prepared"}
	captured, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "captured"})
	if err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	svc := snapshotsportabilityservice.New(
		func(payload []byte, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			calls = append(calls, "load")
			if string(payload) != `{"name":"alpha"}` || loader != nil {
				t.Fatalf("load arguments = %s, %v", payload, loader)
			}
			return stubLoadedSource{dir: "/loaded", cfg: loadedConfig}, nil
		},
		func(source factorydefinitions.FactorySnapshotSource, dir string, replacements map[string]string) (*factorydefinitions.FactorySnapshot, error) {
			calls = append(calls, "capture")
			if source.FactoryConfig() != preparedConfig || source.FactoryDir() != "/selected" || dir != "/selected" || replacements != nil {
				t.Fatalf("capture arguments = %#v, %q, %#v", source, dir, replacements)
			}
			return captured, nil
		},
		func(dir string, cfg *factorydefinitions.FactoryConfig, portable bool) (*factorydefinitions.FactoryConfig, error) {
			calls = append(calls, "prepare")
			if dir != "/selected" || cfg != loadedConfig || !portable {
				t.Fatalf("prepare arguments = %q, %p, %v", dir, cfg, portable)
			}
			return preparedConfig, nil
		},
		stubDecodeSnapshot,
		func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
			t.Fatal("capture materialized files")
			return nil, nil
		},
		func(string, *factorydefinitions.FactoryConfig) error { t.Fatal("capture validated writes"); return nil },
	)
	if len(calls) != 0 {
		t.Fatal("construction invoked a port")
	}
	result, err := svc.CaptureFactorySnapshot(t.Context(), factorydefinitions.CaptureFactorySnapshotRequest{FactoryDir: " /selected ", Canonical: []byte(` {"name":"alpha"} `)})
	if err != nil || result.Snapshot != captured {
		t.Fatalf("capture result=%#v error=%v", result, err)
	}
	if !reflect.DeepEqual(calls, []string{"load", "prepare", "capture"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestCaptureFactorySnapshot_InvalidPayloadReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	svc := newCaptureService(t)

	_, err := svc.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`"string"`)},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"CaptureFactorySnapshot invalid-payload error = %v, want %v",
			err,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}
}

func TestPrepareFactorySnapshotImport_SuccessReturnsPortableFacts(t *testing.T) {
	t.Parallel()

	svc := newSnapshotService(t, stubDecodeSnapshot)
	payload := testSnapshotPayload()

	imported, err := svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport result = %#v, want alpha snapshot facts", imported)
	}
	if imported.Portable.FactoryDir != "/factories/alpha" ||
		len(imported.Portable.Assets) == 0 ||
		imported.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("PrepareFactorySnapshotImport portable = %#v, want portable success facts", imported.Portable)
	}
}

func TestPrepareFactorySnapshotImport_InvalidPayloadReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	svc := newSnapshotService(t, stubDecodeSnapshot)

	_, err := svc.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`["not-object"]`)},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"PrepareFactorySnapshotImport invalid-payload error = %v, want %v",
			err,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}
}

func TestEmptySnapshotRequestsRejectBeforeCallingPorts(t *testing.T) {
	t.Parallel()
	svc := snapshotsportabilityservice.New(
		func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			t.Fatal("empty request called loader")
			return nil, nil
		},
		func(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
			t.Fatal("empty request called capture")
			return nil, nil
		},
		func(string, *factorydefinitions.FactoryConfig, bool) (*factorydefinitions.FactoryConfig, error) {
			t.Fatal("empty request called portable preparation")
			return nil, nil
		},
		func([]byte) (*factorydefinitions.FactorySnapshot, error) {
			t.Fatal("empty request called decoder")
			return nil, nil
		},
		func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
			t.Fatal("empty request wrote files")
			return nil, nil
		},
		func(string, *factorydefinitions.FactoryConfig) error {
			t.Fatal("empty request validated writes")
			return nil
		},
	)
	if _, err := svc.PrepareFactorySnapshotImport(t.Context(), factorydefinitions.PrepareFactorySnapshotImportRequest{}); !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("empty import error = %v, want ErrInvalidFactorySnapshotPayload", err)
	}
	if _, err := svc.MaterializeFactorySnapshot(t.Context(), factorydefinitions.MaterializeFactorySnapshotRequest{}); !errors.Is(err, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf("empty materialization error = %v, want ErrUnsafeFactorySnapshotMaterialize", err)
	}
}

func TestMaterializeFactorySnapshotValidatesBeforeWritingAndReturnsAssetFacts(t *testing.T) {
	t.Parallel()
	calls := []string{}
	var validated *factorydefinitions.FactoryConfig
	svc := snapshotsportabilityservice.New(stubLoadCanonical,
		func(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
			t.Fatal("materialize captured a snapshot")
			return nil, nil
		},
		stubPreparePortable, stubDecodeSnapshot,
		func(dir string, cfg *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
			calls = append(calls, "write")
			if dir != "/target" || cfg != validated {
				t.Fatalf("write arguments = %q, %p", dir, cfg)
			}
			return nil, nil
		},
		func(dir string, cfg *factorydefinitions.FactoryConfig) error {
			calls = append(calls, "validate")
			if dir != "/target" || cfg.Name != "alpha" || cfg.ResourceManifest.BundledFiles[0].TargetPath != "factory/docs/README.md" {
				t.Fatalf("validate arguments = %q, %#v", dir, cfg)
			}
			validated = cfg
			return nil
		})
	snapshot, err := stubDecodeSnapshot(testSnapshotPayload())
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.MaterializeFactorySnapshot(t.Context(), factorydefinitions.MaterializeFactorySnapshotRequest{TargetDir: " /target ", Snapshot: snapshot})
	if err != nil || result.TargetDir != "/target" || result.Portable.FactoryDir != "/target" || len(result.Portable.Assets) != 1 || result.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("materialize result=%#v error=%v", result, err)
	}
	if !reflect.DeepEqual(calls, []string{"validate", "write"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestMaterializeFactorySnapshot_UnsafeTargetReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	svc := newSnapshotService(t, stubDecodeSnapshot)
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "alpha"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	_, unsafeErr := svc.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "../outside",
			Snapshot:  snapshot,
		},
	)
	if !errors.Is(unsafeErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf(
			"MaterializeFactorySnapshot unsafe error = %v, want %v",
			unsafeErr,
			factorydefinitions.ErrUnsafeFactorySnapshotMaterialize,
		)
	}
	if errors.Is(unsafeErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatal("unsafe materialize must not also match ErrInvalidFactorySnapshotPayload")
	}
}

func TestCaptureFactorySnapshotPreservesPortFailuresAndStopsDownstreamCalls(t *testing.T) {
	t.Parallel()
	failure := errors.New("controlled failure")
	for _, tc := range []struct {
		name                                                       string
		loadErr                                                    error
		noSource, noConfig, prepareFails, captureFails, noSnapshot bool
		want                                                       error
		calls                                                      []string
	}{
		{name: "load failure", loadErr: failure, want: failure, calls: []string{"load"}},
		{name: "invalid named definition", loadErr: factorydefinitions.ErrInvalidNamedFactory, want: factorydefinitions.ErrInvalidFactorySnapshotPayload, calls: []string{"load"}},
		{name: "missing source", noSource: true, want: factorydefinitions.ErrInvalidFactorySnapshotPayload, calls: []string{"load"}},
		{name: "missing config", noConfig: true, want: factorydefinitions.ErrInvalidFactorySnapshotPayload, calls: []string{"load"}},
		{name: "prepare failure", prepareFails: true, want: failure, calls: []string{"load", "prepare"}},
		{name: "capture failure", captureFails: true, want: failure, calls: []string{"load", "prepare", "capture"}},
		{name: "missing snapshot", noSnapshot: true, want: factorydefinitions.ErrInvalidFactorySnapshotPayload, calls: []string{"load", "prepare", "capture"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := []string{}
			svc := snapshotsportabilityservice.New(
				func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					calls = append(calls, "load")
					if tc.loadErr != nil || tc.noSource {
						return nil, tc.loadErr
					}
					var cfg *factorydefinitions.FactoryConfig
					if !tc.noConfig {
						cfg = &factorydefinitions.FactoryConfig{Name: "alpha"}
					}
					return stubLoadedSource{dir: "/loaded", cfg: cfg}, nil
				},
				func(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
					calls = append(calls, "capture")
					if tc.captureFails {
						return nil, failure
					}
					return nil, nil
				},
				func(dir string, cfg *factorydefinitions.FactoryConfig, portable bool) (*factorydefinitions.FactoryConfig, error) {
					calls = append(calls, "prepare")
					if dir != "/loaded" || !portable {
						t.Fatalf("fallback arguments = %q, %v", dir, portable)
					}
					if tc.prepareFails {
						return nil, failure
					}
					return cfg, nil
				}, stubDecodeSnapshot,
				func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
					t.Fatal("capture wrote files")
					return nil, nil
				},
				func(string, *factorydefinitions.FactoryConfig) error { t.Fatal("capture validated writes"); return nil },
			)
			result, err := svc.CaptureFactorySnapshot(t.Context(), factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`{"name":"alpha"}`)})
			if !errors.Is(err, tc.want) || result.Snapshot != nil || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("result=%#v error=%v calls=%v", result, err, calls)
			}
		})
	}
}

func TestMaterializeFactorySnapshotFailsClosedAtEachWritePort(t *testing.T) {
	t.Parallel()
	for _, validationFails := range []bool{true, false} {
		t.Run(fmt.Sprint(validationFails), func(t *testing.T) {
			t.Parallel()
			writes := 0
			svc := snapshotsportabilityservice.New(stubLoadCanonical,
				func(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
					return nil, nil
				},
				stubPreparePortable, stubDecodeSnapshot,
				func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
					writes++
					return nil, errors.New("write failed")
				},
				func(string, *factorydefinitions.FactoryConfig) error {
					if validationFails {
						return errors.New("unsafe writes")
					}
					return nil
				})
			snapshot, err := stubDecodeSnapshot(testSnapshotPayload())
			if err != nil {
				t.Fatal(err)
			}
			result, err := svc.MaterializeFactorySnapshot(t.Context(), factorydefinitions.MaterializeFactorySnapshotRequest{TargetDir: "/target", Snapshot: snapshot})
			wantWrites := 1
			if validationFails {
				wantWrites = 0
			}
			if !errors.Is(err, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) || writes != wantWrites || !reflect.DeepEqual(result, factorydefinitions.MaterializeFactorySnapshotResult{}) {
				t.Fatalf("result=%#v error=%v writes=%d", result, err, writes)
			}
		})
	}
}

func TestCanceledSnapshotOperationsDoNotInvokePorts(t *testing.T) {
	t.Parallel()
	unexpected := func() { t.Fatal("canceled operation invoked a port") }
	svc := snapshotsportabilityservice.New(
		func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			unexpected()
			return nil, nil
		},
		func(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
			unexpected()
			return nil, nil
		},
		func(string, *factorydefinitions.FactoryConfig, bool) (*factorydefinitions.FactoryConfig, error) {
			unexpected()
			return nil, nil
		},
		func([]byte) (*factorydefinitions.FactorySnapshot, error) { unexpected(); return nil, nil },
		func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
			unexpected()
			return nil, nil
		},
		func(string, *factorydefinitions.FactoryConfig) error { unexpected(); return nil })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, captureErr := svc.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{})
	_, importErr := svc.PrepareFactorySnapshotImport(ctx, factorydefinitions.PrepareFactorySnapshotImportRequest{})
	_, writeErr := svc.MaterializeFactorySnapshot(ctx, factorydefinitions.MaterializeFactorySnapshotRequest{})
	for _, err := range []error{captureErr, importErr, writeErr} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	}
}
