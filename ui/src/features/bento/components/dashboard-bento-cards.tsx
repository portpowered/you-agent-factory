import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { AgentBentoLayoutItem } from "../../../components/ui";
import {
  CurrentSelectionWidget,
  type useCurrentSelection,
  type useCurrentSelectionDetails,
  type useSelectedProviderSessionState,
} from "../../current-selection/public";
import { ProviderSessionWidget } from "../../provider-session-detail/public";
import { SubmitWorkWidget } from "../../submit-work/public";
import { TerminalWorkWidget } from "../../terminal-work/public";
import { TraceDrilldownWidget, type useTraceDrilldown } from "../../trace-drilldown/public";
import { type useWorkOutcomeChart, WorkOutcomeWidget } from "../../work-outcome/public";
import {
  type useCurrentActivityImportController,
  WorkflowActivityWidget,
} from "../../workflow-activity/public";
import { WorkTotalsWidget } from "../../work-totals/public";
import type { AgentBentoLayoutCard } from "./agent-bento";
import { InlineAddWidgetCard } from "./inline-add-widget-card";
import {
  getDashboardWidgetPickerAvailability,
  type DashboardWidgetPickerWidgetType,
} from "../lib/dashboard-widget-picker";
import { DASHBOARD_WIDGET_IDS } from "../hooks/useDashboardLayout";

export interface DashboardCardBuilderArgs {
  currentSelection: ReturnType<typeof useCurrentSelection>;
  dashboardLayout: AgentBentoLayoutItem[];
  isInlineWidgetPickerOpen: boolean;
  importController: ReturnType<typeof useCurrentActivityImportController>;
  locale?: string;
  now: number;
  onInlineWidgetPickerOpenChange: (open: boolean) => void;
  onSelectInlineWidget: (widgetType: DashboardWidgetPickerWidgetType) => void;
  providerSessionState: ReturnType<typeof useSelectedProviderSessionState>;
  selectedTrace: ReturnType<typeof useTraceDrilldown>["selectedTrace"];
  selectedTraceID: string | null;
  selectedWorkExecutionDetails: ReturnType<
    typeof useCurrentSelectionDetails
  >["selectedWorkExecutionDetails"];
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
  providerSessionState: ReturnType<typeof useSelectedProviderSessionState>;
  selectedTrace: ReturnType<typeof useTraceDrilldown>["selectedTrace"];
  selectedTraceID: string | null;
  selectedWorkExecutionDetails: ReturnType<
    typeof useCurrentSelectionDetails
  >["selectedWorkExecutionDetails"];
  setSelectedTraceID: (traceID: string | null) => void;
  snapshot: DashboardSnapshot;
  traceGridState: ReturnType<typeof useTraceDrilldown>["traceGridState"];
  workChartModel: ReturnType<typeof useWorkOutcomeChart>;
}

export function buildDashboardCards({
  currentSelection,
  dashboardLayout,
  isInlineWidgetPickerOpen,
  importController,
  locale,
  now,
  onInlineWidgetPickerOpenChange,
  onSelectInlineWidget,
  providerSessionState,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  setSelectedTraceID,
  snapshot,
  traceGridState,
  workChartModel,
}: DashboardCardBuilderArgs): AgentBentoLayoutCard[] {
  const pickerAvailability = getDashboardWidgetPickerAvailability(dashboardLayout);

  return dashboardLayout.flatMap((layoutItem) => {
    if (layoutItem.widgetType === DASHBOARD_WIDGET_IDS.addWidget) {
      return [
        {
          id: layoutItem.id,
          widgetType: DASHBOARD_WIDGET_IDS.addWidget,
          children: (
            <InlineAddWidgetCard
              locale={locale}
              onPickerOpenChange={onInlineWidgetPickerOpenChange}
              onSelectWidget={onSelectInlineWidget}
              pickerAvailability={pickerAvailability}
              pickerOpen={isInlineWidgetPickerOpen}
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
        providerSessionState,
        selectedTrace,
        selectedTraceID,
        selectedWorkExecutionDetails,
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
  providerSessionState,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  setSelectedTraceID,
  snapshot,
  traceGridState,
  workChartModel,
}: DashboardWidgetCardBuilderArgs): AgentBentoLayoutCard {
  if (
    layoutItem.widgetType === DASHBOARD_WIDGET_IDS.workTotals ||
    layoutItem.widgetType === DASHBOARD_WIDGET_IDS.workGraph
  ) {
    return buildOverviewWidgetCard({
      currentSelection,
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
      layoutItem,
      locale,
      workChartModel,
    });
  }

  return buildSingletonWidgetCard({
    currentSelection,
    layoutItem,
    locale,
    now,
    providerSessionState,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    setSelectedTraceID,
    snapshot,
    traceGridState,
  });
}

function buildOverviewWidgetCard({
  currentSelection,
  importController,
  layoutItem,
  locale,
  now,
  snapshot,
}: Pick<
  DashboardWidgetCardBuilderArgs,
  "currentSelection" | "importController" | "layoutItem" | "locale" | "now" | "snapshot"
>): AgentBentoLayoutCard {
  switch (layoutItem.widgetType) {
    case DASHBOARD_WIDGET_IDS.workTotals:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: <WorkTotalsWidget locale={locale} snapshot={snapshot} />,
      };
    case DASHBOARD_WIDGET_IDS.workGraph:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <WorkflowActivityWidget
            importController={importController}
            locale={locale}
            now={now}
            onSelectStateNode={currentSelection.selectStateNode}
            onSelectWorkID={currentSelection.selectWorkByID}
            onSelectWorkstation={currentSelection.selectWorkstation}
            selection={currentSelection.selection}
            snapshot={snapshot}
          />
        ),
      };
    default:
      throw new Error(`unsupported overview widget type: ${layoutItem.widgetType}`);
  }
}

function buildDuplicateCapableWidgetCard({
  currentSelection,
  layoutItem,
  locale,
  workChartModel,
}: Pick<
  DashboardWidgetCardBuilderArgs,
  "currentSelection" | "layoutItem" | "locale" | "workChartModel"
>): AgentBentoLayoutCard {
  switch (layoutItem.widgetType) {
    case DASHBOARD_WIDGET_IDS.terminalWork:
      return {
        id: layoutItem.id,
        widgetType: layoutItem.widgetType,
        children: (
          <TerminalWorkWidget
            completedItems={currentSelection.completedWorkItems}
            failedItems={currentSelection.failedWorkItems}
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
  layoutItem,
  locale,
  now,
  providerSessionState,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
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
  | "setSelectedTraceID"
  | "snapshot"
  | "traceGridState"
>): AgentBentoLayoutCard {
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
            locale={locale}
            now={now}
            onSelectProviderSession={providerSessionState.setSelectedProviderSession}
            onSelectTraceID={setSelectedTraceID}
            selectedProviderSessionKey={providerSessionState.selectedProviderSessionKey}
            selectedTrace={selectedTrace}
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
            locale={locale}
            selectedProviderSession={providerSessionState.selectedProviderSession}
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
            locale={locale}
            onSelectWorkID={currentSelection.selectWorkByID}
            state={traceGridState}
            widgetId={layoutItem.id}
          />
        ),
      };
    default:
      throw new Error(`unsupported dashboard widget type: ${layoutItem.widgetType}`);
  }
}
