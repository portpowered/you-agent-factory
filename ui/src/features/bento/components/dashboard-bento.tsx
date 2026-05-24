import { useEffect, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useAppLocale } from "../../../i18n";
import {
  useCurrentSelection,
  useCurrentSelectionDetails,
  useSelectedProviderSessionState,
} from "../../current-selection/public";
import { DashboardImportPreviewDialog } from "../../import/public";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useTraceDrilldown } from "../../trace-drilldown/public";
import { useWorkOutcomeChart } from "../../work-outcome/public";
import { useCurrentActivityImportController } from "../../workflow-activity/public";
import { AgentBentoLayout } from "./agent-bento";
import { buildDashboardCards } from "./dashboard-bento-cards";
import { useDashboardBentoStore } from "../state/dashboardBentoStore";
import {
  getRenderableDashboardLayout,
  useDashboardLayout,
} from "../hooks/useDashboardLayout";
import { useDashboardNow } from "../hooks/useDashboardNow";

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

export interface DashboardBentoProps {
  locale?: string;
}

export function DashboardBento({ locale }: DashboardBentoProps = {}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const { addDashboardWidget, dashboardLayout, persistDashboardLayout } =
    useDashboardLayout();
  const now = useDashboardNow();
  const [isInlineWidgetPickerOpen, setInlineWidgetPickerOpen] = useState(false);
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
  const selectedSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
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
    sessionID: selectedSessionID ?? DEFAULT_FACTORY_SESSION_ID,
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
  const providerSessionState = useSelectedProviderSessionState(currentSelection);
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
    dashboardLayout,
    isInlineWidgetPickerOpen,
    importController,
    locale: resolvedLocale,
    now,
    onInlineWidgetPickerOpenChange: setInlineWidgetPickerOpen,
    onSelectInlineWidget: (widgetType) => {
      addDashboardWidget(widgetType);
      setInlineWidgetPickerOpen(false);
    },
    providerSessionState,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    setSelectedTraceID,
    snapshot,
    traceGridState,
    workChartModel,
  });
  const renderableLayout = getRenderableDashboardLayout(
    dashboardLayout,
    cards.map((card) => card.widgetType),
  );

  if (!selectedSnapshot) {
    return null;
  }

  return (
    <>
      <AgentBentoLayout
        cards={cards}
        layout={renderableLayout}
        locale={resolvedLocale}
        onLayoutChange={persistDashboardLayout}
      />
      <DashboardImportPreviewDialog
        activationState={importController.activationState}
        importPreviewState={importController.importPreviewState}
        locale={resolvedLocale}
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
