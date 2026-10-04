package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformcontentstaging "github.com/portpowered/infinite-you/pkg/platform/contentstaging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// This fixture observes provider forwarding, not the implementation of the
// supplied owner. Native owner tests and public functional journeys own policy.
type artifactForwardingOwner struct {
	request  any
	response any
	err      error
	ctx      context.Context
	calls    int
}

func (owner *artifactForwardingOwner) BuildPortableArtifact(request recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.BuildPortableArtifactResult{}, owner.err
	}
	return owner.response.(recordings.BuildPortableArtifactResult), nil
}
func (owner *artifactForwardingOwner) ValidatePortableArtifact(request recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.ValidatePortableArtifactResult{}, owner.err
	}
	return owner.response.(recordings.ValidatePortableArtifactResult), nil
}
func (owner *artifactForwardingOwner) EncodePortableArtifact(request recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.EncodePortableArtifactResult{}, owner.err
	}
	return owner.response.(recordings.EncodePortableArtifactResult), nil
}
func (owner *artifactForwardingOwner) DecodePortableArtifact(request recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.DecodePortableArtifactResult{}, owner.err
	}
	return owner.response.(recordings.DecodePortableArtifactResult), nil
}
func (owner *artifactForwardingOwner) SummarizePortableArtifact(request recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.SummarizePortableArtifactResult{}, owner.err
	}
	return owner.response.(recordings.SummarizePortableArtifactResult), nil
}
func (owner *artifactForwardingOwner) ExportPortableArtifact(ctx context.Context, request recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error) {
	owner.request = request
	owner.calls++
	owner.ctx = ctx
	if owner.err != nil {
		return recordings.ExportPortableArtifactResult{}, owner.err
	}
	return owner.response.(recordings.ExportPortableArtifactResult), nil
}
func (owner *artifactForwardingOwner) ReadPortableArtifact(ctx context.Context, request recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
	owner.request = request
	owner.calls++
	owner.ctx = ctx
	if owner.err != nil {
		return recordings.ReadPortableArtifactResult{}, owner.err
	}
	return owner.response.(recordings.ReadPortableArtifactResult), nil
}
func TestProvideRecordingsRootForwardsCompletedArtifactOwner(t *testing.T) {
	t.Parallel()
	artifact := recordings.PortableArtifact{SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary: recordings.PortableArtifactSummary{RecordingID: "selected-recording", Reference: "artifact:selected", EventCount: 1, Available: true},
		Events:  []recordings.CanonicalEvent{{ID: "selected-event", Payload: "{}"}}}
	cases := []struct {
		name        string
		failure     error
		request     any
		response    any
		withContext bool
		call        func(recordings.Service, context.Context) (any, error)
	}{
		{name: "BuildPortableArtifact", failure: recordings.ErrPortableArtifactUnavailable, request: recordings.BuildPortableArtifactRequest{RecordingID: "selected-recording"}, response: recordings.BuildPortableArtifactResult{Artifact: artifact}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: "selected-recording"})
			}},
		{name: "ValidatePortableArtifact", failure: recordings.ErrInvalidPortableArtifact, request: recordings.ValidatePortableArtifactRequest{Artifact: artifact}, response: recordings.ValidatePortableArtifactResult{Summary: artifact.Summary}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{Artifact: artifact})
			}},
		{name: "EncodePortableArtifact", failure: recordings.ErrInvalidPortableArtifact, request: recordings.EncodePortableArtifactRequest{Artifact: artifact}, response: recordings.EncodePortableArtifactResult{Payload: []byte("encoded-owner-result")}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{Artifact: artifact})
			}},
		{name: "DecodePortableArtifact", failure: recordings.ErrInvalidPortableArtifact, request: recordings.DecodePortableArtifactRequest{Payload: []byte("owner-input")}, response: recordings.DecodePortableArtifactResult{Artifact: artifact, IgnoredJSONPaths: []string{"$.future"}}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{Payload: []byte("owner-input")})
			}},
		{name: "SummarizePortableArtifact", failure: recordings.ErrInvalidPortableArtifact, request: recordings.SummarizePortableArtifactRequest{Artifact: artifact}, response: recordings.SummarizePortableArtifactResult{Summary: artifact.Summary}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{Artifact: artifact})
			}},
		{name: "ExportPortableArtifact", failure: errors.Join(recordings.ErrPortableArtifactCancelled, context.Canceled, errors.New("operator canceled export")), request: recordings.ExportPortableArtifactRequest{RecordingID: "selected-recording"}, response: recordings.ExportPortableArtifactResult{Reference: "artifact:selected", Artifact: artifact}, withContext: true,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.ExportPortableArtifact(ctx, recordings.ExportPortableArtifactRequest{RecordingID: "selected-recording"})
			}},
		{name: "ReadPortableArtifact", failure: recordings.ErrForeignPortableArtifact, request: recordings.ReadPortableArtifactRequest{RecordingID: "selected-recording", Reference: "artifact:selected"}, response: recordings.ReadPortableArtifactResult{Artifact: artifact, IgnoredJSONPaths: []string{"$.future"}}, withContext: true,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{RecordingID: "selected-recording", Reference: "artifact:selected"})
			}},
	}
	for _, cell := range cases {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			for _, failure := range []error{nil, cell.failure, errors.New("selected-owner-dependency-failure")} {
				owner := &artifactForwardingOwner{response: cell.response, err: failure}
				root := testRecordingsRoot(serviceedges.Edges{}, owner, inertReplayOwner{})
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				result, err := cell.call(root, ctx)
				if err != failure || owner.calls != 1 || !reflect.DeepEqual(owner.request, cell.request) {
					t.Fatalf("forward = %v, calls %d, request %#v; want %v and %#v", err, owner.calls, owner.request, failure, cell.request)
				}
				if failure == nil && !reflect.DeepEqual(result, cell.response) {
					t.Fatalf("result = %#v, want %#v", result, cell.response)
				}
				if cell.withContext && owner.ctx != ctx {
					t.Fatal("owner did not receive caller context")
				}
			}
		})
	}
}

