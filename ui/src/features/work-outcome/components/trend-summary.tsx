import type { ComponentPropsWithoutRef, ReactNode } from "react";

import {
  DashboardDescriptionList,
  DashboardLabel,
  SurfacePanel,
} from "../../../components/ui";
import {
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";
import { cn } from "../../../lib/cn";

export function TrendSummaryGrid({
  children,
  className,
  ...props
}: ComponentPropsWithoutRef<"dl">) {
  return (
    <DashboardDescriptionList
      className={cn("mb-4 grid-cols-1 gap-3 md:grid-cols-3", className)}
      {...props}
    >
      {children}
    </DashboardDescriptionList>
  );
}

export function TrendSummaryMetric({
  label,
  value,
}: {
  label: string;
  value: ReactNode;
}) {
  return (
    <SurfacePanel asChild radius="lg" surface="low">
      <div>
        <DashboardLabel as="dt" className="mb-1">
          {label}
        </DashboardLabel>
        <WidgetSubtitle as="dd" className="m-0">
          {value}
        </WidgetSubtitle>
      </div>
    </SurfacePanel>
  );
}
