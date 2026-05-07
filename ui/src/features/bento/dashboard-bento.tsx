import { useEffect } from "react";

import type { DashboardSnapshot } from "../../api/dashboard/types";
import {
  DashboardImportPreviewDialog,
} from "../import";
import {
  CurrentSelectionWidget,
  useCurrentSelection,
  useCurrentSelectionDetails,
} from "../current-selection";
import { SubmitWorkWidget } from "../submit-work";
import { TerminalWorkWidget } from "../terminal-work";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";
import { TraceDrilldownWidget, useTraceDrilldown } from "../trace-drilldown";
import { useWorkOutcomeChart, WorkOutcomeWidget } from "../work-outcome";
import { WorkTotalsWidget } from "../work-totals";
import {
  useCurrentActivityImportController,
  WorkflowActivityWidget,
} from "../workflow-activity";
import { AgentBentoLayout, type AgentBentoLayoutCard } from "./agent-bento";
import { useDashboardBentoStore } from "./state/dashboardBentoStore";
import { DASHBOARD_WIDGET_IDS, useDashboardLayout } from "./useDashboardLayout";
import { useDashboardNow } from "./useDashboardNow";

const EMPTY_DASHBOARD_SNAPSHOT: DashboardSnapshot = {
  factory_state: "IDLE",
  runtime: {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: true,
    },
  },
  tick_count: 0,
  topology: {
    edges: [],
    submit_work_types: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  },
  uptime_seconds: 0,
};

export function DashboardBento() {
  const { dashboardLayout, persistDashboardLayout } = useDashboardLayout();
  const now = useDashboardNow();
  const incrementRefreshToken = useDashboardBentoStore(
    (state) => state.incrementRefreshToken,
  );
  const resetSelectedTraceID = useDashboardBentoStore(
    (state) => state.resetSelectedTraceID,
  );
  const selectedTraceID = useDashboardBentoStore(
    (state) => state.selectedTraceID,
  );
  const setSelectedTraceID = useDashboardBentoStore(
    (state) => state.setSelectedTraceID,
  );
  const timelineEvents = useFactoryTimelineStore((state) => state.events);
  const selectedTimelineTick = useFactoryTimelineStore(
    (state) => state.selectedTick,
  );
  const worldViewCache = useFactoryTimelineStore(
    (state) => state.worldViewCache,
  );
  const workstationRequestsByDispatchID = useFactoryTimelineStore(
    (state) =>
      state.worldViewCache[state.selectedTick]?.workstationRequestsByDispatchID,
  );
  const selectedSnapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick],
  );
  const snapshot = selectedSnapshot ?? EMPTY_DASHBOARD_SNAPSHOT;

  const currentSelection = useCurrentSelection({
    snapshot,
    workstationRequestsByDispatchID,
  });
  const importController = useCurrentActivityImportController({
    onFactoryActivated: incrementRefreshToken,
  });

  useEffect(() => {
    resetSelectedTraceID();
  }, [resetSelectedTraceID]);

  const { selectedTrace, traceGridState } = useTraceDrilldown(
    currentSelection.selectedWorkID,
    selectedTraceID,
  );
  const { selectedWorkExecutionDetails } = useCurrentSelectionDetails({
    currentSelection,
    selectedTrace,
    snapshot,
    workstationRequestsByDispatchID:
      snapshot.runtime.workstation_requests_by_dispatch_id,
  });
  const workChartModel = useWorkOutcomeChart({
    selectedTimelineTick,
    timelineEvents,
    worldViewCache,
  });
  const cards = buildDashboardCards({
    currentSelection,
    importController,
    now,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    setSelectedTraceID,
    snapshot,
    traceGridState,
    workChartModel,
  });

  if (!selectedSnapshot) {
    return null;
  }

  return (
    <>
      <AgentBentoLayout
        cards={cards}
        layout={dashboardLayout}
        onLayoutChange={persistDashboardLayout}
      />
      <DashboardImportPreviewDialog
        activationState={importController.activationState}
        importPreviewState={importController.importPreviewState}
        onCancel={() => {
          importController.clearActivationError();
          importController.closeImportPreview();
        }}
        onConfirm={(value) => {
          void importController.activateImport(value);
        }}
      />
    </>
  );
}

interface DashboardCardBuilderArgs {
  currentSelection: ReturnType<typeof useCurrentSelection>;
  importController: ReturnType<typeof useCurrentActivityImportController>;
  now: number;
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

function buildDashboardCards({
  currentSelection,
  importController,
  now,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  setSelectedTraceID,
  snapshot,
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
          importController={importController}
          now={now}
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
          failedWorkDetailsByWorkID={
            snapshot.runtime.session.failed_work_details_by_work_id
          }
          now={now}
          onSelectTraceID={setSelectedTraceID}
          selectedTrace={selectedTrace}
          selectedWorkExecutionDetails={selectedWorkExecutionDetails}
          widgetId={DASHBOARD_WIDGET_IDS.currentSelection}
        />
      ),
    },
    {
      id: DASHBOARD_WIDGET_IDS.submitWork,
      children: (
        <SubmitWorkWidget
          submitWorkTypes={snapshot.topology.submit_work_types}
        />
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
