import { FactoryGraphDocNodeView, type FactoryGraphDocNode } from "./semantic-doc-node.js";
import { FactoryGraphConstraintNodeView, FactoryGraphStatePositionNodeView, type FactoryGraphPlaceNode } from "./semantic-place-nodes.js";
import { FactoryGraphResourceNodeView, FactoryGraphWorkerNodeView, FactoryGraphWorkTypeNodeView, type FactoryGraphResourceNode, type FactoryGraphWorkerNode, type FactoryGraphWorkTypeNode } from "./semantic-support-nodes.js";
import { FactoryGraphWorkstationNodeView, type FactoryGraphWorkstationNode } from "./semantic-workstation-node.js";
/** The complete original Factory semantic renderer registry. */
export declare const FACTORY_GRAPH_NODE_TYPES: {
    constraint: typeof FactoryGraphConstraintNodeView;
    doc: typeof FactoryGraphDocNodeView;
    resource: typeof FactoryGraphResourceNodeView;
    statePosition: typeof FactoryGraphStatePositionNodeView;
    worker: typeof FactoryGraphWorkerNodeView;
    workType: typeof FactoryGraphWorkTypeNodeView;
    workstation: typeof FactoryGraphWorkstationNodeView;
};
export type FactoryGraphNode = FactoryGraphWorkstationNode | FactoryGraphPlaceNode | FactoryGraphDocNode | FactoryGraphResourceNode | FactoryGraphWorkerNode | FactoryGraphWorkTypeNode;
