import type { DashboardStreamState } from "../../api/dashboard/types";
import { TickSliderControl } from "../../components/dashboard";
import { cx } from "../../components/ui";
import {
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";
import { useDashboardStreamStore } from "../dashboard/state/dashboardStreamStore";
import { useExportDialogStore } from "../export/state/exportDialogStore";

const PANEL_CLASS =
  "rounded-3xl border border-af-overlay/10 bg-af-surface/72 shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4";
const DASHBOARD_TOOLBAR_CLASS = cx(
  PANEL_CLASS,
  "mb-4 flex flex-wrap items-center gap-4 p-4 px-5",
);
const DASHBOARD_TOOLBAR_ACTION_CLASS =
  "inline-flex items-center gap-2 rounded-lg border border-af-accent/35 bg-af-accent/10 px-3 py-2 text-sm font-bold text-af-accent outline-af-accent transition hover:bg-af-accent/15 focus-visible:outline-2 focus-visible:outline-offset-2";
const DASHBOARD_TITLE_CLASS = cx("m-0", DASHBOARD_PAGE_HEADING_CLASS);
const SESSION_METADATA_CLASS = cx(
  "m-0 flex min-w-0 flex-1 flex-wrap items-center gap-2 [&_dd]:m-0 [&_div]:inline-flex [&_div]:items-center [&_div]:gap-2 [&_div]:rounded-lg [&_div]:bg-af-overlay/4 [&_div]:px-3 [&_div]:py-2",
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);

export function DashboardHeader() {
  const snapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick]?.dashboard,
  );
  const streamState = useDashboardStreamStore((state) => state.streamState);
  const isExportDialogOpen = useExportDialogStore((state) => state.isExportDialogOpen);
  const openExportDialog = useExportDialogStore((state) => state.openExportDialog);

  if (!snapshot) {
    return null;
  }

  return (
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
          <dd className={streamBadgeClassName(streamState.status)}>{streamState.message}</dd>
        </div>
      </dl>
      <button
        aria-expanded={isExportDialogOpen}
        aria-haspopup="dialog"
        className={DASHBOARD_TOOLBAR_ACTION_CLASS}
        onClick={openExportDialog}
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
  );
}

function streamBadgeClassName(status: DashboardStreamState["status"]): string {
  return cx(
    "inline-flex rounded-full px-[0.7rem] py-1",
    status === "live" && "bg-af-success/20 text-af-success-ink",
    status === "connecting" && "bg-af-accent/15 text-af-accent",
    status === "offline" && "bg-af-danger/15 text-af-danger-ink",
  );
}

