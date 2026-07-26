import { forwardRef, type HTMLAttributes } from "react";

import {
  DashboardStatusPill,
  type DashboardStatusPillTone,
} from "../../../../../components/ui/dashboard-status-pill";
import { cn } from "../../../../../lib/cn";

interface CurrentSelectionPillBaseProps
  extends Omit<
    HTMLAttributes<HTMLSpanElement>,
    "onClick" | "onKeyDown" | "onKeyUp" | "tabIndex"
  > {
  tone?: DashboardStatusPillTone;
}

export const CurrentSelectionExecutionPill = forwardRef<
  HTMLSpanElement,
  CurrentSelectionPillBaseProps
>(function CurrentSelectionExecutionPill(
  { className, tone = "info", ...props },
  ref,
) {
  return (
    <DashboardStatusPill
      className={cn("min-h-0 px-2 py-0.5", className)}
      ref={ref}
      typography="supportingCode"
      tone={tone}
      {...props}
    />
  );
});

export const CurrentSelectionBadge = forwardRef<
  HTMLSpanElement,
  CurrentSelectionPillBaseProps
>(function CurrentSelectionBadge(
  { className, tone = "active", ...props },
  ref,
) {
  return (
    <DashboardStatusPill
      className={cn("min-h-0 px-2 py-0.5", className)}
      ref={ref}
      typography="supportingText"
      tone={tone}
      {...props}
    />
  );
});
