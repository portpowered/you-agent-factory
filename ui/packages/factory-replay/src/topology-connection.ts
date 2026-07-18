import type {
  FactoryTopologyConnectionCandidate,
  FactoryTopologyConnectionKind,
  FactoryTopologyConnectionResult,
  FactoryTopologyNode,
  FactoryTopologyNodeKind,
  FactoryTopologyProjectionIssue,
} from "./topology-contract.js";
import { FACTORY_TOPOLOGY_RELATIONSHIPS } from "./topology-contract.js";

function invalidEndpointIssue(
  candidate: FactoryTopologyConnectionCandidate,
  endpoint: "source" | "target",
  reason: "MISSING_HANDLE" | "MISSING_NODE" | "NODE_KIND_MISMATCH",
  expectedNodeKind: FactoryTopologyNodeKind,
  handleId: string,
): FactoryTopologyProjectionIssue {
  const connectionId = `${candidate.kind}:${candidate.sourceNodeId ?? candidate.sourceReference}->${candidate.targetNodeId ?? candidate.targetReference}`;
  const nodeId =
    endpoint === "source" ? candidate.sourceNodeId : candidate.targetNodeId;
  const id = `invalid-endpoint:${connectionId}:${endpoint}:${reason}`;
  return {
    code: "INVALID_CONNECTION_ENDPOINT",
    connectionId,
    connectionKind: candidate.kind,
    endpoint,
    endpointReason: reason,
    expectedNodeKind,
    handleId,
    id,
    message: `Invalid ${endpoint} endpoint for ${candidate.kind} connection ${connectionId}: ${reason}.`,
    nodeId,
    sourceReference: candidate.sourceReference,
    targetReference: candidate.targetReference,
  };
}

/** Validate and project one relationship using the public semantic vocabulary. */
export function projectFactoryTopologyConnection(
  nodes: readonly FactoryTopologyNode[],
  candidate: FactoryTopologyConnectionCandidate,
): FactoryTopologyConnectionResult {
  if (!Object.hasOwn(FACTORY_TOPOLOGY_RELATIONSHIPS, candidate.kind)) {
    const connectionId = `${candidate.kind}:${candidate.sourceNodeId ?? candidate.sourceReference}->${candidate.targetNodeId ?? candidate.targetReference}`;
    return {
      issue: {
        code: "UNSUPPORTED_CONNECTION_KIND",
        connectionId,
        connectionKind: candidate.kind,
        id: `unsupported-connection:${connectionId}`,
        message: `Unsupported Factory topology connection kind ${candidate.kind}.`,
        sourceReference: candidate.sourceReference,
        targetReference: candidate.targetReference,
      },
      ok: false,
    };
  }
  const kind = candidate.kind as FactoryTopologyConnectionKind;
  const relationship = FACTORY_TOPOLOGY_RELATIONSHIPS[kind];
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  for (const endpoint of ["source", "target"] as const) {
    const endpointContract = relationship[endpoint];
    const endpointNodeId =
      endpoint === "source" ? candidate.sourceNodeId : candidate.targetNodeId;
    const node = endpointNodeId ? nodesById.get(endpointNodeId) : undefined;
    if (!node) {
      return {
        issue: invalidEndpointIssue(
          candidate,
          endpoint,
          "MISSING_NODE",
          endpointContract.nodeKind,
          endpointContract.handleId,
        ),
        ok: false,
      };
    }
    if (node.kind !== endpointContract.nodeKind) {
      return {
        issue: invalidEndpointIssue(
          candidate,
          endpoint,
          "NODE_KIND_MISMATCH",
          endpointContract.nodeKind,
          endpointContract.handleId,
        ),
        ok: false,
      };
    }
    if (
      !node.handles.some(
        (handle) =>
          handle.id === endpointContract.handleId && handle.role === endpoint,
      )
    ) {
      return {
        issue: invalidEndpointIssue(
          candidate,
          endpoint,
          "MISSING_HANDLE",
          endpointContract.nodeKind,
          endpointContract.handleId,
        ),
        ok: false,
      };
    }
  }
  const id = `${kind}:${candidate.sourceNodeId}->${candidate.targetNodeId}`;
  return {
    connection: {
      id,
      kind,
      source: {
        handleId: relationship.source.handleId,
        nodeId: candidate.sourceNodeId as string,
      },
      target: {
        handleId: relationship.target.handleId,
        nodeId: candidate.targetNodeId as string,
      },
    },
    ok: true,
  };
}
