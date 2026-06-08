import {
  type CurrentActivityDocNode,
  type DocNodeData,
  DocNodeView,
} from "./current-activity-doc-node";
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
  doc: DocNodeView,
  resource: ResourceNodeView,
  statePosition: StatePositionNodeView,
  worker: WorkerNodeView,
  workType: WorkTypeNodeView,
  workstation: WorkstationNodeView,
};

export { NODE_TYPES };
export type CurrentActivityNode =
  | CurrentActivityWorkstationNode
  | CurrentActivityPlaceNode
  | CurrentActivityDocNode
  | CurrentActivityResourceNode
  | CurrentActivityWorkerNode
  | CurrentActivityWorkTypeNode;
export type {
  ConstraintNodeData,
  DocNodeData,
  ResourceNodeData,
  StatePositionNodeData,
  WorkerNodeData,
  WorkstationNodeData,
  WorkTypeNodeData,
};
