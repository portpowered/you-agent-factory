import { ChevronDown } from "lucide-react";

import { cn } from "../../lib/cn";

export interface ExpandablePanelIconProps {
  expanded: boolean;
  className?: string;
}

const EXPANDABLE_PANEL_ICON_CLASS = "h-3.5 w-3.5 shrink-0 text-current";

export function ExpandablePanelIcon({
  className,
  expanded,
}: ExpandablePanelIconProps) {
  return (
    <ChevronDown
      aria-hidden="true"
      className={cn(
        EXPANDABLE_PANEL_ICON_CLASS,
        "transition-transform duration-150",
        expanded ? "rotate-180" : "rotate-0",
        className,
      )}
      focusable="false"
      strokeWidth={1.8}
    />
  );
}
