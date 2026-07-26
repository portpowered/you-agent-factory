import { useId, useState } from "react";
import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Label, Text } from "@you-agent-factory/components/primitives";
import {
  DisclosureButton,
  type DisclosureButtonProps,
} from "../../../components/ui/disclosure-button";
import { ExpandablePanelIcon } from "../../../components/ui/expandable-panel-icon";
import { cn } from "../../../lib/cn";
import {
  WORK_STATE_PHASE_LEGEND_ORDER,
  workStatePhaseSwatchClassName,
} from "../../factory-graph-editor/lib/work-state/factory-graph-work-state-phase-styling";
import type { FactoryGraphWorkStateType } from "../../factory-graph-editor/lib/work-state/factory-graph-work-state-type";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  EXHAUSTION_WORKSTATION_ICON_METADATA,
  SUPPORTED_WORKSTATION_ICON_METADATA,
} from "../../flowchart/lib/workstation-icon-metadata";
import {
  GraphSemanticIcon,
  type GraphSemanticIconKind,
} from "../../flowchart/components/graph-semantic-icon";
import { getDashboardFlowAxisLegendMessages } from "../messages/dashboard-flow-axis-legend";

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

export interface DashboardFlowAxisLegendPhaseItem {
  id: FactoryGraphWorkStateType;
  label: string;
  swatchClassName: string;
}

export interface DashboardFlowAxisLegendProps {
  ariaLabel?: string;
  className?: string;
  defaultExpanded?: boolean;
  edgeItems: readonly DashboardFlowAxisLegendEdgeItem[];
  iconItems: readonly DashboardFlowAxisLegendIconItem[];
  phaseItems: readonly DashboardFlowAxisLegendPhaseItem[];
  locale?: string;
}

export function getDefaultDashboardFlowAxisLegendEdgeItems(
  locale?: string,
): readonly DashboardFlowAxisLegendEdgeItem[] {
  const messages = getDashboardFlowAxisLegendMessages(locale);

  return [
    {
      id: "active-flow",
      label: messages.edgeLabels.activeFlow,
      tone: "active",
    },
    {
      id: "failure-path",
      label: messages.edgeLabels.failurePath,
      tone: "failure",
    },
  ];
}

export function getDefaultDashboardFlowAxisLegendIconItems(
  locale?: string,
): readonly DashboardFlowAxisLegendIconItem[] {
  const messages = getDashboardFlowAxisLegendMessages(locale);

  return [
    {
      iconClassName: "text-on-surface-subtle",
      kind: "queue",
      label: messages.iconLabels.queue,
    },
    {
      iconClassName: "text-info",
      kind: "processing",
      label: messages.iconLabels.processing,
    },
    {
      iconClassName: "text-success",
      kind: "terminal",
      label: messages.iconLabels.terminal,
    },
    {
      iconClassName: "text-error",
      kind: "failed",
      label: messages.iconLabels.failed,
    },
    {
      iconClassName: "text-success",
      kind: "resource",
      label: messages.iconLabels.resource,
    },
    {
      iconClassName: "text-info",
      kind: "constraint",
      label: messages.iconLabels.constraint,
    },
    {
      iconClassName: "text-error",
      kind: "limit",
      label: messages.iconLabels.limit,
    },
    ...SUPPORTED_WORKSTATION_ICON_METADATA.map((metadata) => ({
      iconClassName: metadata.className,
      kind: metadata.iconKind,
      label: messages.iconLabels[metadata.iconKind],
    })),
    {
      iconClassName: "text-success",
      kind: "active-work",
      label: messages.iconLabels["active-work"],
    },
    {
      iconClassName: EXHAUSTION_WORKSTATION_ICON_METADATA.className,
      kind: EXHAUSTION_WORKSTATION_ICON_METADATA.iconKind,
      label: messages.iconLabels[EXHAUSTION_WORKSTATION_ICON_METADATA.iconKind],
    },
  ];
}

export function getDefaultDashboardFlowAxisLegendPhaseItems(
  locale?: string,
): readonly DashboardFlowAxisLegendPhaseItem[] {
  const messages = getFactoryGraphEditorMessages(locale);

  return WORK_STATE_PHASE_LEGEND_ORDER.map((phase) => ({
    id: phase,
    label: messages.workStatePhaseLegendLabel(phase),
    swatchClassName: workStatePhaseSwatchClassName(phase),
  }));
}

type DashboardFlowAxisLegendToggleProps = DisclosureButtonProps;

function DashboardFlowAxisLegendToggle({
  children,
  className,
  ...props
}: DashboardFlowAxisLegendToggleProps) {
  return (
    <DisclosureButton
      className={cn("dashboard-eyebrow", className)}
      size="pill"
      tone="outline"
      {...props}
    >
      {children}
    </DisclosureButton>
  );
}

function normalizeLabelForAction(ariaLabel: string): string {
  return ariaLabel.charAt(0).toLowerCase() + ariaLabel.slice(1);
}

function edgeSwatchClassName(
  tone: DashboardFlowAxisLegendEdgeItem["tone"],
): string {
  if (tone === "active") {
    return "bg-success";
  }

  return "bg-error";
}

