import { Background, Controls, type FitViewOptions } from "@xyflow/react";
import type { CSSProperties, HTMLAttributes, ReactNode } from "react";

import { cn } from "../../lib/cn";

const DASHBOARD_GRAPH_BACKGROUND_COLOR = "var(--color-outline)";
const DASHBOARD_GRAPH_BACKGROUND_GAP = 24;
const DASHBOARD_GRAPH_BACKGROUND_SIZE = 1;

type CSSPropertiesWithVariables = CSSProperties &
  Record<`--${string}`, string | number>;

const DASHBOARD_GRAPH_CONTROLS_STYLE: CSSPropertiesWithVariables = {
  "--xy-controls-box-shadow": "none",
  "--xy-controls-button-background-color":
    "var(--color-surface-container-high)",
  "--xy-controls-button-background-color-hover-props":
    "var(--color-af-overlay)",
  "--xy-controls-button-background-color-props":
    "var(--color-surface-container-high)",
  "--xy-controls-button-background-color-hover": "var(--color-af-overlay)",
  "--xy-controls-button-border-color": "var(--color-outline)",
  "--xy-controls-button-border-color-props": "var(--color-outline)",
  "--xy-controls-button-color": "var(--color-on-surface-variant)",
  "--xy-controls-button-color-hover": "var(--color-on-surface)",
  "--xy-controls-button-color-hover-props": "var(--color-on-surface)",
  "--xy-controls-button-color-props": "var(--color-on-surface-variant)",
  backgroundColor: "var(--color-surface)",
  border: "1px solid var(--color-outline)",
  borderRadius: 8,
  overflow: "hidden",
};

interface DashboardGraphFrameProps extends HTMLAttributes<HTMLElement> {
  children: ReactNode;
}

export function DashboardGraphFrame({
  children,
  className,
  role,
  ...props
}: DashboardGraphFrameProps) {
  return (
    <section
      className={cn(
        "relative min-h-0 w-full min-w-0 overflow-hidden rounded-3xl border shadow-none transition-colors",
        className,
      )}
      data-dashboard-graph-frame="true"
      role={role ?? "region"}
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
