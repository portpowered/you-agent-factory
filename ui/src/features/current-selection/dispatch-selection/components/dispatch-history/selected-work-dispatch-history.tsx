import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { useCurrentSelectionDispatchHistoryMessages } from "../../../base/components/presentation/current-selection-locale";
import type { SelectedWorkOperationHistoryItem } from "../../../hooks/helpers/selected-work-operation-history";
import { requestDispatchID } from "../../../hooks/helpers/useCurrentSelection.request-helpers";
import { getWorkstationDetailMessages } from "../../../workstation-selection/messages/workstation-detail";
import { ProviderSessionAttempts } from "../../../workstation-selection/components/fields/provider-session-attempts";
import type { SelectedWorkDispatchHistorySectionProps } from "../../lib/detail-card-types";
import { DispatchHistoryCard } from "./selected-work-dispatch-history-card";
import {
  LogicalMoveDispatchHistoryCard,
  OperatorMoveHistoryCard,
} from "./selected-work-operation-history-cards";

type DispatchHistoryMessages = ReturnType<
  typeof useCurrentSelectionDispatchHistoryMessages
>;

function SelectedWorkHistoryExpandableSection({
  children,
  idBase,
  messages,
  title,
}: {
  children: ReactNode;
  idBase: string;
  messages: DispatchHistoryMessages;
  title: string;
}) {
  return (
    <CurrentSelectionExpandableSection
      contentId={`${idBase}-content`}
      defaultExpanded
      headingId={`${idBase}-heading`}
      title={title}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {children}
    </CurrentSelectionExpandableSection>
  );
}

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
  widgetId = "current-selection",
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
      <SelectedWorkHistoryExpandableSection
        idBase={`${widgetId}-work-item-operations`}
        messages={messages}
        title={messages.workOperationsHeading}
      >
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
          showHeading={false}
          title={messages.dispatchHistoryHeading}
          workstationKind={workstationKind}
        />
      </SelectedWorkHistoryExpandableSection>
    );
  }

  if (usingOperationHistory) {
    return (
      <SelectedWorkHistoryExpandableSection
        idBase={`${widgetId}-work-item-operations`}
        messages={messages}
        title={messages.workOperationsHeading}
      >
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
          <WidgetDetailCopy>{messages.workOperationsEmpty}</WidgetDetailCopy>
        )}
      </SelectedWorkHistoryExpandableSection>
    );
  }

  if (requests.length === 0 && fallbackProviderSessions.length > 0) {
    return (
      <SelectedWorkHistoryExpandableSection
        idBase={`${widgetId}-work-item-dispatches`}
        messages={messages}
        title={messages.dispatchHistoryHeading}
      >
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
          showHeading={false}
          title={messages.dispatchHistoryHeading}
          workstationKind={workstationKind}
        />
      </SelectedWorkHistoryExpandableSection>
    );
  }

  return (
    <SelectedWorkHistoryExpandableSection
      idBase={`${widgetId}-work-item-dispatches`}
      messages={messages}
      title={messages.dispatchHistoryHeading}
    >
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
        <WidgetDetailCopy>{messages.dispatchHistoryEmpty}</WidgetDetailCopy>
      )}
    </SelectedWorkHistoryExpandableSection>
  );
}
