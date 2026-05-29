import type { ReactNode } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { getCurrentSelectionShellMessages } from "../../current-selection/messages/current-selection-shell";
import {
  CurrentSelectionWidget,
  type useCurrentSelection,
  type useCurrentSelectionDetails,
  type useSelectedProviderSessionState,
} from "../../current-selection/public";
import { InlineAddWidgetCard } from "../../dashboard-add-card/components/inline-add-widget-card";
import { getProviderSessionWidgetMessages } from "../../provider-session-detail/messages/provider-session-widget";
import { ProviderSessionWidget } from "../../provider-session-detail/public";
import { getSubmitWorkMessages } from "../../submit-work/messages/submit-work";
import { SubmitWorkWidget } from "../../submit-work/public";
import { getTerminalWorkMessages } from "../../terminal-work/messages/terminal-work";
import { TerminalWorkWidget } from "../../terminal-work/public";
import { getTraceDrilldownMessages } from "../../trace-drilldown/messages/trace-drilldown";
import type { useTraceDrilldown } from "../../trace-drilldown/hooks/useTraceDrilldown";
import { TraceDrilldownWidget } from "../../trace-drilldown/public";
import { getWorkOutcomeMessages } from "../../work-outcome/messages/work-outcome";
import {
  type useWorkOutcomeChart,
  WorkOutcomeWidget,
} from "../../work-outcome/public";
import { getWorkTotalsMessages } from "../../work-totals/messages/work-totals";
import { WorkTotalsWidget } from "../../work-totals/public";
import { getWorkflowActivityShellMessages } from "../../workflow-activity/messages/activity-shell";
import type { useCurrentActivityImportController } from "../../workflow-activity/hooks/current-activity-import-controller";
import {
  WorkflowActivityWidget,
} from "../../workflow-activity/public";
import { DASHBOARD_WIDGET_IDS } from "../hooks/useDashboardLayout";
import {
  type DashboardWidgetPickerWidgetType,
  getDashboardWidgetPickerAvailability,
} from "../lib/dashboard-widget-picker";
import type { AgentBentoLayoutCard, AgentBentoLayoutItem } from "./agent-bento";
import { DashboardWidgetRemoveButton } from "./dashboard-widget-remove-button";

export interface DashboardCardBuilderArgs {
  currentSelection: ReturnType<typeof useCurrentSelection>;
  dashboardLayout: AgentBentoLayoutItem[];
  importController: ReturnType<typeof useCurrentActivityImportController>;
  locale?: string;
  now: number;
  onRemoveDashboardWidget: (widgetInstanceID: string) => void;
  onSelectInlineWidget: (widgetType: DashboardWidgetPickerWidgetType) => void;
  providerSessionState: ReturnType<typeof useSelectedProviderSessionState>;
  selectedTrace: ReturnType<typeof useTraceDrilldown>["selectedTrace"];
  selectedTraceID: string | null;
  selectedWorkExecutionDetails: ReturnType<
    typeof useCurrentSelectionDetails
  >["selectedWorkExecutionDetails"];
  selectedWorkRelationshipGraph: ReturnType<
    typeof useCurrentSelectionDetails
  >["selectedWorkRelationshipGraph"];
  setSelectedTraceID: (traceID: string | null) => void;
  snapshot: DashboardSnapshot;
  traceGridState: ReturnType<typeof useTraceDrilldown>["traceGridState"];
  workChartModel: ReturnType<typeof useWorkOutcomeChart>;
}

interface DashboardWidgetCardBuilderArgs {
  currentSelection: ReturnType<typeof useCurrentSelection>;
  importController: ReturnType<typeof useCurrentActivityImportController>;
  layoutItem: AgentBentoLayoutItem;
  locale?: string;
  now: number;
  onRemoveDashboardWidget: (widgetInstanceID: string) => void;
  providerSessionState: ReturnType<typeof useSelectedProviderSessionState>;
  selectedTrace: ReturnType<typeof useTraceDrilldown>["selectedTrace"];
  selectedTraceID: string | null;
  selectedWorkExecutionDetails: ReturnType<
    typeof useCurrentSelectionDetails
  >["selectedWorkExecutionDetails"];
  selectedWorkRelationshipGraph: ReturnType<
    typeof useCurrentSelectionDetails
  >["selectedWorkRelationshipGraph"];
  setSelectedTraceID: (traceID: string | null) => void;
  snapshot: DashboardSnapshot;
  traceGridState: ReturnType<typeof useTraceDrilldown>["traceGridState"];
  workChartModel: ReturnType<typeof useWorkOutcomeChart>;
}

