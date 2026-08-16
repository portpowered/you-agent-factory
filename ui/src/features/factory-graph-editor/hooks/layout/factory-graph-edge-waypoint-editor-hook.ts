import type { XYPosition } from "@xyflow/react";
import type { GraphEdgeInteraction } from "@you-agent-factory/components/graphs";
import type { PointerEvent as ReactPointerEvent } from "react";
import { useCallback, useMemo, useRef, useState } from "react";

import type { FactoryGraphEditorTool } from "../../components/controls/factory-graph-editor-controls";
import { flowMidpointBetweenNodes } from "../../components/flow/factory-graph-edge-waypoint-layer";
import { describeFactoryGraphLayoutEdgeId } from "../../lib/layout/factory-graph-layout-edge-labels";
import { factoryLayoutEdgeWaypoints } from "../../lib/layout/factory-graph-layout-edge-waypoints";
import type {
  FactoryLayout,
  FactoryLayoutPoint,
} from "../../lib/layout/factory-graph-layout-operations";
import { getFactoryGraphEditorMessages } from "../../messages/editor";

const EDGE_DRAG_CLICK_THRESHOLD_PX = 4;

type EdgeDragSession = {
  baseWaypoints: FactoryLayoutPoint[];
  edgeId: string;
  insertIndex: number;
  moved: boolean;
  pointerId: number;
  pointerTarget: SVGPathElement;
  startScreenPosition: { x: number; y: number };
};

type EdgeWaypointPreview = {
  edgeId: string;
  waypoints: FactoryLayoutPoint[];
};

function canEditFactoryGraphEdgeWaypoints(input: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  editorMode: boolean;
}): boolean {
  return (
    input.editorMode &&
    input.canInteractWithEditor &&
    input.activeTool !== "delete" &&
    input.activeTool !== "add"
  );
}

type FactoryGraphEdgeWaypointNode = {
  id: string;
  position: XYPosition;
};

function resolveEdgeNodePositions(
  nodes: readonly FactoryGraphEdgeWaypointNode[],
  edgeId: string,
): { source: XYPosition; target: XYPosition } | null {
  const { sourceId, targetId } = describeFactoryGraphLayoutEdgeId(edgeId);
  const sourceNode = nodes.find((node) => node.id === sourceId);
  const targetNode = nodes.find((node) => node.id === targetId);
  if (!sourceNode || !targetNode) {
    return null;
  }

  return {
    source: sourceNode.position,
    target: targetNode.position,
  };
}

function squaredDistanceToSegment(
  point: FactoryLayoutPoint,
  start: FactoryLayoutPoint,
  end: FactoryLayoutPoint,
): number {
  const deltaX = end.x - start.x;
  const deltaY = end.y - start.y;
  const lengthSquared = deltaX * deltaX + deltaY * deltaY;
  if (lengthSquared === 0) {
    const offsetX = point.x - start.x;
    const offsetY = point.y - start.y;
    return offsetX * offsetX + offsetY * offsetY;
  }

  const projection = Math.max(
    0,
    Math.min(
      1,
      ((point.x - start.x) * deltaX + (point.y - start.y) * deltaY) /
        lengthSquared,
    ),
  );
  const closestX = start.x + projection * deltaX;
  const closestY = start.y + projection * deltaY;
  const offsetX = point.x - closestX;
  const offsetY = point.y - closestY;
  return offsetX * offsetX + offsetY * offsetY;
}

function resolveEdgeWaypointInsertIndex(
  nodes: readonly FactoryGraphEdgeWaypointNode[],
  edgeId: string,
  waypoints: readonly FactoryLayoutPoint[],
  position: FactoryLayoutPoint,
): number {
  const endpoints = resolveEdgeNodePositions(nodes, edgeId);
  const routePoints = endpoints
    ? [endpoints.source, ...waypoints, endpoints.target]
    : [...waypoints];
  if (routePoints.length < 2) {
    return waypoints.length;
  }

  let closestSegmentIndex = 0;
  let closestDistance = Number.POSITIVE_INFINITY;
  for (let index = 0; index < routePoints.length - 1; index += 1) {
    const distance = squaredDistanceToSegment(
      position,
      routePoints[index],
      routePoints[index + 1],
    );
    if (distance < closestDistance) {
      closestDistance = distance;
      closestSegmentIndex = index;
    }
  }

  return Math.min(closestSegmentIndex, waypoints.length);
}

