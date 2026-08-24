import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../../lib/cn";
import {
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "./dashboard-typography";

const DASHBOARD_STATUS_PILL_BASE_CLASS =
  "inline-flex items-center justify-center gap-2 rounded-full border font-semibold";
const DASHBOARD_STATUS_PILL_SIZE_CLASS = {
  compact: "min-h-6 px-2 py-0.5 text-xs",
  default: "min-h-8 px-3 py-1 text-xs",
} as const;

/** Non-interactive status labels. Use accent tones for brand emphasis, semantic tones for state meaning only. */
const DASHBOARD_STATUS_PILL_TONE_CLASS = {
  active:
    "border-primary bg-primary-container text-on-primary factory-light:text-on-primary-container",
  danger: "border-af-danger-border bg-error-container text-on-error-container",
  info: "border-af-info-border bg-info-container text-on-info-container",
  neutral: "border-outline bg-surface-container-low text-on-surface-variant",
  success:
    "border-af-success-border bg-success-container text-on-success-container",
  warning:
    "border-af-warning-border bg-warning-container text-on-warning-container factory-light:bg-warning factory-light:text-on-warning",
} as const;
const DASHBOARD_STATUS_PILL_TYPOGRAPHY_CLASS = {
  none: null,
  supportingCode: DASHBOARD_SUPPORTING_CODE_CLASS,
  supportingText: DASHBOARD_SUPPORTING_TEXT_CLASS,
} as const;

export type DashboardStatusPillTone =
  keyof typeof DASHBOARD_STATUS_PILL_TONE_CLASS;
export type DashboardStatusPillSize =
  keyof typeof DASHBOARD_STATUS_PILL_SIZE_CLASS;
export type DashboardStatusPillTypography =
  keyof typeof DASHBOARD_STATUS_PILL_TYPOGRAPHY_CLASS;

export interface DashboardStatusPillProps
  extends Omit<
    HTMLAttributes<HTMLSpanElement>,
    "onClick" | "onKeyDown" | "onKeyUp" | "tabIndex"
  > {
  size?: DashboardStatusPillSize;
  typography?: DashboardStatusPillTypography;
  tone?: DashboardStatusPillTone;
}

export const DashboardStatusPill = forwardRef<
  HTMLSpanElement,
  DashboardStatusPillProps
>(function DashboardStatusPill(
  {
    className,
    role,
    size = "default",
    tone = "neutral",
    typography = "none",
    ...props
  },
  ref,
) {
  return (
    <span
      className={cn(
        DASHBOARD_STATUS_PILL_BASE_CLASS,
        DASHBOARD_STATUS_PILL_SIZE_CLASS[size],
        DASHBOARD_STATUS_PILL_TONE_CLASS[tone],
        DASHBOARD_STATUS_PILL_TYPOGRAPHY_CLASS[typography],
        className,
      )}
      ref={ref}
      role={role}
      {...props}
    />
  );
});
