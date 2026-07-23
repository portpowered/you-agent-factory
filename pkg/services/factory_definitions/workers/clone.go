package workerconfig

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts/namevalue"
	factoryresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/resource"
)

// Clone returns a detached copy of a Worker definition.
func Clone(def Config) Config {
	def.Description = cloneDescription(def.Description)
	def.Args = append([]string(nil), def.Args...)
	def.Resources = append([]factoryresource.Config(nil), def.Resources...)
	def.Operations = cloneModelOperations(def.Operations)
	def.Auth = cloneHostedWorkerAuth(def.Auth)
	def.Linear = cloneHostedLinearWorker(def.Linear)
	if def.AgentTools != nil {
		agentTools := *def.AgentTools
		def.AgentTools = &agentTools
	}
	return def
}

func cloneDescription(description *namevalue.Config) *namevalue.Config {
	if description == nil {
		return nil
	}
	clone := *description
	clone.Locales = append([]string(nil), description.Locales...)
	if description.Values != nil {
		clone.Values = make(map[string]string, len(description.Values))
		for locale, value := range description.Values {
			clone.Values[locale] = value
		}
	}
	return &clone
}

func cloneModelOperations(operations []ModelOperation) []ModelOperation {
	if len(operations) == 0 {
		return nil
	}
	cloned := make([]ModelOperation, len(operations))
	for i, operation := range operations {
		cloned[i] = ModelOperation{
			Name:    operation.Name,
			Inputs:  cloneModelOperationSlots(operation.Inputs),
			Outputs: cloneModelOperationSlots(operation.Outputs),
		}
	}
	return cloned
}

func cloneModelOperationSlots(slots []ModelOperationSlot) []ModelOperationSlot {
	if len(slots) == 0 {
		return nil
	}
	cloned := make([]ModelOperationSlot, len(slots))
	for i, slot := range slots {
		cloned[i] = ModelOperationSlot{
			Name:         slot.Name,
			ContentTypes: append([]string(nil), slot.ContentTypes...),
			Required:     slot.Required,
		}
	}
	return cloned
}

func cloneHostedWorkerAuth(cfg *HostedWorkerAuthConfig) *HostedWorkerAuthConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func cloneHostedLinearWorker(cfg *HostedLinearWorkerConfig) *HostedLinearWorkerConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.TeamIDs = append([]string(nil), cfg.TeamIDs...)
	cloned.StateIDs = append([]string(nil), cfg.StateIDs...)
	if cfg.Claim != nil {
		claim := *cfg.Claim
		cloned.Claim = &claim
	}
	return &cloned
}
