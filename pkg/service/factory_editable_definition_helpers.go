package service

import factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"

func workTypeNames(workTypes *[]factoryapi.WorkType) []string {
	if workTypes == nil {
		return nil
	}
	names := make([]string, 0, len(*workTypes))
	for _, workType := range *workTypes {
		names = append(names, workType.Name)
	}
	return names
}

func workerNames(workers *[]factoryapi.Worker) []string {
	if workers == nil {
		return nil
	}
	names := make([]string, 0, len(*workers))
	for _, worker := range *workers {
		names = append(names, worker.Name)
	}
	return names
}

func resourceNames(resources *[]factoryapi.Resource) []string {
	if resources == nil {
		return nil
	}
	names := make([]string, 0, len(*resources))
	for _, resource := range *resources {
		names = append(names, resource.Name)
	}
	return names
}

func workstationNames(workstations *[]factoryapi.Workstation) []string {
	if workstations == nil {
		return nil
	}
	names := make([]string, 0, len(*workstations))
	for _, workstation := range *workstations {
		names = append(names, workstation.Name)
	}
	return names
}

func workStateSet(workTypes *[]factoryapi.WorkType) map[string]bool {
	states := make(map[string]bool)
	if workTypes == nil {
		return states
	}
	for _, workType := range *workTypes {
		for _, state := range workType.States {
			states[workType.Name+":"+state.Name] = true
		}
	}
	return states
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func editableFactoryErrorTarget(kind, id, field string) factoryapi.ErrorTarget {
	target := factoryapi.ErrorTarget{Kind: kind}
	if id != "" {
		target.Id = &id
	}
	if field != "" {
		target.Field = &field
	}
	return target
}
