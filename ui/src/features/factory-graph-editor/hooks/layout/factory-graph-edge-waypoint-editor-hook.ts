import type { XYPosition } from "@xyflow/react";
import { useCallback, useMemo, useState } from "react";

import type { FactoryGraphEditorTool } from "../../components/controls/factory-graph-editor-controls";
import { flowMidpointBetweenNodes } from "../../components/flow/factory-graph-edge-waypoint-layer";
import { describeFactoryGraphLayoutEdgeId } from "../../lib/layout/factory-graph-layout-edge-labels";
import { factoryLayoutEdgeWaypoints } from "../../lib/layout/factory-graph-layout-edge-waypoints";
import type { FactoryLayout } from "../../lib/layout/factory-graph-layout-operations";
import { getFactoryGraphEditorMessages } from "../../messages/editor";

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
  const [selectedWaypointEdgeId, setSelectedWaypointEdgeId] = useState<
    string | null
  >(null);
  const canEditWaypoints = canEditFactoryGraphEdgeWaypoints(input);

  const selectedEdgeWaypoints = useMemo(
    () =>
      selectedWaypointEdgeId
        ? (factoryLayoutEdgeWaypoints(input.layout, selectedWaypointEdgeId) ??
          [])
        : [],
    [input.layout, selectedWaypointEdgeId],
  );

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

  const handleEditorEdgeClick = useCallback(
    (edgeId: string) => {
      if (!input.canInteractWithEditor || !input.editorMode) {
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

  return {
    canEditWaypoints,
    clearSelectedWaypointEdge: () => setSelectedWaypointEdgeId(null),
    handleAddSelectedEdgeWaypoint,
    handleEditorEdgeClick,
    handleEditorEdgeDoubleClick,
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
