import { useId, useState } from "react";

import {
  DashboardActionButton,
  type DashboardActionButtonProps,
} from "../../../components/ui";

const INLINE_TOOLTIP_CLASS =
  "pointer-events-none absolute left-1/2 top-full z-30 mt-2 -translate-x-1/2 rounded-xl border border-af-border-strong bg-af-surface-raised px-3 py-2 text-center text-xs font-medium text-af-text shadow-af-panel";

export function FactoryGraphEditorTooltipActionButton({
  children,
  tooltip,
  ...props
}: DashboardActionButtonProps & {
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
        <span className={INLINE_TOOLTIP_CLASS} id={tooltipID} role="tooltip">
          {tooltip}
        </span>
      ) : null}
    </div>
  );
}
