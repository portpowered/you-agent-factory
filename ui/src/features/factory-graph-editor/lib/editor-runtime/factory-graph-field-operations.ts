import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeFieldUpdate,
  FactoryResource,
  FactoryWorker,
  FactoryWorkType,
} from "../draft/factory-graph-draft-types";
import { nodeKeyId } from "../draft/factory-graph-draft-types";
import type { FactoryGraphOperationResult } from "../operations/factory-graph-operations";

export type { FactoryGraphNodeFieldUpdate } from "../draft/factory-graph-draft-types";

export function updateFactoryGraphNodeField(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  update: FactoryGraphNodeFieldUpdate;
}): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  const nextFactoryDefinition = structuredClone(options.baseFactoryDefinition);
  const update = options.update;

  switch (update.kind) {
    case "resource":
      return updateResourceField(
        nextFactoryDefinition,
        update.name,
        update.value,
      );
    case "worker":
      return updateWorkerField(
        nextFactoryDefinition,
        update.name,
        update.value,
      );
    case "work-state":
      return updateWorkState(nextFactoryDefinition, update);
    case "workstation":
      return updateWorkstationField(nextFactoryDefinition, update);
  }
}

function updateResourceField(
  factoryDefinition: CanonicalFactoryDefinition,
  name: string,
  capacity: FactoryResource["capacity"],
): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  const resource = factoryDefinition.resources?.find(
    (entry) => entry.name === name,
  );
  const missing = missingNodeResult(name);

  if (!resource) {
    return missing;
  }

  resource.capacity = capacity;
  return {
    ok: true,
    value: factoryDefinition,
  };
}

function updateWorkerField(
  factoryDefinition: CanonicalFactoryDefinition,
  name: string,
  model: FactoryWorker["model"],
): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  const worker = factoryDefinition.workers?.find(
    (entry) => entry.name === name,
  );
  if (!worker) {
    return missingNodeResult(name);
  }

  worker.model = model;
  return {
    ok: true,
    value: factoryDefinition,
  };
}

function updateWorkstationField(
  factoryDefinition: CanonicalFactoryDefinition,
  update: Extract<FactoryGraphNodeFieldUpdate, { kind: "workstation" }>,
): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  const workstation = factoryDefinition.workstations?.find(
    (entry) => entry.name === update.name,
  );
  if (!workstation) {
    return missingNodeResult(update.name);
  }

  if (update.field === "behavior") {
    workstation.behavior = update.value;
  } else if (update.field === "body") {
    workstation.body = update.value;
  } else {
    workstation.worker = update.value;
  }

  return {
    ok: true,
    value: factoryDefinition,
  };
}

function updateWorkState(
  factoryDefinition: CanonicalFactoryDefinition,
  update: Extract<FactoryGraphNodeFieldUpdate, { kind: "work-state" }>,
): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  const workType = factoryDefinition.workTypes?.find(
    (entry: FactoryWorkType) => entry.name === update.workTypeName,
  );
  const workState = workType?.states.find(
    (entry) => entry.name === update.stateName,
  );

  if (!workState) {
    return {
      message: `Graph node "${nodeKeyId({
        kind: "work-state",
        stateName: update.stateName,
        workTypeName: update.workTypeName,
      })}" was not found.`,
      ok: false,
      reason: "NODE_NOT_FOUND",
    };
  }

  workState.type = update.value;
  return {
    ok: true,
    value: factoryDefinition,
  };
}

function missingNodeResult(
  nodeId: string,
): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  return {
    message: `Graph node "${nodeId}" was not found.`,
    ok: false,
    reason: "NODE_NOT_FOUND",
  };
}
