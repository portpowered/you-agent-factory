import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeChange,
  type NodeProps,
} from "@xyflow/react";
import type { CSSProperties } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { cx } from "../../components/dashboard/classnames";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../components/dashboard/typography";
import type { DashboardWorkRelation } from "../../../api/dashboard/types";
import {
  getCachedTraceGraphLayout,
  layoutTraceGraphWithElk,
  traceGraphLayoutKey,
} from "./trace-elk-layout";

const GRAPH_SHELL_CLASS =
  "h-[60rem] min-h-[40rem] overflow-hidden rounded-xl border border-af-overlay/8 bg-af-overlay/4";
const RELATION_NODE_CLASS =
  "flex h-full min-w-0 w-full flex-col gap-[0.35rem] overflow-hidden rounded-lg border border-af-overlay/10 bg-af-canvas px-3 py-3 text-left text-af-ink shadow-[0_10px_30px_rgba(15,23,42,0.06)] transition-colors";
const RELATION_NODE_ACTIVE_CLASS = "hover:border-af-accent/28 hover:bg-af-accent/8";
const RELATION_EDGE_STROKE = "var(--color-af-edge-muted)";
const GRAPH_BACKGROUND_COLOR = "var(--color-af-edge-muted-soft)";
const GRAPH_BACKGROUND_GAP = 24;
const GRAPH_BACKGROUND_SIZE = 1;
const RELATION_NODE_WIDTH = 220;
const RELATION_NODE_HEIGHT = 112;
const GRAPH_FIT_VIEW_OPTIONS = { maxZoom: 1.5, padding: 0.08 } as const;

type CSSPropertiesWithVariables = CSSProperties & Record<`--${string}`, string | number>;

const GRAPH_CONTROLS_STYLE: CSSPropertiesWithVariables = {
  "--xy-controls-box-shadow": "none",
  "--xy-controls-button-background-color-props":
    "rgb(from var(--color-af-surface) r g b / 0.94)",
  "--xy-controls-button-background-color-hover-props":
    "rgb(from var(--color-af-overlay) r g b / 0.1)",
  "--xy-controls-button-border-color-props":
    "rgb(from var(--color-af-overlay) r g b / 0.08)",
  "--xy-controls-button-color-props": "rgb(from var(--color-af-ink) r g b / 0.72)",
  "--xy-controls-button-color-hover-props": "var(--color-af-ink)",
  backgroundColor: "rgb(from var(--color-af-surface) r g b / 0.88)",
  border: "1px solid rgb(from var(--color-af-overlay) r g b / 0.08)",
  borderRadius: 8,
  overflow: "hidden",
};

interface RelationFlowNodeData extends Record<string, unknown> {
  label: string;
  onSelectWorkID?: (workID: string) => void;
  selectable: boolean;
  workID?: string;
}

type RelationFlowNode = Node<RelationFlowNodeData, "relation-work">;

const RELATION_NODE_TYPES = {
  "relation-work": RelationWorkNode,
};

export interface TraceRelationFlowProps {
  onSelectWorkID?: (workID: string) => void;
  relations: DashboardWorkRelation[];
}

