import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../../lib/cn";

const DASHBOARD_STATUS_PILL_BASE_CLASS =
  "inline-flex min-h-8 items-center justify-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold";

/** Non-interactive status labels. Use accent tones for brand emphasis, semantic tones for state meaning only. */
const DASHBOARD_STATUS_PILL_TONE_CLASS = {
  active: "border-af-accent-border bg-af-accent-surface text-on-surface",
  danger: "border-af-danger-border bg-af-danger-surface text-af-danger-text",
  info: "border-af-info-border bg-af-info-surface text-af-info-text",
  neutral: "border-outline bg-surface-container-low text-on-surface-variant",
  success:
    "border-af-success-border bg-af-success-surface text-af-success-text",
  warning:
    "border-af-warning-border bg-af-warning-surface text-af-warning-text",
} as const;

export type DashboardStatusPillTone =
  keyof typeof DASHBOARD_STATUS_PILL_TONE_CLASS;

export interface DashboardStatusPillProps
  extends Omit<
    HTMLAttributes<HTMLSpanElement>,
    "onClick" | "onKeyDown" | "onKeyUp" | "tabIndex"
  > {
  tone?: DashboardStatusPillTone;
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
