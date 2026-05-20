

import type { WorkstationNodeData } from "./current-activity-workstation-node";
import {
  ConstraintNodeView,
  type ConstraintNodeData,
  type CurrentActivityPlaceNode,
  ResourceNodeView,
  type ResourceNodeData,
  StatePositionNodeView,
  type StatePositionNodeData,
} from "./current-activity-place-node";
import {
  type CurrentActivityWorkstationNode,
  WorkstationNodeView,
} from "./current-activity-workstation-node";

const NODE_TYPES = {
  constraint: ConstraintNodeView,
  resource: ResourceNodeView,
  statePosition: StatePositionNodeView,
  workstation: WorkstationNodeView,
};

export { NODE_TYPES as CURRENT_ACTIVITY_NODE_TYPES };
export type CurrentActivityNode = CurrentActivityWorkstationNode | CurrentActivityPlaceNode;
export type { ConstraintNodeData, ResourceNodeData, StatePositionNodeData, WorkstationNodeData };

