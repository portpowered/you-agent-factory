package functional_test

import "github.com/portpowered/infinite-you/pkg/interfaces"

// persistTestPipelineConfig returns a simple 2-stage pipeline config.
func persistTestPipelineConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "stage1", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "step-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "step1",
				WorkerTypeName: "step-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}},
			},
			{
				Name:           "finish",
				WorkerTypeName: "step-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			},
		},
	}
}
