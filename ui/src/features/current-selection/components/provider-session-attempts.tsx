import { useEffect, useState } from "react";
import type { DashboardProviderSession } from "../../../api/dashboard/types";

import { getProviderSessionLogTarget } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../components/dashboard/widget-board";
import {
  CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
  CURRENT_SELECTION_BADGE_CLASS,
  EXECUTION_PILL_CLASS,
  HISTORY_HEADER_CLASS,
  HISTORY_TOGGLE_CLASS,
  PROVIDER_SESSION_CARD_CLASS,
  PROVIDER_SESSION_SELECTION_BUTTON_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";
import type {
  CollapsibleProviderSessionAttemptsProps,
  ProviderSessionAttemptsProps,
  ProviderSessionLogAccessProps,
} from "./detail-card-types";
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "../../provider-session-detail/lib/provider-session-ref";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { useCurrentSelectionOperationalEnumMessages } from "./current-selection-locale";

const DEFAULT_PROVIDER_SESSION_ATTEMPT_MESSAGES = getWorkstationDetailMessages(undefined);

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

  const parts = [session.provider, localizedProviderSessionKind(session.kind, messages)].filter(
    (value): value is string => value !== undefined && value !== "",
  );
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
  const [expanded, setExpanded] = useState(false);
  const historyID = `workstation-run-history-${resetKey}`;
  const itemCountLabel = historyItemCountLabel
    ? historyItemCountLabel(attempts.length)
    : messages.historyRunCountLabel(attempts.length);
  const resolvedCollapseActionLabel =
    collapseActionLabel ?? messages.collapseAction;
  const resolvedExpandActionLabel = expandActionLabel ?? messages.expandAction;
  const resolvedTitle = title ?? messages.runHistoryHeading;

  useEffect(() => {
    setExpanded(false);
  }, []);

  return (
    <section aria-labelledby={`${historyID}-heading`} className="mt-4 grid gap-2.5">
      <div className={HISTORY_HEADER_CLASS}>
        <div className="grid min-w-0 gap-1">
          <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={`${historyID}-heading`}>
            {resolvedTitle}
          </h4>
          <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
            {itemCountLabel}
          </p>
        </div>
        <button
          aria-controls={historyID}
          aria-expanded={expanded}
          className={HISTORY_TOGGLE_CLASS}
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? resolvedCollapseActionLabel : resolvedExpandActionLabel}
        </button>
      </div>
      {expanded ? (
        <div id={historyID}>
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
        </div>
      ) : null}
    </section>
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

  return (
    <section className="mt-4 grid gap-2.5 [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{resolvedTitle}</h4>
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
        const requestSelected = selectedRequestDispatchID === attempt.dispatch_id;

        return (
          <article
            className={cn(
              PROVIDER_SESSION_CARD_CLASS,
              isCurrentDispatch && CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
            )}
            key={`${attempt.dispatch_id}-${attempt.provider_session?.id}`}
          >
            <div className="flex items-start justify-between gap-3">
              <strong>{renderHeading(attempt)}</strong>
              <span className={EXECUTION_PILL_CLASS}>{attempt.dispatch_id}</span>
            </div>
            <div className="mt-2 grid gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
                  {outcome.label}
                </p>
                {isCurrentDispatch ? (
                  <span className={CURRENT_SELECTION_BADGE_CLASS}>
                    {messages.currentDispatchLabel}
                  </span>
                ) : null}
              </div>
              {outcome.rawOutcomeLabel ? (
                <p className={DASHBOARD_SUPPORTING_CODE_CLASS}>
                  {outcome.rawOutcomeLabel}
                </p>
              ) : null}
            </div>
            {loadableProviderSession && onSelectProviderSession ? (
              <button
                aria-label={messages.selectProviderSessionLabel(
                  providerSessionLabel,
                  attempt.dispatch_id,
                )}
                aria-pressed={providerSessionSelected}
                className={cn(
                  "mt-2",
                  PROVIDER_SESSION_SELECTION_BUTTON_CLASS,
                  providerSessionSelected && CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
                )}
                onClick={() => onSelectProviderSession(loadableProviderSession)}
                type="button"
              >
                <span className={DASHBOARD_SUPPORTING_TEXT_CLASS}>
                  {providerSessionSelected
                    ? messages.providerSessionSelectedAction
                    : messages.providerSessionSelectAction}
                </span>
                <code className={DASHBOARD_BODY_CODE_CLASS}>{providerSessionLabel}</code>
              </button>
            ) : (
              <div className="mt-2 grid gap-1">
                <code className={DASHBOARD_BODY_CODE_CLASS}>{providerSessionLabel}</code>
                <p className={REQUEST_SELECTION_STATUS_CLASS}>
                  {messages.providerSessionSelectionUnavailable}
                </p>
              </div>
            )}
            <ProviderSessionLogAccess
              messages={messages}
              session={attempt.provider_session}
              startedAt={attempt.diagnostics?.provider?.request_metadata?.request_time}
            />
            <div className="mt-2 grid gap-2">
              {attempt.work_items && attempt.work_items.length > 0 ? (
                onSelectWorkID ? (
                  attempt.work_items.map((workItem) => {
                    const selected = selectedWorkID === workItem.work_id;

                    return (
                      <button
                        aria-label={messages.selectWorkItemLabel(
                          workItem.display_name || workItem.work_id,
                        )}
                        aria-pressed={selected}
                        className={WORK_SELECTION_BUTTON_CLASS}
                        key={`${attempt.dispatch_id}-${workItem.work_id}`}
                        onClick={() => onSelectWorkID(workItem.work_id)}
                        type="button"
                      >
                        {selected
                          ? messages.workSelectedAction
                          : messages.openNamedWorkItemAction(
                              workItem.display_name || workItem.work_id,
                            )}
                      </button>
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
                  <button
                    aria-label={messages.selectWorkstationRequestLabel(
                      request.dispatch_id,
                    )}
                    aria-pressed={requestSelected}
                    className={WORK_SELECTION_BUTTON_CLASS}
                    onClick={() => onSelectWorkstationRequest(request)}
                    type="button"
                  >
                    {requestSelected
                      ? messages.requestSelectedAction
                      : messages.openRequestDetailsAction}
                  </button>
                ) : (
                  <p className={REQUEST_SELECTION_STATUS_CLASS}>
                    {messages.requestDetailsUnavailable(attempt.dispatch_id)}
                  </p>
                )
              ) : null}
            </div>
          </article>
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
            "w-fit rounded-lg font-bold text-af-accent underline underline-offset-4 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          href={logTarget.href}
          title={logTarget.display}
        >
          {messages.providerSessionLogAction}
        </a>
      ) : (
        <span className={cn("font-bold text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.providerSessionLogUnavailable}
        </span>
      )}
    </div>
  );
}
