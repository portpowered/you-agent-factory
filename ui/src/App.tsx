import "./styles.css";

import { useCallback, useEffect, useState } from "react";

import { cx } from "./components/dashboard/classnames";
import {
  AgentBentoLayout,
  TickSliderControl,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "./components/dashboard";
import {
  CurrentSelectionWidget,
  useCurrentSelection,
  useCurrentSelectionDetails,
} from "./features/current-selection";
import { ExportFactoryDialog, useCurrentFactoryExport } from "./features/export";
import { SubmitWorkWidget } from "./features/submit-work";
import { TerminalWorkWidget } from "./features/terminal-work";
import {
  TraceDrilldownWidget,
  useTraceDrilldown,
} from "./features/trace-drilldown";
import {
  WorkOutcomeWidget,
  useWorkOutcomeChart,
} from "./features/work-outcome";
import { WorkTotalsWidget } from "./features/work-totals";
import { WorkflowActivityWidget } from "./features/workflow-activity";
import { useDashboardSnapshot } from "./hooks/dashboard/useDashboard";
import {
  DASHBOARD_WIDGET_IDS,
  useDashboardLayout,
} from "./hooks/dashboard/useDashboardLayout";
import { useDashboardNow } from "./hooks/dashboard/useDashboardNow";
import { useFactoryTimelineStore } from "./state/factoryTimelineStore";

const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-5 max-[720px]:p-4";
const PANEL_CLASS =
  "rounded-3xl border border-af-overlay/10 bg-af-surface/72 shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4";
const STATUS_PANEL_CLASS = cx(PANEL_CLASS, "mb-4 p-5 px-6");
const DASHBOARD_TOOLBAR_CLASS = cx(
  PANEL_CLASS,
  "mb-4 flex flex-wrap items-center gap-4 p-4 px-5",
);
const DASHBOARD_TOOLBAR_ACTION_CLASS =
  "inline-flex items-center gap-2 rounded-lg border border-af-accent/35 bg-af-accent/10 px-3 py-2 text-sm font-bold text-af-accent outline-af-accent transition hover:bg-af-accent/15 focus-visible:outline-2 focus-visible:outline-offset-2";
const EYEBROW_CLASS =
  "mb-[0.65rem] text-xs font-bold uppercase tracking-[0.16em] text-af-accent";
const DASHBOARD_TITLE_CLASS = cx("m-0", DASHBOARD_PAGE_HEADING_CLASS);
const DETAIL_COPY_CLASS = cx("m-0 max-w-80", DASHBOARD_BODY_TEXT_CLASS);
const SESSION_METADATA_CLASS = cx(
  "m-0 flex min-w-0 flex-1 flex-wrap items-center gap-2 [&_dd]:m-0 [&_div]:inline-flex [&_div]:items-center [&_div]:gap-2 [&_div]:rounded-lg [&_div]:bg-af-overlay/4 [&_div]:px-3 [&_div]:py-2",
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);

export function App() {
  const [dashboardRefreshToken, setDashboardRefreshToken] = useState(0);
  const { snapshot, streamState, isInitialLoading, error } = useDashboardSnapshot({
    refreshToken: dashboardRefreshToken,
  });
  const { dashboardLayout, persistDashboardLayout } = useDashboardLayout();
  const now = useDashboardNow();
  const [isExportDialogOpen, setIsExportDialogOpen] = useState(false);
  const [selectedTraceID, setSelectedTraceID] = useState<string | null>(null);
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
    setSelectedTraceID(null);
  }, [currentSelection.selectedWorkID]);
  const refreshDashboard = useCallback(() => {
    setDashboardRefreshToken((currentValue) => currentValue + 1);
  }, []);
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
    workstationRequestsByDispatchID: snapshot?.runtime.workstation_requests_by_dispatch_id,
  });
  const workChartModel = useWorkOutcomeChart({
    selectedTimelineTick,
    timelineEvents,
    worldViewCache,
  });
  const { currentFactoryExport, isPreparing: isPreparingCurrentFactoryExport } =
    useCurrentFactoryExport(isExportDialogOpen);

  if (isInitialLoading) {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <section className={STATUS_PANEL_CLASS}>
          <p className={EYEBROW_CLASS}>Agent Factory</p>
          <h1 className={DASHBOARD_TITLE_CLASS}>Loading dashboard</h1>
        </section>
      </main>
    );
  }

  if (error instanceof Error) {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <section className={cx(STATUS_PANEL_CLASS, "border-af-danger/45")}>
          <p className={EYEBROW_CLASS}>Agent Factory</p>
          <h1 className={DASHBOARD_TITLE_CLASS}>Dashboard unavailable</h1>
          <p className={DETAIL_COPY_CLASS}>{error.message}</p>
        </section>
      </main>
    );
  }

  if (!snapshot) {
    return null;
  }

  return (
    <main className={DASHBOARD_SHELL_CLASS}>
      <section className={DASHBOARD_TOOLBAR_CLASS} aria-label="dashboard summary">
        <h1 className={DASHBOARD_TITLE_CLASS}>Agent Factory</h1>

        <TickSliderControl />

        <dl className={SESSION_METADATA_CLASS}>
          <div>
            <dt>Factory state</dt>
            <dd>{snapshot.factory_state}</dd>
          </div>

          <div>
            <dt>Stream</dt>
            <dd
              className={cx(
                "inline-flex rounded-full px-[0.7rem] py-1",
                streamState.status === "live" && "bg-af-success/20 text-af-success-ink",
                streamState.status === "connecting" && "bg-af-accent/15 text-af-accent",
                streamState.status === "offline" && "bg-af-danger/15 text-af-danger-ink",
              )}
            >
              {streamState.message}
            </dd>
          </div>
        </dl>

        <button
          aria-expanded={isExportDialogOpen}
          aria-haspopup="dialog"
          className={DASHBOARD_TOOLBAR_ACTION_CLASS}
          onClick={() => {
            setIsExportDialogOpen(true);
          }}
          type="button"
        >
          <svg
            aria-hidden="true"
            fill="none"
            height="18"
            stroke="currentColor"
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth="1.8"
            viewBox="0 0 24 24"
            width="18"
          >
            <path d="M12 4v11" />
            <path d="M8.5 11.5L12 15l3.5-3.5" />
            <path d="M5 19h14" />
          </svg>
          <span>Export PNG</span>
        </button>
      </section>

      <AgentBentoLayout
        cards={[
          {
            id: DASHBOARD_WIDGET_IDS.workTotals,
            children: <WorkTotalsWidget snapshot={snapshot} />,
          },
          {
            id: DASHBOARD_WIDGET_IDS.workGraph,
            children: (
              <WorkflowActivityWidget
                now={now}
                onFactoryActivated={refreshDashboard}
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
        ]}
        layout={dashboardLayout}
        onLayoutChange={persistDashboardLayout}
      />

      <ExportFactoryDialog
        factory={currentFactoryExport.ok ? currentFactoryExport.factoryDefinition : null}
        isPreparing={isPreparingCurrentFactoryExport}
        initialFactoryName={
          currentFactoryExport.ok ? currentFactoryExport.factoryDefinition.name : "agent-factory"
        }
        isOpen={isExportDialogOpen}
        onClose={() => {
          setIsExportDialogOpen(false);
        }}
        preparationFailure={currentFactoryExport.ok ? null : currentFactoryExport}
      />
    </main>
  );
}
