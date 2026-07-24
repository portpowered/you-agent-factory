import { WidgetSubtitle } from "@you-agent-factory/components/recipes";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { DescriptionList, Label, SurfacePanel } from "../../../components/ui";
import { cn } from "../../../lib/cn";

export function TrendSummaryGrid({
  children,
  className,
  ...props
}: ComponentPropsWithoutRef<"dl">) {
  return (
    <DescriptionList
      className={cn("mb-4 grid-cols-1 gap-3 md:grid-cols-3", className)}
      {...props}
    >
      {children}
    </DescriptionList>
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
        <Label as="dt" className="mb-1">
          {label}
        </Label>
        <WidgetSubtitle as="dd" className="m-0">
          {value}
        </WidgetSubtitle>
      </div>
    </SurfacePanel>
  );
}
