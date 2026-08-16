import { useStore } from "@xyflow/react";
import { FactoryGraphGroupRegionLayer } from "@you-agent-factory/factory-graph/group-regions";
import { useCallback } from "react";
import { cn } from "../../../../../lib/cn";
import type { FactoryLayoutPoint } from "../../../lib/layout/factory-graph-layout-operations";
import type { FactoryLayoutGroup } from "../../../lib/layout/visual-groups/factory-graph-layout-groups";
import type {
  GroupOutlineEdge,
  ResizeCorner,
} from "./factory-graph-visual-group-layer-geometry";
import { FactoryGraphVisualGroupAffordances } from "./interaction/factory-graph-visual-group-affordances";
import { useFactoryGraphVisualGroupLayerInteractions } from "./interaction/factory-graph-visual-group-layer-interactions";

export function FactoryGraphVisualGroupLayer({
  canEdit = false,
  groupAriaLabel,
  groupOutlineAriaLabel,
  groups,
  onMoveGroup,
  onResizeGroup,
  onSelectGroup,
  resizeHandleAriaLabel,
  selectedGroupId,
}: {
  canEdit?: boolean;
  groupAriaLabel: (group: FactoryLayoutGroup) => string;
  groupOutlineAriaLabel?: (
    group: FactoryLayoutGroup,
    edge: GroupOutlineEdge,
  ) => string;
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
  const transform = useStore((state) => state.transform);
  const [translateX, translateY, zoom] = transform;
  const {
    handleGroupAffordanceKeyDown,
    handleGroupPointerDown,
    handlePointerCancel,
    handlePointerMove,
    handlePointerUp,
    handleResizeKeyDown,
    handleResizePointerDown,
    resolveBounds,
  } = useFactoryGraphVisualGroupLayerInteractions({
    canEdit,
    onMoveGroup,
    onResizeGroup,
    onSelectGroup,
    selectedGroupId,
  });

  const projectGroupBounds = useCallback(
    (bounds: FactoryLayoutGroup["bounds"]) => ({
      height: bounds.height * zoom,
      width: bounds.width * zoom,
      x: bounds.x * zoom + translateX,
      y: bounds.y * zoom + translateY,
    }),
    [translateX, translateY, zoom],
  );

  const resolvedGroups = groups.map((group) => ({
    ...group,
    bounds: resolveBounds(group),
  }));

  if (resolvedGroups.length === 0) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-0 z-[5]"
      data-factory-visual-group-layer=""
    >
      <FactoryGraphGroupRegionLayer groups={resolvedGroups} />
      {resolvedGroups.map((group) => {
        const projected = projectGroupBounds(group.bounds);
        const selected = selectedGroupId === group.id;
        const accessibleLabel =
          groupAriaLabel(group) || group.label?.trim() || group.id;

        return (
          <div
            className={cn(
              "pointer-events-none absolute overflow-visible rounded-2xl text-left",
              selected
                ? "border-2 border-primary ring-2 ring-af-overlay-focus"
                : undefined,
            )}
            data-factory-visual-group={group.id}
            key={group.id}
            style={{
              height: projected.height,
              left: projected.x,
              top: projected.y,
              width: projected.width,
            }}
          >
            <FactoryGraphVisualGroupAffordances
              accessibleLabel={accessibleLabel}
              canEdit={canEdit}
              groupOutlineAriaLabel={
                groupOutlineAriaLabel
                  ? (edge) => groupOutlineAriaLabel(group, edge)
                  : undefined
              }
              handleGroupAffordanceKeyDown={handleGroupAffordanceKeyDown(group)}
              handleGroupPointerDown={handleGroupPointerDown(group)}
              handlePointerCancel={handlePointerCancel}
              handlePointerMove={handlePointerMove}
              handlePointerUp={handlePointerUp}
              handleResizeKeyDown={(corner) =>
                handleResizeKeyDown(group, corner)
              }
              handleResizePointerDown={(corner) =>
                handleResizePointerDown(group, corner)
              }
              resizeHandleAriaLabel={resizeHandleAriaLabel}
              selected={selected}
              showResizeHandles={Boolean(onResizeGroup) && selected && canEdit}
            />
          </div>
        );
      })}
    </div>
  );
}
