import { useEffect } from "react";

import { useAppLocale } from "../../../i18n";
import { useCurrentSelectionDetails } from "../../current-selection/hooks/useCurrentSelectionDetails";
import { useSelectedProviderSessionState } from "../../current-selection/work-selection/hooks/useSelectedProviderSessionState";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import type { FactoryImportConfirmInput } from "../../import/lib/factory-import-save-choice";
import { DashboardImportPreviewDialog } from "../../import/public";
import { useTraceDrilldown } from "../../trace-drilldown/hooks/useTraceDrilldown";
import { useWorkOutcomeChart } from "../../work-outcome/hooks/useWorkOutcomeChart";
import { useCurrentActivityImportController } from "../../workflow-activity/hooks/current-activity-import-controller";
import { useDashboardBentoSnapshot } from "../hooks/use-dashboard-bento-snapshot";
import {
  getRenderableDashboardLayout,
  useDashboardLayout,
} from "../hooks/useDashboardLayout";
import { useDashboardNow } from "../hooks/useDashboardNow";
import { useDashboardBentoStore } from "../state/dashboardBentoStore";
import { AgentBentoLayout } from "./agent-bento";
import {
  buildDashboardCards,
  type DashboardCardBuilderArgs,
} from "./dashboard-bento-cards";

function useDashboardBentoSelectionState() {
  return {
    incrementRefreshToken: useDashboardBentoStore(
      (state) => state.incrementRefreshToken,
    ),
    resetSelectedTraceID: useDashboardBentoStore(
      (state) => state.resetSelectedTraceID,
    ),
    selectedTraceID: useDashboardBentoStore((state) => state.selectedTraceID),
    setSelectedTraceID: useDashboardBentoStore(
      (state) => state.setSelectedTraceID,
    ),
  };
}

export interface DashboardBentoProps {
  locale?: string;
}

export function DashboardBento({ locale }: DashboardBentoProps = {}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const {
    addDashboardWidget,
    dashboardLayout,
    persistDashboardLayout,
    removeDashboardWidget,
  } = useDashboardLayout();
  const now = useDashboardNow();
  const {
    incrementRefreshToken,
    resetSelectedTraceID,
    selectedTraceID,
    setSelectedTraceID,
  } = useDashboardBentoSelectionState();
  const { rawSessionID, sessionID } = useDashboardSession();
  const {
    currentSelection,
    selectedSnapshot,
    selectedTimelineTick,
    snapshot,
    timelineEvents,
    worldViewCache,
  } = useDashboardBentoSnapshot(sessionID);
  const importController = useCurrentActivityImportController({
    locale: resolvedLocale,
    onFactoryActivated: incrementRefreshToken,
    sessionID: rawSessionID,
  });

  useEffect(() => {
    resetSelectedTraceID();
  }, [resetSelectedTraceID]);

  const { selectedTrace, traceGridState } = useTraceDrilldown(
    currentSelection.selectedWorkID,
    selectedTraceID,
    resolvedLocale,
  );
  const providerSessionState =
    useSelectedProviderSessionState(currentSelection);
  const { selectedWorkExecutionDetails, selectedWorkRelationshipGraph } =
    useCurrentSelectionDetails({
      currentSelection,
      selectedTrace,
      snapshot,
      workstationRequestsByDispatchID:
        snapshot.runtime.workstation_requests_by_dispatch_id,
    });
  const workChartModel = useWorkOutcomeChart({
    locale: resolvedLocale,
    selectedTimelineTick,
    timelineEvents,
    worldViewCache,
  });
  const cards = buildDashboardCardLayouts({
    addDashboardWidget,
    currentSelection,
    dashboardLayout,
    importController,
    locale: resolvedLocale,
    now,
    onRemoveDashboardWidget: removeDashboardWidget,
    providerSessionState,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    selectedWorkRelationshipGraph,
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
        currentSessionFactoryName={snapshot.factory?.name ?? "factory"}
        importPreviewState={importController.importPreviewState}
        locale={resolvedLocale}
        onCancel={createDashboardImportPreviewCancelHandler(importController)}
        onConfirm={createDashboardImportPreviewConfirmHandler(importController)}
      />
    </>
  );
}

function buildDashboardCardLayouts({
  addDashboardWidget,
  currentSelection,
  dashboardLayout,
  importController,
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
}: Omit<DashboardCardBuilderArgs, "onSelectInlineWidget"> & {
  addDashboardWidget: ReturnType<
    typeof useDashboardLayout
  >["addDashboardWidget"];
}) {
  return buildDashboardCards({
    currentSelection,
    dashboardLayout,
    importController,
    locale,
    now,
    onRemoveDashboardWidget,
    onSelectInlineWidget: (widgetType) => {
      addDashboardWidget(widgetType);
    },
    providerSessionState,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    selectedWorkRelationshipGraph,
    setSelectedTraceID,
    snapshot,
    traceGridState,
    workChartModel,
  });
}

function createDashboardImportPreviewCancelHandler(
  importController: ReturnType<typeof useCurrentActivityImportController>,
) {
  return () => {
    importController.clearActivationError();
    importController.closeImportPreview();
  };
}

function createDashboardImportPreviewConfirmHandler(
  importController: ReturnType<typeof useCurrentActivityImportController>,
) {
  return (input: FactoryImportConfirmInput) => {
    void importController.activateImport(input);
  };
}
