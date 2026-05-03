import { useId, useState } from "react";
import { cx } from "../../lib/cx";
import { GraphSemanticIcon } from "../flowchart/graph-semantic-icon";
import type { GraphSemanticIconKind } from "../flowchart/graph-semantic-icon";
import {
  EXHAUSTION_WORKSTATION_ICON_METADATA,
  SUPPORTED_WORKSTATION_ICON_METADATA,
} from "../flowchart/workstation-icon-metadata";

export interface DashboardFlowAxisLegendEdgeItem {
  id: string;
  label: string;
  tone: "active" | "failure";
}

export interface DashboardFlowAxisLegendIconItem {
  iconClassName: string;
  kind: GraphSemanticIconKind;
  label: string;
}

export interface DashboardFlowAxisLegendProps {
  ariaLabel?: string;
  className?: string;
  defaultExpanded?: boolean;
  edgeItems: readonly DashboardFlowAxisLegendEdgeItem[];
  iconItems: readonly DashboardFlowAxisLegendIconItem[];
}

export const DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS = [
  { id: "active-flow", label: "Active flow", tone: "active" },
  { id: "failure-path", label: "Failure path", tone: "failure" },
] satisfies readonly DashboardFlowAxisLegendEdgeItem[];

export const DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS = [
  { iconClassName: "text-af-ink/58", kind: "queue", label: "Queue" },
  { iconClassName: "text-af-info/78", kind: "processing", label: "Processing" },
  { iconClassName: "text-af-success-ink/76", kind: "terminal", label: "Terminal" },
  { iconClassName: "text-af-danger-ink/78", kind: "failed", label: "Failed state" },
  { iconClassName: "text-af-success-ink/76", kind: "resource", label: "Resource" },
  { iconClassName: "text-af-info/74", kind: "constraint", label: "Constraint" },
  { iconClassName: "text-af-danger-ink/74", kind: "limit", label: "Limit" },
  ...SUPPORTED_WORKSTATION_ICON_METADATA.map((metadata) => ({
    iconClassName: metadata.className,
    kind: metadata.iconKind,
    label: metadata.label,
  })),
  { iconClassName: "text-af-success-ink", kind: "active-work", label: "Active work" },
  {
    iconClassName: EXHAUSTION_WORKSTATION_ICON_METADATA.className,
    kind: EXHAUSTION_WORKSTATION_ICON_METADATA.iconKind,
    label: EXHAUSTION_WORKSTATION_ICON_METADATA.label,
  },
] satisfies readonly DashboardFlowAxisLegendIconItem[];

const DEFAULT_CONTAINER_CLASS =
  "pointer-events-none z-10 flex flex-col items-start gap-2 max-[720px]:items-stretch";
const TOGGLE_BUTTON_CLASS =
  "dashboard-eyebrow pointer-events-auto inline-flex items-center gap-2 rounded-full border border-af-accent/35 bg-af-surface/92 px-[0.8rem] py-[0.55rem] text-af-accent shadow-af-card backdrop-blur-[14px] transition-colors hover:bg-af-overlay/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/45";
const PANEL_CLASS =
  "dashboard-body-sm pointer-events-auto max-w-[28rem] rounded-lg border border-af-overlay/8 bg-af-surface/88 px-3 py-3 text-af-ink/78 shadow-af-card backdrop-blur-[14px] max-[720px]:w-full max-[720px]:max-w-none";
const PANEL_HEADER_CLASS = "mb-2 flex items-center justify-between gap-3";
const PANEL_TITLE_CLASS =
  "dashboard-eyebrow m-0 text-af-accent";
const COLLAPSE_BUTTON_CLASS =
  "dashboard-eyebrow shrink-0 cursor-pointer rounded-full border border-af-accent/35 bg-af-accent/10 px-[0.7rem] py-[0.45rem] text-af-accent";
const ITEMS_LIST_CLASS =
  "m-0 grid list-none grid-cols-2 gap-x-3 gap-y-2 p-0 max-[520px]:grid-cols-1";

function normalizeLabelForAction(ariaLabel: string): string {
  return ariaLabel.charAt(0).toLowerCase() + ariaLabel.slice(1);
}

