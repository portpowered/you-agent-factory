import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";

import type { FactoryResourceOccupancyProjection } from "./load-contract.js";

export interface FactoryDispatchRouteEvidence {
  stateId?: string;
  stateName?: string;
  workTypeId?: string;
}

export interface FactoryActiveDispatchEvidence {
  id: string;
  inputRoutes?: readonly FactoryDispatchRouteEvidence[];
  resourceNames?: readonly string[];
  startedTick: number;
  transitionId?: string;
  workIds?: readonly string[];
}

export interface FactoryDispatchOverlayEvidenceStatus {
  resources: "known" | "unavailable";
  route: "known" | "unavailable";
  work: "known" | "unavailable";
  worker: "known" | "unavailable";
  workstation: "known" | "unavailable";
}

export interface FactoryDispatchOverlayProjection {
  connectionIds: string[];
  dispatchId: string;
  evidence: FactoryDispatchOverlayEvidenceStatus;
  id: string;
  resourceIds?: string[];
  resourceNodeIds?: string[];
  startedTick: number;
  transitionId?: string;
  workerId?: string;
  workerNodeId?: string;
  workIds?: string[];
  workProjectionIds?: string[];
  workstationId?: string;
  workstationNodeId?: string;
}

export interface FactoryActivityProjectionIssue {
  code:
    | "CONTRADICTORY_DISPATCH_EVIDENCE"
    | "INVALID_RESOURCE_CAPACITY"
    | "MISSING_FACTORY"
    | "RESOURCE_CAPACITY_EXCEEDED"
    | "UNAVAILABLE_TOPOLOGY_PATH"
    | "UNRESOLVED_RESOURCE"
    | "UNRESOLVED_ROUTE"
    | "UNRESOLVED_WORKER"
    | "UNRESOLVED_WORKSTATION";
  dispatchId?: string;
  id: string;
  message: string;
  reference?: string;
  resourceId?: string;
}

export interface FactoryActivityProjection {
  activeDispatchOverlays: FactoryDispatchOverlayProjection[];
  activeWorkstationNodeIds: string[];
  issues: FactoryActivityProjectionIssue[];
  resourceOccupancy: FactoryResourceOccupancyProjection[];
  selectedTick: number;
}

export interface FactoryActivityProjectionInput {
  activeDispatches: readonly FactoryActiveDispatchEvidence[];
  factory?: FactoryDefinition;
  selectedTick: number;
}

export interface FactoryActivityAtTickInput {
  events: readonly FactoryEvent[];
  tick: number;
}
