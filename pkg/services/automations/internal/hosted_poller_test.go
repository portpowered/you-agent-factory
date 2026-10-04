package internal_test

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"testing"
	"time"
)

func hostedLinearPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
}

func hostedLinearPollerWorker() *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
}

func waitForPollerSubmission(t *testing.T, submitted *recordingSubmitter, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls, _ := submitted.snapshot()
		if calls >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls, _ := submitted.snapshot()
	t.Fatalf("timed out waiting for %d poller submission(s); got %d", want, calls)
}
