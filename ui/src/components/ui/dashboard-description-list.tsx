import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../../lib/cn";
import { DASHBOARD_BODY_TEXT_CLASS } from "./dashboard-typography";

export interface DashboardDescriptionListProps
  extends HTMLAttributes<HTMLDListElement> {}

export const DashboardDescriptionList = forwardRef<
  HTMLDListElement,
  DashboardDescriptionListProps
>(function DashboardDescriptionList({ className, ...props }, ref) {
  return (
    <dl
      className={cn(
        "m-0 grid gap-1.5 [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:gap-2",
        DASHBOARD_BODY_TEXT_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});