export function buildDashboardCards({
  currentSelection,
  dashboardLayout,
  importController,
  locale,
  now,
  onRemoveDashboardWidget,
  onSelectInlineWidget,
  providerSessionState,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  selectedWorkRelationshipGraph,
  setSelectedTraceID,
  snapshot,
  traceGridState,
  workChartModel,
}: DashboardCardBuilderArgs): AgentBentoLayoutCard[] {
  const pickerAvailability =
    getDashboardWidgetPickerAvailability(dashboardLayout);

  return dashboardLayout.flatMap((layoutItem) => {
    if (layoutItem.hidden) {
      return [];
    }

    if (layoutItem.widgetType === DASHBOARD_WIDGET_IDS.addWidget) {
      return [
        {
          id: layoutItem.id,
          widgetType: DASHBOARD_WIDGET_IDS.addWidget,
          children: (
            <InlineAddWidgetCard
              locale={locale}
              onSelectWidget={onSelectInlineWidget}
              pickerAvailability={pickerAvailability}
            />
          ),
        },
      ];
    }

    return [
      buildWidgetCard({
        currentSelection,
        importController,
        layoutItem,
        locale,
        now,
        onRemoveDashboardWidget,
        providerSessionState,
        selectedTrace,
        selectedTraceID,
        selectedWorkExecutionDetails,
        selectedWorkRelationshipGraph,
        setSelectedTraceID,
        snapshot,
        traceGridState,
        workChartModel,
      }),
    ];
  });
}

function buildWidgetCard({
  currentSelection,
  importController,
  layoutItem,
  locale,
  now,
  onRemoveDashboardWidget,
  providerSessionState,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  selectedWorkRelationshipGraph,
  setSelectedTraceID,
  snapshot,
  traceGridState,
  workChartModel,
}: DashboardWidgetCardBuilderArgs): AgentBentoLayoutCard {
  const removeAction = (
    <DashboardWidgetRemoveButton
      locale={locale}
      onClick={() => onRemoveDashboardWidget(layoutItem.id)}
      widgetTitle={getDashboardWidgetTitle(layoutItem.widgetType, locale)}
    />
  );

  if (
    layoutItem.widgetType === DASHBOARD_WIDGET_IDS.workTotals ||
    layoutItem.widgetType === DASHBOARD_WIDGET_IDS.workGraph
  ) {
    return buildOverviewWidgetCard({
      currentSelection,
      headerAction: removeAction,
      importController,
      layoutItem,
      locale,
      now,
      snapshot,
    });
  }

  if (
    layoutItem.widgetType === DASHBOARD_WIDGET_IDS.terminalWork ||
    layoutItem.widgetType === DASHBOARD_WIDGET_IDS.workOutcomeChart
  ) {
    return buildDuplicateCapableWidgetCard({
      currentSelection,
      headerAction: removeAction,
      layoutItem,
      locale,
      workChartModel,
    });
  }

  return buildSingletonWidgetCard({
    currentSelection,
    headerAction: removeAction,
    layoutItem,
    locale,
    now,
    providerSessionState,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    selectedWorkRelationshipGraph,
    setSelectedTraceID,
    snapshot,
    traceGridState,
  });
}

function buildOverviewWidgetCard({
  currentSelection,
  headerAction,
  importController,
  layoutItem,
  locale,
  now,
  snapshot,
}: Pick<
  DashboardWidgetCardBuilderArgs,
  | "currentSelection"
  | "importController"
  | "layoutItem"
  | "locale"
  | "now"
  | "snapshot"
> & {
  headerAction: ReactNode;
}): AgentBentoLayoutCard {
  switch (layoutItem.widgetType) {
    case DASHBOARD_WIDGET_IDS.workTotals:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <WorkTotalsWidget
            headerAction={headerAction}
            locale={locale}
            snapshot={snapshot}
          />
        ),
      };
    case DASHBOARD_WIDGET_IDS.workGraph:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <WorkflowActivityWidget
            headerAction={headerAction}
            importController={importController}
            locale={locale}
            now={now}
            onSelectStateNode={currentSelection.selectStateNode}
            onSelectWorkID={currentSelection.selectWorkByID}
            onSelectWorkstation={currentSelection.selectWorkstation}
            selection={currentSelection.selection}
            snapshot={snapshot}
            widgetInstanceID={layoutItem.id}
          />
        ),
      };
    default:
      throw new Error(
        `unsupported overview widget type: ${layoutItem.widgetType}`,
      );
  }
}

