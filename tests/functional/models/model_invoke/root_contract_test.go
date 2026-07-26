package model_invoke_test

import (
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestModelsRootScopeCapabilityRequestsValidateForFactoryPeer(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:scope-1")
	if err != nil {
		t.Fatalf("Parse RuntimeScopeRef: %v", err)
	}
	lease, err := (models.ModelLeaseRef{}).Parse("models:lease-1")
	if err != nil {
		t.Fatalf("Parse ModelLeaseRef: %v", err)
	}
	invocation, err := (models.ModelInvocationRef{}).Parse("models:invocation-1")
	if err != nil {
		t.Fatalf("Parse ModelInvocationRef: %v", err)
	}
	requests := []struct {
		name     string
		validate func() error
	}{
		{"get catalog", func() error {
			return (models.GetModelRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"readiness", func() error {
			return (models.GetModelReadinessRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"prepare assets", func() error {
			return (models.PrepareModelAssetsRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"inspect assets", func() error {
			return (models.InspectModelAssetsRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"remove assets", func() error {
			return (models.RemoveModelAssetsRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"ensure host", func() error {
			return (models.EnsureModelHostRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"inspect host", func() error {
			return (models.InspectModelHostRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"stop host", func() error {
			return (models.StopModelHostRequest{Scope: scope, Name: "local-model"}).Validate()
		}},
		{"acquire lease", func() error {
			return (models.AcquireModelLeaseRequest{
				Scope: scope, Name: "local-model", Holder: "factory-peer",
			}).Validate()
		}},
		{"get lease", func() error {
			return (models.GetModelLeaseRequest{Scope: scope, Lease: lease}).Validate()
		}},
		{"release lease", func() error {
			return (models.ReleaseModelLeaseRequest{Scope: scope, Lease: lease}).Validate()
		}},
		{"invoke", func() error {
			return (models.InvokeModelRequest{
				Scope: scope, Lease: lease, ModelName: "local-model",
				Operation: "generate", Holder: "factory-peer",
				Input: models.InferenceInput{ContentType: "text/plain", Content: "hello"},
			}).Validate()
		}},
		{"cancel", func() error {
			return (models.CancelInvocationRequest{Scope: scope, Invocation: invocation}).Validate()
		}},
	}
	for _, request := range requests {
		if err := request.validate(); err != nil {
			t.Fatalf("%s request: %v", request.name, err)
		}
	}
}

func TestModelsRootDetachedValuesDoNotSharePeerMutation(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, time.July, 26, 6, 0, 0, 0, time.UTC)
	leaseRef, err := (models.ModelLeaseRef{}).Parse("models:lease-1")
	if err != nil {
		t.Fatalf("Parse ModelLeaseRef: %v", err)
	}
	lease := models.ModelLease{
		Lease: leaseRef, ModelName: "local-model", Holder: "factory-peer",
		ExpiresAt: expires, Status: models.ModelLeaseStatusActive,
	}
	result := models.InvokeModelResult{
		Content: []models.InferenceContent{{ContentType: "text/plain", Content: "detached"}},
		Artifacts: []models.InferenceArtifact{{
			Name: "result.txt", Properties: map[string]string{"digest": "sha256:original"},
		}},
	}
	cloned := result.Clone()
	cloned.Content[0].Content = "mutated"
	cloned.Artifacts[0].Properties["digest"] = "sha256:mutated"
	if result.Content[0].Content != "detached" ||
		result.Artifacts[0].Properties["digest"] != "sha256:original" {
		t.Fatalf("InvokeModelResult retained clone mutation: %#v", result)
	}
	if lease.ExpiresAt != expires || lease.Lease != leaseRef {
		t.Fatalf("ModelLease lost detached identity facts: %#v", lease)
	}
}
