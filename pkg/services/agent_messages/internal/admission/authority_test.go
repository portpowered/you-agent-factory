package admission

import (
	"context"
	"errors"
	"reflect"
	"testing"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// These fixtures control public read/validation collaborators. They do not
// construct Worker Sessions, persistence, Events, or an application graph.
type authorityReader struct {
	observations map[string]workersessions.Observation
	requests     []workersessions.GetObservationByWorkerSessionIDRequest
	err          error
}

func (r *authorityReader) read(_ context.Context, request workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	r.requests = append(r.requests, request)
	if r.err != nil {
		return workersessions.Observation{}, r.err
	}
	observation, ok := r.observations[request.WorkerSessionID]
	if !ok || (request.FactorySessionID != "" && observation.FactorySessionID != request.FactorySessionID) {
		return workersessions.Observation{}, errors.New("controlled absent owner")
	}
	return observation.Clone(), nil
}

func authorityObservation(worker, factory, work, requester string) workersessions.Observation {
	observation := workersessions.Observation{
		WorkerSessionID: worker, FactorySessionID: factory, State: workersessions.StateRunning,
		Labels: []string{"project:shared", "workstation:project-lead"},
	}
	if work != "" {
		observation.WorkIDs = []string{work}
	}
	if requester != "" {
		observation.Requester = &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: requester}
	}
	return observation
}

func TestAuthorizerRequiresExactActiveCaller(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"absent", "partial token", "partial worker", "forged", "foreign", "terminal", "owner lost", "restarted", "valid"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			caller := &workersessions.CallerIdentity{WorkerSessionID: "worker", Token: "execution-token"}
			reader := &authorityReader{observations: map[string]workersessions.Observation{
				"worker": authorityObservation("worker", "factory", "work", ""),
			}}
			caller, valid := configureCaller(cell, caller, reader)
			authorizer := NewAuthorizer(func(_ context.Context, received *workersessions.CallerIdentity) error {
				if !valid || received.WorkerSessionID != "worker" || received.Token != "execution-token" {
					return errors.New("private diagnostics execution-token")
				}
				return nil
			}, reader.read)
			identity, err := authorizer.Authenticate(context.Background(), caller)
			if cell != "valid" {
				if !errors.Is(err, agentmessages.ErrNotPermitted) || identity.Chain != "" {
					t.Fatalf("invalid authority returned identity=%v, err=%v", identity, err)
				}
				if cell != "terminal" && len(reader.requests) != 0 {
					t.Fatal("unauthenticated caller reached identity reads")
				}
				return
			}
			if err != nil || identity.Observation.WorkerSessionID != "worker" || identity.Chain == "" {
				t.Fatalf("valid authority: identity=%v, err=%v", identity, err)
			}
		})
	}
}

func configureCaller(cell string, caller *workersessions.CallerIdentity, reader *authorityReader) (*workersessions.CallerIdentity, bool) {
	switch cell {
	case "absent":
		return nil, false
	case "partial token":
		caller.Token = ""
	case "partial worker":
		caller.WorkerSessionID = ""
	case "forged", "foreign", "owner lost", "restarted":
		return caller, false
	case "terminal":
		observation := reader.observations["worker"]
		observation.State = workersessions.StateCompleted
		reader.observations["worker"] = observation
	}
	return caller, true
}

func TestAuthorizerRequesterPolicy(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name, sender, recipient string
		allowed                 bool
	}{
		{"own lead", "lane", "lead", true},
		{"own worker", "lead", "lane", true},
		{"ancestor", "child", "lead", true},
		{"descendant", "lead", "child", true},
		{"direct requester", "direct", "lane", true},
		{"project siblings", "lane", "sibling", false},
		{"foreign project", "lane", "foreign", false},
		{"unattributed root", "lane", "root", false},
		{"same session", "lane", "lane", false},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			reader := &authorityReader{observations: map[string]workersessions.Observation{
				"lead":         authorityObservation("lead", "factory", "project-work", ""),
				"lane":         authorityObservation("lane", "factory", "lane-work", "lead"),
				"sibling":      authorityObservation("sibling", "factory", "sibling-work", "lead"),
				"child":        authorityObservation("child", "factory", "child-work", "lane"),
				"direct":       authorityObservation("direct", "", "", "lane"),
				"foreign":      authorityObservation("foreign", "other", "foreign-work", "foreign-lead"),
				"foreign-lead": authorityObservation("foreign-lead", "other", "foreign-project-work", ""),
				"root":         authorityObservation("root", "factory", "root-work", ""),
			}}
			authorizer := NewAuthorizer(nil, reader.read)
			sender, err := authorizer.Recipient(context.Background(), cell.sender, "")
			if err != nil {
				t.Fatal(err)
			}
			recipient, err := authorizer.Recipient(context.Background(), cell.recipient, "")
			if err != nil {
				t.Fatal(err)
			}
			err = authorizer.PermitSend(context.Background(), sender, recipient)
			if cell.allowed && err != nil || !cell.allowed && !errors.Is(err, agentmessages.ErrNotPermitted) {
				t.Fatalf("PermitSend = %v, allowed=%v", err, cell.allowed)
			}
		})
	}
}

