import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  type Edge,
  Handle,
  MarkerType,
  type Node,
  type NodeChange,
  type NodeProps,
  Position,
  ReactFlow,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { DashboardWorkRelation } from "../../../api/dashboard/types";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
  DashboardGraphFrame,
} from "../../../components/dashboard/dashboard-graph";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import { GraphNodeButton } from "../../../components/ui/graph-node-button";
import { cn } from "../../../lib/cn";
import {
  getCachedTraceGraphLayout,
  layoutTraceGraphWithElk,
  traceGraphLayoutKey,
} from "../lib/trace-elk-layout";
import { failOnTraceReactFlowError } from "../lib/trace-react-flow-error";
import { useMeasuredTraceGraphViewport } from "../lib/use-measured-trace-graph-viewport";
import { projectTraceRelationsToFactoryGraph } from "../lib/trace-relation-factory-graph";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

const GRAPH_SHELL_CLASS =
  "max-w-full min-w-80 resize overflow-hidden border-transparent bg-af-surface-subtle";
const GRAPH_SHELL_STYLE = { height: 352, minHeight: 288 };
const GRAPH_VIEWPORT_STYLE = { height: "100%", width: "100%" };
const RELATION_NODE_CLASS =
  "flex h-full min-w-0 w-full flex-col gap-2 overflow-hidden rounded-lg border px-3 py-3 text-left text-af-text shadow-af-card transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const RELATION_NODE_ACTIVE_CLASS =
  "hover:border-af-accent-border hover:bg-af-accent-surface";
const RELATION_NODE_BADGE_CLASS =
  "inline-flex rounded-full border px-2 py-0.5 text-[0.68rem] font-semibold uppercase tracking-[0.08em]";
const RELATION_STATE_BADGE_DANGER_CLASS =
  "border-af-danger-border bg-af-danger-surface text-af-danger-text";
const RELATION_STATE_BADGE_SUCCESS_CLASS =
  "border-af-success-border bg-af-success-surface text-af-success-text";
const RELATION_STATE_BADGE_WARNING_CLASS =
  "border-af-warning-border bg-af-warning-surface text-af-warning-text";
const RELATION_NODE_TONE_DEFAULT_CLASS = "border-af-border bg-af-surface";
const RELATION_NODE_TONE_DANGER_CLASS =
  "border-af-danger-border bg-af-danger-surface";
const RELATION_NODE_TONE_SUCCESS_CLASS =
  "border-af-success-border bg-af-success-surface";
const RELATION_NODE_TONE_WARNING_CLASS =
  "border-af-warning-border bg-af-warning-surface";
const RELATION_NODE_WIDTH = 220;
const RELATION_NODE_HEIGHT = 112;
const GRAPH_FIT_VIEW_OPTIONS = { maxZoom: 1.5, padding: 0.08 } as const;

interface RelationFlowNodeData extends Record<string, unknown> {
  label: string;
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  relationStates: string[];
  relationTypes: string[];
  selectable: boolean;
  workID?: string;
}

type RelationFlowNode = Node<RelationFlowNodeData, "relation-work">;

const RELATION_NODE_TYPES = { "relation-work": RelationWorkNode };

export interface TraceRelationFlowProps {
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  relations: DashboardWorkRelation[];
}

