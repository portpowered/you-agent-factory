import type { FactoryValidationTarget } from "../../../api/factory-validation";

export type FactoryValidationSubjectLocation =
  FactoryValidationTarget["subject"]["location"];

export interface FactoryValidationHandleError {
  code: string;
  message: string;
}

export interface FactoryValidationGraphProjection {
  handleErrorsByNodeId: ReadonlyMap<string, ReadonlyMap<string, FactoryValidationHandleError>>;
  workstationMessagesByNodeId: ReadonlyMap<string, readonly FactoryValidationTarget[]>;
}

const WORKSTATION_GRAPH_NODE_PREFIX = "workstation:";

export function factoryGraphNodeIdForWorkstation(subjectId: string): string {
  return `${WORKSTATION_GRAPH_NODE_PREFIX}${subjectId}`;
}

export function workstationHandleIdForValidationLocation(
  location: FactoryValidationSubjectLocation,
): string | null {
  switch (location) {
    case "ON_REJECTION":
      return "workstation-on-rejection-source";
    case "ON_FAILURE":
      return "workstation-on-failure-source";
    case "OUTPUTS":
      return "workstation-output-source";
    default:
      return null;
  }
}

export function projectFactoryValidationTargets(
  targets: readonly FactoryValidationTarget[],
): FactoryValidationGraphProjection {
  const handleErrorsByNodeId = new Map<
    string,
    Map<string, FactoryValidationHandleError>
  >();
  const workstationMessagesByNodeId = new Map<
    string,
    FactoryValidationTarget[]
  >();

  for (const target of targets) {
    if (target.subject.type !== "WORKSTATION") {
      continue;
    }

    const nodeId = factoryGraphNodeIdForWorkstation(target.subject.id);
    const workstationMessages =
      workstationMessagesByNodeId.get(nodeId) ?? [];
    workstationMessages.push(target);
    workstationMessagesByNodeId.set(nodeId, workstationMessages);

    const handleId = workstationHandleIdForValidationLocation(
      target.subject.location,
    );
    if (!handleId) {
      continue;
    }

    const handleErrors =
      handleErrorsByNodeId.get(nodeId) ?? new Map<string, FactoryValidationHandleError>();
    handleErrors.set(handleId, {
      code: target.code,
      message: target.message,
    });
    handleErrorsByNodeId.set(nodeId, handleErrors);
  }

  return {
    handleErrorsByNodeId,
    workstationMessagesByNodeId,
  };
}

export function validationHandleErrorsForNode(
  projection: FactoryValidationGraphProjection,
  nodeId: string,
): ReadonlyMap<string, FactoryValidationHandleError> | undefined {
  return projection.handleErrorsByNodeId.get(nodeId);
}