func TestAuthorizerScopedAddressAndAmbiguity(t *testing.T) {
	t.Parallel()
	reader := &authorityReader{observations: map[string]workersessions.Observation{
		"recipient": authorityObservation("recipient", "selected-factory", "work", ""),
	}}
	authorizer := NewAuthorizer(nil, reader.read)
	identity, err := authorizer.Recipient(context.Background(), "recipient", "selected-factory")
	if err != nil || identity.Observation.FactorySessionID != "selected-factory" ||
		reader.requests[0].FactorySessionID != "selected-factory" {
		t.Fatalf("selected scope was lost: identity=%v, err=%v, requests=%v", identity, err, reader.requests)
	}
	if _, err := authorizer.Recipient(context.Background(), "unknown", "selected-factory"); !errors.Is(err, agentmessages.ErrRecipientNotFound) {
		t.Fatalf("unknown recipient = %v", err)
	}
	work := "project-work"
	reader.err = &workersessions.AmbiguousAddressError{Candidates: []workersessions.AddressCandidate{
		{FactorySessionID: "b", WorkerSessionID: "legacy", WorkID: &work},
		{FactorySessionID: "a", WorkerSessionID: "legacy"},
	}}
	_, err = authorizer.Recipient(context.Background(), "legacy", "")
	var ambiguous *workersessions.AmbiguousAddressError
	if !errors.As(err, &ambiguous) || !errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) || len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguity lost candidates: %v", err)
	}
	if ambiguous.Candidates[0].FactorySessionID != "a" || ambiguous.Candidates[1].WorkID == &work {
		t.Fatal("ambiguity candidates were not ordered and detached")
	}
}

func TestAuthorizerRevalidatesWithoutRetainingCredentials(t *testing.T) {
	t.Parallel()
	valid := true
	caller := &workersessions.CallerIdentity{WorkerSessionID: "worker", Token: "caller-token"}
	reader := &authorityReader{observations: map[string]workersessions.Observation{
		"worker": authorityObservation("worker", "", "", ""),
	}}
	authorizer := NewAuthorizer(func(_ context.Context, received *workersessions.CallerIdentity) error {
		if !valid {
			return workersessions.ErrCallerInvalid
		}
		received.Token = "collaborator-mutated"
		return nil
	}, reader.read)
	if _, err := authorizer.Authenticate(context.Background(), caller); err != nil {
		t.Fatal(err)
	}
	if caller.Token != "caller-token" {
		t.Fatal("caller credential was not detached")
	}
	valid = false
	if err := authorizer.Revalidate(context.Background(), caller); !errors.Is(err, agentmessages.ErrNotPermitted) {
		t.Fatalf("owner loss retained authority: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := authorizer.Revalidate(ctx, caller); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestAuthorizerInboxUsesScopedWorkAndVerifiedContinuation(t *testing.T) {
	t.Parallel()
	old := authorityObservation("old", "factory", "work", "lead")
	old.State, old.SuccessorWorkerSessionID = workersessions.StateCompleted, "new"
	current := authorityObservation("new", "factory", "work", "lead")
	current.PredecessorWorkerSessionID = "old"
	redispatched := authorityObservation("redispatched", "factory", "work", "lead")
	foreign := authorityObservation("foreign", "another-factory", "work", "lead")
	reader := &authorityReader{observations: map[string]workersessions.Observation{
		"old": old, "new": current, "redispatched": redispatched, "foreign": foreign,
	}}
	authorizer := NewAuthorizer(nil, reader.read)
	identities := make(map[string]Identity)
	for worker := range reader.observations {
		identity, err := authorizer.Recipient(context.Background(), worker, "")
		if err != nil {
			t.Fatal(err)
		}
		identities[worker] = identity
	}
	original := identities["old"]
	if identities["new"].Chain != original.Chain || !identities["new"].Matches(original.Chain, original.Work) ||
		!identities["redispatched"].Matches(original.Chain, original.Work) || identities["foreign"].Matches(original.Chain, original.Work) {
		t.Fatal("inbox did not retain scoped continuation/own Work authority")
	}
	if (Identity{}).Matches(original.Chain, original.Work) {
		t.Fatal("absent identity granted inbox authority")
	}
	if scopedIdentity("a/b", "c") == scopedIdentity("a", "b/c") {
		t.Fatal("opaque ID tuple collision")
	}
}

func TestAuthorizerRejectsBrokenLineageAndRequesterCycles(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"missing predecessor", "wrong successor", "foreign predecessor", "active predecessor", "requester cycle"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			previous := authorityObservation("old", "factory", "", "")
			previous.State, previous.SuccessorWorkerSessionID = workersessions.StateCompleted, "new"
			current := authorityObservation("new", "factory", "", "")
			current.PredecessorWorkerSessionID = "old"
			reader := &authorityReader{observations: map[string]workersessions.Observation{
				"old": previous, "new": current,
				"target": authorityObservation("target", "factory", "", ""),
			}}
			switch cell {
			case "missing predecessor":
				delete(reader.observations, "old")
			case "wrong successor":
				previous.SuccessorWorkerSessionID = "someone-else"
				reader.observations["old"] = previous
			case "foreign predecessor":
				previous.FactorySessionID = "foreign"
				reader.observations["old"] = previous
			case "active predecessor":
				previous.State = workersessions.StateRunning
				reader.observations["old"] = previous
			case "requester cycle":
				current.PredecessorWorkerSessionID = ""
				current.Requester = &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "new"}
				reader.observations["new"] = current
			}
			authorizer := NewAuthorizer(nil, reader.read)
			identity, err := authorizer.Recipient(context.Background(), "new", "")
			if cell == "requester cycle" {
				target, targetErr := authorizer.Recipient(context.Background(), "target", "")
				if err != nil || targetErr != nil {
					t.Fatalf("controlled identities = %v, %v", err, targetErr)
				}
				err = authorizer.PermitSend(context.Background(), identity, target)
				if !errors.Is(err, agentmessages.ErrNotPermitted) {
					t.Fatalf("cycle granted authority: %v", err)
				}
			} else if !errors.Is(err, agentmessages.ErrRecipientNotFound) {
				t.Fatalf("broken lineage = %v", err)
			}
		})
	}
}

