import { forwardRef } from "react";

import {
  DashboardActionButton,
  type DashboardActionButtonProps,
} from "../../../components/ui";
import { cn } from "../../../lib/cn";

const DASHBOARD_HEADER_ACTION_BUTTON_CLASS = "shrink-0";

export interface DashboardHeaderActionButtonProps
  extends DashboardActionButtonProps {
  compact?: boolean;
}

export const DashboardHeaderActionButton =
  forwardRef<HTMLButtonElement, DashboardHeaderActionButtonProps>(
    function DashboardHeaderActionButton(
      { className, compact = false, iconOnly, tone = "outline", ...props },
      ref,
    ) {
      return (
        <DashboardActionButton
          className={cn(DASHBOARD_HEADER_ACTION_BUTTON_CLASS, className)}
          data-dashboard-header-action="neutral"
          iconOnly={compact || iconOnly}
          ref={ref}
          tone={tone}
          {...props}
        />
      );
    },
  );
