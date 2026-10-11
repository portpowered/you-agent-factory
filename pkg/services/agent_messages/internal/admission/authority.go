package admission

import (
	"context"
	"encoding/json"
	"errors"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// Identity contains server-held, nonsecret facts. Chain and Work are scoped
// identities suitable for durable inbox and successful-send quota accounting.
// Labels and message correlation never participate in authority.
type Identity struct {
	Observation workersessions.Observation
	Chain       string
	Work        string
}

type ObservationReader func(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error)

// Authorizer consumes only the public Worker Sessions caller and read ports.
// It does not retain credentials or call execution/control capabilities.
type Authorizer struct {
	validate workersessions.CallerValidator
	read     ObservationReader
}

func NewAuthorizer(validate workersessions.CallerValidator, read ObservationReader) *Authorizer {
	return &Authorizer{validate: validate, read: read}
}

// Revalidate is also called under the admission lock immediately before a
// durable write. An earlier successful check never grants durable authority.
func (a *Authorizer) Revalidate(ctx context.Context, caller *workersessions.CallerIdentity) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if caller == nil || !validID(caller.WorkerSessionID) || !validID(caller.Token) || a.validate == nil {
		return agentmessages.ErrNotPermitted
	}
	if a.validate(ctx, caller.Clone()) != nil {
		return agentmessages.ErrNotPermitted
	}
	return nil
}

// Authenticate resolves the exact active principal. A selected recipient
// Factory Session cannot redirect this lookup to another caller's owner.
func (a *Authorizer) Authenticate(ctx context.Context, caller *workersessions.CallerIdentity) (Identity, error) {
	if err := a.Revalidate(ctx, caller); err != nil {
		return Identity{}, err
	}
	identity, err := a.resolve(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: caller.WorkerSessionID})
	if ctx.Err() != nil {
		return Identity{}, ctx.Err()
	}
	if err != nil || identity.Observation.State != workersessions.StateRunning {
		return Identity{}, agentmessages.ErrNotPermitted
	}
	return identity, nil
}

// Recipient preserves the public ambiguity contract and explicit owner scope.
// Other collaborator failures omit their diagnostics and fail closed.
func (a *Authorizer) Recipient(ctx context.Context, worker, factory string) (Identity, error) {
	identity, err := a.resolve(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: worker, FactorySessionID: factory,
	})
	if ctx.Err() != nil {
		return Identity{}, ctx.Err()
	}
	if errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) {
		var ambiguous *workersessions.AmbiguousAddressError
		if errors.As(err, &ambiguous) {
			return Identity{}, ambiguous.Clone()
		}
		return Identity{}, workersessions.ErrWorkerSessionAmbiguous
	}
	if err != nil {
		return Identity{}, agentmessages.ErrRecipientNotFound
	}
	return identity, nil
}

func (a *Authorizer) resolve(ctx context.Context, request workersessions.GetObservationByWorkerSessionIDRequest) (Identity, error) {
	if ctx.Err() != nil {
		return Identity{}, ctx.Err()
	}
	if request.Validate() != nil || a.read == nil {
		return Identity{}, agentmessages.ErrNotPermitted
	}
	observation, err := a.read(ctx, request)
	if err != nil {
		return Identity{}, err
	}
	if observation.WorkerSessionID != request.WorkerSessionID || !observation.State.Valid() ||
		(request.FactorySessionID != "" && observation.FactorySessionID != request.FactorySessionID) {
		return Identity{}, agentmessages.ErrNotPermitted
	}
	observation = observation.Clone()
	root, err := a.chainRoot(ctx, observation)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Observation: observation, Chain: scopedIdentity(observation.FactorySessionID, root), Work: workIdentity(observation)}, nil
}

func (a *Authorizer) chainRoot(ctx context.Context, observation workersessions.Observation) (string, error) {
	seen := make(map[string]bool)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if seen[observation.WorkerSessionID] {
			return "", agentmessages.ErrNotPermitted
		}
		seen[observation.WorkerSessionID] = true
		previous := observation.PredecessorWorkerSessionID
		if previous == "" {
			return observation.WorkerSessionID, nil
		}
		parent, err := a.read(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
			WorkerSessionID: previous, FactorySessionID: observation.FactorySessionID,
		})
		if err != nil || parent.WorkerSessionID != previous || parent.FactorySessionID != observation.FactorySessionID ||
			parent.SuccessorWorkerSessionID != observation.WorkerSessionID || !parent.State.Terminal() {
			return "", agentmessages.ErrNotPermitted
		}
		observation = parent
	}
}

// PermitSend allows only an evidenced requester/descendant relationship.
// Sharing a project label, Work, or requester does not authorize siblings.
func (a *Authorizer) PermitSend(ctx context.Context, sender, recipient Identity) error {
	if sender.Chain == "" || recipient.Chain == "" || sender.Chain == recipient.Chain {
		return agentmessages.ErrNotPermitted
	}
	up, err := a.hasRequester(ctx, sender, recipient)
	if err != nil {
		return err
	}
	if up {
		return nil
	}
	down, err := a.hasRequester(ctx, recipient, sender)
	if err != nil {
		return err
	}
	if !down {
		return agentmessages.ErrNotPermitted
	}
	return nil
}

func (a *Authorizer) hasRequester(ctx context.Context, child, target Identity) (bool, error) {
	seen := make(map[string]bool)
	matched := false
	for {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if seen[child.Chain] {
			return false, agentmessages.ErrNotPermitted
		}
		seen[child.Chain] = true
		requester := child.Observation.Requester
		if requester == nil {
			return matched, nil
		}
		if requester.Kind != "WORKER_SESSION" || !validID(requester.WorkerSessionID) {
			return false, agentmessages.ErrNotPermitted
		}
		parent, err := a.resolve(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: requester.WorkerSessionID})
		if err != nil {
			return false, agentmessages.ErrNotPermitted
		}
		if parent.Chain == target.Chain || sameRequesterWork(requester, parent, target) {
			matched = true
		}
		child = parent
	}
}

func sameRequesterWork(requester *workersessions.Requester, parent, target Identity) bool {
	// Redispatch of the requester's own Project Work is authorized only when
	// the recorded requester explicitly names that verified, scoped Work.
	return requester.WorkID != "" && parent.Work != "" && parent.Work == target.Work &&
		parent.Work == scopedIdentity(parent.Observation.FactorySessionID, requester.WorkID)
}

// Matches uses only identities retained at admission. Operator
// observation is handled by the caller and must never invoke recipient writes.
func (i Identity) Matches(chain, work string) bool {
	return i.Chain != "" && (i.Chain == chain || (i.Work != "" && i.Work == work))
}

func workIdentity(observation workersessions.Observation) string {
	if len(observation.WorkIDs) > 0 && validID(observation.WorkIDs[0]) && observation.FactorySessionID != "" {
		return scopedIdentity(observation.FactorySessionID, observation.WorkIDs[0])
	}
	if observation.Direct && observation.Correlation != nil && validID(observation.Correlation.WorkID) &&
		validID(observation.Correlation.FactorySessionID) {
		return scopedIdentity(observation.Correlation.FactorySessionID, observation.Correlation.WorkID)
	}
	return ""
}

func scopedIdentity(factory, id string) string {
	// A tuple encoding avoids delimiter collisions in opaque public IDs.
	encoded, _ := json.Marshal([2]string{factory, id})
	return digest(encoded)
}
