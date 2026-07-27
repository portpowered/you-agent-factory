import { useReactFlow, useStore } from "@xyflow/react";
import { useCallback, useRef, useState } from "react";
import { cn } from "../../../../../lib/cn";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type { FactoryLayoutPoint } from "../../../lib/layout/factory-graph-layout-operations";
import {
  type FactoryLayoutGroup,
  factoryLayoutGroupColorCssVariable,
  factoryLayoutGroupColorSurfaceCssVariable,
} from "../../../lib/layout/visual-groups/factory-graph-layout-groups";
import {
  isDragBeyondClickThreshold,
  RESIZE_CORNERS,
  type ResizeCorner,
  resizeBoundsFromCorner,
  resizeHandleStyle,
} from "./factory-graph-visual-group-layer-geometry";

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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: pointer drag sessions stay colocated with resize handles.
export function FactoryGraphVisualGroupLayer({
  canEdit = false,
  groupAriaLabel,
  groups,
  onMoveGroup,
  onResizeGroup,
  onSelectGroup,
  resizeHandleAriaLabel,
  selectedGroupId,
}: {
  canEdit?: boolean;
  groupAriaLabel: (group: FactoryLayoutGroup) => string;
  groups: readonly FactoryLayoutGroup[];
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
  resizeHandleAriaLabel: (corner: ResizeCorner) => string;
  selectedGroupId: string | null;
}) {
  const { getNodes, screenToFlowPosition, setNodes } = useReactFlow();
  const transform = useStore((state) => state.transform);
  const dragSessionRef = useRef<DragSession | null>(null);
  const [previewBoundsByGroupId, setPreviewBoundsByGroupId] = useState<
    Record<string, FactoryLayoutGroup["bounds"]>
  >({});
  const [translateX, translateY, zoom] = transform;

  const resolveBounds = useCallback(
    (group: FactoryLayoutGroup) =>
      previewBoundsByGroupId[group.id] ?? group.bounds,
    [previewBoundsByGroupId],
  );

  const projectGroupBounds = useCallback(
    (bounds: FactoryLayoutGroup["bounds"]) => ({
      height: bounds.height * zoom,
      width: bounds.width * zoom,
      x: bounds.x * zoom + translateX,
      y: bounds.y * zoom + translateY,
    }),
    [translateX, translateY, zoom],
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

  const handleGroupKeyDown = useCallback(
    (groupId: string) => (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (event.key !== "Enter" && event.key !== " ") {
        return;
      }

      event.preventDefault();
      event.stopPropagation();
      onSelectGroup(groupId);
    },
    [onSelectGroup],
  );

  const handleGroupPointerDown = useCallback(
    (group: FactoryLayoutGroup) =>
      (event: React.PointerEvent<HTMLDivElement>) => {
        if (!canEdit || !onMoveGroup) {
          return;
        }

        event.stopPropagation();
        event.preventDefault();

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
    [canEdit, getNodes, onMoveGroup, screenToFlowPosition],
  );

  const handleResizePointerDown = useCallback(
    (group: FactoryLayoutGroup, corner: ResizeCorner) =>
      (event: React.PointerEvent<HTMLButtonElement>) => {
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
    (event: React.PointerEvent<HTMLElement>) => {
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
    (event: React.PointerEvent<HTMLElement>) => {
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

  if (groups.length === 0) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-0 z-[1]"
      data-factory-visual-group-layer=""
    >
      {groups.map((group) => {
        const bounds = resolveBounds(group);
        const projected = projectGroupBounds(bounds);
        const selected = selectedGroupId === group.id;
        const accent = factoryLayoutGroupColorCssVariable(group.color);
        const fill = factoryLayoutGroupColorSurfaceCssVariable(group.color);

        return (
          <div
            className={cn(
              "pointer-events-auto absolute overflow-visible rounded-xl border-2 text-left shadow-none",
              selected
                ? "border-primary ring-2 ring-af-overlay-focus"
                : "border-outline",
            )}
            data-factory-visual-group={group.id}
            key={group.id}
            style={{
              backgroundColor: fill,
              borderColor: selected ? undefined : accent,
              height: projected.height,
              left: projected.x,
              top: projected.y,
              width: projected.width,
            }}
          >
            {/* biome-ignore lint/a11y/useSemanticElements: group body must not nest resize handle buttons. */}
            <div
              aria-label={groupAriaLabel(group)}
              aria-pressed={selected}
              className={cn(
                "h-full w-full cursor-grab overflow-hidden rounded-xl active:cursor-grabbing",
                "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary",
              )}
              data-factory-visual-group-body=""
              onKeyDown={handleGroupKeyDown(group.id)}
              onPointerDown={handleGroupPointerDown(group)}
              onPointerMove={handlePointerMove}
              onPointerUp={handlePointerUp}
              role="button"
              tabIndex={canEdit ? 0 : -1}
            >
              <span
                className="block truncate px-3 py-2 text-sm font-medium text-on-surface"
                data-factory-visual-group-label=""
              >
                {group.label?.trim() || group.id}
              </span>
            </div>
            {selected && canEdit && onResizeGroup
              ? RESIZE_CORNERS.map((corner) => (
                  <GraphNodeButton
                    aria-label={resizeHandleAriaLabel(corner)}
                    className={cn(
                      "pointer-events-auto absolute z-10 h-4 w-4 min-h-11 min-w-11 rounded-sm border-2 border-primary bg-surface shadow-sm sm:h-4 sm:w-4 sm:min-h-0 sm:min-w-0",
                    )}
                    data-factory-visual-group-resize={corner}
                    key={corner}
                    onPointerDown={handleResizePointerDown(group, corner)}
                    onPointerMove={handlePointerMove}
                    onPointerUp={handlePointerUp}
                    style={resizeHandleStyle(corner)}
                    tabIndex={-1}
                  />
                ))
              : null}
          </div>
        );
      })}
    </div>
  );
}
