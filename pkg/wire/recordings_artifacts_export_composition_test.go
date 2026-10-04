package wire

import (
	"context"
	"errors"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"reflect"
	"testing"
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
