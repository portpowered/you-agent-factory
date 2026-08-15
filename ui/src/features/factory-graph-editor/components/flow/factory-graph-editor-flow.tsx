import type { Edge } from "@xyflow/react";
import { FACTORY_GRAPH_NODE_TYPES } from "@you-agent-factory/factory-graph";

import { FACTORY_GRAPH_EDGE_TYPES } from "../../../graphs/components/factory-graph-edge";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphTopology,
  FactoryWorkstation,
} from "../../lib/draft/factory-graph-draft-types";
import type { FactoryGraphConnectionEndpoint } from "../../lib/editor/factory-graph-editor-connections";
import { createFactoryGraphWorkstationResolver } from "../../lib/editor/factory-graph-editor-connections";
import type { FactoryGraphWorkerRuntimeStatus } from "../../lib/editor-runtime/factory-graph-editor-runtime";
import type { FactoryLayout } from "../../lib/layout/factory-graph-layout-operations";
import {
  type FactoryGraphReactFlowNode,
  projectFactoryGraphToReactFlow,
} from "../../lib/projection/factory-graph-react-flow-projection";
import type { FactoryValidationGraphProjection } from "../../lib/projection/factory-validation-graph-projection";

/** The editor consumes the package registry without a host-local renderer. */
export const FACTORY_GRAPH_EDITOR_NODE_TYPES = FACTORY_GRAPH_NODE_TYPES;
export {
  FACTORY_GRAPH_EDGE_TYPES,
  FACTORY_GRAPH_EDGE_TYPES as FACTORY_GRAPH_EDITOR_EDGE_TYPES,
};

export function buildFactoryGraphEditorFlowModel(input: {
  canEditConnections: boolean;
  factoryDefinition?: CanonicalFactoryDefinition | null;
  layout?: FactoryLayout | null;
  layoutPositionsByNodeId?: ReadonlyMap<string, { x: number; y: number }>;
  locale?: string;
  onConnectionAnchorClick?: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingAdditionNodeIds: ReadonlySet<string>;
  pendingRemovalEdgeIds: ReadonlySet<string>;
  pendingRemovalNodeIds: ReadonlySet<string>;
  topology: FactoryGraphTopology;
  validationProjection?: FactoryValidationGraphProjection;
  workstations?: readonly FactoryWorkstation[];
  workerStatusByName?: ReadonlyMap<string, FactoryGraphWorkerRuntimeStatus>;
}): {
  edges: Edge[];
  nodes: FactoryGraphReactFlowNode[];
} {
  return projectFactoryGraphToReactFlow({
    factoryDefinition: input.factoryDefinition,
    filterEdgesToRenderedHandles: true,
    editor: {
      canEditConnections: input.canEditConnections,
      onConnectionAnchorClick: input.onConnectionAnchorClick,
      pendingAdditionEdgeIds: input.pendingAdditionEdgeIds,
      pendingAdditionNodeIds: input.pendingAdditionNodeIds,
      pendingConnectionSource: input.pendingConnectionSource,
      pendingRemovalEdgeIds: input.pendingRemovalEdgeIds,
      pendingRemovalNodeIds: input.pendingRemovalNodeIds,
      validationProjection: input.validationProjection,
    },
    layout: input.layout,
    layoutPositionsByNodeId: input.layoutPositionsByNodeId,
    locale: input.locale,
    mode: "editor",
    runtime: {
      workerStatusByName: input.workerStatusByName,
    },
    topology: input.topology,
    workstations: input.workstations,
    workstationResolver: createFactoryGraphWorkstationResolver(
      input.workstations,
      input.factoryDefinition?.workers,
    ),
  });
}
