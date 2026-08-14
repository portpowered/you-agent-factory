import { useReactFlow } from "@xyflow/react";
import type { KeyboardEvent, PointerEvent } from "react";
import { useCallback, useRef, useState } from "react";
import type { FactoryLayoutPoint } from "../../../../lib/layout/factory-graph-layout-operations";
import type { FactoryLayoutGroup } from "../../../../lib/layout/visual-groups/factory-graph-layout-groups";
import {
  GROUP_KEYBOARD_MOVE_STEP,
  GROUP_KEYBOARD_RESIZE_STEP,
  isDragBeyondClickThreshold,
  keyboardDelta,
  type ResizeCorner,
  resizeBoundsFromCorner,
} from "../factory-graph-visual-group-layer-geometry";

type DragSession =
  | {
      kind: "move";
      groupId: string;
      memberNodeIds: readonly string[];
      pointerId: number;
      startFlowPosition: FactoryLayoutPoint;
      startMemberPositions: Map<string, FactoryLayoutPoint>;
      startBounds: FactoryLayoutGroup["bounds"];
      startScreenPosition: FactoryLayoutPoint;
      moved: boolean;
    }
  | {
      kind: "resize";
      corner: ResizeCorner;
      groupId: string;
      pointerId: number;
      startBounds: FactoryLayoutGroup["bounds"];
      startFlowPosition: FactoryLayoutPoint;
    };

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: pointer and keyboard sessions share one React Flow interaction boundary.
export function useFactoryGraphVisualGroupLayerInteractions({
  canEdit,
  onMoveGroup,
  onResizeGroup,
  onSelectGroup,
  selectedGroupId,
}: {
  canEdit: boolean;
  onMoveGroup?: (
    groupId: string,
    delta: FactoryLayoutPoint,
    startMemberPositions: ReadonlyMap<string, FactoryLayoutPoint>,
  ) => void;
  onResizeGroup?: (
    groupId: string,
    bounds: FactoryLayoutGroup["bounds"],
  ) => void;
  onSelectGroup: (groupId: string) => void;
  selectedGroupId: string | null;
}) {
  const { getNodes, screenToFlowPosition, setNodes } = useReactFlow();
  const dragSessionRef = useRef<DragSession | null>(null);
  const [previewBoundsByGroupId, setPreviewBoundsByGroupId] = useState<
    Record<string, FactoryLayoutGroup["bounds"]>
  >({});

  const resolveBounds = useCallback(
    (group: FactoryLayoutGroup) =>
      previewBoundsByGroupId[group.id] ?? group.bounds,
    [previewBoundsByGroupId],
  );

  const updateMemberNodePreview = useCallback(
    (
      memberNodeIds: readonly string[],
      startMemberPositions: ReadonlyMap<string, FactoryLayoutPoint>,
      delta: FactoryLayoutPoint,
    ) => {
      if (memberNodeIds.length === 0) {
        return;
      }

      const memberNodeIdSet = new Set(memberNodeIds);
      setNodes((currentNodes) =>
        currentNodes.map((node) => {
          const factoryGraphNodeId =
            (node.data as { factoryGraphNodeId?: string } | undefined)
              ?.factoryGraphNodeId ?? node.id;
          if (!memberNodeIdSet.has(factoryGraphNodeId)) {
            return node;
          }

          const startPosition = startMemberPositions.get(factoryGraphNodeId);
          if (!startPosition) {
            return node;
          }

          return {
            ...node,
            position: {
              x: startPosition.x + delta.x,
              y: startPosition.y + delta.y,
            },
          };
        }),
      );
    },
    [setNodes],
  );

  const handleGroupAffordanceKeyDown = useCallback(
    (group: FactoryLayoutGroup) =>
      (event: KeyboardEvent<HTMLButtonElement>) => {
        if (!canEdit) {
          return;
        }

        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          event.stopPropagation();
          onSelectGroup(group.id);
          return;
        }

        const delta = keyboardDelta(
          event.key,
          event.shiftKey
            ? GROUP_KEYBOARD_MOVE_STEP * 4
            : GROUP_KEYBOARD_MOVE_STEP,
        );
        if (!delta || !onMoveGroup) {
          return;
        }

        event.preventDefault();
        event.stopPropagation();
        if (selectedGroupId !== group.id) {
          onSelectGroup(group.id);
        }
        onMoveGroup(group.id, delta, new Map());
      },
    [canEdit, onMoveGroup, onSelectGroup, selectedGroupId],
  );

  const handleResizeKeyDown = useCallback(
    (group: FactoryLayoutGroup, corner: ResizeCorner) =>
      (event: KeyboardEvent<HTMLButtonElement>) => {
        if (!canEdit || !onResizeGroup) {
          return;
        }

        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          event.stopPropagation();
          onSelectGroup(group.id);
          return;
        }

        const delta = keyboardDelta(
          event.key,
          event.shiftKey
            ? GROUP_KEYBOARD_RESIZE_STEP * 4
            : GROUP_KEYBOARD_RESIZE_STEP,
        );
        if (!delta) {
          return;
        }

        event.preventDefault();
        event.stopPropagation();
        if (selectedGroupId !== group.id) {
          onSelectGroup(group.id);
        }
        onResizeGroup(
          group.id,
          resizeBoundsFromCorner(group.bounds, corner, delta),
        );
      },
    [canEdit, onResizeGroup, onSelectGroup, selectedGroupId],
  );

  const handleGroupPointerDown = useCallback(
    (group: FactoryLayoutGroup) => (event: PointerEvent<HTMLButtonElement>) => {
      if (!canEdit) {
        return;
      }

      event.stopPropagation();
      event.preventDefault();

      if (!onMoveGroup) {
        onSelectGroup(group.id);
        return;
      }

      const startFlowPosition = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });
      const memberNodeIds = group.nodeIds ?? [];
      const startMemberPositions = new Map<string, FactoryLayoutPoint>();
      for (const node of getNodes()) {
        const factoryGraphNodeId =
          (node.data as { factoryGraphNodeId?: string } | undefined)
            ?.factoryGraphNodeId ?? node.id;
        if (!memberNodeIds.includes(factoryGraphNodeId)) {
          continue;
        }

        startMemberPositions.set(factoryGraphNodeId, {
          x: node.position.x,
          y: node.position.y,
        });
      }

      dragSessionRef.current = {
        groupId: group.id,
        kind: "move",
        memberNodeIds,
        moved: false,
        pointerId: event.pointerId,
        startBounds: { ...group.bounds },
        startFlowPosition,
        startMemberPositions,
        startScreenPosition: {
          x: event.clientX,
          y: event.clientY,
        },
      };
      setPreviewBoundsByGroupId((current) => ({
        ...current,
        [group.id]: { ...group.bounds },
      }));
      event.currentTarget.setPointerCapture?.(event.pointerId);
    },
    [canEdit, getNodes, onMoveGroup, onSelectGroup, screenToFlowPosition],
  );

  const handleResizePointerDown = useCallback(
    (group: FactoryLayoutGroup, corner: ResizeCorner) =>
      (event: PointerEvent<HTMLButtonElement>) => {
        if (!canEdit || !onResizeGroup) {
          return;
        }

        event.stopPropagation();
        event.preventDefault();
        dragSessionRef.current = {
          corner,
          groupId: group.id,
          kind: "resize",
          pointerId: event.pointerId,
          startBounds: { ...group.bounds },
          startFlowPosition: screenToFlowPosition({
            x: event.clientX,
            y: event.clientY,
          }),
        };
        setPreviewBoundsByGroupId((current) => ({
          ...current,
          [group.id]: { ...group.bounds },
        }));
        event.currentTarget.setPointerCapture?.(event.pointerId);
      },
    [canEdit, onResizeGroup, screenToFlowPosition],
  );

  const handlePointerMove = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      const dragSession = dragSessionRef.current;
      if (!dragSession || dragSession.pointerId !== event.pointerId) {
        return;
      }

      const currentFlowPosition = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });

      if (dragSession.kind === "move") {
        if (
          !dragSession.moved &&
          isDragBeyondClickThreshold(dragSession.startScreenPosition, {
            x: event.clientX,
            y: event.clientY,
          })
        ) {
          dragSession.moved = true;
        }

        if (!dragSession.moved) {
          return;
        }

        const delta = {
          x: currentFlowPosition.x - dragSession.startFlowPosition.x,
          y: currentFlowPosition.y - dragSession.startFlowPosition.y,
        };
        const nextBounds = {
          ...dragSession.startBounds,
          x: dragSession.startBounds.x + delta.x,
          y: dragSession.startBounds.y + delta.y,
        };
        setPreviewBoundsByGroupId((current) => ({
          ...current,
          [dragSession.groupId]: nextBounds,
        }));
        updateMemberNodePreview(
          dragSession.memberNodeIds,
          dragSession.startMemberPositions,
          delta,
        );
        return;
      }

      const delta = {
        x: currentFlowPosition.x - dragSession.startFlowPosition.x,
        y: currentFlowPosition.y - dragSession.startFlowPosition.y,
      };
      const nextBounds = resizeBoundsFromCorner(
        dragSession.startBounds,
        dragSession.corner,
        delta,
      );
      setPreviewBoundsByGroupId((current) => ({
        ...current,
        [dragSession.groupId]: nextBounds,
      }));
    },
    [screenToFlowPosition, updateMemberNodePreview],
  );

  const handlePointerUp = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      const dragSession = dragSessionRef.current;
      if (!dragSession || dragSession.pointerId !== event.pointerId) {
        return;
      }

      dragSessionRef.current = null;
      if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
        event.currentTarget.releasePointerCapture?.(event.pointerId);
      }

      const currentFlowPosition = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });
      const delta = {
        x: currentFlowPosition.x - dragSession.startFlowPosition.x,
        y: currentFlowPosition.y - dragSession.startFlowPosition.y,
      };

      if (dragSession.kind === "move") {
        setPreviewBoundsByGroupId((current) => {
          const next = { ...current };
          delete next[dragSession.groupId];
          return next;
        });

        if (!dragSession.moved) {
          updateMemberNodePreview(
            dragSession.memberNodeIds,
            dragSession.startMemberPositions,
            { x: 0, y: 0 },
          );
          onSelectGroup(dragSession.groupId);
          return;
        }

        onMoveGroup?.(
          dragSession.groupId,
          delta,
          dragSession.startMemberPositions,
        );
        return;
      }

      const nextBounds = resizeBoundsFromCorner(
        dragSession.startBounds,
        dragSession.corner,
        delta,
      );
      setPreviewBoundsByGroupId((current) => {
        const next = { ...current };
        delete next[dragSession.groupId];
        return next;
      });
      onResizeGroup?.(dragSession.groupId, nextBounds);
    },
    [
      onMoveGroup,
      onResizeGroup,
      onSelectGroup,
      screenToFlowPosition,
      updateMemberNodePreview,
    ],
  );

  return {
    handleGroupAffordanceKeyDown,
    handleGroupPointerDown,
    handlePointerMove,
    handlePointerUp,
    handleResizeKeyDown,
    handleResizePointerDown,
    resolveBounds,
  };
}
