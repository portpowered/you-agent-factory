package service

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/internal/reclamation"
)

func TestRemoveModelAssetsCompletedPolicyControlsRemovalOutcome(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name                             string
		measureErr, removeErr, verifyErr error
	}{
		{name: "removed"},
		{name: "measurement failed", measureErr: models.ErrAssetCancelled},
		{name: "removal failed", removeErr: models.ErrModelCacheRemovalFailed},
		{name: "verification failed", verifyErr: models.ErrModelCacheRemovalFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			policy := &completedRemovalPolicy{measureErr: test.measureErr, removeErr: test.removeErr, verifyErr: test.verifyErr}
			// The operation consumes its completed policy; no scopes, child
			// service or real filesystem is needed to prove delegation.
			assets := &service{reclamation: policy}
			target := genericRemovalTarget{modelName: "selected", modelRoot: "cache/selected", revision: "revision", revisionPath: "cache/selected/revision"}
			result, err := assets.removeManagedModelTarget(t.Context(), models.RemoveModelAssetsRequest{Name: "selected"}, models.RemoveModelAssetsResult{}, target, nil)
			wantErr := errors.Join(test.measureErr, test.removeErr, test.verifyErr)
			if wantErr != nil {
				if err == nil || result.BytesRemoved != 0 || result.Outcome == models.AssetRemovalRemoved {
					t.Fatalf("failed policy returned success: %#v, %v", result, err)
				}
				for _, cause := range []error{test.measureErr, test.removeErr, test.verifyErr} {
					if cause != nil && !errors.Is(err, cause) {
						t.Fatalf("lost cause %v: %v", cause, err)
					}
				}
				return
			}
			if err != nil || result.BytesRemoved != 17 || result.Outcome != models.AssetRemovalRemoved || result.ModelName != target.modelName || result.Revision != target.revision {
				t.Fatalf("policy result = %#v, %v", result, err)
			}
			if policy.measured != target.revisionPath || policy.removed != target.revisionPath || policy.verifiedParent != target.modelRoot || policy.verifiedChild != target.revision {
				t.Fatalf("wrong selected policy inputs: %#v", policy)
			}
		})
	}
}

type completedRemovalPolicy struct {
	reclamation.Policy
	measureErr, removeErr, verifyErr                 error
	measured, removed, verifiedParent, verifiedChild string
}

func (p *completedRemovalPolicy) MeasureRevisionBytes(_ context.Context, path string) (int64, error) {
	p.measured = path
	return 17, p.measureErr
}
func (p *completedRemovalPolicy) RemoveManagedTree(_ context.Context, path string) error {
	p.removed = path
	return p.removeErr
}
func (p *completedRemovalPolicy) VerifyManagedPathRemoved(_ context.Context, parent, child string) error {
	p.verifiedParent, p.verifiedChild = parent, child
	return p.verifyErr
}
