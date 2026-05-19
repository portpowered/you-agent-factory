import { forwardRef } from "react";

import { Button, type ButtonProps } from "../../components/ui/button";
import { cn } from "../../lib/cn";

const DASHBOARD_HEADER_ACTION_BUTTON_CLASS = "shrink-0";
const DASHBOARD_HEADER_ACTION_BUTTON_COMPACT_CLASS =
  "h-10 w-10 rounded-lg px-0 py-0";

export interface DashboardHeaderActionButtonProps extends ButtonProps {
  compact?: boolean;
}

export const DashboardHeaderActionButton =
  forwardRef<HTMLButtonElement, DashboardHeaderActionButtonProps>(
    function DashboardHeaderActionButton(
      { className, compact = false, size = "icon", tone = "outline", ...props },
      ref,
    ) {
      return (
        <Button
          className={cn(
            DASHBOARD_HEADER_ACTION_BUTTON_CLASS,
            compact && DASHBOARD_HEADER_ACTION_BUTTON_COMPACT_CLASS,
            className,
          )}
          data-dashboard-header-action="neutral"
          ref={ref}
          size={size}
          tone={tone}
          {...props}
        />
      );
    },
  );
