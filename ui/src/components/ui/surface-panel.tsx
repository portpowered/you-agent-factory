import { Slot } from "@radix-ui/react-slot";
import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../../lib/cn";

type SurfacePanelPadding = "compact" | "default" | "none";
type SurfacePanelRadius = "lg" | "xl" | "2xl" | "full";
type SurfacePanelSurface = "high" | "low";
type SurfacePanelTone = "default" | "accent" | "selected";

export interface SurfacePanelProps extends HTMLAttributes<HTMLDivElement> {
  asChild?: boolean;
  padding?: SurfacePanelPadding;
  radius?: SurfacePanelRadius;
  surface?: SurfacePanelSurface;
  tone?: SurfacePanelTone;
}

const SURFACE_PANEL_BASE_CLASS = "border border-outline";
const SURFACE_PANEL_PADDING_CLASS: Record<SurfacePanelPadding, string> = {
  compact: "p-2",
  default: "p-3",
  none: "",
};
const SURFACE_PANEL_RADIUS_CLASS: Record<SurfacePanelRadius, string> = {
  lg: "rounded-lg",
  xl: "rounded-xl",
  "2xl": "rounded-2xl",
  full: "rounded-full",
};
const SURFACE_PANEL_SURFACE_CLASS: Record<SurfacePanelSurface, string> = {
  high: "bg-surface-container-high",
  low: "bg-surface-container-low",
};
const SURFACE_PANEL_TONE_CLASS: Record<SurfacePanelTone, string> = {
  default: "",
  accent: "border-primary text-on-surface",
  selected: "border-primary bg-primary-container text-on-primary-container",
};

export function surfacePanelVariants({
  className,
  padding = "default",
  radius = "xl",
  surface = "high",
  tone = "default",
}: Pick<
  SurfacePanelProps,
  "className" | "padding" | "radius" | "surface" | "tone"
>) {
  return cn(
    SURFACE_PANEL_BASE_CLASS,
    SURFACE_PANEL_PADDING_CLASS[padding],
    SURFACE_PANEL_RADIUS_CLASS[radius],
    SURFACE_PANEL_SURFACE_CLASS[surface],
    SURFACE_PANEL_TONE_CLASS[tone],
    className,
  );
}

export const SurfacePanel = forwardRef<HTMLDivElement, SurfacePanelProps>(
  function SurfacePanel(
    {
      asChild = false,
      className,
      padding = "default",
      radius = "xl",
      surface = "high",
      tone = "default",
      ...props
    },
    ref,
  ) {
    const Component = asChild ? Slot : "div";

    return (
      <Component
        className={surfacePanelVariants({
          className,
          padding,
          radius,
          surface,
          tone,
        })}
        ref={ref}
        {...props}
      />
    );
  },
);
