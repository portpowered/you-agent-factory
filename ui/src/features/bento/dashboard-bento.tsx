import { useEffect } from "react";

import type { DashboardSnapshot } from "../../api/dashboard/types";
import { type AgentBentoLayoutCard, AgentBentoLayout } from "../../components/ui";
import {
  CurrentSelectionWidget,
  useCurrentSelection,
  useCurrentSelectionDetails,
} from "../current-selection";
import { SubmitWorkWidget } from "../submit-work";
import { TerminalWorkWidget } from "../terminal-work";
import { TraceDrilldownWidget, useTraceDrilldown } from "../trace-drilldown";
import { useWorkOutcomeChart, WorkOutcomeWidget } from "../work-outcome";
import { WorkTotalsWidget } from "../work-totals";
import { WorkflowActivityWidget } from "../workflow-activity";
import {
  DASHBOARD_WIDGET_IDS,
  useDashboardLayout,
} from "../../hooks/dashboard/useDashboardLayout";
import { useDashboardNow } from "../../hooks/dashboard/useDashboardNow";
import { useDashboardAppStore } from "../../state/dashboardAppStore";
import { useFactoryTimelineStore } from "../../state/factoryTimelineStore";

interface DashboardBentoProps {
  snapshot: DashboardSnapshot;
}

export function DashboardBento({ snapshot }: DashboardBentoProps) {
  const { dashboardLayout, persistDashboardLayout } = useDashboardLayout();
  const now = useDashboardNow();
  const incrementRefreshToken = useDashboardAppStore((state) => state.incrementRefreshToken);
  const resetSelectedTraceID = useDashboardAppStore((state) => state.resetSelectedTraceID);
  const selectedTraceID = useDashboardAppStore((state) => state.selectedTraceID);
  const setSelectedTraceID = useDashboardAppStore((state) => state.setSelectedTraceID);
  const timelineEvents = useFactoryTimelineStore((state) => state.events);
  const selectedTimelineTick = useFactoryTimelineStore((state) => state.selectedTick);
  const worldViewCache = useFactoryTimelineStore((state) => state.worldViewCache);
  const workstationRequestsByDispatchID = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick]?.workstationRequestsByDispatchID,
  );
  const currentSelection = useCurrentSelection({
    snapshot,
    workstationRequestsByDispatchID,
  });

  useEffect(() => {
    resetSelectedTraceID();
  }, [currentSelection.selectedWorkID, resetSelectedTraceID]);

  const { selectedTrace, traceGridState } = useTraceDrilldown(
    currentSelection.selectedWorkID,
    selectedTraceID,
  );
  const {
    selectedWorkExecutionDetails,
    terminalWorkExecutionDetails,
  } = useCurrentSelectionDetails({
    currentSelection,
    selectedTrace,
    snapshot,
    workstationRequestsByDispatchID: snapshot.runtime.workstation_requests_by_dispatch_id,
  });
  const workChartModel = useWorkOutcomeChart({
    selectedTimelineTick,
    timelineEvents,
    worldViewCache,
  });
  const cards = buildDashboardCards({
    currentSelection,
    incrementRefreshToken,
    now,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    setSelectedTraceID,
    snapshot,
    terminalWorkExecutionDetails,
    traceGridState,
    workChartModel,
  });

  return (
    <AgentBentoLayout
      cards={cards}
      layout={dashboardLayout}
      onLayoutChange={persistDashboardLayout}
    />
  );
}

interface DashboardCardBuilderArgs {
  currentSelection: ReturnType<typeof useCurrentSelection>;
  incrementRefreshToken: () => void;
  now: number;
  selectedTrace: ReturnType<typeof useTraceDrilldown>["selectedTrace"];
  selectedTraceID: string | null;
  selectedWorkExecutionDetails: ReturnType<
    typeof useCurrentSelectionDetails
  >["selectedWorkExecutionDetails"];
  setSelectedTraceID: (traceID: string | null) => void;
  snapshot: DashboardSnapshot;
  terminalWorkExecutionDetails: ReturnType<
    typeof useCurrentSelectionDetails
  >["terminalWorkExecutionDetails"];
  traceGridState: ReturnType<typeof useTraceDrilldown>["traceGridState"];
  workChartModel: ReturnType<typeof useWorkOutcomeChart>;
}

function buildDashboardCards({
  currentSelection,
  incrementRefreshToken,
  now,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  setSelectedTraceID,
  snapshot,
  terminalWorkExecutionDetails,
  traceGridState,
  workChartModel,
}: DashboardCardBuilderArgs): AgentBentoLayoutCard[] {
  return [
    {
      id: DASHBOARD_WIDGET_IDS.workTotals,
      children: <WorkTotalsWidget snapshot={snapshot} />,
    },
    {
      id: DASHBOARD_WIDGET_IDS.workGraph,
      children: (
        <WorkflowActivityWidget
          now={now}
          onFactoryActivated={incrementRefreshToken}
          onSelectStateNode={currentSelection.selectStateNode}
          onSelectWorkItem={currentSelection.selectWorkItem}
          onSelectWorkstation={currentSelection.selectWorkstation}
          selection={currentSelection.selection}
          snapshot={snapshot}
        />
      ),
    },
    {
      id: DASHBOARD_WIDGET_IDS.terminalWork,
      children: (
        <TerminalWorkWidget
          completedItems={currentSelection.completedWorkItems}
          failedItems={currentSelection.failedWorkItems}
          onSelectItem={currentSelection.openTerminalWorkDetail}
          selectedItem={currentSelection.terminalWorkDetail}
          widgetId={DASHBOARD_WIDGET_IDS.terminalWork}
        />
      ),
    },
    {
      id: DASHBOARD_WIDGET_IDS.workOutcomeChart,
      children: (
        <WorkOutcomeWidget
          model={workChartModel}
          widgetId={DASHBOARD_WIDGET_IDS.workOutcomeChart}
        />
      ),
    },
    {
      id: DASHBOARD_WIDGET_IDS.currentSelection,
      children: (
        <CurrentSelectionWidget
          activeTraceID={selectedTraceID ?? selectedTrace?.trace_id ?? null}
          currentSelection={currentSelection}
          failedWorkDetailsByWorkID={snapshot.runtime.session.failed_work_details_by_work_id}
          now={now}
          onSelectTraceID={setSelectedTraceID}
          selectedTrace={selectedTrace}
          selectedWorkExecutionDetails={selectedWorkExecutionDetails}
          terminalWorkExecutionDetails={terminalWorkExecutionDetails}
          widgetId={DASHBOARD_WIDGET_IDS.currentSelection}
        />
      ),
    },
    {
      id: DASHBOARD_WIDGET_IDS.submitWork,
      children: (
        <SubmitWorkWidget submitWorkTypes={snapshot.topology.submit_work_types} />
      ),
    },
    {
      id: DASHBOARD_WIDGET_IDS.trace,
      children: (
        <TraceDrilldownWidget
          onSelectWorkID={currentSelection.selectWorkByID}
          state={traceGridState}
          widgetId={DASHBOARD_WIDGET_IDS.trace}
        />
      ),
    },
  ];
}
