import type { FactoryValidationTarget } from "../../../api/factory-validation";

export type FactoryValidationSubjectLocation =
  FactoryValidationTarget["subject"]["location"];

export interface FactoryValidationHandleError {
  code: string;
  message: string;
}

export interface FactoryValidationNodeError {
  code: string;
  message: string;
}

export interface FactoryValidationGraphProjection {
  handleErrorsByNodeId: ReadonlyMap<
    string,
    ReadonlyMap<string, FactoryValidationHandleError>
  >;
  nodeErrorsByNodeId: ReadonlyMap<string, FactoryValidationNodeError>;
  workstationMessagesByNodeId: ReadonlyMap<
    string,
    readonly FactoryValidationTarget[]
  >;
  workTypeMessagesByNodeId: ReadonlyMap<
    string,
    readonly FactoryValidationTarget[]
  >;
  workStateMessagesByNodeId: ReadonlyMap<
    string,
    readonly FactoryValidationTarget[]
  >;
}

const WORKSTATION_GRAPH_NODE_PREFIX = "workstation:";
const WORK_TYPE_GRAPH_NODE_PREFIX = "work-type:";
const WORK_STATE_GRAPH_NODE_PREFIX = "work-state:";

export function factoryGraphNodeIdForWorkstation(subjectId: string): string {
  return `${WORKSTATION_GRAPH_NODE_PREFIX}${subjectId}`;
}

export function factoryGraphNodeIdForWorkType(subjectId: string): string {
  return `${WORK_TYPE_GRAPH_NODE_PREFIX}${subjectId}`;
}

export function factoryGraphNodeIdForWorkState(subjectId: string): string {
  const separatorIndex = subjectId.indexOf(":");
  if (separatorIndex <= 0 || separatorIndex >= subjectId.length - 1) {
    return `${WORK_STATE_GRAPH_NODE_PREFIX}${subjectId}`;
  }

  const workTypeName = subjectId.slice(0, separatorIndex);
  const stateName = subjectId.slice(separatorIndex + 1);
  return `${WORK_STATE_GRAPH_NODE_PREFIX}${workTypeName}:${stateName}`;
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
  const nodeErrorsByNodeId = new Map<string, FactoryValidationNodeError>();
  const workstationMessagesByNodeId = new Map<
    string,
    FactoryValidationTarget[]
  >();
  const workTypeMessagesByNodeId = new Map<string, FactoryValidationTarget[]>();
  const workStateMessagesByNodeId = new Map<
    string,
    FactoryValidationTarget[]
  >();

  for (const target of targets) {
    switch (target.subject.type) {
      case "WORKSTATION": {
        const nodeId = factoryGraphNodeIdForWorkstation(target.subject.id);
        const workstationMessages =
          workstationMessagesByNodeId.get(nodeId) ?? [];
        workstationMessages.push(target);
        workstationMessagesByNodeId.set(nodeId, workstationMessages);

        const handleId = workstationHandleIdForValidationLocation(
          target.subject.location,
        );
        if (!handleId) {
          break;
        }

        const handleErrors =
          handleErrorsByNodeId.get(nodeId) ??
          new Map<string, FactoryValidationHandleError>();
        handleErrors.set(handleId, {
          code: target.code,
          message: target.message,
        });
        handleErrorsByNodeId.set(nodeId, handleErrors);
        break;
      }
      case "WORK_TYPE": {
        if (target.subject.location !== "STATES") {
          break;
        }

        const nodeId = factoryGraphNodeIdForWorkType(target.subject.id);
        const workTypeMessages = workTypeMessagesByNodeId.get(nodeId) ?? [];
        workTypeMessages.push(target);
        workTypeMessagesByNodeId.set(nodeId, workTypeMessages);
        nodeErrorsByNodeId.set(nodeId, {
          code: target.code,
          message: target.message,
        });
        break;
      }
      case "WORK_STATE": {
        if (target.subject.location !== "TERMINAL") {
          break;
        }

        const nodeId = factoryGraphNodeIdForWorkState(target.subject.id);
        const workStateMessages = workStateMessagesByNodeId.get(nodeId) ?? [];
        workStateMessages.push(target);
        workStateMessagesByNodeId.set(nodeId, workStateMessages);
        nodeErrorsByNodeId.set(nodeId, {
          code: target.code,
          message: target.message,
        });
        break;
      }
      default:
        break;
    }
  }

  return {
    handleErrorsByNodeId,
    nodeErrorsByNodeId,
    workstationMessagesByNodeId,
    workTypeMessagesByNodeId,
    workStateMessagesByNodeId,
  };
}

export function validationHandleErrorsForNode(
  projection: FactoryValidationGraphProjection,
  nodeId: string,
): ReadonlyMap<string, FactoryValidationHandleError> | undefined {
  return projection.handleErrorsByNodeId.get(nodeId);
}

export function validationNodeErrorForNode(
  projection: FactoryValidationGraphProjection,
  nodeId: string,
): FactoryValidationNodeError | undefined {
  return projection.nodeErrorsByNodeId.get(nodeId);
}
