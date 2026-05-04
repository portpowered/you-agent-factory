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
import { DashboardBrandLockup } from "./dashboard-brand-lockup";

const PANEL_CLASS =
  "rounded-3xl border border-af-overlay/10 bg-af-surface/72 shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4";
const DASHBOARD_TOOLBAR_CLASS = cx(
  PANEL_CLASS,
  "mb-4 flex flex-wrap items-center gap-4 p-4 px-5",
);
const DASHBOARD_TOOLBAR_ACTION_CLASS =
  "inline-flex h-11 w-11 items-center justify-center rounded-lg border border-af-accent/35 bg-af-accent/10 text-af-accent outline-af-accent transition hover:bg-af-accent/15 focus-visible:outline-2 focus-visible:outline-offset-2";
const DASHBOARD_TITLE_CLASS = cx("m-0", DASHBOARD_PAGE_HEADING_CLASS);
const STREAM_STATUS_SHELL_CLASS = cx(
  "flex min-w-0 flex-1 items-center justify-end",
  "max-[720px]:order-4 max-[720px]:w-full max-[720px]:justify-start",
);
const STREAM_STATUS_CLASS = cx(
  "inline-flex h-11 w-11 items-center justify-center rounded-full border border-af-overlay/12 bg-af-overlay/4",
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);

export function DashboardHeader() {
  const snapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick],
  );
  const streamState = useDashboardStreamStore((state) => state.streamState);
  const isExportDialogOpen = useExportDialogStore(
    (state) => state.isExportDialogOpen,
  );
  const openExportDialog = useExportDialogStore(
    (state) => state.openExportDialog,
  );

  if (!snapshot) {
    return null;
  }

  return (
    <section className={DASHBOARD_TOOLBAR_CLASS} aria-label="dashboard summary">
      <h1 className={DASHBOARD_TITLE_CLASS}>
        <DashboardBrandLockup wordmarkClassName="truncate" />
      </h1>
      <TickSliderControl />
      <div className={STREAM_STATUS_SHELL_CLASS}>
        <div
          aria-label={streamStatusLabel(streamState.status)}
          className={streamStatusClassName(streamState.status)}
          role="status"
        >
          <StreamStatusIcon status={streamState.status} />
        </div>
      </div>
      <button
        aria-label="Export PNG"
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
          <path d="M14 5h5v5" />
          <path d="M10 14 19 5" />
          <path d="M19 13v5a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" />
        </svg>
      </button>
    </section>
  );
}

function streamStatusClassName(status: DashboardStreamState["status"]): string {
  return cx(
    STREAM_STATUS_CLASS,
    status === "live" &&
      "border-af-success/30 bg-af-success/16 text-af-success-ink",
    status === "connecting" &&
      "border-af-accent/30 bg-af-accent/12 text-af-accent",
    status === "offline" &&
      "border-af-danger/30 bg-af-danger/12 text-af-danger-ink",
  );
}

function streamStatusLabel(status: DashboardStreamState["status"]): string {
  if (status === "live") {
    return "Infinite You event stream live";
  }
  if (status === "offline") {
    return "Infinite You event stream offline";
  }

  return "Infinite You event stream connecting";
}

function StreamStatusIcon({
  status,
}: {
  status: DashboardStreamState["status"];
}) {
  if (status === "live") {
    return (
      <span
        aria-hidden="true"
        className="relative inline-flex size-3.5 items-center justify-center"
      >
        <span className="absolute inline-flex size-full animate-ping rounded-full bg-current opacity-35" />
        <span className="relative inline-flex size-2.5 rounded-full bg-current" />
      </span>
    );
  }

  if (status === "offline") {
    return (
      <svg
        aria-hidden="true"
        fill="none"
        height="16"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
        viewBox="0 0 16 16"
        width="16"
      >
        <circle cx="8" cy="8" r="4.25" />
        <path d="M4.75 11.25 11.25 4.75" />
      </svg>
    );
  }

  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 16 16"
      width="16"
    >
      <circle cx="8" cy="8" r="4.25" strokeDasharray="1.6 2.2" />
      <path d="M8 5v3" />
      <circle cx="8" cy="11" r="0.75" fill="currentColor" stroke="none" />
    </svg>
  );
}
