import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";

export interface FactoryWorkStateOccupancyEvidence {
  id: string;
  stateId?: string;
  stateName?: string;
  workTypeId?: string;
}

export interface FactoryResourceClaimEvidence {
  resourceName: string;
  quantity?: number;
}

export interface FactoryActiveResourceClaimsEvidence {
  id: string;
  resourceClaims?: readonly FactoryResourceClaimEvidence[];
}

export interface FactoryWorkStateCountProjection {
  count?: number;
  evidence: "known" | "unavailable";
  workIds?: string[];
  workStateId: string;
  workStateNodeId: string;
  workTypeId: string;
}

export interface FactoryResourceOccupancyProjection {
  availableQuantity?: number;
  capacity?: number;
  capacityEvidence: "known" | "unavailable";
  evidence: "known" | "unavailable";
  occupiedQuantity?: number;
  resourceId: string;
  resourceNodeId: string;
}

export interface FactoryLoadProjectionIssue {
  code:
    | "CONTRADICTORY_RESOURCE_CLAIM"
    | "CONTRADICTORY_WORK_STATE"
    | "INVALID_RESOURCE_CAPACITY"
    | "MISSING_FACTORY"
    | "RESOURCE_CAPACITY_EXCEEDED"
    | "UNRESOLVED_RESOURCE_CLAIM"
    | "UNRESOLVED_WORK_STATE";
  dispatchId?: string;
  id: string;
  message: string;
  reference?: string;
  resourceId?: string;
  workId?: string;
}

export interface FactoryLoadProjection {
  issues: FactoryLoadProjectionIssue[];
  resourceOccupancy: FactoryResourceOccupancyProjection[];
  selectedTick: number;
  workStateCounts: FactoryWorkStateCountProjection[];
}

export interface FactoryLoadProjectionInput {
  activeDispatches?: readonly FactoryActiveResourceClaimsEvidence[];
  factory?: FactoryDefinition;
  selectedTick: number;
  works?: readonly FactoryWorkStateOccupancyEvidence[];
}

export interface FactoryLoadAtTickInput {
  events: readonly FactoryEvent[];
  tick: number;
}
