import type { DashboardProviderSession } from "../../../../api/dashboard/types";

import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { getProviderSessionLogTarget } from "../../../../components/ui/formatters";
import { DETAIL_COPY_CLASS } from "../../../../components/ui/widget-frame";
import { cn } from "../../../../lib/cn";
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "../../../provider-session-detail/lib/provider-session-ref";
import { CurrentSelectionExpandableSection } from "../../base/components/current-selection-expandable-section";
import { CurrentSelectionHistoryCard } from "../../base/components/current-selection-history-card";
import { useCurrentSelectionOperationalEnumMessages } from "../../base/components/current-selection-locale";
import {
  CurrentSelectionBadge,
  CurrentSelectionExecutionPill,
} from "../../base/components/current-selection-pill";
import { CurrentSelectionSectionHeader } from "../../base/components/current-selection-section-header";
import { CurrentSelectionSelectableButton } from "../../base/components/current-selection-selectable-button";
import {
  CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
} from "../../base/components/detail-card-shared";
import type {
  CollapsibleProviderSessionAttemptsProps,
  ProviderSessionAttemptsProps,
  ProviderSessionLogAccessProps,
} from "../../base/components/detail-card-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";

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
  title,
  workstationKind,
  workstationRequestsByDispatchID,
}: ProviderSessionAttemptsProps) {
  const resolvedTitle = title ?? messages.requestHistoryHeading;
  const headingId = `${resolvedTitle}-heading`
    .toLowerCase()
    .replace(/\s+/g, "-");

  return (
    <section
      aria-labelledby={headingId}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <CurrentSelectionSectionHeader
        headingId={headingId}
        title={resolvedTitle}
      />
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
    return <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>;
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
            className={
              isCurrentDispatch
                ? CURRENT_SELECTION_ACCENT_SURFACE_CLASS
                : undefined
            }
            highlighted={isCurrentDispatch}
            key={`${attempt.dispatch_id}-${attempt.provider_session?.id}`}
          >
            <div className="flex items-start justify-between gap-3">
              <strong>{renderHeading(attempt)}</strong>
              <CurrentSelectionExecutionPill>
                {attempt.dispatch_id}
              </CurrentSelectionExecutionPill>
            </div>
            <div className="mt-2 grid gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <p
                  className={cn(
                    "m-0 text-on-surface-variant",
                    DASHBOARD_BODY_TEXT_CLASS,
                  )}
                >
                  {outcome.label}
                </p>
                {isCurrentDispatch ? (
                  <CurrentSelectionBadge>
                    {messages.currentDispatchLabel}
                  </CurrentSelectionBadge>
                ) : null}
              </div>
              {outcome.rawOutcomeLabel ? (
                <p className={DASHBOARD_SUPPORTING_CODE_CLASS}>
                  {outcome.rawOutcomeLabel}
                </p>
              ) : null}
            </div>
            {loadableProviderSession && onSelectProviderSession ? (
              <CurrentSelectionSelectableButton
                aria-label={messages.selectProviderSessionLabel(
                  providerSessionLabel,
                  attempt.dispatch_id,
                )}
                className={cn(
                  "mt-2",
                  providerSessionSelected &&
                    CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
                )}
                onClick={() => onSelectProviderSession(loadableProviderSession)}
                selected={providerSessionSelected}
                variant="card"
              >
                <span className={DASHBOARD_SUPPORTING_TEXT_CLASS}>
                  {providerSessionSelected
                    ? messages.providerSessionSelectedAction
                    : messages.providerSessionSelectAction}
                </span>
                <code className={DASHBOARD_BODY_CODE_CLASS}>
                  {providerSessionLabel}
                </code>
              </CurrentSelectionSelectableButton>
            ) : (
              <div className="mt-2 grid gap-1">
                <code className={DASHBOARD_BODY_CODE_CLASS}>
                  {providerSessionLabel}
                </code>
                <p className={REQUEST_SELECTION_STATUS_CLASS}>
                  {messages.providerSessionSelectionUnavailable}
                </p>
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
                <p className={REQUEST_SELECTION_STATUS_CLASS}>
                  {messages.workDetailsUnavailable(attempt.dispatch_id)}
                </p>
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
                  <p className={REQUEST_SELECTION_STATUS_CLASS}>
                    {messages.requestDetailsUnavailable(attempt.dispatch_id)}
                  </p>
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
        <a
          className={cn(
            "w-fit rounded-lg font-bold text-primary underline underline-offset-4 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          href={logTarget.href}
          title={logTarget.display}
        >
          {messages.providerSessionLogAction}
        </a>
      ) : (
        <span
          className={cn(
            "font-bold text-on-surface-variant",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
        >
          {messages.providerSessionLogUnavailable}
        </span>
      )}
    </div>
  );
}
