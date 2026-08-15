import {
  DashboardStatusPill,
  type DashboardStatusPillTone,
} from "../../../../components/ui/dashboard-status-pill";
import type { DashboardCardAsyncState } from "../../../dashboard/lib/dashboard-card-state";
import { getDashboardCardStateMessages } from "../../../dashboard/messages/dashboard-card-state";

export interface DashboardCardStateBannerProps {
  locale?: string;
  state: DashboardCardAsyncState;
}

export function DashboardCardStateBanner({
  locale,
  state,
}: DashboardCardStateBannerProps) {
  const messages = getDashboardCardStateMessages(locale);
  const stateLabel = dashboardCardStateLabel(state, messages);
  const temporalLabel =
    state.temporal === "historical"
      ? messages.historicalLabel
      : messages.liveLabel;

  return (
    <div
      aria-label={messages.statusAriaLabel(stateLabel, temporalLabel)}
      aria-live="polite"
      className="flex min-h-7 min-w-0 items-center gap-1.5 overflow-x-auto px-1"
      data-dashboard-card-content-state={state.content}
      data-dashboard-card-freshness-state={state.freshness}
      data-dashboard-card-temporal-state={state.temporal}
      role="status"
    >
      <DashboardStatusPill
        className="shrink-0"
        size="compact"
        tone={dashboardCardStateTone(state)}
      >
        {stateLabel}
      </DashboardStatusPill>
      {stateLabel === temporalLabel ? null : (
        <span className="shrink-0 text-xs font-semibold text-on-surface-variant">
          {temporalLabel}
        </span>
      )}
    </div>
  );
}

function dashboardCardStateLabel(
  state: DashboardCardAsyncState,
  messages: ReturnType<typeof getDashboardCardStateMessages>,
): string {
  switch (state.freshness) {
    case "failed":
      return messages.unavailableLabel;
    case "reconnecting":
      return messages.reconnectingLabel;
    case "recovering":
      return messages.recoveringLabel;
    case "stale":
      return messages.staleLabel;
    case "fresh":
      switch (state.content) {
        case "error":
          return messages.unavailableLabel;
        case "known-empty":
          return messages.knownEmptyLabel;
        case "loading":
          return messages.loadingLabel;
        case "populated":
          return state.temporal === "historical"
            ? messages.historicalLabel
            : messages.liveLabel;
      }
  }
}

function dashboardCardStateTone(
  state: DashboardCardAsyncState,
): DashboardStatusPillTone {
  switch (state.freshness) {
    case "failed":
      return "danger";
    case "reconnecting":
    case "stale":
      return "warning";
    case "recovering":
      return "info";
    case "fresh":
      if (state.content === "loading") {
        return "info";
      }
      return state.content === "known-empty" || state.temporal === "historical"
        ? "neutral"
        : "success";
  }
}
