import type { ComponentPropsWithoutRef, ReactNode } from "react";

import { SurfacePanel } from "@you-agent-factory/components/layout";
import { cn } from "../../../../lib/cn";

type FactoryGraphEditorFloatingSurfacePlacement =
  | "bottomToolbar"
  | "topRight"
  | "topRightInset";

interface FactoryGraphEditorFloatingSurfaceProps
  extends ComponentPropsWithoutRef<"section"> {
  children: ReactNode;
  placement: FactoryGraphEditorFloatingSurfacePlacement;
}

const FLOATING_SURFACE_BASE_CLASS =
  "pointer-events-auto absolute z-20 flex flex-wrap items-center gap-2 shadow-af-panel backdrop-blur-[16px]";
const FLOATING_SURFACE_PLACEMENT_CLASS: Record<
  FactoryGraphEditorFloatingSurfacePlacement,
  string
> = {
  bottomToolbar:
    "bottom-4 left-1/2 -translate-x-1/2 flex-nowrap overflow-hidden max-md:bottom-3 max-md:left-4 max-md:right-4 max-md:justify-start max-md:gap-1.5 max-md:translate-x-0 max-md:flex-nowrap",
  topRight: "right-7 top-7 max-md:left-4 max-md:right-4 max-md:top-4",
  topRightInset: "right-4 top-4 max-md:left-4 max-md:right-4 max-md:top-4",
};

export function FactoryGraphEditorFloatingSurface({
  children,
  className,
  placement,
  ...props
}: FactoryGraphEditorFloatingSurfaceProps) {
  return (
    <SurfacePanel
      asChild
      className={cn(
        FLOATING_SURFACE_BASE_CLASS,
        FLOATING_SURFACE_PLACEMENT_CLASS[placement],
        className,
      )}
      padding="compact"
      radius="full"
    >
      <section {...props}>{children}</section>
    </SurfacePanel>
  );
}