export function TraceRelationFlow({
  onSelectWorkID,
  relations,
}: TraceRelationFlowProps) {
  const graph = useMemo(
    () => buildRelationGraph(relations),
    [relations],
  );
  const graphDimensions = useMemo(
    () =>
      new Map(graph.nodes.map((node) => [node.id, {
        height: RELATION_NODE_HEIGHT,
        id: node.id,
        width: RELATION_NODE_WIDTH,
      }])),
    [graph.nodes],
  );
  const layoutKey = useMemo(
    () => traceGraphLayoutKey(graph.nodes, graph.edges, graphDimensions),
    [graph.edges, graph.nodes, graphDimensions],
  );
  const [layoutedNodes, setLayoutedNodes] = useState<RelationFlowNode[]>(
    () => getCachedTraceGraphLayout(layoutKey, graph.nodes) ?? graph.nodes,
  );

  useEffect(() => {
    setLayoutedNodes(getCachedTraceGraphLayout(layoutKey, graph.nodes) ?? graph.nodes);
  }, [graph.nodes, layoutKey]);

  useEffect(() => {
    let cancelled = false;

    void layoutTraceGraphWithElk(
      graph.nodes,
      graph.edges,
      graphDimensions,
    ).then((nextNodes) => {
      if (!cancelled) {
        setLayoutedNodes(nextNodes);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [graph.edges, graph.nodes, graphDimensions, layoutKey]);

  const baseNodes = useMemo<RelationFlowNode[]>(() => {
    const positionsByID = new Map(
      layoutedNodes.map((node) => [node.id, node.position]),
    );

    return graph.nodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        onSelectWorkID,
        selectable: Boolean(node.data.workID && onSelectWorkID),
      },
      position: positionsByID.get(node.id) ?? node.position,
    }));
  }, [graph.nodes, layoutedNodes, onSelectWorkID]);
  const [nodes, setNodes] = useState<RelationFlowNode[]>(baseNodes);

  useEffect(() => {
    setNodes((currentNodes) => {
      const currentPositions = new Map(
        currentNodes.map((node) => [node.id, node.position]),
      );

      return baseNodes.map((node) => ({
        ...node,
        position: currentPositions.get(node.id) ?? node.position,
      }));
    });
  }, [baseNodes]);

  const handleNodesChange = useCallback((changes: NodeChange<RelationFlowNode>[]) => {
    setNodes((currentNodes) => applyNodeChanges(changes, currentNodes));
  }, []);

  if (relations.length === 0) {
    return <span>None</span>;
  }

  return (
    <section
      aria-label="Batch relation graph"
      className={GRAPH_SHELL_CLASS}
      data-trace-relation-flow
    >
      <ReactFlow
        defaultEdgeOptions={{
          animated: false,
          markerEnd: {
            color: RELATION_EDGE_STROKE,
            type: MarkerType.ArrowClosed,
          },
          style: { stroke: RELATION_EDGE_STROKE, strokeWidth: 1.7 },
          type: "smoothstep",
        }}
        edges={graph.edges}
        fitView
        fitViewOptions={GRAPH_FIT_VIEW_OPTIONS}
        key={layoutKey}
        maxZoom={2}
        minZoom={0.35}
        nodes={nodes}
        nodesDraggable={true}
        nodeTypes={RELATION_NODE_TYPES}
        onNodesChange={handleNodesChange}
        panOnDrag
        proOptions={{ hideAttribution: true }}
        zoomOnScroll
      >
        <Background
          color={GRAPH_BACKGROUND_COLOR}
          gap={GRAPH_BACKGROUND_GAP}
          size={GRAPH_BACKGROUND_SIZE}
        />
        <Controls
          fitViewOptions={GRAPH_FIT_VIEW_OPTIONS}
          showInteractive={false}
          style={GRAPH_CONTROLS_STYLE}
        />
      </ReactFlow>
    </section>
  );
}

function RelationWorkNode({
  data,
}: NodeProps<RelationFlowNode>) {
  const content = (
    <>
      <Handle className="opacity-0" position={Position.Left} type="target" />
      <Handle className="opacity-0" position={Position.Right} type="source" />
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Work</span>
      <strong
        className={cx("text-sm text-af-ink [overflow-wrap:anywhere]", DASHBOARD_BODY_TEXT_CLASS)}
      >
        {data.label}
      </strong>
    </>
  );

  if (data.selectable && data.workID && data.onSelectWorkID) {
    return (
      <button
        className={cx(RELATION_NODE_CLASS, RELATION_NODE_ACTIVE_CLASS)}
        onClick={() => data.onSelectWorkID?.(data.workID!)}
        title={data.workID}
        type="button"
      >
        {content}
      </button>
    );
  }

  return (
    <article className={RELATION_NODE_CLASS} title={data.workID}>
      {content}
    </article>
  );
}

function buildRelationGraph(
  relations: DashboardWorkRelation[],
): {
  edges: Edge[];
  nodes: RelationFlowNode[];
} {
  const nodeRecords = new Map<
    string,
    { id: string; label: string; order: number; workID?: string }
  >();
  const edgeRecords: Edge[] = [];

  relations.forEach((relation, index) => {
    const source = relationEndpoint(relation, "source", index);
    const target = relationEndpoint(relation, "target", index);

    if (!nodeRecords.has(source.id)) {
      nodeRecords.set(source.id, {
        id: source.id,
        label: source.label,
        order: index * 2,
        workID: source.workID,
      });
    }

    if (!nodeRecords.has(target.id)) {
      nodeRecords.set(target.id, {
        id: target.id,
        label: target.label,
        order: index * 2 + 1,
        workID: target.workID,
      });
    }

    edgeRecords.push({
      id: relationEdgeID(relation, index),
      source: source.id,
      target: target.id,
    });
  });

  return {
    edges: edgeRecords,
    nodes: [...nodeRecords.values()].map((record) => ({
      data: {
        label: record.label,
        workID: record.workID,
        selectable: false,
      },
      id: record.id,
      position: { x: 0, y: record.order * (RELATION_NODE_HEIGHT + 20) },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "relation-work",
    })),
  };
}

function relationEndpoint(
  relation: DashboardWorkRelation,
  side: "source" | "target",
  index: number,
): { id: string; label: string; workID?: string } {
  if (side === "source") {
    const workID = relation.source_work_id?.trim();
    return {
      id: workID || `relation-${index}-source`,
      label: relation.source_work_name || workID || "Unknown source",
      workID: workID || undefined,
    };
  }

  const workID = relation.target_work_id.trim();
  return {
    id: workID,
    label: relation.target_work_name || workID,
    workID,
  };
}

function relationEdgeID(relation: DashboardWorkRelation, index: number): string {
  return [
    relation.type,
    relation.source_work_id ?? `source-${index}`,
    relation.target_work_id,
    relation.required_state ?? "",
    relation.request_id ?? "",
  ].join("|");
}
