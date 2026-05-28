import {
  type ConstraintNodeData,
  ConstraintNodeView,
  type CurrentActivityPlaceNode,
  type StatePositionNodeData,
  StatePositionNodeView,
} from "./current-activity-place-node";
import {
  type CurrentActivityResourceNode,
  type ResourceNodeData,
  ResourceNodeView,
} from "./current-activity-resource-node";
import {
  type CurrentActivityWorkTypeNode,
  type WorkTypeNodeData,
  WorkTypeNodeView,
} from "./current-activity-work-type-node";
import {
  type CurrentActivityWorkerNode,
  type WorkerNodeData,
  WorkerNodeView,
} from "./current-activity-worker-node";
import type { WorkstationNodeData } from "./current-activity-workstation-node";
import {
  type CurrentActivityWorkstationNode,
  WorkstationNodeView,
} from "./current-activity-workstation-node";

const NODE_TYPES = {
  constraint: ConstraintNodeView,
  resource: ResourceNodeView,
  statePosition: StatePositionNodeView,
  worker: WorkerNodeView,
  workType: WorkTypeNodeView,
  workstation: WorkstationNodeView,
};

export { NODE_TYPES as CURRENT_ACTIVITY_NODE_TYPES };
export type CurrentActivityNode =
  | CurrentActivityWorkstationNode
  | CurrentActivityPlaceNode
  | CurrentActivityResourceNode
  | CurrentActivityWorkerNode
  | CurrentActivityWorkTypeNode;
export type {
  ConstraintNodeData,
  ResourceNodeData,
  StatePositionNodeData,
  WorkerNodeData,
  WorkstationNodeData,
  WorkTypeNodeData,
};
