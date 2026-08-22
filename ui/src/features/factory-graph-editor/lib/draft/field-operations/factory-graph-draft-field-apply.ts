import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeFieldUpdate,
} from "../factory-graph-draft-types";

export function applyFactoryGraphNodeFieldChanges(
  factoryDefinition: CanonicalFactoryDefinition,
  changes: readonly FactoryGraphNodeFieldUpdate[],
): void {
  for (const change of changes) {
    switch (change.kind) {
      case "resource": {
        const resource = factoryDefinition.resources?.find(
          (entry) => entry.name === change.name,
        );
        if (resource) {
          resource.capacity = change.value;
        }
        break;
      }
      case "worker": {
        const worker = factoryDefinition.workers?.find(
          (entry) => entry.name === change.name,
        );
        if (worker) {
          worker.model = change.value;
        }
        break;
      }
      case "work-state": {
        const state = factoryDefinition.workTypes
          ?.find((entry) => entry.name === change.workTypeName)
          ?.states.find((entry) => entry.name === change.stateName);
        if (state) {
          state.type = change.value;
        }
        break;
      }
      case "workstation": {
        const workstation = factoryDefinition.workstations?.find(
          (entry) => entry.name === change.name,
        );
        if (!workstation) {
          break;
        }
        if (change.field === "behavior") {
          workstation.behavior = change.value;
        } else if (change.field === "body") {
          workstation.body = change.value;
        } else {
          workstation.worker = change.value;
        }
        break;
      }
    }
  }
}
