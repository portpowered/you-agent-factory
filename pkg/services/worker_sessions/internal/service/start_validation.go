package service

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// callerMetadataLocked resolves credentials against this process's running
// owner. The token disambiguates equal public IDs in different Factory Sessions.
// No archived observation or ambient identity can restore caller authority.
func (r *registry) callerMetadataLocked(caller *workersessions.CallerIdentity, metadata *workersessions.SessionMetadata) (*workersessions.SessionMetadata, error) {
	if caller == nil {
		return metadata.Clone(), nil
	}
	entropy, err := base64.RawURLEncoding.Strict().DecodeString(caller.Token)
	if err != nil || len(entropy) != 32 || r.stopping {
		return nil, workersessions.ErrCallerInvalid
	}
	var owner *workersessions.Session
	for address, token := range r.executionTokens {
		session := r.sessions[address]
		if publicWorkerID(address) != caller.WorkerSessionID || session.State != workersessions.StateRunning ||
			subtle.ConstantTimeCompare([]byte(token), []byte(caller.Token)) != 1 {
			continue
		}
		if owner != nil {
			return nil, workersessions.ErrCallerInvalid
		}
		owner = &session
	}
	if owner == nil {
		return nil, workersessions.ErrCallerInvalid
	}
	derived := owner.Metadata.Clone()
	if derived == nil {
		derived = &workersessions.SessionMetadata{}
	}
	derived.Requester = &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: caller.WorkerSessionID}
	derived.Labels = []string{"parent:" + caller.WorkerSessionID}
	if derived.Correlation != nil {
		derived.Requester.WorkID = derived.Correlation.WorkID
	}
	return derived, nil
}

func (r *registry) ValidateCaller(_ context.Context, caller *workersessions.CallerIdentity) error {
	_, err := r.resolveCallerMetadata(caller.Clone(), nil)
	return err
}

func (r *registry) resolveCallerMetadata(caller *workersessions.CallerIdentity, metadata *workersessions.SessionMetadata) (*workersessions.SessionMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callerMetadataLocked(caller, metadata)
}

// Runtime children retain their actual Work, Factory Session and labels. Only
// the requester is derived from the admitted caller, under the reservation lock.
func (r *registry) reserveInvocation(req workersessions.InvokeSessionRequest, runtimeOwned bool) error {
	if !runtimeOwned || req.Caller == nil {
		return r.reserveWithCaller(req.ID, req.Metadata, req.Caller)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	verified, err := r.callerMetadataLocked(req.Caller, nil)
	if err != nil {
		return err
	}
	metadata := req.Metadata.Clone()
	if metadata == nil {
		metadata = &workersessions.SessionMetadata{}
	}
	metadata.Requester = verified.Requester
	parent := "parent:" + req.Caller.WorkerSessionID
	if !slices.Contains(metadata.Labels, parent) {
		metadata.Labels = append(metadata.Labels, parent)
	}
	if err := metadata.Validate(); err != nil {
		return err
	}
	r.reserveIfAbsentLocked(req.ID, metadata)
	return nil
}

// Caller verification and metadata capture share the reservation lock,
// including when reusing an identity with already authoritative metadata.
func (r *registry) reserveWithCaller(id string, metadata *workersessions.SessionMetadata, caller *workersessions.CallerIdentity) error {
	if caller == nil {
		r.reserveIfAbsent(id, metadata)
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	derived, err := r.callerMetadataLocked(caller, metadata)
	if err != nil {
		return err
	}
	r.reserveIfAbsentLocked(id, derived)
	return nil
}

func (r *registry) lookupStart(req workersessions.StartRequest) (*startReplay, error) {
	tuple := startTupleFor(req)
	r.mu.RLock()
	defer r.mu.RUnlock()
	replay := r.startReplays[req.RequestID]
	if replay != nil && !reflect.DeepEqual(replay.tuple, tuple) {
		return nil, workersessions.ErrStartRequestIDConflict
	}
	return replay, nil
}

func (r *registry) validateStartExecution(ctx context.Context, executor workers.Service, req workersessions.StartRequest) error {
	request, err := executeRequestFromSessionDispatch(req.Execution)
	if err == nil {
		err = executor.ValidateExecution(ctx, request)
	}
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// Availability is an admission failure even when Workers wraps it in its
	// invalid-request sentinel. Transport errors expose bounded messages.
	if errors.Is(err, providers.ErrProviderUnavailable) || errors.Is(err, workers.ErrExecuteUnavailable) {
		return fmt.Errorf("%w: %w", workersessions.ErrStartAdmissionFailed, err)
	}
	return fmt.Errorf("%w: %w", workersessions.ErrInvalidExecutionRequest, err)
}

// bindExecutionIdentityEnvironment runs only after the safe restart recipe has
// been prepared. Caller revalidation and credential installation share the
// registry lock, fencing owner loss before execution admission. Tokens are
// process-local, never Session or recipe fields.
func (r *registry) bindExecutionIdentityEnvironment(id string, caller *workersessions.CallerIdentity) ([]string, error) {
	entropy := make([]byte, 32)
	r.tokenEntropyMu.Lock()
	_, err := io.ReadFull(r.tokenEntropy, entropy)
	r.tokenEntropyMu.Unlock()
	if err != nil {
		return nil, errors.New("worker sessions: token entropy unavailable")
	}
	token := base64.RawURLEncoding.EncodeToString(entropy)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.callerMetadataLocked(caller, nil); err != nil {
		return nil, err
	}
	session, exists := r.sessions[id]
	if !exists || (session.State != workersessions.StateStarting && session.State != workersessions.StateRunning) || r.stopping {
		return nil, workersessions.ErrSessionNotStartable
	}
	if r.executionTokens == nil {
		r.executionTokens = make(map[string]string)
	}
	if r.executionSecrets == nil {
		r.executionSecrets = make(map[string][]string)
	}
	r.executionTokens[id] = token
	r.executionSecrets[id] = append(r.executionSecrets[id], token)
	session = cloneSession(session)
	session.ID = publicWorkerID(id)
	environment := append(session.IdentityEnvironment(), "YOU_WORKER_SESSION_TOKEN="+token)
	if endpoint := r.executionEndpointLocked(); endpoint != "" {
		environment = append(environment, "YOU_SERVER="+endpoint)
	}
	return environment, nil
}