func TestAuthorizerDetachesIdentityFacts(t *testing.T) {
	t.Parallel()
	observation := authorityObservation("worker", "factory", "work", "lead")
	reader := &authorityReader{observations: map[string]workersessions.Observation{"worker": observation}}
	authorizer := NewAuthorizer(nil, reader.read)
	identity, err := authorizer.Recipient(context.Background(), "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	identity.Observation.Labels[0] = "changed"
	identity.Observation.WorkIDs[0] = "changed"
	identity.Observation.Requester.WorkerSessionID = "changed"
	if !reflect.DeepEqual(reader.observations["worker"], observation) {
		t.Fatal("returned identity mutated server-held facts")
	}
}

func TestAuthorizerProjectRequesterWorkRequiresEvidence(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name, recordedWork, selectedFactory string
		allowed                             bool
	}{
		{"verified own Project Work", "project-work", "factory", true},
		{"missing requester Work", "", "factory", false},
		{"different requester Work", "invented-work", "factory", false},
		{"foreign Work scope", "project-work", "foreign-factory", false},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			lane := authorityObservation("lane", "factory", "lane-work", "original-lead")
			lane.Requester.WorkID = cell.recordedWork
			reader := &authorityReader{observations: map[string]workersessions.Observation{
				"lane":          lane,
				"original-lead": authorityObservation("original-lead", "factory", "project-work", ""),
				"new-lead":      authorityObservation("new-lead", cell.selectedFactory, "project-work", ""),
			}}
			authorizer := NewAuthorizer(nil, reader.read)
			sender, err := authorizer.Recipient(context.Background(), "lane", "")
			if err != nil {
				t.Fatal(err)
			}
			recipient, err := authorizer.Recipient(context.Background(), "new-lead", "")
			if err != nil {
				t.Fatal(err)
			}
			err = authorizer.PermitSend(context.Background(), sender, recipient)
			if cell.allowed && err != nil || !cell.allowed && !errors.Is(err, agentmessages.ErrNotPermitted) {
				t.Fatalf("project ownership = %v, allowed=%v", err, cell.allowed)
			}
			if cell.allowed {
				if err := authorizer.PermitSend(context.Background(), recipient, sender); err != nil {
					t.Fatalf("lead's own worker = %v", err)
				}
			}
		})
	}
}

func TestAuthorizerChecksCompleteRequesterChain(t *testing.T) {
	t.Parallel()
	child := authorityObservation("child", "factory", "", "parent")
	parent := authorityObservation("parent", "factory", "", "child")
	reader := &authorityReader{observations: map[string]workersessions.Observation{"child": child, "parent": parent}}
	authorizer := NewAuthorizer(nil, reader.read)
	sender, err := authorizer.Recipient(context.Background(), "child", "")
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := authorizer.Recipient(context.Background(), "parent", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.PermitSend(context.Background(), sender, recipient); !errors.Is(err, agentmessages.ErrNotPermitted) {
		t.Fatalf("target found inside requester cycle granted authority: %v", err)
	}
}

func TestAuthorizerDirectWorkIdentityAndCancellation(t *testing.T) {
	t.Parallel()
	observation := authorityObservation("direct", "", "", "")
	observation.Direct = true
	observation.Correlation = &workersessions.Correlation{FactorySessionID: "factory", WorkID: "work"}
	if workIdentity(observation) != scopedIdentity("factory", "work") {
		t.Fatal("direct caller lost server-held own Work correlation")
	}
	observation.Direct = false
	if workIdentity(observation) != "" {
		t.Fatal("descriptive correlation granted hosted Work authority")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	authorizer := NewAuthorizer(nil, nil)
	if _, err := authorizer.Recipient(ctx, "worker", "factory"); !errors.Is(err, context.Canceled) {
		t.Fatalf("recipient cancellation = %v", err)
	}
}
