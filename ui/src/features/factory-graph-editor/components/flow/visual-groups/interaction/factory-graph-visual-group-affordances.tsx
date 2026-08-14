import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type { KeyboardEvent, PointerEvent } from "react";
import {
  GROUP_OUTLINE_EDGES,
  type GroupOutlineEdge,
  groupOutlineAffordanceStyle,
  RESIZE_CORNERS,
  type ResizeCorner,
  resizeHandleStyle,
} from "../factory-graph-visual-group-layer-geometry";

export function FactoryGraphVisualGroupAffordances({
  accessibleLabel,
  canEdit,
  groupOutlineAriaLabel,
  handleGroupAffordanceKeyDown,
  handleGroupPointerDown,
  handlePointerMove,
  handlePointerUp,
  handleResizeKeyDown,
  handleResizePointerDown,
  resizeHandleAriaLabel,
  selected,
  showResizeHandles,
}: {
  accessibleLabel: string;
  canEdit: boolean;
  groupOutlineAriaLabel?: (edge: GroupOutlineEdge) => string;
  handleGroupAffordanceKeyDown: (
    event: KeyboardEvent<HTMLButtonElement>,
  ) => void;
  handleGroupPointerDown: (event: PointerEvent<HTMLButtonElement>) => void;
  handlePointerMove: (event: PointerEvent<HTMLElement>) => void;
  handlePointerUp: (event: PointerEvent<HTMLElement>) => void;
  handleResizeKeyDown: (
    corner: ResizeCorner,
  ) => (event: KeyboardEvent<HTMLButtonElement>) => void;
  handleResizePointerDown: (
    corner: ResizeCorner,
  ) => (event: PointerEvent<HTMLButtonElement>) => void;
  resizeHandleAriaLabel: (corner: ResizeCorner) => string;
  selected: boolean;
  showResizeHandles: boolean;
}) {
  return (
    <>
      <div
        aria-hidden="true"
        className="pointer-events-none h-full w-full overflow-hidden rounded-xl"
        data-factory-visual-group-body=""
      ></div>
      <GraphNodeButton
        aria-label={accessibleLabel}
        aria-pressed={selected}
        className="pointer-events-auto absolute -top-3 right-3 left-3 z-10 h-6 rounded-lg border-0 bg-transparent focus-visible:ring-2 focus-visible:ring-af-focus-ring"
        data-factory-visual-group-label=""
        onKeyDown={handleGroupAffordanceKeyDown}
        onPointerDown={handleGroupPointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        tabIndex={canEdit ? 0 : -1}
      >
        <span className="sr-only">{accessibleLabel}</span>
      </GraphNodeButton>
      {GROUP_OUTLINE_EDGES.map((edge) => (
        <GraphNodeButton
          aria-label={groupOutlineAriaLabel?.(edge) ?? accessibleLabel}
          aria-pressed={selected}
          className="pointer-events-auto absolute z-10 cursor-grab bg-transparent active:cursor-grabbing"
          data-factory-visual-group-outline={edge}
          key={edge}
          onKeyDown={handleGroupAffordanceKeyDown}
          onPointerDown={handleGroupPointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          style={groupOutlineAffordanceStyle(edge)}
          tabIndex={canEdit ? 0 : -1}
        >
          <span className="sr-only">{accessibleLabel}</span>
        </GraphNodeButton>
      ))}
      {showResizeHandles
        ? RESIZE_CORNERS.map((corner) => (
            <GraphNodeButton
              aria-label={resizeHandleAriaLabel(corner)}
              className="pointer-events-auto absolute z-10 h-4 w-4 min-h-11 min-w-11 rounded-sm border-2 border-primary bg-surface shadow-sm sm:h-4 sm:w-4 sm:min-h-0 sm:min-w-0"
              data-factory-visual-group-resize={corner}
              key={corner}
              onKeyDown={handleResizeKeyDown(corner)}
              onPointerDown={handleResizePointerDown(corner)}
              onPointerMove={handlePointerMove}
              onPointerUp={handlePointerUp}
              style={resizeHandleStyle(corner)}
            />
          ))
        : null}
    </>
  );
}
