import { useId, useState } from "react";

import {
  DashboardActionButton,
  type DashboardActionButtonProps,
} from "../../../../components/ui/dashboard-action-button";
import { cn } from "../../../../lib/cn";

export type FactoryGraphEditorTooltipPlacement = "above" | "below";

const TOOLTIP_PLACEMENT_CLASS: Record<
  FactoryGraphEditorTooltipPlacement,
  string
> = {
  above: "bottom-full mb-2",
  below: "top-full mt-2",
};

export function FactoryGraphEditorTooltipActionButton({
  children,
  placement = "below",
  tooltip,
  ...props
}: DashboardActionButtonProps & {
  placement?: FactoryGraphEditorTooltipPlacement;
  tooltip: string;
}) {
  const tooltipID = useId();
  const [tooltipVisible, setTooltipVisible] = useState(false);

  return (
    <div className="relative inline-flex">
      <DashboardActionButton
        {...props}
        aria-describedby={tooltipVisible ? tooltipID : undefined}
        onBlur={(event) => {
          props.onBlur?.(event);
          setTooltipVisible(false);
        }}
        onFocus={(event) => {
          props.onFocus?.(event);
          setTooltipVisible(true);
        }}
        onMouseEnter={(event) => {
          props.onMouseEnter?.(event);
          setTooltipVisible(true);
        }}
        onMouseLeave={(event) => {
          props.onMouseLeave?.(event);
          setTooltipVisible(false);
        }}
      >
        {children}
      </DashboardActionButton>
      {tooltipVisible ? (
        <span
          className={cn(
            "pointer-events-none absolute left-1/2 z-30 -translate-x-1/2 rounded-xl border border-outline-variant bg-surface-container-high px-3 py-2 text-center text-xs font-medium text-on-surface shadow-af-panel",
            TOOLTIP_PLACEMENT_CLASS[placement],
          )}
          id={tooltipID}
          role="tooltip"
        >
          {tooltip}
        </span>
      ) : null}
    </div>
  );
}
