import { useEffect } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useAppLocale } from "../../../i18n";
import { useCurrentSelectionDetails } from "../../current-selection/hooks/core/useCurrentSelectionDetails";
import { useSelectedProviderSessionState } from "../../current-selection/work-selection/hooks/useSelectedProviderSessionState";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { DashboardImportPreviewDialog } from "../../import/components/dashboard-import-preview-dialog";
import type { FactoryImportConfirmInput } from "../../import/lib/factory-import-save-choice";
import { useTraceDrilldown } from "../../trace-drilldown/hooks/useTraceDrilldown";
import { useWorkOutcomeChart } from "../../work-outcome/hooks/useWorkOutcomeChart";
import { useCurrentActivityImportController } from "../../workflow-activity/hooks/current-activity-import-controller";
import {
  type DashboardWorkOutcomeStream,
  useDashboardBentoSnapshot,
} from "../hooks/use-dashboard-bento-snapshot";
import {
  createDashboardLayoutScope,
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
import { DashboardLayoutDiagnostics } from "./diagnostics/dashboard-layout-diagnostics";

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
  workOutcomeStream?: DashboardWorkOutcomeStream;
}

export function DashboardBento({
  locale,
  workOutcomeStream,
}: DashboardBentoProps = {}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const now = useDashboardNow();
  const {
    incrementRefreshToken,
    resetSelectedTraceID,
    selectedTraceID,
    setSelectedTraceID,
  } = useDashboardBentoSelectionState();
  const { factoryPath, rawSessionID, sessionID } = useDashboardSession();
  const {
    currentSelection,
    dashboardCardStateContext,
    materializedWorkOutcomeState,
    selectedSnapshot,
    selectedTimelineTick,
    snapshot,
    workOutcomeHydrationStatus,
  } = useDashboardBentoSnapshot(sessionID, workOutcomeStream);
  const layoutScope = createDashboardLayoutScope(
    resolveDashboardLayoutFactoryID(snapshot, factoryPath),
    sessionID,
  );
  const {
    addDashboardWidget,
    dashboardLayout,
    dashboardLayoutDiagnostics,
    persistDashboardLayout,
    removeDashboardWidget,
  } = useDashboardLayout(layoutScope);
  const importController = useCurrentActivityImportController({
    currentFactoryDefinition: snapshot.factory,
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
    workOutcomeStream?.identity,
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
    hydrationStatus: workOutcomeHydrationStatus,
    locale: resolvedLocale,
    materializedWorkOutcomeState,
    selectedTimelineTick,
  });
  const cards = buildDashboardCardLayouts({
    addDashboardWidget,
    currentSelection,
    dashboardCardStateContext,
    dashboardLayout,
    importController,
    isCurrent: selectedTimelineTick === snapshot.tick_count,
    locale: resolvedLocale,
    now,
    onRemoveDashboardWidget: removeDashboardWidget,
    providerSessionState,
    selectedSessionID: rawSessionID,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    selectedWorkRelationshipGraph,
    setSelectedTraceID,
    snapshot,
    traceGridState,
    workOutcomeHydrationStatus,
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
      <DashboardLayoutDiagnostics
        diagnostics={dashboardLayoutDiagnostics}
        locale={resolvedLocale}
      />
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
  dashboardCardStateContext,
  dashboardLayout,
  importController,
  isCurrent,
  locale,
  now,
  onRemoveDashboardWidget,
  providerSessionState,
  selectedSessionID,
  selectedTrace,
  selectedTraceID,
  selectedWorkExecutionDetails,
  selectedWorkRelationshipGraph,
  setSelectedTraceID,
  snapshot,
  traceGridState,
  workOutcomeHydrationStatus,
  workChartModel,
}: Omit<DashboardCardBuilderArgs, "onSelectInlineWidget"> & {
  addDashboardWidget: ReturnType<
    typeof useDashboardLayout
  >["addDashboardWidget"];
}) {
  return buildDashboardCards({
    currentSelection,
    dashboardCardStateContext,
    dashboardLayout,
    importController,
    isCurrent,
    locale,
    now,
    onRemoveDashboardWidget,
    onSelectInlineWidget: (widgetType) => {
      addDashboardWidget(widgetType);
    },
    providerSessionState,
    selectedSessionID,
    selectedTrace,
    selectedTraceID,
    selectedWorkExecutionDetails,
    selectedWorkRelationshipGraph,
    setSelectedTraceID,
    snapshot,
    traceGridState,
    workOutcomeHydrationStatus,
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

function resolveDashboardLayoutFactoryID(
  snapshot: DashboardSnapshot,
  factoryPath: string,
): string {
  const candidates = [
    snapshot.factory?.id,
    snapshot.runtime.session.bracket?.factory_id,
    snapshot.factory?.factoryDirectory,
    snapshot.factory?.sourceDirectory,
    factoryPath,
  ];
  const factoryID = candidates.find(
    (candidate): candidate is string =>
      typeof candidate === "string" && candidate.trim().length > 0,
  );
  return factoryID?.trim() ?? factoryPath;
}
