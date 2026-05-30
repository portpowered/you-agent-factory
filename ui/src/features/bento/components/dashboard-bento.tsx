import { useEffect, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useAppLocale } from "../../../i18n";
import { useCurrentSelection } from "../../current-selection/hooks/useCurrentSelection";
import { useCurrentSelectionDetails } from "../../current-selection/hooks/useCurrentSelectionDetails";
import { useSelectedProviderSessionState } from "../../current-selection/work-selection/hooks/useSelectedProviderSessionState";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import type { FactoryImportSaveChoice } from "../../../api/named-factory";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import { DashboardImportPreviewDialog } from "../../import/public";
import { useFactoryImportActivationTarget } from "../../import/hooks/use-factory-import-activation-target";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useTraceDrilldown } from "../../trace-drilldown/hooks/useTraceDrilldown";
import { useWorkOutcomeChart } from "../../work-outcome/hooks/useWorkOutcomeChart";
import { useCurrentActivityImportController } from "../../workflow-activity/hooks/current-activity-import-controller";
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

function useDashboardBentoTimelineSnapshot() {
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

  return {
    selectedSnapshot,
    selectedTimelineTick,
    snapshot: selectedSnapshot ?? EMPTY_DASHBOARD_SNAPSHOT,
    timelineEvents,
    workstationRequestsByDispatchID,
    worldViewCache,
  };
}

function useDashboardBentoImportState(
  sessionID: string | null,
  locale: string,
  onFactoryActivated: () => void,
) {
  const importController = useCurrentActivityImportController({
    locale,
    onFactoryActivated,
    sessionID,
  });
  const [importSaveChoice, setImportSaveChoice] =
    useState<FactoryImportSaveChoice>("REPLACE_CURRENT");
  const readyImportPreview =
    importController.importPreviewState.status === "ready"
      ? importController.importPreviewState
      : null;
  const importActivationTarget = useFactoryImportActivationTarget({
    enabled: readyImportPreview !== null,
    preferredFactoryName: readyImportPreview?.value?.factory?.name,
    sessionID,
  });

  useEffect(() => {
    if (readyImportPreview) {
      setImportSaveChoice("REPLACE_CURRENT");
    }
  }, [readyImportPreview]);

  return {
    importActivationTarget,
    importController,
    importSaveChoice,
    setImportSaveChoice,
  };
}

function DashboardBentoImportPreviewDialog({
  importActivationTarget,
  importController,
  importSaveChoice,
  locale,
  onImportSaveChoiceChange,
  sessionID,
}: {
  importActivationTarget: ReturnType<
    typeof useFactoryImportActivationTarget
  >;
  importController: ReturnType<typeof useCurrentActivityImportController>;
  importSaveChoice: FactoryImportSaveChoice;
  locale: string;
  onImportSaveChoiceChange: (choice: FactoryImportSaveChoice) => void;
  sessionID: string | null;
}) {
  return (
    <DashboardImportPreviewDialog
      activationState={importController.activationState}
      createTargetFactoryName={importActivationTarget.createTargetFactoryName}
      currentFactoryName={importActivationTarget.currentFactoryName}
      importPreviewState={importController.importPreviewState}
      importSaveChoice={importSaveChoice}
      locale={locale}
      onCancel={createDashboardImportPreviewCancelHandler(importController)}
      onConfirm={createDashboardImportPreviewConfirmHandler(importController)}
      onImportSaveChoiceChange={onImportSaveChoiceChange}
      sessionID={sessionID}
    />
  );
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
    selectedSnapshot,
    selectedTimelineTick,
    snapshot,
    timelineEvents,
    workstationRequestsByDispatchID,
    worldViewCache,
  } = useDashboardBentoTimelineSnapshot();

  const currentSelection = useCurrentSelection({
    sessionID,
    snapshot,
    workstationRequestsByDispatchID,
  });
  const {
    importActivationTarget,
    importController,
    importSaveChoice,
    setImportSaveChoice,
  } = useDashboardBentoImportState(
    rawSessionID,
    resolvedLocale,
    incrementRefreshToken,
  );

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
      <DashboardBentoImportPreviewDialog
        importActivationTarget={importActivationTarget}
        importController={importController}
        importSaveChoice={importSaveChoice}
        locale={resolvedLocale}
        onImportSaveChoiceChange={setImportSaveChoice}
        sessionID={rawSessionID}
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
  return (value: FactoryPngImportValue, choice: FactoryImportSaveChoice) => {
    void importController.activateImport(value, choice);
  };
}
