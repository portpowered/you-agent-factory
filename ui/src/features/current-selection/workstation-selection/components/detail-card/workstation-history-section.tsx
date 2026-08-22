import { formatWorkItemLabel } from "../../../../../components/ui/formatters";
import type { WorkstationHistorySectionProps } from "../../lib/keys/detail-card-types";
import { CollapsibleProviderSessionAttempts } from "../fields/provider-session-attempts";
import { WorkstationRequestHistorySection } from "./workstation-request-history-section";

export function WorkstationHistorySection({
  collapseActionLabel,
  expandActionLabel,
  locale,
  messages,
  now,
  onSelectProviderSession,
  onSelectWorkID,
  onSelectWorkstationRequest,
  providerSessions,
  selectedNodeID,
  selectedProviderSessionKey,
  selectedRequest,
  selectedWorkID,
  workstationKind,
  workstationRequests,
  workstationRequestsByDispatchID,
}: WorkstationHistorySectionProps) {
  if (workstationRequests.length > 0) {
    return (
      <WorkstationRequestHistorySection
        key={`workstation-request-history:${selectedNodeID}`}
        locale={locale}
        messages={messages}
        now={now}
        onSelectWorkID={onSelectWorkID}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        requests={workstationRequests}
        resetKey={selectedNodeID}
        selectedRequest={selectedRequest}
        selectedWorkID={selectedWorkID}
      />
    );
  }

  return (
    <CollapsibleProviderSessionAttempts
      key={`workstation-run-history:${selectedNodeID}`}
      attempts={providerSessions}
      collapseActionLabel={collapseActionLabel}
      emptyMessage={messages.noWorkstationRuns}
      expandActionLabel={expandActionLabel}
      historyItemCountLabel={messages.historyRunCountLabel}
      messages={messages}
      onSelectProviderSession={onSelectProviderSession}
      onSelectWorkID={onSelectWorkID}
      onSelectWorkstationRequest={onSelectWorkstationRequest}
      renderHeading={(attempt) =>
        attempt.work_items?.map(formatWorkItemLabel).join(", ") ||
        messages.unknownWorkLabel
      }
      resetKey={selectedNodeID}
      selectedProviderSessionKey={selectedProviderSessionKey}
      selectedRequestDispatchID={selectedRequest?.dispatch_id}
      selectedWorkID={selectedWorkID}
      title={messages.runHistoryHeading}
      workstationKind={workstationKind}
      workstationRequestsByDispatchID={workstationRequestsByDispatchID}
    />
  );
}
