import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { DashboardProviderSession } from "../../../../../api/dashboard/types";
import { ButtonLink, Code, Text } from "../../../../../components/ui";
import { getProviderSessionLogTarget } from "../../../../../components/ui/formatters";
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "../../../../provider-session-detail/lib/provider-session-ref";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import type {
  CollapsibleProviderSessionAttemptsProps,
  ProviderSessionAttemptsProps,
  ProviderSessionLogAccessProps,
} from "../../../base/components/detail-card/detail-card-types";
import { CurrentSelectionSectionHeader } from "../../../base/components/layout/current-selection-section-header";
import { useCurrentSelectionOperationalEnumMessages } from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionBadge } from "../../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSelectableButton } from "../../../base/components/presentation/current-selection-selectable-button";
import { CurrentSelectionSupportingText } from "../../../base/public";
import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "../../../history/public";
import { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import type { WorkstationDetailMessages } from "../../messages/workstation-detail-types";

const DEFAULT_PROVIDER_SESSION_ATTEMPT_MESSAGES =
  getWorkstationDetailMessages(undefined);

type ProviderSessionLabelMessages = Pick<
  WorkstationDetailMessages,
  "localizeProviderSessionKind" | "unavailableValue"
>;

function formatLocalizedProviderSessionLabel(
  session: DashboardProviderSession | undefined,
  messages: ProviderSessionLabelMessages,
): string {
  if (!session?.id) {
    return messages.unavailableValue;
  }

  const parts = [
    session.provider,
    localizedProviderSessionKind(session.kind, messages),
  ].filter((value): value is string => value !== undefined && value !== "");
  if (parts.length === 0) {
    return session.id;
  }

  return `${parts.join(" / ")} / ${session.id}`;
}

function localizedProviderSessionKind(
  kind: string | undefined,
  messages: ProviderSessionLabelMessages,
): string | undefined {
  const normalizedKind = kind?.trim();
  if (!normalizedKind) {
    return undefined;
  }

  return messages.localizeProviderSessionKind(normalizedKind);
}

export function CollapsibleProviderSessionAttempts({
  attempts,
  collapseActionLabel,
  currentDispatchID,
  emptyMessage,
  expandActionLabel,
  historyItemCountLabel,
  messages = DEFAULT_PROVIDER_SESSION_ATTEMPT_MESSAGES,
  onSelectProviderSession,
  onSelectWorkID,
  onSelectWorkstationRequest,
  renderHeading,
  resetKey,
  selectedProviderSessionKey,
  selectedRequestDispatchID,
  selectedWorkID,
  title,
  workstationKind,
  workstationRequestsByDispatchID,
}: CollapsibleProviderSessionAttemptsProps) {
  const historyID = `workstation-run-history-${resetKey}`;
  const itemCountLabel = historyItemCountLabel
    ? historyItemCountLabel(attempts.length)
    : messages.historyRunCountLabel(attempts.length);
  const resolvedCollapseActionLabel =
    collapseActionLabel ?? messages.collapseAction;
  const resolvedExpandActionLabel = expandActionLabel ?? messages.expandAction;
  const resolvedTitle = title ?? messages.runHistoryHeading;

  return (
    <CurrentSelectionExpandableSection
      contentId={historyID}
      headingId={`${historyID}-heading`}
      resetKey={resetKey}
      supportingText={itemCountLabel}
      title={resolvedTitle}
      toggleLabel={(expanded) =>
        expanded ? resolvedCollapseActionLabel : resolvedExpandActionLabel
      }
    >
      <ProviderSessionAttemptList
        attempts={attempts}
        currentDispatchID={currentDispatchID}
        emptyMessage={emptyMessage}
        messages={messages}
        onSelectProviderSession={onSelectProviderSession}
        onSelectWorkID={onSelectWorkID}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        renderHeading={renderHeading}
        selectedProviderSessionKey={selectedProviderSessionKey}
        selectedRequestDispatchID={selectedRequestDispatchID}
        selectedWorkID={selectedWorkID}
        workstationKind={workstationKind}
        workstationRequestsByDispatchID={workstationRequestsByDispatchID}
      />
    </CurrentSelectionExpandableSection>
  );
}

export function ProviderSessionAttempts({
  attempts,
  currentDispatchID,
  emptyMessage,
  messages = DEFAULT_PROVIDER_SESSION_ATTEMPT_MESSAGES,
  onSelectProviderSession,
  onSelectWorkID,
  onSelectWorkstationRequest,
  renderHeading,
  selectedProviderSessionKey,
  selectedRequestDispatchID,
  selectedWorkID,
  showHeading = true,
  title,
  workstationKind,
  workstationRequestsByDispatchID,
}: ProviderSessionAttemptsProps) {
  const resolvedTitle = title ?? messages.requestHistoryHeading;
  const headingId = `${resolvedTitle}-heading`
    .toLowerCase()
    .replace(/\s+/g, "-");

  const attemptList = (
    <ProviderSessionAttemptList
      attempts={attempts}
      currentDispatchID={currentDispatchID}
      emptyMessage={emptyMessage}
      messages={messages}
      onSelectProviderSession={onSelectProviderSession}
      onSelectWorkID={onSelectWorkID}
      onSelectWorkstationRequest={onSelectWorkstationRequest}
      renderHeading={renderHeading}
      selectedProviderSessionKey={selectedProviderSessionKey}
      selectedRequestDispatchID={selectedRequestDispatchID}
      selectedWorkID={selectedWorkID}
      workstationKind={workstationKind}
      workstationRequestsByDispatchID={workstationRequestsByDispatchID}
    />
  );

  if (!showHeading) {
    return attemptList;
  }

  return (
    <section
      aria-labelledby={headingId}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <CurrentSelectionSectionHeader
        headingId={headingId}
        title={resolvedTitle}
      />
      {attemptList}
    </section>
  );
}

function ProviderSessionAttemptList({
  attempts,
  currentDispatchID,
  emptyMessage,
  messages = DEFAULT_PROVIDER_SESSION_ATTEMPT_MESSAGES,
  onSelectProviderSession,
  onSelectWorkID,
  onSelectWorkstationRequest,
  renderHeading,
  selectedProviderSessionKey,
  selectedRequestDispatchID,
  selectedWorkID,
  workstationKind,
  workstationRequestsByDispatchID,
}: ProviderSessionAttemptsProps) {
  const enumMessages = useCurrentSelectionOperationalEnumMessages();

  if (attempts.length === 0) {
    return <WidgetDetailCopy>{emptyMessage}</WidgetDetailCopy>;
  }

  return (
    <div className="grid gap-3">
      {attempts.map((attempt) => {
        const outcome = enumMessages.localizeWorkstationRunOutcome(
          attempt.outcome,
          workstationKind,
        );
        const isCurrentDispatch = currentDispatchID === attempt.dispatch_id;
        const loadableProviderSession = getLoadableProviderSessionRef(attempt);
        const providerSessionLabel = formatLocalizedProviderSessionLabel(
          attempt.provider_session,
          messages,
        );
        const providerSessionSelected =
          loadableProviderSession !== null &&
          selectedProviderSessionKey ===
            providerSessionSelectionKey(loadableProviderSession);
        const request = workstationRequestsByDispatchID?.[attempt.dispatch_id];
        const requestSelected =
          selectedRequestDispatchID === attempt.dispatch_id;

        return (
          <CurrentSelectionHistoryCard
            highlighted={isCurrentDispatch}
            key={`${attempt.dispatch_id}-${attempt.provider_session?.id}`}
          >
            <CurrentSelectionHistoryCardHeader
              badges={
                isCurrentDispatch ? (
                  <CurrentSelectionBadge>
                    {messages.currentDispatchLabel}
                  </CurrentSelectionBadge>
                ) : null
              }
              identifier={attempt.dispatch_id}
              subtitle={outcome.label}
              title={renderHeading(attempt)}
            />
            {outcome.rawOutcomeLabel ? (
              <div className="mt-2 grid gap-1">
                <Code as="p" className="m-0" size="supporting">
                  {outcome.rawOutcomeLabel}
                </Code>
              </div>
            ) : null}
            {loadableProviderSession && onSelectProviderSession ? (
              <CurrentSelectionSelectableButton
                aria-label={messages.selectProviderSessionLabel(
                  providerSessionLabel,
                  attempt.dispatch_id,
                )}
                className="mt-2"
                onClick={() => onSelectProviderSession(loadableProviderSession)}
                selected={providerSessionSelected}
                variant="card"
              >
                <Text as="span" variant="supporting">
                  {providerSessionSelected
                    ? messages.providerSessionSelectedAction
                    : messages.providerSessionSelectAction}
                </Text>
                <Code>{providerSessionLabel}</Code>
              </CurrentSelectionSelectableButton>
            ) : (
              <div className="mt-2 grid gap-1">
                <Code>{providerSessionLabel}</Code>
                <CurrentSelectionSupportingText tone="status">
                  {messages.providerSessionSelectionUnavailable}
                </CurrentSelectionSupportingText>
              </div>
            )}
            <ProviderSessionLogAccess
              messages={messages}
              session={attempt.provider_session}
              startedAt={
                attempt.diagnostics?.provider?.request_metadata?.request_time
              }
            />
            <div className="mt-2 grid gap-2">
              {attempt.work_items && attempt.work_items.length > 0 ? (
                onSelectWorkID ? (
                  attempt.work_items.map((workItem) => {
                    const selected = selectedWorkID === workItem.work_id;

                    return (
                      <CurrentSelectionSelectableButton
                        aria-label={messages.selectWorkItemLabel(
                          workItem.display_name || workItem.work_id,
                        )}
                        key={`${attempt.dispatch_id}-${workItem.work_id}`}
                        onClick={() => onSelectWorkID(workItem.work_id)}
                        selected={selected}
                      >
                        {selected
                          ? messages.workSelectedAction
                          : messages.openNamedWorkItemAction(
                              workItem.display_name || workItem.work_id,
                            )}
                      </CurrentSelectionSelectableButton>
                    );
                  })
                ) : null
              ) : (
                <CurrentSelectionSupportingText tone="status">
                  {messages.workDetailsUnavailable(attempt.dispatch_id)}
                </CurrentSelectionSupportingText>
              )}
              {onSelectWorkstationRequest ? (
                request ? (
                  <CurrentSelectionSelectableButton
                    aria-label={messages.selectWorkstationRequestLabel(
                      request.dispatch_id,
                    )}
                    onClick={() => onSelectWorkstationRequest(request)}
                    selected={requestSelected}
                  >
                    {requestSelected
                      ? messages.requestSelectedAction
                      : messages.openRequestDetailsAction}
                  </CurrentSelectionSelectableButton>
                ) : (
                  <CurrentSelectionSupportingText tone="status">
                    {messages.requestDetailsUnavailable(attempt.dispatch_id)}
                  </CurrentSelectionSupportingText>
                )
              ) : null}
            </div>
          </CurrentSelectionHistoryCard>
        );
      })}
    </div>
  );
}

function ProviderSessionLogAccess({
  messages = DEFAULT_PROVIDER_SESSION_ATTEMPT_MESSAGES,
  session,
  startedAt,
}: ProviderSessionLogAccessProps) {
  const logTarget = getProviderSessionLogTarget(session, startedAt);

  return (
    <div className="mt-2 grid min-w-0 gap-1">
      {logTarget ? (
        <ButtonLink
          className="w-fit"
          href={logTarget.href}
          size="sm"
          title={logTarget.display}
          tone="outline"
        >
          {messages.providerSessionLogAction}
        </ButtonLink>
      ) : (
        <Text as="span" className="font-bold text-on-surface-variant">
          {messages.providerSessionLogUnavailable}
        </Text>
      )}
    </div>
  );
}
