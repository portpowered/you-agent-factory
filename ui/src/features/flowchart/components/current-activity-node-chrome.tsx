import type { ComponentPropsWithoutRef, ReactNode } from "react";

import { DashboardStatusPill } from "../../../components/ui/dashboard-status-pill";
import { cn } from "../../../lib/cn";

const ACTIVITY_GRAPH_NODE_TITLE_CLASS_NAME =
  "block min-w-0 truncate whitespace-nowrap font-bold leading-tight text-on-surface";

type ActivityGraphNodeBadgeTone =
  | "danger"
  | "info"
  | "neutral"
  | "success"
  | "warning";
type ActivityGraphNodeBadgeWeight = "body" | "label";
export type ActivityGraphNodeSurfaceTone =
  | "danger"
  | "info"
  | "neutral"
  | "neutralHigh"
  | "primary"
  | "resource"
  | "success"
  | "warning"
  | "workState"
  | "workstation";

const BADGE_WEIGHT_CLASS_NAME: Record<ActivityGraphNodeBadgeWeight, string> = {
  body: "font-mono text-[0.68rem]",
  // hardcoded-ui-copy-exception: non-product-diagnostic
  label: "text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
};
const SURFACE_TONE_CLASS_NAME: Record<ActivityGraphNodeSurfaceTone, string> = {
  danger: "border-af-danger-border bg-error-container",
  // hardcoded-ui-copy-exception: non-product-diagnostic
  info: "border-info-border bg-info-container",
  neutral: "border-outline bg-surface",
  neutralHigh: "border-outline-variant bg-surface-container-high",
  primary: "border-primary bg-primary-container",
  resource: "border-outline bg-background",
  success: "border-af-success-border bg-success-container",
  warning: "border-af-warning-border bg-warning-container",
  // hardcoded-ui-copy-exception: non-product-diagnostic
  workState: "border-info-border bg-info-container",
  workstation: "border-outline-variant bg-surface-container-highest",
};

export function activityGraphNodeTitleClassName(className?: string) {
  return cn(ACTIVITY_GRAPH_NODE_TITLE_CLASS_NAME, className);
}

export function activityGraphNodeSurfaceClassName(
  tone: ActivityGraphNodeSurfaceTone,
) {
  return SURFACE_TONE_CLASS_NAME[tone];
}

export function ActivityGraphNodeBadge({
  children,
  className,
  tone = "neutral",
  weight = "body",
  ...rest
}: ComponentPropsWithoutRef<"span"> & {
  children: ReactNode;
  tone?: ActivityGraphNodeBadgeTone;
  weight?: ActivityGraphNodeBadgeWeight;
}) {
  return (
    <DashboardStatusPill
      className={cn(
        "w-fit gap-1 px-2 py-0.5 leading-none",
        BADGE_WEIGHT_CLASS_NAME[weight],
        className,
      )}
      size="compact"
      tone={tone}
      typography="none"
      {...rest}
    >
      {children}
    </DashboardStatusPill>
  );
}
