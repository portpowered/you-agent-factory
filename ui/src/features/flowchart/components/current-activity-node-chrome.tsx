import type { ComponentPropsWithoutRef, ReactNode } from "react";

import { cn } from "../../../lib/cn";

const ACTIVITY_GRAPH_NODE_TITLE_CLASS_NAME =
  "block min-w-0 truncate whitespace-nowrap font-bold leading-tight text-af-ink";

type ActivityGraphNodeBadgeTone =
  | "danger"
  | "info"
  | "neutral"
  | "success"
  | "warning";
type ActivityGraphNodeBadgeWeight = "body" | "label";

const BADGE_TONE_CLASS_NAME: Record<ActivityGraphNodeBadgeTone, string> = {
  danger: "border-af-danger-border bg-af-danger-surface text-af-danger-ink",
  info: "border-af-info-border bg-af-info-surface text-af-info",
  neutral: "border-af-border bg-af-surface-subtle text-af-text-muted",
  success: "border-af-success-border bg-af-success-surface text-af-success-ink",
  warning: "border-af-warning-border bg-af-warning-surface text-af-warning-ink",
};

const BADGE_WEIGHT_CLASS_NAME: Record<ActivityGraphNodeBadgeWeight, string> = {
  body: "font-mono text-[0.68rem]",
  label: "text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
};

export function activityGraphNodeTitleClassName(className?: string) {
  return cn(ACTIVITY_GRAPH_NODE_TITLE_CLASS_NAME, className);
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
    <span
      className={cn(
        "inline-flex w-fit items-center gap-1 rounded-full border px-2 py-0.5 leading-none",
        BADGE_TONE_CLASS_NAME[tone],
        BADGE_WEIGHT_CLASS_NAME[weight],
        className,
      )}
      {...rest}
    >
      {children}
    </span>
  );
}
