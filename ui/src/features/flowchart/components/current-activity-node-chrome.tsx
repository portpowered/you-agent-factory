import type { ComponentPropsWithoutRef, ReactNode } from "react";
import {
  factoryGraphNodeSurfaceClassName,
  factoryGraphNodeTitleClassName,
  type FactoryGraphNodeSurfaceTone,
} from "@you-agent-factory/factory-graph";

import { DashboardStatusPill } from "../../../components/ui/dashboard-status-pill";
import { cn } from "../../../lib/cn";


type ActivityGraphNodeBadgeTone =
  | "danger"
  | "info"
  | "neutral"
  | "success"
  | "warning";
type ActivityGraphNodeBadgeWeight = "body" | "label";
export type ActivityGraphNodeSurfaceTone = FactoryGraphNodeSurfaceTone;

const BADGE_WEIGHT_CLASS_NAME: Record<ActivityGraphNodeBadgeWeight, string> = {
  body: "font-mono text-[0.68rem]",
  // hardcoded-ui-copy-exception: non-product-diagnostic
  label: "text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
};
export function activityGraphNodeTitleClassName(className?: string) {
  return factoryGraphNodeTitleClassName(className);
}

export function activityGraphNodeSurfaceClassName(
  tone: ActivityGraphNodeSurfaceTone,
) {
  return factoryGraphNodeSurfaceClassName(tone);
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
