package service

import "github.com/portpowered/infinite-you/pkg/services/workers"

func cloneOpeningOptions(
	opening workers.RuntimeBuildOpeningOptions,
) workers.RuntimeBuildOpeningOptions {
	clone := opening
	if opening.InvocationSkipPermissionsOverride != nil {
		value := *opening.InvocationSkipPermissionsOverride
		clone.InvocationSkipPermissionsOverride = &value
	}
	clone.MockWorkers = cloneMockWorkersConfig(opening.MockWorkers)
	return clone
}

func cloneMockWorkersConfig(config *workers.MockWorkersConfig) *workers.MockWorkersConfig {
	if config == nil {
		return nil
	}
	clone := *config
	clone.MockWorkers = make([]workers.MockWorkerConfig, len(config.MockWorkers))
	for index, worker := range config.MockWorkers {
		clone.MockWorkers[index] = worker
		clone.MockWorkers[index].WorkInputs = append(
			[]workers.MockWorkInputSelector(nil),
			worker.WorkInputs...,
		)
		if worker.ScriptConfig != nil {
			script := *worker.ScriptConfig
			script.Args = append([]string(nil), worker.ScriptConfig.Args...)
			if worker.ScriptConfig.Env != nil {
				script.Env = make(map[string]string, len(worker.ScriptConfig.Env))
				for key, value := range worker.ScriptConfig.Env {
					script.Env[key] = value
				}
			}
			clone.MockWorkers[index].ScriptConfig = &script
		}
		if worker.RejectConfig != nil {
			rejection := *worker.RejectConfig
			if worker.RejectConfig.ExitCode != nil {
				exitCode := *worker.RejectConfig.ExitCode
				rejection.ExitCode = &exitCode
			}
			clone.MockWorkers[index].RejectConfig = &rejection
		}
	}
	return &clone
}
