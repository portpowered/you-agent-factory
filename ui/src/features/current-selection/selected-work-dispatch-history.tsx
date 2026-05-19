import { cx } from "../../lib/cx";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import type {
  SelectedWorkDispatchHistorySectionProps,
} from "./detail-card-types";
import { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";
import { ProviderSessionAttempts } from "./provider-session-attempts";
import { DispatchHistoryCard } from "./selected-work-dispatch-history-card";

export function SelectedWorkDispatchHistorySection({
  activeTraceID,
  currentDispatchID,
  fallbackProviderSessions,
  onSelectProviderSession,
  onSelectTraceID,
  onSelectWorkID,
  requests,
  selectedProviderSessionKey,
  selectedWorkID,
  traceTargetId,
  workstationKind,
}: SelectedWorkDispatchHistorySectionProps) {
  const messages = useCurrentSelectionDispatchHistoryMessages();

  if (requests.length === 0 && fallbackProviderSessions.length > 0) {
    return (
      <ProviderSessionAttempts
        attempts={fallbackProviderSessions}
        currentDispatchID={currentDispatchID}
        emptyMessage={messages.dispatchHistoryEmpty}
        onSelectProviderSession={onSelectProviderSession}
        onSelectWorkID={onSelectWorkID}
        renderHeading={(attempt) => attempt.workstation_name || attempt.transition_id}
        selectedProviderSessionKey={selectedProviderSessionKey}
        selectedWorkID={selectedWorkID}
        title={messages.dispatchHistoryHeading}
        workstationKind={workstationKind}
      />
    );
  }

  return (
    <section
      aria-labelledby="selected-work-dispatch-history-heading"
      className="mt-4 grid gap-2.5"
    >
      <div className="grid gap-1">
        <h4
          className={DASHBOARD_SECTION_HEADING_CLASS}
          id="selected-work-dispatch-history-heading"
        >
          {messages.dispatchHistoryHeading}
        </h4>
        <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {messages.dispatchHistoryCountLabel(requests.length)}
        </p>
      </div>
      {requests.length > 0 ? (
        <div className="grid gap-3">
          {requests.map((request) => (
            <DispatchHistoryCard
              activeTraceID={activeTraceID}
              currentDispatchID={currentDispatchID}
              key={request.dispatch_id}
              onSelectTraceID={onSelectTraceID}
              onSelectWorkID={onSelectWorkID}
              request={request}
              selectedWorkID={selectedWorkID}
              traceTargetId={traceTargetId}
            />
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.dispatchHistoryEmpty}</p>
      )}
    </section>
  );
}
