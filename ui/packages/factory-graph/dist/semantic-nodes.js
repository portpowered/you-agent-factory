import { FactoryGraphDocNodeView, } from "./semantic-doc-node.js";
import { FactoryGraphConstraintNodeView, FactoryGraphStatePositionNodeView, } from "./semantic-place-nodes.js";
import { FactoryGraphResourceNodeView, FactoryGraphWorkerNodeView, FactoryGraphWorkTypeNodeView, } from "./semantic-support-nodes.js";
import { FactoryGraphWorkstationNodeView, } from "./semantic-workstation-node.js";
/** The complete original Factory semantic renderer registry. */
export const FACTORY_GRAPH_NODE_TYPES = {
    constraint: FactoryGraphConstraintNodeView,
    doc: FactoryGraphDocNodeView,
    resource: FactoryGraphResourceNodeView,
    statePosition: FactoryGraphStatePositionNodeView,
    worker: FactoryGraphWorkerNodeView,
    workType: FactoryGraphWorkTypeNodeView,
    workstation: FactoryGraphWorkstationNodeView,
};
