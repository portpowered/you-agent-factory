import type { FactoryDefinition } from "./contracts.js";
import type { FactoryVisualizationLayoutIssue } from "./visualization-layout-contracts.js";

type InputPath = readonly (string | number)[];

function entityIdentifier(
  explicitId: string | undefined,
  fallbackName: string,
): string {
  const normalizedId = explicitId?.trim();
  return normalizedId ? normalizedId : fallbackName;
}

function addSubjectNode(
  nodeIds: Set<string>,
  kind: "doc" | "resource" | "worker" | "work-type" | "workstation",
  explicitId: string | undefined,
  fallbackName: string,
): void {
  nodeIds.add(`${kind}:${entityIdentifier(explicitId, fallbackName)}`);
}

/**
 * Project the canonical node IDs that the existing Factory graph projection
 * derives from authored entities and references. This is data-only and does
 * not read or update graph-editor state.
 */
export function factoryVisualizationCanonicalNodeIds(
  factory: Readonly<FactoryDefinition>,
): ReadonlySet<string> {
  const nodeIds = new Set<string>();
  const resourceIds = new Map<string, string>();
  const workerIds = new Map<string, string>();
  const workTypeIds = new Map<string, string>();
  const stateIds = new Map<string, ReadonlyMap<string, string>>();

  for (const file of factory.supportingFiles?.bundledFiles ?? []) {
    const targetPath = file.targetPath?.trim();
    if (targetPath) addSubjectNode(nodeIds, "doc", targetPath, targetPath);
  }

  for (const resource of factory.resources ?? []) {
    if (resource.id?.trim()) resourceIds.set(resource.name, resource.id);
    addSubjectNode(nodeIds, "resource", resource.id, resource.name);
  }

  for (const worker of factory.workers ?? []) {
    if (worker.id?.trim()) workerIds.set(worker.name, worker.id);
    addSubjectNode(nodeIds, "worker", worker.id, worker.name);
    for (const resource of worker.resources ?? []) {
      addSubjectNode(
        nodeIds,
        "resource",
        resourceIds.get(resource.name),
        resource.name,
      );
    }
  }

  for (const workType of factory.workTypes ?? []) {
    if (workType.id?.trim()) workTypeIds.set(workType.name, workType.id);
    addSubjectNode(nodeIds, "work-type", workType.id, workType.name);
    const workTypeStateIds = new Map<string, string>();
    for (const state of workType.states) {
      if (state.id?.trim()) workTypeStateIds.set(state.name, state.id);
      nodeIds.add(
        `work-state:${entityIdentifier(workType.id, workType.name)}:${entityIdentifier(state.id, state.name)}`,
      );
    }
    stateIds.set(workType.name, workTypeStateIds);
  }

  const addWorkState = (workTypeName: string, stateName: string): void => {
    nodeIds.add(
      `work-state:${entityIdentifier(workTypeIds.get(workTypeName), workTypeName)}:${entityIdentifier(stateIds.get(workTypeName)?.get(stateName), stateName)}`,
    );
  };

  for (const workstation of factory.workstations ?? []) {
    addSubjectNode(nodeIds, "workstation", workstation.id, workstation.name);
    const workerName = workstation.worker.trim();
    if (workerName) {
      addSubjectNode(nodeIds, "worker", workerIds.get(workerName), workerName);
    }
    for (const resource of workstation.resources ?? []) {
      addSubjectNode(
        nodeIds,
        "resource",
        resourceIds.get(resource.name),
        resource.name,
      );
    }
    for (const route of [
      ...(workstation.inputs ?? []),
      ...(workstation.outputs ?? []),
      ...(workstation.onContinue ?? []),
      ...(workstation.onFailure ?? []),
      ...(workstation.onRejection ?? []),
    ]) {
      addWorkState(route.workType, route.state);
    }
  }

  return nodeIds;
}

export function validateCanonicalNodeId(
  nodeId: string,
  path: InputPath,
  canonicalNodeIds: ReadonlySet<string>,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  if (nodeId.trim().length === 0) {
    issues.push({
      category: "semantic",
      code: "empty_node_id",
      path,
      message: "Expected a non-blank canonical topology node ID.",
    });
  } else if (!canonicalNodeIds.has(nodeId)) {
    issues.push({
      category: "semantic",
      code: "unknown_canonical_node_id",
      path,
      message: `Canonical topology node ${nodeId} does not exist in the supplied Factory.`,
    });
  }
}
