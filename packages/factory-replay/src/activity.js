/** @param {string | undefined} explicitID @param {string} name */
function entityID(explicitID, name) {
  return explicitID?.trim() || name;
}

/** @param {string} id */
function workstationNodeID(id) {
  return `workstation:${id}`;
}

/** @param {string} id */
function resourceNodeID(id) {
  return `resource:${id}`;
}

/**
 * Project active Dispatch and resource occupancy evidence from one selected
 * canonical replay state without mutating caller-owned data.
 *
 * @param {import("./index.d.ts").FactoryActivityProjectionInput} input
 * @returns {import("./index.d.ts").FactoryActivityProjection}
 */
export function projectFactoryActivity(input) {
  const issues = new Map();
  const { resourcesByName, workstationsByTransition } = indexFactory(
    input.factory,
  );

  const occupiedByResource = new Map();
  const occupancyEvidenceAvailable = input.activeDispatches.every(
    (dispatch) => dispatch.resourceNames !== undefined,
  );
  const activeDispatches = input.activeDispatches.map((evidence) => {
    const resolved = workstationsByTransition.get(evidence.transitionId);
    const resourceIds = [];
    for (const resourceName of evidence.resourceNames ?? []) {
      const resource = resourcesByName.get(resourceName);
      if (!resource) {
        addIssue(
          issues,
          "UNRESOLVED_RESOURCE",
          `dispatch:${evidence.id}:resource:${resourceName}`,
          `Dispatch ${evidence.id} refers to unknown resource ${resourceName}.`,
          evidence.id,
          resourceName,
        );
        continue;
      }
      resourceIds.push(resource.id);
      occupiedByResource.set(
        resource.id,
        (occupiedByResource.get(resource.id) ?? 0) + 1,
      );
    }
    if (!resolved) {
      addIssue(
        issues,
        "UNRESOLVED_WORKSTATION",
        `dispatch:${evidence.id}:workstation:${evidence.transitionId}`,
        `Dispatch ${evidence.id} refers to unknown workstation transition ${evidence.transitionId}.`,
        evidence.id,
        evidence.transitionId,
      );
    }
    const workerName = resolved?.workstation.worker?.trim();
    const worker = workerName
      ? (input.factory?.workers ?? []).find(
          (candidate) =>
            candidate.name === workerName || candidate.id === workerName,
        )
      : undefined;
    if (workerName && !worker) {
      addIssue(
        issues,
        "UNRESOLVED_WORKER",
        `dispatch:${evidence.id}:worker:${workerName}`,
        `Dispatch ${evidence.id} refers to unknown worker ${workerName}.`,
        evidence.id,
        workerName,
      );
    }
    return {
      id: evidence.id,
      resourceIds: [...resourceIds].sort(),
      startedTick: evidence.startedTick,
      transitionId: evidence.transitionId,
      workIds: [...new Set(evidence.workIds)].sort(),
      ...(resolved
        ? {
            workstationId: resolved.id,
            workstationNodeId: workstationNodeID(resolved.id),
          }
        : {}),
      ...(worker
        ? { workerId: entityID(worker.id, worker.name) }
        : {}),
    };
  });

  const resourceOccupancy = projectResourceOccupancy(
    resourcesByName,
    occupiedByResource,
    occupancyEvidenceAvailable,
    issues,
  );

  return {
    activeDispatches: activeDispatches.sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    activeWorkstationIds: [
      ...new Set(
        activeDispatches.flatMap((dispatch) =>
          dispatch.workstationId ? [dispatch.workstationId] : [],
        ),
      ),
    ].sort(),
    issues: [...issues.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    resourceOccupancy: resourceOccupancy.sort((left, right) =>
      left.resourceId.localeCompare(right.resourceId),
    ),
    selectedTick: input.selectedTick,
  };
}

/** @param {import("@you-agent-factory/client").FactoryDefinition | undefined} factory */
function indexFactory(factory) {
  const resourcesByName = new Map();
  const workstationsByTransition = new Map();
  for (const resource of factory?.resources ?? []) {
    resourcesByName.set(resource.name, {
      capacity: Math.max(0, resource.capacity),
      id: entityID(resource.id, resource.name),
    });
  }
  for (const workstation of factory?.workstations ?? []) {
    const id = entityID(workstation.id, workstation.name);
    workstationsByTransition.set(workstation.name, { id, workstation });
    if (workstation.id?.trim()) {
      workstationsByTransition.set(workstation.id, { id, workstation });
    }
  }
  return { resourcesByName, workstationsByTransition };
}

/**
 * @param {Map<string, {capacity: number, id: string}>} resourcesByName
 * @param {Map<string, number>} occupiedByResource
 * @param {boolean} evidenceAvailable
 * @param {Map<string, import("./index.d.ts").FactoryActivityProjectionIssue>} issues
 * @returns {import("./index.d.ts").FactoryResourceOccupancyProjection[]}
 */
function projectResourceOccupancy(
  resourcesByName,
  occupiedByResource,
  evidenceAvailable,
  issues,
) {
  return [...resourcesByName.values()].map((resource) => {
    if (!evidenceAvailable) {
      return {
        capacity: resource.capacity,
        evidence: "unavailable",
        resourceId: resource.id,
        resourceNodeId: resourceNodeID(resource.id),
      };
    }
    const occupiedQuantity = occupiedByResource.get(resource.id) ?? 0;
    if (occupiedQuantity > resource.capacity) {
      addIssue(
        issues,
        "RESOURCE_CAPACITY_EXCEEDED",
        `resource:${resource.id}:capacity-exceeded`,
        `Resource ${resource.id} has ${occupiedQuantity} occupied units for capacity ${resource.capacity}.`,
        undefined,
        resource.id,
      );
    }
    return {
      availableQuantity: Math.max(0, resource.capacity - occupiedQuantity),
      capacity: resource.capacity,
      evidence: "known",
      occupiedQuantity,
      resourceId: resource.id,
      resourceNodeId: resourceNodeID(resource.id),
    };
  });
}

/**
 * @param {Map<string, import("./index.d.ts").FactoryActivityProjectionIssue>} issues
 * @param {import("./index.d.ts").FactoryActivityProjectionIssue["code"]} code
 * @param {string} id
 * @param {string} message
 * @param {string | undefined} dispatchId
 * @param {string | undefined} reference
 */
function addIssue(issues, code, id, message, dispatchId, reference) {
  issues.set(id, {
    code,
    id,
    message,
    ...(dispatchId ? { dispatchId } : {}),
    ...(reference ? { reference } : {}),
  });
}