function insertEdgeWaypoint(
  waypoints: readonly FactoryLayoutPoint[],
  position: FactoryLayoutPoint,
  insertIndex: number,
): FactoryLayoutPoint[] {
  const nextWaypoints = waypoints.map((point) => ({ ...point }));
  nextWaypoints.splice(insertIndex, 0, { x: position.x, y: position.y });
  return nextWaypoints;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: coordinates edge selection, layout mutation, and editor controls.
export function useFactoryGraphEdgeWaypointEditor(input: {
  activeTool: FactoryGraphEditorTool;
  addEdgeWaypoint: (
    edgeId: string,
    position: { x: number; y: number },
    insertIndex?: number,
  ) => void;
  canInteractWithEditor: boolean;
  editorMode: boolean;
  handleEditorEdgeDelete: (edgeId: string) => void;
  layout: FactoryLayout;
  locale?: string | null;
  moveEdgeWaypoint: (
    edgeId: string,
    waypointIndex: number,
    position: { x: number; y: number },
  ) => void;
  removeEdgeWaypoint: (edgeId: string, waypointIndex: number) => void;
  nodes: readonly FactoryGraphEdgeWaypointNode[];
}) {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const edgeDragSessionRef = useRef<EdgeDragSession | null>(null);
  const suppressNextEdgeClickRef = useRef<string | null>(null);
  const [edgeWaypointPreview, setEdgeWaypointPreview] =
    useState<EdgeWaypointPreview | null>(null);
  const [selectedWaypointEdgeId, setSelectedWaypointEdgeId] = useState<
    string | null
  >(null);
  const canEditWaypoints = canEditFactoryGraphEdgeWaypoints(input);

  const selectedEdgeWaypoints = useMemo(() => {
    if (!selectedWaypointEdgeId) {
      return [];
    }

    if (edgeWaypointPreview?.edgeId === selectedWaypointEdgeId) {
      return edgeWaypointPreview.waypoints;
    }

    return (
      factoryLayoutEdgeWaypoints(input.layout, selectedWaypointEdgeId) ?? []
    );
  }, [edgeWaypointPreview, input.layout, selectedWaypointEdgeId]);

  const selectedEdgeDescription = useMemo(() => {
    if (!selectedWaypointEdgeId) {
      return null;
    }

    const { kind, sourceId, targetId } = describeFactoryGraphLayoutEdgeId(
      selectedWaypointEdgeId,
    );
    return {
      edgeKindLabel: messages.edgeKindLabel(
        kind as Parameters<typeof messages.edgeKindLabel>[0],
      ),
      edgeSourceLabel: sourceId,
      edgeTargetLabel: targetId,
    };
  }, [messages, selectedWaypointEdgeId]);

  const releaseEdgePointerCapture = useCallback((session: EdgeDragSession) => {
    if (session.pointerTarget.hasPointerCapture(session.pointerId)) {
      session.pointerTarget.releasePointerCapture(session.pointerId);
    }
  }, []);

  const clearEdgeDragSession = useCallback(() => {
    const session = edgeDragSessionRef.current;
    if (!session) {
      return;
    }

    edgeDragSessionRef.current = null;
    releaseEdgePointerCapture(session);
    setEdgeWaypointPreview(null);
  }, [releaseEdgePointerCapture]);

  const handleEditorEdgePointerDown = useCallback(
    (
      edgeId: string,
      event: ReactPointerEvent<SVGPathElement>,
      flowPosition: FactoryLayoutPoint,
    ) => {
      if (!canEditWaypoints) {
        return;
      }

      event.stopPropagation();
      event.preventDefault();
      suppressNextEdgeClickRef.current = null;
      const baseWaypoints =
        factoryLayoutEdgeWaypoints(input.layout, edgeId) ?? [];
      edgeDragSessionRef.current = {
        baseWaypoints,
        edgeId,
        insertIndex: resolveEdgeWaypointInsertIndex(
          input.nodes,
          edgeId,
          baseWaypoints,
          flowPosition,
        ),
        moved: false,
        pointerId: event.pointerId,
        pointerTarget: event.currentTarget,
        startScreenPosition: {
          x: event.clientX,
          y: event.clientY,
        },
      };
      event.currentTarget.setPointerCapture(event.pointerId);
    },
    [canEditWaypoints, input.layout, input.nodes],
  );

  const handleEditorEdgePointerMove = useCallback(
    (
      event: ReactPointerEvent<SVGPathElement>,
      flowPosition: FactoryLayoutPoint,
    ) => {
      const session = edgeDragSessionRef.current;
      if (!session || session.pointerId !== event.pointerId) {
        return;
      }

      if (
        !session.moved &&
        Math.hypot(
          event.clientX - session.startScreenPosition.x,
          event.clientY - session.startScreenPosition.y,
        ) < EDGE_DRAG_CLICK_THRESHOLD_PX
      ) {
        return;
      }

      session.moved = true;
      setSelectedWaypointEdgeId(session.edgeId);
      setEdgeWaypointPreview({
        edgeId: session.edgeId,
        waypoints: insertEdgeWaypoint(
          session.baseWaypoints,
          flowPosition,
          session.insertIndex,
        ),
      });
    },
    [],
  );

  const handleEditorEdgePointerUp = useCallback(
    (
      event: ReactPointerEvent<SVGPathElement>,
      flowPosition: FactoryLayoutPoint,
    ) => {
      const session = edgeDragSessionRef.current;
      if (!session || session.pointerId !== event.pointerId) {
        return;
      }

      edgeDragSessionRef.current = null;
      releaseEdgePointerCapture(session);
      setEdgeWaypointPreview(null);
      if (!session.moved) {
        setSelectedWaypointEdgeId(session.edgeId);
        suppressNextEdgeClickRef.current = session.edgeId;
        return;
      }

      input.addEdgeWaypoint(session.edgeId, flowPosition, session.insertIndex);
      setSelectedWaypointEdgeId(session.edgeId);
      suppressNextEdgeClickRef.current = session.edgeId;
    },
    [input.addEdgeWaypoint, releaseEdgePointerCapture],
  );

  const handleEditorEdgePointerCancel = useCallback(
    (event: ReactPointerEvent<SVGPathElement>) => {
      const session = edgeDragSessionRef.current;
      if (!session || session.pointerId !== event.pointerId) {
        return;
      }

      clearEdgeDragSession();
      event.preventDefault();
      event.stopPropagation();
    },
    [clearEdgeDragSession],
  );

  const handleEditorEdgeClick = useCallback(
    (edgeId: string) => {
      if (!input.canInteractWithEditor || !input.editorMode) {
        return;
      }

      if (suppressNextEdgeClickRef.current === edgeId) {
        suppressNextEdgeClickRef.current = null;
        return;
      }

      if (input.activeTool === "delete") {
        input.handleEditorEdgeDelete(edgeId);
        return;
      }

      if (!canEditWaypoints) {
        return;
      }

      setSelectedWaypointEdgeId((current) =>
        current === edgeId ? null : edgeId,
      );
    },
    [canEditWaypoints, input],
  );

  const handleAddSelectedEdgeWaypoint = useCallback(
    (position?: { x: number; y: number }) => {
      if (!selectedWaypointEdgeId || !canEditWaypoints) {
        return;
      }

      const explicitPosition =
        position ??
        (() => {
          const endpoints = resolveEdgeNodePositions(
            input.nodes,
            selectedWaypointEdgeId,
          );
          if (!endpoints) {
            return null;
          }
          return flowMidpointBetweenNodes(endpoints.source, endpoints.target);
        })();

      if (!explicitPosition) {
        return;
      }

      input.addEdgeWaypoint(selectedWaypointEdgeId, explicitPosition);
    },
    [canEditWaypoints, input, selectedWaypointEdgeId],
  );

  const handleMoveSelectedEdgeWaypoint = useCallback(
    (
      edgeId: string,
      waypointIndex: number,
      position: { x: number; y: number },
    ) => {
      if (!canEditWaypoints || edgeId !== selectedWaypointEdgeId) {
        return;
      }

      input.moveEdgeWaypoint(edgeId, waypointIndex, position);
    },
    [canEditWaypoints, input, selectedWaypointEdgeId],
  );

  const handleRemoveSelectedEdgeWaypoint = useCallback(
    (edgeId: string, waypointIndex: number) => {
      if (!canEditWaypoints || edgeId !== selectedWaypointEdgeId) {
        return;
      }

      input.removeEdgeWaypoint(edgeId, waypointIndex);
    },
    [canEditWaypoints, input, selectedWaypointEdgeId],
  );

  const handleEditorEdgeDoubleClick = useCallback(
    (edgeId: string, position: { x: number; y: number }) => {
      if (!canEditWaypoints) {
        return;
      }

      setSelectedWaypointEdgeId(edgeId);
      input.addEdgeWaypoint(edgeId, position);
    },
    [canEditWaypoints, input],
  );

  const edgeWaypointPreviews = useMemo(() => {
    if (!edgeWaypointPreview) {
      return undefined;
    }

    return new Map<string, readonly FactoryLayoutPoint[]>([
      [edgeWaypointPreview.edgeId, edgeWaypointPreview.waypoints],
    ]);
  }, [edgeWaypointPreview]);

  const edgePointerInteraction = useCallback(
    (edgeId: string): GraphEdgeInteraction | undefined => {
      if (!canEditWaypoints) {
        return undefined;
      }

      return {
        onPointerCancel: handleEditorEdgePointerCancel,
        onPointerDown: (event, flowPosition) =>
          handleEditorEdgePointerDown(edgeId, event, flowPosition),
        onPointerMove: handleEditorEdgePointerMove,
        onPointerUp: handleEditorEdgePointerUp,
      };
    },
    [
      canEditWaypoints,
      handleEditorEdgePointerCancel,
      handleEditorEdgePointerDown,
      handleEditorEdgePointerMove,
      handleEditorEdgePointerUp,
    ],
  );

  return {
    canEditWaypoints,
    clearSelectedWaypointEdge: () => setSelectedWaypointEdgeId(null),
    handleAddSelectedEdgeWaypoint,
    handleEditorEdgeClick,
    handleEditorEdgeDoubleClick,
    edgePointerInteraction,
    edgeWaypointPreviews,
    handleMoveSelectedEdgeWaypoint,
    handleRemoveSelectedEdgeWaypoint,
    selectedEdgeDescription,
    selectedEdgeWaypoints,
    selectedWaypointEdgeId,
    waypointAriaLabel: messages.edgeWaypointHandleLabel,
    waypointControls: selectedWaypointEdgeId
      ? {
          addWaypointLabel: messages.edgeWaypointAddLabel,
          edgeKindLabel: selectedEdgeDescription?.edgeKindLabel ?? "",
          edgeSourceLabel: selectedEdgeDescription?.edgeSourceLabel ?? "",
          edgeTargetLabel: selectedEdgeDescription?.edgeTargetLabel ?? "",
          fieldKindLabel: messages.edgeWaypointKindLabel,
          fieldSourceLabel: messages.edgeWaypointSourceLabel,
          fieldTargetLabel: messages.edgeWaypointTargetLabel,
          onAddWaypoint: () => handleAddSelectedEdgeWaypoint(),
          onRemoveWaypoint: (waypointIndex: number) =>
            handleRemoveSelectedEdgeWaypoint(
              selectedWaypointEdgeId,
              waypointIndex,
            ),
          removeWaypointLabel: messages.edgeWaypointRemoveLabel,
          selectedEdgeLabel: messages.edgeWaypointSelectedLabel,
          waypointCount: selectedEdgeWaypoints.length,
        }
      : null,
  };
}
