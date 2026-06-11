import { useReactFlow, useStore } from "@xyflow/react";
import { useCallback, useRef, useState } from "react";

import { cn } from "../../../../lib/cn";
import {
  FACTORY_LAYOUT_GROUP_MIN_SIZE,
  type FactoryLayoutGroup,
  factoryLayoutGroupColorCssVariable,
  factoryLayoutGroupColorSurfaceCssVariable,
} from "../../lib/layout/factory-graph-layout-groups";
import type { FactoryLayoutPoint } from "../../lib/layout/factory-graph-layout-operations";

const DRAG_CLICK_THRESHOLD_PX = 4;

type ResizeCorner = "ne" | "nw" | "se" | "sw";

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
        const delta = {
          x: currentFlowPosition.x - dragSession.startFlowPosition.x,
          y: currentFlowPosition.y - dragSession.startFlowPosition.y,
        };
        if (
          !dragSession.moved &&
          Math.hypot(
            event.clientX - dragSession.startScreenPosition.x,
            event.clientY - dragSession.startScreenPosition.y,
          ) >= DRAG_CLICK_THRESHOLD_PX
        ) {
          dragSession.moved = true;
        }

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
    [onMoveGroup, onResizeGroup, onSelectGroup, screenToFlowPosition],
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
                  <button
                    aria-label={resizeHandleAriaLabel(corner)}
                    className={cn(
                      "pointer-events-auto absolute z-10 h-4 w-4 min-h-11 min-w-11 rounded-sm border-2 border-primary bg-surface shadow-sm sm:h-4 sm:w-4 sm:min-h-0 sm:min-w-0",
                      "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary",
                    )}
                    data-factory-visual-group-resize={corner}
                    key={corner}
                    onPointerDown={handleResizePointerDown(group, corner)}
                    onPointerMove={handlePointerMove}
                    onPointerUp={handlePointerUp}
                    style={resizeHandleStyle(corner)}
                    type="button"
                  />
                ))
              : null}
          </div>
        );
      })}
    </div>
  );
}

const RESIZE_CORNERS: readonly ResizeCorner[] = ["nw", "ne", "sw", "se"];

function resizeHandleStyle(corner: ResizeCorner): React.CSSProperties {
  switch (corner) {
    case "nw":
      return { cursor: "nwse-resize", left: -8, top: -8 };
    case "ne":
      return { cursor: "nesw-resize", right: -8, top: -8 };
    case "sw":
      return { bottom: -8, cursor: "nesw-resize", left: -8 };
    case "se":
      return { bottom: -8, cursor: "nwse-resize", right: -8 };
  }
}

function resizeBoundsFromCorner(
  startBounds: FactoryLayoutGroup["bounds"],
  corner: ResizeCorner,
  delta: FactoryLayoutPoint,
): FactoryLayoutGroup["bounds"] {
  const minWidth = FACTORY_LAYOUT_GROUP_MIN_SIZE.width;
  const minHeight = FACTORY_LAYOUT_GROUP_MIN_SIZE.height;

  switch (corner) {
    case "se":
      return {
        height: Math.max(minHeight, startBounds.height + delta.y),
        width: Math.max(minWidth, startBounds.width + delta.x),
        x: startBounds.x,
        y: startBounds.y,
      };
    case "sw": {
      const width = Math.max(minWidth, startBounds.width - delta.x);
      const widthDelta = width - startBounds.width;
      return {
        height: Math.max(minHeight, startBounds.height + delta.y),
        width,
        x: startBounds.x - widthDelta,
        y: startBounds.y,
      };
    }
    case "ne": {
      const height = Math.max(minHeight, startBounds.height - delta.y);
      const heightDelta = height - startBounds.height;
      return {
        height,
        width: Math.max(minWidth, startBounds.width + delta.x),
        x: startBounds.x,
        y: startBounds.y - heightDelta,
      };
    }
    case "nw": {
      const width = Math.max(minWidth, startBounds.width - delta.x);
      const height = Math.max(minHeight, startBounds.height - delta.y);
      const widthDelta = width - startBounds.width;
      const heightDelta = height - startBounds.height;
      return {
        height,
        width,
        x: startBounds.x - widthDelta,
        y: startBounds.y - heightDelta,
      };
    }
  }
}
