package modelhost

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

func defaultLlamaCppServerStartBuilder(
	identity Identity,
	inspection CacheInspection,
	worker *interfaces.WorkerConfig,
) (ProcessStartSpec, error) {
	if worker == nil {
		return ProcessStartSpec{}, fmt.Errorf("local model worker is required for supervised backend %q", identity.Backend)
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		command = localmodels.DefaultOmniVoiceCommand
	}
	healthEndpoint, args, err := supervisedHealthEndpointAndArgs(worker.Args)
	if err != nil {
		return ProcessStartSpec{}, err
	}
	if strings.TrimSpace(inspection.CachePath) == "" {
		return ProcessStartSpec{}, fmt.Errorf("%w: cache path is required for supervised runtime %q", ErrMissingAssets, identity.Name)
	}
	args = append([]string{"serve"}, args...)
	args = append(args, "--cache-path", inspection.CachePath)
	return ProcessStartSpec{
		Command:        command,
		Args:           args,
		HealthEndpoint: healthEndpoint,
	}, nil
}

func supervisedHealthEndpointAndArgs(workerArgs []string) (string, []string, error) {
	args := append([]string(nil), workerArgs...)
	for i := 0; i < len(args); i++ {
		if args[i] != supervisedHealthEndpointFlag {
			continue
		}
		if i+1 >= len(args) {
			return "", nil, fmt.Errorf("flag %q requires a value", supervisedHealthEndpointFlag)
		}
		endpoint := strings.TrimSpace(args[i+1])
		remaining := append(append([]string(nil), args[:i]...), args[i+2:]...)
		if endpoint == "" {
			return "", nil, fmt.Errorf("flag %q requires a non-empty value", supervisedHealthEndpointFlag)
		}
		return endpoint, remaining, nil
	}
	return "", args, fmt.Errorf("supervised llama.cpp runtime requires worker arg %q", supervisedHealthEndpointFlag)
}
