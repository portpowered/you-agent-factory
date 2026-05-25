import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../../lib/cn";

const DASHBOARD_STATUS_PILL_BASE_CLASS =
  "inline-flex min-h-8 items-center justify-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold";
const DASHBOARD_STATUS_PILL_TONE_CLASS = {
  active: "border-af-accent-border bg-af-accent-surface text-af-text",
  danger: "border-af-danger-border bg-af-danger-surface text-af-danger-text",
  neutral: "border-af-border bg-af-surface-subtle text-af-text-muted",
  warning: "border-af-warning-border bg-af-warning-surface text-af-warning-text",
};

export interface DashboardStatusPillProps
  extends Omit<
    HTMLAttributes<HTMLSpanElement>,
    "onClick" | "onKeyDown" | "onKeyUp" | "tabIndex"
  > {
  tone?: keyof typeof DASHBOARD_STATUS_PILL_TONE_CLASS;
}

export const DashboardStatusPill = forwardRef<
  HTMLSpanElement,
  DashboardStatusPillProps
>(function DashboardStatusPill(
  { className, role, tone = "neutral", ...props },
  ref,
) {
  return (
    <span
      className={cn(
        DASHBOARD_STATUS_PILL_BASE_CLASS,
        DASHBOARD_STATUS_PILL_TONE_CLASS[tone],
        className,
      )}
      ref={ref}
      role={role}
      {...props}
    />
  );
});
