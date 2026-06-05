import { DashboardHeading } from "../../../../components/ui";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import { useCurrentSelectionDispatchHistoryMessages } from "../../base/components/current-selection-locale";
import type { SelectedWorkOperationHistoryItem } from "../../hooks/selected-work-operation-history";
import { requestDispatchID } from "../../hooks/useCurrentSelection.request-helpers";
import { getWorkstationDetailMessages } from "../../workstation-selection/messages/workstation-detail";
import { ProviderSessionAttempts } from "../../workstation-selection/public";
import type { SelectedWorkDispatchHistorySectionProps } from "../lib/detail-card-types";
import { DispatchHistoryCard } from "./selected-work-dispatch-history-card";
import {
  LogicalMoveDispatchHistoryCard,
  OperatorMoveHistoryCard,
} from "./selected-work-operation-history-cards";

function operationHistoryItemKey(
  item: SelectedWorkOperationHistoryItem,
): string {
  switch (item.kind) {
    case "operator-move":
      return item.move.request_id ?? `${item.move.tick}:${item.move.sequence}`;
    case "logical-move-dispatch":
    case "workstation":
      return requestDispatchID(item.request);
  }
}

function renderOperationHistoryItem({
  activeTraceID,
  currentDispatchID,
  item,
  onSelectProviderSession,
  onSelectTraceID,
  onSelectWorkID,
  selectedProviderSessionKey,
  selectedWorkID,
}: {
  activeTraceID?: string | null;
  currentDispatchID?: string | null;
  item: SelectedWorkOperationHistoryItem;
  onSelectProviderSession?: SelectedWorkDispatchHistorySectionProps["onSelectProviderSession"];
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedProviderSessionKey?: string | null;
  selectedWorkID: string;
}) {
  switch (item.kind) {
    case "operator-move":
      return (
        <OperatorMoveHistoryCard
          key={operationHistoryItemKey(item)}
          move={item.move}
        />
      );
    case "logical-move-dispatch":
      return (
        <LogicalMoveDispatchHistoryCard
          currentDispatchID={currentDispatchID}
          key={operationHistoryItemKey(item)}
          request={item.request}
          selectedWorkID={selectedWorkID}
        />
      );
    case "workstation":
      return (
        <DispatchHistoryCard
          activeTraceID={activeTraceID}
          currentDispatchID={currentDispatchID}
          key={operationHistoryItemKey(item)}
          onSelectProviderSession={onSelectProviderSession}
          onSelectTraceID={onSelectTraceID}
          onSelectWorkID={onSelectWorkID}
          request={item.request}
          selectedProviderSessionKey={selectedProviderSessionKey}
          selectedWorkID={selectedWorkID}
        />
      );
  }
}

export function SelectedWorkDispatchHistorySection({
  activeTraceID,
  currentDispatchID,
  fallbackProviderSessions,
  locale,
  onSelectProviderSession,
  onSelectTraceID,
  onSelectWorkID,
  operationHistory,
  requests,
  selectedProviderSessionKey,
  selectedWorkID,
  workstationKind,
}: SelectedWorkDispatchHistorySectionProps) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const providerSessionMessages = getWorkstationDetailMessages(locale);
  const fallbackHeading = (workstationName?: string) =>
    workstationName || messages.workstationUnavailableValue;
  const usingOperationHistory = operationHistory !== undefined;

  if (
    usingOperationHistory &&
    operationHistory.length === 0 &&
    fallbackProviderSessions.length > 0
  ) {
    return (
      <ProviderSessionAttempts
        attempts={fallbackProviderSessions}
        currentDispatchID={currentDispatchID}
        emptyMessage={messages.dispatchHistoryEmpty}
        messages={providerSessionMessages}
        onSelectProviderSession={onSelectProviderSession}
        onSelectWorkID={onSelectWorkID}
        renderHeading={(attempt) => fallbackHeading(attempt.workstation_name)}
        selectedProviderSessionKey={selectedProviderSessionKey}
        selectedWorkID={selectedWorkID}
        title={messages.dispatchHistoryHeading}
        workstationKind={workstationKind}
      />
    );
  }

  if (usingOperationHistory) {
    return (
      <section
        aria-labelledby="selected-work-operation-history-heading"
        className="mt-4 grid gap-2.5"
      >
        <DashboardHeading
          as="h4"
          className="m-0"
          id="selected-work-operation-history-heading"
        >
          {messages.workOperationsHeading}
        </DashboardHeading>
        {operationHistory.length > 0 ? (
          <div className="grid gap-3">
            {operationHistory.map((item) =>
              renderOperationHistoryItem({
                activeTraceID,
                currentDispatchID,
                item,
                onSelectProviderSession,
                onSelectTraceID,
                onSelectWorkID,
                selectedProviderSessionKey,
                selectedWorkID,
              }),
            )}
          </div>
        ) : (
          <DetailCopy>{messages.workOperationsEmpty}</DetailCopy>
        )}
      </section>
    );
  }

  if (requests.length === 0 && fallbackProviderSessions.length > 0) {
    return (
      <ProviderSessionAttempts
        attempts={fallbackProviderSessions}
        currentDispatchID={currentDispatchID}
        emptyMessage={messages.dispatchHistoryEmpty}
        messages={providerSessionMessages}
        onSelectProviderSession={onSelectProviderSession}
        onSelectWorkID={onSelectWorkID}
        renderHeading={(attempt) => fallbackHeading(attempt.workstation_name)}
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
      <DashboardHeading
        as="h4"
        className="m-0"
        id="selected-work-dispatch-history-heading"
      >
        {messages.dispatchHistoryHeading}
      </DashboardHeading>
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
            />
          ))}
        </div>
      ) : (
        <DetailCopy>{messages.dispatchHistoryEmpty}</DetailCopy>
      )}
    </section>
  );
}
