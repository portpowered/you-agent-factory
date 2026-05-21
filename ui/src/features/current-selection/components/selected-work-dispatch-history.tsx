import {
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../components/dashboard/widget-board";
import { getWorkstationDetailMessages } from "../messages";
import type {
  SelectedWorkDispatchHistorySectionProps,
} from "../detail-card-types";
import { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";
import { ProviderSessionAttempts } from "./provider-session-attempts";
import { DispatchHistoryCard } from "./selected-work-dispatch-history-card";

export function SelectedWorkDispatchHistorySection({
  activeTraceID,
  currentDispatchID,
  fallbackProviderSessions,
  locale,
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
  const providerSessionMessages = getWorkstationDetailMessages(locale);

  if (requests.length === 0 && fallbackProviderSessions.length > 0) {
    return (
      <ProviderSessionAttempts
        attempts={fallbackProviderSessions}
        currentDispatchID={currentDispatchID}
        emptyMessage={messages.dispatchHistoryEmpty}
        messages={providerSessionMessages}
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
      <h4
        className={DASHBOARD_SECTION_HEADING_CLASS}
        id="selected-work-dispatch-history-heading"
      >
        {messages.dispatchHistoryHeading}
      </h4>
      {requests.length > 0 ? (
        <div className="grid gap-3">
          {requests.map((request) => (
            <DispatchHistoryCard
              activeTraceID={activeTraceID}
              currentDispatchID={currentDispatchID}
              key={request.dispatch_id}
              onSelectProviderSession={onSelectProviderSession}
              onSelectTraceID={onSelectTraceID}
              onSelectWorkID={onSelectWorkID}
              request={request}
              selectedProviderSessionKey={selectedProviderSessionKey}
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