function buildDuplicateCapableWidgetCard({
  currentSelection,
  headerAction,
  layoutItem,
  locale,
  workChartModel,
}: Pick<
  DashboardWidgetCardBuilderArgs,
  "currentSelection" | "layoutItem" | "locale" | "workChartModel"
> & {
  headerAction: ReactNode;
}): AgentBentoLayoutCard {
  switch (layoutItem.widgetType) {
    case DASHBOARD_WIDGET_IDS.terminalWork:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <TerminalWorkWidget
            completedItems={currentSelection.completedWorkItems}
            failedItems={currentSelection.failedWorkItems}
            headerAction={headerAction}
            locale={locale}
            onSelectItem={currentSelection.openTerminalWorkDetail}
            selectedItem={currentSelection.terminalWorkDetail}
            widgetId={layoutItem.id}
          />
        ),
      };
    case DASHBOARD_WIDGET_IDS.workOutcomeChart:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <WorkOutcomeWidget
            headerAction={headerAction}
            locale={locale}
            model={workChartModel}
            widgetId={layoutItem.id}
          />
        ),
      };
    default:
      throw new Error(
        `unsupported duplicate-capable widget type: ${layoutItem.widgetType}`,
      );
  }
}

function buildSingletonWidgetCard({
  currentSelection,
  headerAction,
  layoutItem,
  locale,
  now,
  providerSessionState,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  selectedWorkRelationshipGraph,
  setSelectedTraceID,
  snapshot,
  traceGridState,
}: Pick<
  DashboardWidgetCardBuilderArgs,
  | "currentSelection"
  | "layoutItem"
  | "locale"
  | "now"
  | "providerSessionState"
  | "selectedTrace"
  | "selectedTraceID"
  | "selectedWorkExecutionDetails"
  | "selectedWorkRelationshipGraph"
  | "setSelectedTraceID"
  | "snapshot"
  | "traceGridState"
> & {
  headerAction: ReactNode;
}): AgentBentoLayoutCard {
  switch (layoutItem.widgetType) {
    case DASHBOARD_WIDGET_IDS.currentSelection:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <CurrentSelectionWidget
            activeTraceID={selectedTraceID ?? selectedTrace?.trace_id ?? null}
            currentSelection={currentSelection}
            failedWorkDetailsByWorkID={
              snapshot.runtime.session.failed_work_details_by_work_id
            }
            headerAction={headerAction}
            locale={locale}
            now={now}
            onSelectProviderSession={
              providerSessionState.setSelectedProviderSession
            }
            onSelectTraceID={setSelectedTraceID}
            selectedProviderSessionKey={
              providerSessionState.selectedProviderSessionKey
            }
            selectedTrace={selectedTrace}
            selectedWorkRelationshipGraph={selectedWorkRelationshipGraph}
            selectedWorkExecutionDetails={selectedWorkExecutionDetails}
            widgetId={layoutItem.id}
          />
        ),
      };
    case DASHBOARD_WIDGET_IDS.providerSession:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <ProviderSessionWidget
            headerAction={headerAction}
            locale={locale}
            selectedProviderSession={
              providerSessionState.selectedProviderSession
            }
            widgetId={layoutItem.id}
          />
        ),
      };
    case DASHBOARD_WIDGET_IDS.submitWork:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <SubmitWorkWidget
            headerAction={headerAction}
            locale={locale}
            submitWorkTypes={snapshot.topology.submit_work_types}
          />
        ),
      };
    case DASHBOARD_WIDGET_IDS.trace:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <TraceDrilldownWidget
            headerAction={headerAction}
            locale={locale}
            onSelectWorkID={currentSelection.selectWorkByID}
            state={traceGridState}
            widgetId={layoutItem.id}
          />
        ),
      };
    default:
      throw new Error(
        `unsupported dashboard widget type: ${layoutItem.widgetType}`,
      );
  }
}

function getDashboardWidgetTitle(widgetType: string, locale?: string): string {
  switch (widgetType) {
    case DASHBOARD_WIDGET_IDS.currentSelection:
      return getCurrentSelectionShellMessages(locale).title;
    case DASHBOARD_WIDGET_IDS.providerSession:
      return getProviderSessionWidgetMessages(locale).title;
    case DASHBOARD_WIDGET_IDS.submitWork:
      return getSubmitWorkMessages(locale).cardTitle;
    case DASHBOARD_WIDGET_IDS.terminalWork:
      return getTerminalWorkMessages(locale).cardTitle;
    case DASHBOARD_WIDGET_IDS.trace:
      return getTraceDrilldownMessages(locale).title;
    case DASHBOARD_WIDGET_IDS.workGraph:
      return getWorkflowActivityShellMessages(locale).widgetTitle;
    case DASHBOARD_WIDGET_IDS.workOutcomeChart:
      return getWorkOutcomeMessages(locale).chart.cardTitle;
    case DASHBOARD_WIDGET_IDS.workTotals:
      return getWorkTotalsMessages(locale).cardTitle;
    default:
      return widgetType;
  }
}
