import type { EdgeProps } from "@xyflow/react";
import {
  GraphEdge,
  type GraphEdgeData,
} from "@you-agent-factory/components/graphs";

export type { GraphEdgeData as FactoryGraphEdgeData };

export const FACTORY_GRAPH_EDGE_TYPES = {
  factoryEditorEdge: FactoryGraphEdge,
};

function FactoryGraphEdge(props: EdgeProps) {
  return (
    <GraphEdge
      {...props}
      edgeClassName="agent-factory-editor-edge"
      labelClassName="agent-factory-editor-edge-label pointer-events-none fill-on-surface-subtle text-[11px] font-semibold"
    />
  );
}
