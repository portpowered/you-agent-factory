import { useStore } from "@xyflow/react";

import { cn } from "../../../../lib/cn";
import {
  type FactoryLayoutGroup,
  factoryLayoutGroupColorCssVariable,
  factoryLayoutGroupColorSurfaceCssVariable,
} from "../../lib/layout/factory-graph-layout-groups";
export function FactoryGraphVisualGroupLayer({
  groups,
  onSelectGroup,
  selectedGroupId,
  groupAriaLabel,
}: {
  groupAriaLabel: (group: FactoryLayoutGroup) => string;
  groups: readonly FactoryLayoutGroup[];
  onSelectGroup: (groupId: string) => void;
  selectedGroupId: string | null;
}) {
  const transform = useStore((state) => state.transform);
  const [translateX, translateY, zoom] = transform;

  if (groups.length === 0) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-0 z-[1]"
      data-factory-visual-group-layer=""
    >
      {groups.map((group) => {
        const projected = projectGroupBounds(group.bounds, {
          translateX,
          translateY,
          zoom,
        });
        const selected = selectedGroupId === group.id;
        const accent = factoryLayoutGroupColorCssVariable(group.color);
        const fill = factoryLayoutGroupColorSurfaceCssVariable(group.color);

        return (
          <button
            aria-label={groupAriaLabel(group)}
            aria-pressed={selected}
            className={cn(
              "pointer-events-auto absolute overflow-hidden rounded-xl border-2 text-left shadow-none",
              "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary",
              selected
                ? "border-primary ring-2 ring-af-overlay-focus"
                : "border-outline",
            )}
            data-factory-visual-group={group.id}
            key={group.id}
            onClick={(event) => {
              event.stopPropagation();
              onSelectGroup(group.id);
            }}
            style={{
              backgroundColor: fill,
              borderColor: selected ? undefined : accent,
              height: projected.height,
              left: projected.x,
              top: projected.y,
              width: projected.width,
            }}
            type="button"
          >
            <span
              className="block truncate px-3 py-2 text-sm font-medium text-on-surface"
              data-factory-visual-group-label=""
            >
              {group.label?.trim() || group.id}
            </span>
          </button>
        );
      })}
    </div>
  );
}

function projectGroupBounds(
  bounds: FactoryLayoutGroup["bounds"],
  transform: { translateX: number; translateY: number; zoom: number },
) {
  const x = bounds.x * transform.zoom + transform.translateX;
  const y = bounds.y * transform.zoom + transform.translateY;
  return {
    height: bounds.height * transform.zoom,
    width: bounds.width * transform.zoom,
    x,
    y,
  };
}