// timestampTestSource is a Now-only source: runtime logical ticks cannot advance it.
type timestampTestSource struct{ nanos atomic.Int64 }

func (source *timestampTestSource) Now() time.Time { return time.Unix(0, source.nanos.Load()).UTC() }

func TestSelectedTimestampProvidersUseProcessSource(t *testing.T) {
	t.Parallel()
	source := &timestampTestSource{}
	base := time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC)
	source.nanos.Store(base.UnixNano())
	artifactNow := provideRuntimeArtifactClock(source)
	defaults := provideCLIRunDefaults(nil, provideRecordingsCLIAdapter(), source)
	reserver, err := provideRuntimeArtifactPathReserver()
	if err != nil {
		t.Fatal(err)
	}
	planner := provideLiveRecordingTargetPlanner(reserver, source)
	home := t.TempDir()
	for index, instant := range []time.Time{base, base.Add(24 * time.Hour)} {
		source.nanos.Store(instant.UnixNano())
		if got := artifactNow(); !got.Equal(instant) {
			t.Fatalf("artifact time = %v, want %v", got, instant)
		}
		if got := defaults.Clock.Now(); !got.Equal(instant) {
			t.Fatalf("CLI time = %v, want %v", got, instant)
		}
		id := []string{"7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba", "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3bb"}[index]
		target, err := planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: home, CanonicalSessionID: id, ReportedSessionID: "~default"})
		if err != nil {
			t.Fatal(err)
		}
		datedSuffix := filepath.Join(instant.Format("2006"), instant.Format("01"), instant.Format("02"), id+".json")
		if !strings.HasSuffix(target.ServicePath, datedSuffix) || target.ReportedPath != target.ServicePath {
			t.Fatalf("target = %#v, want shared path ending %q", target, datedSuffix)
		}
	}
}

type timestampStagingFiles struct {
	platformcontentstaging.FileSystem
	root string
}

func (files timestampStagingFiles) MkdirTemp(_ string, pattern string) (string, error) {
	return os.MkdirTemp(files.root, pattern)
}

func TestSelectedStagingClockPreservesOverride(t *testing.T) {
	t.Parallel()
	for _, overridden := range []bool{false, true} {
		t.Run(strconv.FormatBool(overridden), func(t *testing.T) {
			t.Parallel()
			base := time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC)
			selected, specialized := &timestampTestSource{}, &timestampTestSource{}
			selected.nanos.Store(base.UnixNano())
			specialized.nanos.Store(base.Add(time.Hour).UnixNano())
			edges := serviceedges.Edges{WorkContentStagingFileSystem: timestampStagingFiles{root: t.TempDir()}}
			effective := selected
			if overridden {
				edges.WorkContentStagingClock = specialized
				effective = specialized
			}
			issuedAt := effective.Now()
			staging, err := provideWorkContentStagingService(edges, selected)
			if err != nil {
				t.Fatal(err)
			}
			staged, err := staging.StageContent(t.Context(), work.StageContentRequest{ItemType: "image", FileName: "image.png", MediaType: "image/png", Content: []byte("test image")})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := staging.CleanupContent(t.Context(), staged.StagedFileRef); err != nil {
					t.Error(err)
				}
			}()
			if overridden {
				selected.nanos.Store(base.Add(2 * time.Hour).UnixNano())
			}
			effective.nanos.Store(issuedAt.Add(time.Hour - time.Nanosecond).UnixNano())
			resolved, err := staging.ResolveContent(t.Context(), staged.StagedFileRef)
			if err != nil || !resolved.ExpiresAt.Equal(issuedAt.Add(time.Hour)) {
				t.Fatalf("before expiry = %#v, %v", resolved, err)
			}
			effective.nanos.Store(issuedAt.Add(time.Hour).UnixNano())
			if _, err := staging.ResolveContent(t.Context(), staged.StagedFileRef); !errors.Is(err, work.ErrStagedContentExpired) {
				t.Fatalf("exact expiry = %v", err)
			}
		})
	}
}