export function TraceRelationFlow({
  locale,
  onSelectWorkID,
  relations,
}: TraceRelationFlowProps) {
  const graph = useMemo(
    () => buildRelationGraph(relations, locale),
    [locale, relations],
  );
  const graphDimensions = useMemo(
    () =>
      new Map(
        graph.nodes.map((node) => [
          node.id,
          {
            height: RELATION_NODE_HEIGHT,
            id: node.id,
            width: RELATION_NODE_WIDTH,
          },
        ]),
      ),
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
    setLayoutedNodes(
      getCachedTraceGraphLayout(layoutKey, graph.nodes) ?? graph.nodes,
    );
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
  }, [graph.edges, graph.nodes, graphDimensions]);

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
    setNodes(baseNodes);
  }, [baseNodes]);

  const handleNodesChange = useCallback(
    (changes: NodeChange<RelationFlowNode>[]) => {
      setNodes((currentNodes) => applyNodeChanges(changes, currentNodes));
    },
    [],
  );
  const { graphViewportReady, graphViewportRef } =
    useMeasuredTraceGraphViewport();

  if (relations.length === 0) {
    return <span>{getTraceDrilldownMessages(locale).noBatchRelations}</span>;
  }

  return (
    <DashboardGraphFrame
      aria-label={getTraceDrilldownMessages(locale).batchRelationGraphLabel}
      className={GRAPH_SHELL_CLASS}
      data-trace-relation-flow
      style={GRAPH_SHELL_STYLE}
    >
      <div
        className="h-full min-w-0 w-full"
        data-trace-graph-viewport
        ref={graphViewportRef}
        style={GRAPH_VIEWPORT_STYLE}
      >
        {graphViewportReady ? (
          <ReactFlow
            defaultEdgeOptions={{
              animated: false,
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
            onError={failOnTraceReactFlowError}
            panOnDrag
            proOptions={{ hideAttribution: true }}
            zoomOnScroll
          >
            <DashboardGraphBackground />
            <DashboardGraphControls fitViewOptions={GRAPH_FIT_VIEW_OPTIONS} />
          </ReactFlow>
        ) : null}
      </div>
    </DashboardGraphFrame>
  );
}

function RelationWorkNode({ data }: NodeProps<RelationFlowNode>) {
  const messages = getTraceDrilldownMessages(data.locale);
  const handleSelectWork = () => {
    if (data.workID && data.onSelectWorkID) {
      data.onSelectWorkID(data.workID);
    }
  };

  const content = (
    <>
      <Handle className="opacity-0" position={Position.Left} type="target" />
      <Handle className="opacity-0" position={Position.Right} type="source" />
      <div className="flex flex-wrap items-center gap-1.5">
        <span
          className={cn(
            RELATION_NODE_BADGE_CLASS,
            "border-af-accent-border bg-af-accent-surface text-af-accent",
            DASHBOARD_SUPPORTING_LABEL_CLASS,
          )}
        >
          {messages.workItemsLabel}
        </span>
        {data.relationTypes.slice(0, 1).map((relationType) => (
          <span
            className={cn(
              RELATION_NODE_BADGE_CLASS,
              "border-af-info-border bg-af-info-surface text-af-info",
              DASHBOARD_SUPPORTING_LABEL_CLASS,
            )}
            key={relationType}
          >
            {messages.localizeRelationType(relationType)}
          </span>
        ))}
        {data.relationStates.slice(0, 1).map((relationState) => (
          <span
            className={cn(
              RELATION_NODE_BADGE_CLASS,
              relationStateToneClassName(relationState),
              DASHBOARD_SUPPORTING_LABEL_CLASS,
            )}
            key={relationState}
          >
            {messages.localizeRelationState(relationState)}
          </span>
        ))}
      </div>
      <strong
        className={cn(
          "text-sm text-af-text [overflow-wrap:anywhere]",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
      >
        {data.label}
      </strong>
    </>
  );

  if (data.selectable && data.workID && data.onSelectWorkID) {
    return (
      <GraphNodeButton
        aria-label={data.label}
        className={cn(
          RELATION_NODE_CLASS,
          relationNodeToneClassName(data.relationStates),
          RELATION_NODE_ACTIVE_CLASS,
        )}
        onClick={handleSelectWork}
        title={data.workID}
      >
        {content}
      </GraphNodeButton>
    );
  }

  return (
    <article
      className={cn(
        RELATION_NODE_CLASS,
        relationNodeToneClassName(data.relationStates),
      )}
      title={data.workID}
    >
      {content}
    </article>
  );
}

function buildRelationGraph(
  relations: DashboardWorkRelation[],
  locale?: string,
): {
  edges: Edge[];
  nodes: RelationFlowNode[];
} {
  const projection = projectTraceRelationsToFactoryGraph(relations, { locale });

  return {
    edges: projection.topology.edges.map((edge) => {
      const overlay = projection.edgeOverlaysByEdgeId.get(edge.id);
      const relationLike: DashboardWorkRelation = {
        request_id: overlay?.requestId,
        required_state: overlay?.requiredState,
        source_work_id:
          projection.endpointKeyByNodeId.get(edge.sourceId) ?? edge.sourceId,
        target_work_id:
          projection.endpointKeyByNodeId.get(edge.targetId) ?? edge.targetId,
        type: overlay?.relationType ?? "RELATED_TO",
      };

      return {
        ariaLabel: overlay?.ariaLabel,
        id: edge.id,
        markerEnd: {
          color: relationEdgeStroke(relationLike),
          type: MarkerType.ArrowClosed,
        },
        source:
          projection.endpointKeyByNodeId.get(edge.sourceId) ?? edge.sourceId,
        style: relationEdgeStyle(relationLike),
        target:
          projection.endpointKeyByNodeId.get(edge.targetId) ?? edge.targetId,
      };
    }),
    nodes: projection.topology.nodes.map((node, index) => {
      const overlay = projection.overlaysByNodeId.get(node.id);
      return {
        data: {
          label: overlay?.displayLabel ?? node.label,
          locale,
          relationStates: overlay?.relationStates ?? [],
          relationTypes: overlay?.relationTypes ?? [],
          selectable: false,
          workID: overlay?.workID,
        },
        id: overlay?.endpointKey ?? node.id,
        position: { x: 0, y: index * (RELATION_NODE_HEIGHT + 20) },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
        type: "relation-work",
      };
    }),
  };
}

function relationStateToneClassName(relationState: string): string {
  const normalizedState = relationState.trim().toUpperCase();
  if (
    normalizedState === "FAILED" ||
    normalizedState === "FAIL" ||
    normalizedState === "REJECTED"
  ) {
    return RELATION_STATE_BADGE_DANGER_CLASS;
  }

  if (
    normalizedState === "DONE" ||
    normalizedState === "ACCEPTED" ||
    normalizedState === "COMPLETED"
  ) {
    return RELATION_STATE_BADGE_SUCCESS_CLASS;
  }

  return RELATION_STATE_BADGE_WARNING_CLASS;
}

function relationNodeToneClassName(relationStates: string[]): string {
  const primaryState = relationStates[0];
  if (!primaryState) {
    return RELATION_NODE_TONE_DEFAULT_CLASS;
  }

  const toneClassName = relationStateToneClassName(primaryState);
  if (toneClassName === RELATION_STATE_BADGE_DANGER_CLASS) {
    return RELATION_NODE_TONE_DANGER_CLASS;
  }
  if (toneClassName === RELATION_STATE_BADGE_SUCCESS_CLASS) {
    return RELATION_NODE_TONE_SUCCESS_CLASS;
  }

  return RELATION_NODE_TONE_WARNING_CLASS;
}

function relationEdgeStroke(relation: DashboardWorkRelation): string {
  if (relation.required_state) {
    const toneClassName = relationStateToneClassName(relation.required_state);
    if (toneClassName === RELATION_STATE_BADGE_DANGER_CLASS) {
      return "var(--color-af-danger-text)";
    }
    if (toneClassName === RELATION_STATE_BADGE_SUCCESS_CLASS) {
      return "var(--color-af-success)";
    }

    return "var(--color-af-warning-text)";
  }

  if (relation.type === "PARENT_CHILD") {
    return "var(--color-af-accent)";
  }

  return "var(--color-af-edge-muted)";
}

function relationEdgeStyle(relation: DashboardWorkRelation) {
  return {
    stroke: relationEdgeStroke(relation),
    strokeDasharray: relation.required_state ? "7 5" : undefined,
    strokeWidth: relation.required_state ? 2 : 1.7,
  };
}