function edgeSwatchClassName(tone: DashboardFlowAxisLegendEdgeItem["tone"]): string {
  if (tone === "active") {
    return "bg-af-success";
  }

  return "bg-af-danger";
}

function LegendToggleGlyph() {
  return (
    <svg
      aria-hidden="true"
      className="h-3.5 w-3.5 shrink-0"
      fill="none"
      viewBox="0 0 16 16"
      xmlns="http://www.w3.org/2000/svg"
    >
      <circle cx="3.5" cy="4" fill="currentColor" r="1.15" />
      <circle cx="3.5" cy="8" fill="currentColor" r="1.15" />
      <circle cx="3.5" cy="12" fill="currentColor" r="1.15" />
      <path
        d="M6.25 4H13M6.25 8H13M6.25 12H13"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.4"
      />
    </svg>
  );
}

function DashboardFlowAxisLegendItems({
  edgeItems,
  iconItems,
}: Pick<DashboardFlowAxisLegendProps, "edgeItems" | "iconItems">) {
  return (
    <ul className={ITEMS_LIST_CLASS}>
      {edgeItems.map((item) => (
        <li className="flex items-center gap-2" data-legend-edge={item.id} key={item.id}>
          <span
            className={cx("h-[0.18rem] w-7 rounded-full", edgeSwatchClassName(item.tone))}
            data-legend-flow={item.tone === "active" ? "" : undefined}
          />
          <span className="dashboard-body-sm min-w-0 text-af-ink/78 [overflow-wrap:anywhere]">
            {item.label}
          </span>
        </li>
      ))}
      {iconItems.map((item) => (
        <li className="flex min-w-0 items-center gap-2" data-legend-icon={item.kind} key={item.kind}>
          <GraphSemanticIcon
            className={cx("h-4 w-4", item.iconClassName)}
            kind={item.kind}
            label={`${item.label} legend icon`}
          />
          <span className="dashboard-body-sm min-w-0 text-af-ink/78 [overflow-wrap:anywhere]">
            {item.label}
          </span>
        </li>
      ))}
    </ul>
  );
}

export function DashboardFlowAxisLegend({
  ariaLabel = "Graph legend",
  className,
  defaultExpanded = false,
  edgeItems,
  iconItems,
}: DashboardFlowAxisLegendProps) {
  const panelId = useId();
  const [expanded, setExpanded] = useState(defaultExpanded);
  const actionTargetLabel = normalizeLabelForAction(ariaLabel);

  return (
    <div
      className={cx(DEFAULT_CONTAINER_CLASS, className)}
      data-dashboard-flow-axis-legend=""
      data-legend-expanded={expanded ? "true" : "false"}
    >
      {expanded ? (
        <aside
          aria-label={ariaLabel}
          className={PANEL_CLASS}
          data-dashboard-flow-axis-legend-panel=""
          id={panelId}
        >
          <div className={PANEL_HEADER_CLASS}>
            <div className="flex min-w-0 items-center gap-2">
              <LegendToggleGlyph />
              <h3 className={PANEL_TITLE_CLASS}>{ariaLabel}</h3>
            </div>
            <button
              aria-controls={panelId}
              aria-expanded="true"
              aria-label={`Collapse ${actionTargetLabel}`}
              className={COLLAPSE_BUTTON_CLASS}
              data-dashboard-flow-axis-legend-toggle=""
              onClick={() => setExpanded(false)}
              type="button"
            >
              Collapse
            </button>
          </div>
          <DashboardFlowAxisLegendItems edgeItems={edgeItems} iconItems={iconItems} />
        </aside>
      ) : (
        <button
          aria-controls={panelId}
          aria-expanded="false"
          aria-label={`Expand ${actionTargetLabel}`}
          className={TOGGLE_BUTTON_CLASS}
          data-dashboard-flow-axis-legend-toggle=""
          onClick={() => setExpanded(true)}
          type="button"
        >
          <LegendToggleGlyph />
          <span>Legend</span>
        </button>
      )}
    </div>
  );
}