function DashboardFlowAxisLegendItems({
  edgeItems,
  iconItems,
  locale,
  phaseItems,
}: Pick<
  DashboardFlowAxisLegendProps,
  "edgeItems" | "iconItems" | "locale" | "phaseItems"
>) {
  const messages = getDashboardFlowAxisLegendMessages(locale);
  const editorMessages = getFactoryGraphEditorMessages(locale);

  return (
    <div className="flex flex-col gap-3">
      {edgeItems.length > 0 ? (
        <ul className="m-0 grid list-none grid-cols-1 gap-x-3 gap-y-2 p-0 sm:grid-cols-2">
          {edgeItems.map((item) => (
            <li
              className="flex items-center gap-2"
              data-legend-edge={item.id}
              key={item.id}
            >
              <span
                className={cn(
                  "h-1 w-7 rounded-full",
                  edgeSwatchClassName(item.tone),
                )}
                data-legend-flow={item.tone === "active" ? "" : undefined}
              />
              <Text
                as="span"
                className="min-w-0 text-on-surface-variant [overflow-wrap:anywhere]"
              >
                {item.label}
              </Text>
            </li>
          ))}
        </ul>
      ) : null}
      {phaseItems.length > 0 ? (
        <section aria-label={editorMessages.workStatePhaseLegendAriaLabel}>
          <ul className="m-0 grid list-none grid-cols-1 gap-x-3 gap-y-2 p-0 sm:grid-cols-2">
            {phaseItems.map((item) => (
              <li
                className="flex items-center gap-2"
                data-legend-phase={item.id}
                key={item.id}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    "h-3 w-3 shrink-0 rounded-sm border",
                    item.swatchClassName,
                  )}
                />
                <Text
                  as="span"
                  className="min-w-0 text-on-surface-variant [overflow-wrap:anywhere]"
                >
                  {item.label}
                </Text>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {iconItems.length > 0 ? (
        <ul className="m-0 grid list-none grid-cols-1 gap-x-3 gap-y-2 p-0 sm:grid-cols-2">
          {iconItems.map((item) => (
            <li
              className="flex min-w-0 items-center gap-2"
              data-legend-icon={item.kind}
              key={item.kind}
            >
              <GraphSemanticIcon
                className={cn("h-4 w-4", item.iconClassName)}
                kind={item.kind}
                label={messages.iconLabel(item.label)}
              />
              <Text
                as="span"
                className="min-w-0 text-on-surface-variant [overflow-wrap:anywhere]"
              >
                {item.label}
              </Text>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function DashboardFlowAxisLegend({
  ariaLabel,
  className,
  defaultExpanded = false,
  edgeItems,
  iconItems,
  phaseItems,
  locale,
}: DashboardFlowAxisLegendProps) {
  const messages = getDashboardFlowAxisLegendMessages(locale);
  const panelId = useId();
  const [expanded, setExpanded] = useState(defaultExpanded);
  const resolvedAriaLabel = ariaLabel ?? messages.title;
  const actionTargetLabel = normalizeLabelForAction(resolvedAriaLabel);

  return (
    <div
      className={cn(
        "pointer-events-none z-10 flex flex-col items-stretch gap-2 md:items-start",
        className,
      )}
      data-dashboard-flow-axis-legend=""
      data-legend-expanded={expanded ? "true" : "false"}
    >
      {expanded ? (
        <SurfacePanel
          aria-label={resolvedAriaLabel}
          asChild
          className="dashboard-body-sm pointer-events-auto w-full text-on-surface-variant shadow-af-card backdrop-blur-md md:max-w-md"
          data-dashboard-flow-axis-legend-panel=""
          id={panelId}
          radius="lg"
        >
          <aside>
            <div className="mb-2 flex items-center justify-between gap-3">
              <div className="flex min-w-0 items-center gap-2">
                <Label as="h3" className="m-0 text-primary">
                  {resolvedAriaLabel}
                </Label>
              </div>
              <DashboardFlowAxisLegendToggle
                aria-label={messages.collapseToggleLabel(actionTargetLabel)}
                className="shrink-0"
                controlsID={panelId}
                data-dashboard-flow-axis-legend-toggle=""
                expanded={true}
                onClick={() => setExpanded(false)}
              >
                <ExpandablePanelIcon expanded={true} />
                {messages.collapseLabel}
              </DashboardFlowAxisLegendToggle>
            </div>
            <DashboardFlowAxisLegendItems
              edgeItems={edgeItems}
              iconItems={iconItems}
              locale={locale}
              phaseItems={phaseItems}
            />
          </aside>
        </SurfacePanel>
      ) : (
        <DashboardFlowAxisLegendToggle
          aria-label={messages.expandToggleLabel(actionTargetLabel)}
          className="pointer-events-auto shadow-af-card backdrop-blur-md"
          controlsID={panelId}
          data-dashboard-flow-axis-legend-toggle=""
          expanded={false}
          onClick={() => setExpanded(true)}
        >
          <ExpandablePanelIcon expanded={false} />
          <span>{messages.minimizedLabel}</span>
        </DashboardFlowAxisLegendToggle>
      )}
    </div>
  );
}
