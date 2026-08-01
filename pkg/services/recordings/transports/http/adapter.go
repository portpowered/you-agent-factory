// Package http owns HTTP adaptation for Recordings operations.
//
// The top-level HTTP transport registers generated routes and composes this
// adapter when PSS fan-in arrives. Request decoding, generated contract mapping,
// Recordings root invocation, error mapping, and streaming policy for
// Recordings-owned HTTP operations remain here with the owning service.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Adapter maps Recordings service values at the outward HTTP boundary.
type Adapter struct {
	root recordings.Service
}

// NewAdapter constructs the Recordings HTTP representation adapter.
func NewAdapter(root recordings.Service) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
}

// Root returns the accepted Recordings root consumed by adapter-owned operations.
func (a *Adapter) Root() recordings.Service {
	if a == nil {
		return nil
	}
	return a.root
}

// invokeSubscribeFrom forwards subscribe requests through the accepted Recordings root.
func (a *Adapter) invokeSubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if a == nil || a.root == nil {
		return recordings.SubscribeResult{}, errors.New("recordings service is required")
	}
	return a.root.SubscribeFrom(ctx, request)
}

func (a *Adapter) invokeQueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	if a == nil || a.root == nil {
		return recordings.RecordingStatusResult{}, errors.New("recordings service is required")
	}
	return a.root.QueryRecordingStatus(request)
}

func (a *Adapter) invokeBuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	if a == nil || a.root == nil {
		return recordings.BuildPortableArtifactResult{}, errors.New("recordings service is required")
	}
	return a.root.BuildPortableArtifact(request)
}

func (a *Adapter) invokeReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if a == nil || a.root == nil {
		return recordings.ReconstructWorldStateResult{}, errors.New("recordings service is required")
	}
	return a.root.ReconstructWorldState(request)
}

func (a *Adapter) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (a *Adapter) writeError(w http.ResponseWriter, status int, message, code string) {
	a.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

func writeSSEDataJSON(w http.ResponseWriter, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}
