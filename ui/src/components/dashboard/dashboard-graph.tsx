import {
  Background,
  Controls,
  type FitViewOptions,
} from "@xyflow/react";
import type { CSSProperties, HTMLAttributes, ReactNode } from "react";

import { cn } from "../../lib/cn";

const DASHBOARD_GRAPH_BACKGROUND_COLOR = "var(--color-af-edge-muted-soft)";
const DASHBOARD_GRAPH_BACKGROUND_GAP = 24;
const DASHBOARD_GRAPH_BACKGROUND_SIZE = 1;

type CSSPropertiesWithVariables = CSSProperties &
  Record<`--${string}`, string | number>;

const DASHBOARD_GRAPH_CONTROLS_STYLE: CSSPropertiesWithVariables = {
  "--xy-controls-box-shadow": "none",
  "--xy-controls-button-background-color-hover-props":
    "rgb(from var(--color-af-overlay) r g b / 0.1)",
  "--xy-controls-button-background-color-props":
    "rgb(from var(--color-af-surface) r g b / 0.94)",
  "--xy-controls-button-border-color-props":
    "rgb(from var(--color-af-overlay) r g b / 0.08)",
  "--xy-controls-button-color-hover-props": "var(--color-af-ink)",
  "--xy-controls-button-color-props":
    "rgb(from var(--color-af-ink) r g b / 0.72)",
  backgroundColor: "rgb(from var(--color-af-surface) r g b / 0.88)",
  border: "1px solid rgb(from var(--color-af-overlay) r g b / 0.08)",
  borderRadius: 8,
  overflow: "hidden",
};

interface DashboardGraphFrameProps extends HTMLAttributes<HTMLElement> {
  children: ReactNode;
}

export function DashboardGraphFrame({
  children,
  className,
  ...props
}: DashboardGraphFrameProps) {
  return (
    <section
      className={cn(
        "relative h-full min-h-0 overflow-hidden rounded-3xl border transition-colors",
        className,
      )}
      data-dashboard-graph-frame="true"
      {...props}
    >
      {children}
    </section>
  );
}

export function DashboardGraphBackground() {
  return (
    <Background
      color={DASHBOARD_GRAPH_BACKGROUND_COLOR}
      gap={DASHBOARD_GRAPH_BACKGROUND_GAP}
      size={DASHBOARD_GRAPH_BACKGROUND_SIZE}
    />
  );
}

export function DashboardGraphControls({
  fitViewOptions,
}: {
  fitViewOptions: FitViewOptions;
}) {
  return (
    <Controls
      fitViewOptions={fitViewOptions}
      showInteractive={false}
      style={DASHBOARD_GRAPH_CONTROLS_STYLE}
    />
  );
}
